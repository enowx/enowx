package ui

import (
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Settings → Hooks: what runs when something happens in a session, on
// every agent (proto.Hook), kept by the connected host's daemon: the
// host's own, its projects' (their .enowx/hooks.json, run once allowed)
// and its sessions'; the changes agents proposed; their log.

// hooksState is the section's. It belongs to the main thread.
type hooksState struct {
	host   string
	asked  bool
	loaded bool
	info   proto.HooksInfo
	log    []proto.HookRun
	err    string
	tab    int // 0 the hooks, 1 the log
	busy   bool

	form   hookForm
	review proto.ProjectHooks // the project file being reviewed
	seeing bool
}

// hookPlace is where a hook is kept: the host, a project (Dir) or a
// session.
type hookPlace struct {
	scope, dir, session string
}

var hostPlace = hookPlace{scope: proto.HookScopeHost}

func (a *App) hooksOf() *hooksState {
	if a.hooks.host != a.host.ID || !a.hooks.asked {
		a.hooks = hooksState{host: a.host.ID, asked: true}
		a.loadHooks()
	}
	return &a.hooks
}

func (a *App) loadHooks() {
	cl, host := a.client, a.host.ID
	if cl == nil {
		return
	}
	dir := ""
	if p, ok := a.currentProject(); ok {
		dir = p.Path
	}
	go func() {
		info, err := cl.Hooks(dir)
		log, _ := cl.HookLog()
		a.onMain(func() {
			if a.hooks.host != host {
				return
			}
			a.hooks.loaded, a.hooks.info, a.hooks.log, a.hooks.err = true, info, log, ""
			if err != nil {
				a.hooks.err = client.Explain(err)
			}
		})
	}()
}

// hooksEvent asks for the hooks again when the daemon says they changed
// (an agent proposed one, another app saved).
func (a *App) hooksEvent(ev proto.Event) {
	if ev.Name != proto.EventHooksChanged {
		return
	}
	a.onMain(func() {
		if a.hooks.asked {
			a.loadHooks()
		}
	})
}

func (a *App) hooksSection(c *ui.Context) {
	settingsColumn(c, "Hooks", "", nil, func() { a.hooksBody(c) })
}

func (a *App) hooksBody(c *ui.Context) {
	st := a.hooksOf()
	sectionIntro(c, "Hooks", "Run a command, give the agent context, refuse or change what it is about to do, notify or call a webhook when something happens in a session, on every agent: in the agent where it has hooks of its own (Claude Code, Codex), else as "+a.host.Name+" sees it happen.")
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		if i := widget.Segmented(c, []string{"Hooks", "Log"}, st.tab, nil); i >= 0 && i != st.tab {
			st.tab = i
			if i == 1 {
				a.loadHooks()
			}
		}
		ui.Spacer(c)
		if st.tab == 1 {
			if widget.GhostButton(c, "Refresh").Clicked() {
				a.loadHooks()
			}
			return
		}
		a.hookTemplateMenu(c, st)
		if widget.PrimaryButton(c, icon.Plus, "Add a hook", "").Disabled(!st.loaded).Clicked() {
			st.form.start(proto.Hook{On: true, Event: proto.HookToolBefore, Action: proto.HookActionRun}, hostPlace, true)
		}
	})
	switch {
	case st.err != "":
		widget.Notice(c, icon.Alert, theme.Error, st.err, "", nil)
		return
	case !st.loaded:
		widget.Spinner(c, 14)
		return
	}
	if st.tab == 1 {
		a.hookLogList(c, st)
		return
	}
	for _, p := range st.info.Proposals {
		a.hookProposal(c.Key("proposal:"+p.ID), st, p)
	}
	for _, ph := range st.info.Pending {
		a.pendingHooks(c.Key("pending:"+ph.Dir), st, ph)
	}
	host := a.hooksIn(st, hostPlace)
	if len(host) == 0 && len(st.info.Projects) == 0 && len(st.info.Sessions) == 0 {
		ui.Text(c, "None yet. Refuse rm -rf and pushes to main, format the files an agent edits, add your project's rules to every prompt, or ping a channel when a turn is done: start from a template.").
			FontSize(12).TextColor(theme.Muted)
	}
	a.hookGroup(c.Key("host"), st, "On "+a.host.Name, "Synced to your other hosts with your settings.", hostPlace, host)
	for _, ph := range st.info.Projects {
		place := hookPlace{scope: proto.HookScopeProject, dir: ph.Dir}
		a.hookGroup(c.Key("project:"+ph.Dir), st, "Project "+filepath.Base(ph.Dir), "In its .enowx/hooks.json, committed with it.", place, ph.Hooks)
	}
	for _, sh := range st.info.Sessions {
		place := hookPlace{scope: proto.HookScopeSession, session: sh.Session}
		a.hookGroup(c.Key("session:"+sh.Session), st, "Session "+a.hookSessionName(sh.Session), "Until it ends.", place, sh.Hooks)
	}
	a.hookFormSheet(c, st)
	a.hookReviewSheet(c, st)
}

// hookGroup lists the hooks kept in place, under its title.
func (a *App) hookGroup(c *ui.Context, st *hooksState, title, about string, place hookPlace, list []proto.Hook) {
	if len(list) == 0 {
		return
	}
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(theme.TextUI).TextColor(theme.TextSoft)
			ui.Text(c, about).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
		})
		for _, h := range list {
			a.hookRow(c.Key(h.ID), st, place, h)
		}
	})
}

// hookSessionName names session id as the session list does.
func (a *App) hookSessionName(id string) string {
	i := slices.IndexFunc(a.sessions, func(s proto.SessionInfo) bool { return s.ID == id })
	if i < 0 {
		return id
	}
	return a.floatingSessionName(a.sessions[i])
}

// hooksIn are the hooks kept in place.
func (a *App) hooksIn(st *hooksState, place hookPlace) []proto.Hook {
	switch place.scope {
	case proto.HookScopeProject:
		for _, ph := range st.info.Projects {
			if ph.Dir == place.dir {
				return slices.Clone(ph.Hooks)
			}
		}
		return nil
	case proto.HookScopeSession:
		for _, sh := range st.info.Sessions {
			if sh.Session == place.session {
				return slices.Clone(sh.Hooks)
			}
		}
		return nil
	}
	return slices.DeleteFunc(slices.Clone(st.info.Hooks), func(h proto.Hook) bool { return h.Project != "" || h.Session != "" })
}

// saveHook puts h in place: in place of the hook with its ID, appended
// when it has none there, or that one removed; then saves the list.
func (a *App) saveHook(st *hooksState, place hookPlace, h proto.Hook, remove bool) {
	cl := a.client
	if cl == nil {
		return
	}
	list := a.hooksIn(st, place)
	i := slices.IndexFunc(list, func(x proto.Hook) bool { return h.ID != "" && x.ID == h.ID })
	switch {
	case remove && i >= 0:
		list = slices.Delete(list, i, i+1)
	case i >= 0:
		list[i] = h
	case !remove:
		list = append(list, h)
	}
	st.busy = true
	go func() {
		var err error
		switch place.scope {
		case proto.HookScopeProject:
			err = cl.SaveProjectHooks(place.dir, list)
		case proto.HookScopeSession:
			err = cl.SaveSessionHooks(place.session, list)
		default:
			_, err = cl.SaveHooks(list)
		}
		a.onMain(func() {
			st.busy = false
			if err != nil {
				if st.form.open {
					st.form.err = client.Explain(err)
				} else {
					a.status = client.Explain(err)
				}
				return
			}
			st.form.open = false
			a.loadHooks()
		})
	}()
}

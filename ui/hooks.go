package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"time"

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
	// Each card's Test: running, and what it did.
	testing map[string]bool
	tested  map[string]proto.HookRun
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
	st := a.hooksOf()
	widget.PageHeader(c, "Hooks", "Run on what agents do", func() {
		if i := widget.Segmented(c, []string{"Hooks", "Log"}, st.tab, nil); i >= 0 && i != st.tab {
			st.tab = i
			if i == 1 {
				a.loadHooks()
			}
		}
		if widget.Button(c, widget.Secondary, icon.Sync, "Refresh", "").Clicked() {
			a.loadHooks()
		}
		if st.tab == 0 && widget.PrimaryButton(c, icon.Plus, "Add a hook", "").Disabled(!st.loaded).Clicked() {
			startNewHook(st)
		}
	})
	switch {
	case st.err != "":
		widget.PageBody(c, true, func() { widget.Notice(c, icon.Alert, theme.Error, st.err, "", nil) })
		return
	case !st.loaded:
		widget.PageBody(c, true, func() { widget.Spinner(c, 14) })
		return
	}
	widget.FigsBand(c, a.hookFigs(st))
	widget.PageBody(c, true, func() {
		if st.tab == 1 {
			a.hookLogList(c, st)
			return
		}
		a.hooksTab(c, st)
	})
	a.hookFormSheet(c, st)
	a.hookReviewSheet(c, st)
}

func startNewHook(st *hooksState) {
	st.form.start(proto.Hook{On: true, Event: proto.HookToolBefore, Action: proto.HookActionRun}, hostPlace, true)
}

// hookFigs are the section's figures, all from what the daemon sent.
func (a *App) hookFigs(st *hooksState) []widget.Fig {
	hooks := 0
	for _, g := range a.hookGroups(st) {
		hooks += len(g.list)
	}
	waiting := len(st.info.Proposals)
	for _, ph := range st.info.Pending {
		waiting += len(ph.Hooks)
	}
	failed := 0
	took := make([]int64, 0, len(st.log))
	for _, r := range st.log {
		took = append(took, r.Ms)
		if !r.Blocked && (r.Error != "" || r.Exit != 0) {
			failed++
		}
	}
	wait := widget.Fig{Label: "Waiting to be allowed", Value: strconv.Itoa(waiting)}
	if waiting > 0 {
		wait.Tone = theme.Warning
	}
	return []widget.Fig{
		{Label: "Hooks", Value: strconv.Itoa(hooks)},
		wait,
		{Label: "Runs in the log", Value: strconv.Itoa(len(st.log))},
		{Label: "Failed", Value: strconv.Itoa(failed)},
		{Label: "Typical run", Value: typicalMs(took)},
	}
}

// hooksTab is what agents proposed and projects want allowed, the hooks, and
// how they ran of late.
func (a *App) hooksTab(c *ui.Context, st *hooksState) {
	ui.Row(c.Key("tools")).FillWidth().Shrink(0).Gap(theme.S3).Children(func() {
		ui.Row(c).Width(270).Shrink(0).Children(func() { a.hookTemplateMenu(c, st) })
	})
	for _, p := range st.info.Proposals {
		a.hookProposal(c.Key("proposal:"+p.ID), st, p)
	}
	for _, ph := range st.info.Pending {
		a.pendingHooks(c.Key("pending:"+ph.Dir), st, ph)
	}
	groups := a.hookGroups(st)
	none := true
	for _, g := range groups {
		none = none && len(g.list) == 0
	}
	if none {
		ui.Text(c, "None yet. Refuse rm -rf and pushes to main, format the files an agent edits, add your project's rules to every prompt, or ping a channel when a turn is done: start from a template.").
			FontSize(theme.TextUI).TextColor(theme.TextSoft).FillWidth()
	}
	// One grid for every place: each card's chip says where it's kept, and a
	// place with no hooks shows nothing.
	widget.Cols(c.Key("hook-cards"), 3).Children(func() {
		for _, g := range groups {
			for _, h := range g.list {
				a.hookRow(c.Key(g.place.scope+g.place.dir+g.place.session+h.ID), st, g, h)
			}
		}
		a.hookAddCard(c, st)
	})
	if len(st.log) > 0 {
		a.hookRecent(c, st)
	}
}

// hookGroup is the hooks kept in one place: the host's, synced to the user's
// other hosts; a project's, in its .enowx/hooks.json; a session's, until it ends.
type hookGroup struct {
	chip  string
	place hookPlace
	list  []proto.Hook
}

func (a *App) hookGroups(st *hooksState) []hookGroup {
	groups := []hookGroup{{"On " + a.host.Name, hostPlace, a.hooksIn(st, hostPlace)}}
	for _, ph := range st.info.Projects {
		groups = append(groups, hookGroup{"In project " + filepath.Base(ph.Dir), hookPlace{scope: proto.HookScopeProject, dir: ph.Dir}, ph.Hooks})
	}
	for _, sh := range st.info.Sessions {
		groups = append(groups, hookGroup{"In session " + a.hookSessionName(sh.Session), hookPlace{scope: proto.HookScopeSession, session: sh.Session}, sh.Hooks})
	}
	return groups
}

// hookAddCard closes the grid: agents can propose hooks too.
func (a *App) hookAddCard(c *ui.Context, st *hooksState) {
	widget.GridCard(c.Key("add-card")).Children(func() {
		widget.CardTitle(c, nil, "Propose or add", nil)
		widget.CardNote(c, "An agent can propose a hook too; it runs once you allow it.", 0)
		widget.CardFoot(c, func() {
			ui.Spacer(c)
			if widget.Button(c, widget.Secondary, icon.Plus, "Add a hook", "").Clicked() {
				startNewHook(st)
			}
		})
	})
}

// hookRecent is the log in short: how many ran of each event, the latest.
func (a *App) hookRecent(c *ui.Context, st *hooksState) {
	counts := map[string]int{}
	var events []string
	largest := 0
	for _, r := range st.log {
		if counts[r.Event] == 0 {
			events = append(events, r.Event)
		}
		counts[r.Event]++
		largest = max(largest, counts[r.Event])
	}
	slices.SortStableFunc(events, func(x, y string) int { return counts[y] - counts[x] })
	widget.TwoParts(c, func() {
		widget.Heading(c, "Runs by event")
		for _, e := range events {
			widget.TallyRow(c, hookEventNames[e], counts[e], largest)
		}
	}, func() {
		widget.Heading(c, "Latest runs")
		for i, r := range st.log[:min(6, len(st.log))] {
			col, status := hookRunStatus(r)
			what := hookRunName(r)
			if r.Session != "" {
				what += " · " + a.hookSessionName(r.Session)
			}
			widget.RecordRow(c.Key(i), clockOrDay(r.Start), what, col, status)
		}
	})
}

// clockOrDay says when, in the room of a record's first column: the time
// today, the day before.
func clockOrDay(t time.Time) string {
	t = t.Local()
	if now := time.Now(); t.YearDay() == now.YearDay() && t.Year() == now.Year() {
		return t.Format("15:04")
	}
	return t.Format("Jan 2")
}

// typicalMs says how long the middle of these took, "–" with none.
func typicalMs(took []int64) string {
	if len(took) == 0 {
		return "–"
	}
	sorted := slices.Sorted(slices.Values(took))
	return msWords(sorted[len(sorted)/2])
}

// msWords says how long, in milliseconds up to a second.
func msWords(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%d ms", ms)
	}
	return fmt.Sprintf("%.1f s", float64(ms)/1000)
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

package ui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Hibernating sessions by hand (proto.MethodSessionHibernate): the
// session menu's Hibernate, for the session or those picked with ⌘ or ⇧
// clicks, ⌘⇧H, and a project's Hibernate idle sessions. What would lose
// work is planned first and shown for the user to confirm; an agent
// asking to hibernate sessions has the user confirm too.

// hibernateForm asks before hibernating sessions that would lose work,
// or answers an agent asking. It belongs to the main thread.
type hibernateForm struct {
	cl    *client.Client
	ids   []string
	plans []proto.HibernatePlan
	// asker is the session of the agent asking, whose answer reply sends.
	asker string
	reply func(yes bool)
	until time.Time // past it, the daemon waits no more
}

// hibernateTargets are the sessions a Hibernate on s acts on: those
// picked when s is one of them, else s.
func (a *App) hibernateTargets(s proto.SessionInfo) []string {
	if a.picked[s.ID] && len(a.picked) > 1 {
		var ids []string
		for _, x := range a.sessions {
			if a.picked[x.ID] {
				ids = append(ids, x.ID)
			}
		}
		return ids
	}
	return []string{s.ID}
}

// canHibernate reports whether s's agent could hibernate and come back.
func canHibernate(s proto.SessionInfo) bool {
	return !s.Exited && !s.Queued && s.Parent == "" && s.NotesFor == "" && s.Agent != "" && s.Agent != agent.Custom && s.Agent != agent.Shell
}

// hibernate plans hibernating sessions ids, then hibernates them when
// that loses no work, else asks.
func (a *App) hibernate(ids []string) {
	cl := a.client
	if cl == nil || len(ids) == 0 {
		return
	}
	go func() {
		res, err := cl.HibernateSessions(proto.SessionHibernateParams{IDs: ids, DryRun: true})
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			if !slices.ContainsFunc(res.Plans, func(p proto.HibernatePlan) bool { return p.Busy() || p.Blocked != "" }) {
				a.runHibernate(cl, proto.SessionHibernateParams{IDs: ids})
				return
			}
			a.hibernating = &hibernateForm{cl: cl, ids: ids, plans: res.Plans}
		})
	}()
}

// hibernateIdle hibernates the sessions of project p (every project's
// with ""), those with nothing to lose.
func (a *App) hibernateIdle(project string) {
	var ids []string
	for _, s := range a.sessions {
		if (project == "" || s.Project == project) && canHibernate(s) && !isWorking(s) {
			ids = append(ids, s.ID)
		}
	}
	if len(ids) == 0 {
		a.notify("No idle session to hibernate")
		return
	}
	a.runHibernate(a.client, proto.SessionHibernateParams{IDs: ids})
}

// runHibernate hibernates as p says, and says what it freed.
func (a *App) runHibernate(cl *client.Client, p proto.SessionHibernateParams) {
	if cl == nil {
		return
	}
	go func() {
		res, err := cl.HibernateSessions(p)
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			a.picked = nil
			a.notify(hibernateSummary(res))
		})
	}()
}

// hibernateSummary says what a hibernation did, in a line.
func hibernateSummary(res proto.SessionHibernateResult) string {
	var freed uint64
	kept := 0
	for _, pl := range res.Plans {
		if slices.Contains(res.Hibernated, pl.ID) {
			freed += pl.RSS
		} else if !slices.Contains(res.Waiting, pl.ID) {
			kept++
		}
	}
	var parts []string
	if n := len(res.Hibernated); n > 0 {
		parts = append(parts, fmt.Sprintf("Hibernated %s, freeing about %s", textfmt.Plural(n, "a session", fmt.Sprintf("%d sessions", n)), textfmt.Bytes(freed)))
	}
	if n := len(res.Waiting); n > 0 {
		parts = append(parts, fmt.Sprintf("%s once %s done", textfmt.Plural(n, "1 hibernates", fmt.Sprintf("%d hibernate", n)), textfmt.Plural(n, "its agent is", "their agents are")))
	}
	if kept > 0 {
		parts = append(parts, fmt.Sprintf("%d kept: %s work", kept, textfmt.Plural(kept, "it would stop", "they would stop")))
	}
	if len(parts) == 0 {
		return "Nothing to hibernate"
	}
	return strings.Join(parts, "; ")
}

// wake wakes hibernated session s on its conversation.
func (a *App) wake(s proto.SessionInfo) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		info, err := cl.WakeSession(s.ID)
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			a.upsert(info)
		})
	}()
}

// setAutoHibernate keeps s from hibernating on its own, or lets it again.
func (a *App) setAutoHibernate(s proto.SessionInfo, never bool) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		err := cl.SetAutoHibernate(proto.SessionAutoHibernateParams{ID: s.ID, Never: never})
		if err != nil {
			a.onMain(func() { a.status = client.Explain(err) })
		}
	}()
}

// hibernateItems are a session's menu items for hibernating it.
func (a *App) hibernateItems(c *ui.Context, s proto.SessionInfo, done func()) {
	if s.Hibernated {
		if widget.MenuItem(c, icon.Play, "Wake", false) {
			a.wake(s)
			done()
		}
		return
	}
	if !canHibernate(s) {
		return
	}
	label := "Hibernate (" + modKey + "⇧H)"
	if ids := a.hibernateTargets(s); len(ids) > 1 {
		label = fmt.Sprintf("Hibernate %d sessions", len(ids))
	}
	if widget.MenuItem(c, icon.Moon, label, false) {
		a.hibernate(a.hibernateTargets(s))
		done()
	}
	never := "Don't hibernate on its own"
	if s.NoAutoHibernate {
		never = "Let it hibernate on its own"
	}
	if widget.MenuItem(c, icon.Moon, never, false) {
		a.setAutoHibernate(s, !s.NoAutoHibernate)
		done()
	}
}

// hibernateAsked answers an agent asking to hibernate sessions: the user
// says, in the app.
func (a *App) hibernateAsked(cl *client.Client, req proto.AppRequest, reply func(any, error)) {
	var p proto.SessionHibernateParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		reply(nil, err)
		return
	}
	go func() {
		dry := p
		dry.DryRun, dry.From = true, ""
		res, err := cl.HibernateSessions(dry)
		a.onMain(func() {
			if err != nil {
				reply(nil, err)
				return
			}
			asker := p.From
			if s, ok := a.sessionByID(p.From); ok {
				asker = a.sessionName(s)
			}
			a.hibernateAsks = append(a.hibernateAsks, &hibernateForm{cl: cl, plans: res.Plans, asker: asker, until: askUntil(hostAskLife), reply: func(yes bool) { reply(yes, nil) }})
		})
	}()
}

// hibernateDialog asks before hibernating what would lose work.
func (a *App) hibernateDialog(c *ui.Context) {
	f := a.hibernating
	if f == nil {
		return
	}
	open := true
	answer := func(yes bool, p proto.SessionHibernateParams) {
		a.hibernating = nil
		if yes {
			a.runHibernate(f.cl, p)
		}
	}
	working := slices.ContainsFunc(f.plans, func(pl proto.HibernatePlan) bool { return pl.Working })
	title := "Hibernate " + textfmt.Plural(len(f.plans), "this session", fmt.Sprintf("these %d sessions", len(f.plans))) + "?"
	widget.Dialog(c, &open, 460, 14, func() {
		widget.DialogHeader(c, title, hibernateFrees(f.plans), false)
		hibernatePlans(c, f.plans)
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				answer(false, proto.SessionHibernateParams{})
			}
			if working && widget.GhostButton(c, "Once done").Tooltip("Hibernate each when its agent is done").Clicked() {
				answer(true, proto.SessionHibernateParams{IDs: f.ids, WhenDone: true})
			}
			if widget.Button(c, widget.Danger, icon.Moon, "Hibernate now", "").Clicked() {
				answer(true, proto.SessionHibernateParams{IDs: f.ids, Force: true})
			}
		})
	})
	if !open && a.hibernating == f {
		answer(false, proto.SessionHibernateParams{})
	}
}

// hibernateAsk is the card of the oldest agent asking to hibernate sessions.
func (a *App) hibernateAsk(c *ui.Context) *widget.Ask {
	if len(a.hibernateAsks) == 0 {
		return nil
	}
	f := a.hibernateAsks[0]
	answer := func(yes bool) func() {
		return func() {
			a.hibernateAsks = slices.DeleteFunc(a.hibernateAsks, func(x *hibernateForm) bool { return x == f })
			f.reply(yes)
		}
	}
	return &widget.Ask{Key: fmt.Sprintf("hibernate:%p", f), Icon: icon.Moon, Title: "“" + f.asker + "” asks to hibernate",
		About: "An agent asks to hibernate " + textfmt.Plural(len(f.plans), "this session", "these sessions") + ". " + hibernateFrees(f.plans),
		Body:  func() { hibernatePlans(c, f.plans) },
		Buttons: []widget.AskButton{
			{Kind: widget.Secondary, Label: "Don't", Do: answer(false)},
			{Kind: widget.Danger, Icon: icon.Moon, Label: "Hibernate", Do: answer(true)},
		}}
}

// hibernateFrees says what hibernating plans does and gives back.
func hibernateFrees(plans []proto.HibernatePlan) string {
	var freed uint64
	var cpu float64
	for _, pl := range plans {
		if pl.Blocked == "" {
			freed, cpu = freed+pl.RSS, cpu+pl.CPU
		}
	}
	return fmt.Sprintf("Their agents stop and come back on their conversation when shown. Frees about %s and %.0f%% CPU.", textfmt.Bytes(freed), cpu)
}

func hibernatePlans(c *ui.Context, plans []proto.HibernatePlan) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		for _, pl := range plans {
			hibernatePlanCard(c.Key(pl.ID), pl)
		}
	})
}

// hibernatePlanCard is what hibernating a session would stop and free.
func hibernatePlanCard(c *ui.Context, pl proto.HibernatePlan) {
	ui.Column(c).FillWidth().Gap(4).Padding(10).Radius(theme.RadiusControl).Border(1, theme.Border).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.Moon).FontSize(12).TextColor(theme.TextSoft)
			name := pl.Name
			if name == "" {
				name = pl.ID
			}
			ui.Text(c, name).FontSize(theme.TextUI).TextColor(theme.TextStrong).SingleLine().Shrink(1)
			ui.Spacer(c)
			if pl.Blocked == "" {
				ui.Text(c, "frees "+textfmt.Bytes(pl.RSS)).FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
		})
		line := func(text string, col ui.Color) {
			ui.Text(c, text).FontSize(theme.TextLabel).TextColor(col).PaddingX(20)
		}
		if pl.Blocked != "" {
			line("Stays as it is: "+pl.Blocked, theme.Muted)
			return
		}
		if pl.Working {
			line("Its agent is working: it stops mid-task", theme.Warning)
		}
		for _, port := range pl.Ports {
			line(fmt.Sprintf("Serves port %d, which stops", port), theme.Warning)
		}
		if pl.Display {
			line("Shows an app, which closes", theme.Warning)
		}
		for _, pr := range pl.Procs {
			col, where := theme.Muted, ""
			if pr.Left {
				col, where = theme.Warning, ", left in the background"
			}
			line(fmt.Sprintf("Stops %s (%s%s)", pr.Name, textfmt.Bytes(pr.RSS), where), col)
		}
		if !pl.Busy() {
			line("Nothing is lost", theme.Muted)
		}
	})
}

// hibernatedLabel is a hibernated session's status: by whom and since.
func hibernatedLabel(s proto.SessionInfo) string {
	h := s.HibernatedHow
	if h == nil {
		return "hibernated"
	}
	by := "hibernated"
	if h.Manual {
		by = "hibernated by you"
	}
	since := h.Since.Local()
	if time.Since(since) > 20*time.Hour {
		return by + " · since " + since.Format("2 Jan")
	}
	return by + " · since " + since.Format("15:04")
}

// pick adds s to the sessions picked for Hibernate, or takes it out; the
// session shown is picked with the first.
func (a *App) pick(s proto.SessionInfo) {
	if a.picked == nil {
		a.picked = map[string]bool{}
		if a.selected != "" {
			a.picked[a.selected] = true
		}
	}
	if a.picked[s.ID] {
		delete(a.picked, s.ID)
	} else {
		a.picked[s.ID] = true
	}
}

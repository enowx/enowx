package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A fan-out gives one prompt to several agents, each on a branch of its
// own (proto.MethodSessionFanOut). Compare lists the group's sessions with
// what each changed, to keep one and discard the others.

// alsoField picks more agents to give the new session's prompt to, when
// it starts on a new branch: each gets its own.
func (a *App) alsoField(c *ui.Context) {
	f := &a.newSession
	if _, ok := agent.Find(f.agent); !ok || f.branchMode != proto.BranchNew {
		return
	}
	widget.Field(c, "Also give it to (optional)", func() {
		ui.Text(c, "Each agent works on a branch of its own, named after it, to compare them and keep the best.").
			FontSize(11.5).TextColor(theme.Muted)
		ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
			for _, ag := range a.agents {
				if !ag.Installed || ag.Command == "" {
					continue
				}
				on := slices.Contains(f.also, ag.ID)
				b := ui.ButtonBase(c.Key(ag.ID)).Gap(6).Padding(4, 9).Radius(theme.RadiusControl).Cursor(ui.CursorPointer).
					Border(1, theme.Border).Label(ag.Name)
				if on {
					b.Border(1, theme.Accent).Background(theme.AccentSoft)
				}
				b.Children(func() {
					agentTile(c, ag.ID, 16)
					ui.Text(c, ag.Name).FontSize(12)
				})
				if b.Clicked() {
					if on {
						f.also = slices.DeleteFunc(f.also, func(id string) bool { return id == ag.ID })
					} else {
						f.also = append(f.also, ag.ID)
					}
				}
			}
		})
	})
}

// startFanOut starts the form's sessions, one per agent, then compares them.
func (a *App) startFanOut(cl *client.Client, params proto.SessionCreateParams) {
	f := &a.newSession
	f.busy, f.err = true, ""
	agents := append([]string{params.Agent}, f.also...)
	go func() {
		list, err := cl.FanOut(proto.FanOutParams{SessionCreateParams: params, Agents: agents})
		a.onMain(func() {
			f.busy = false
			for _, s := range list {
				a.upsert(s)
			}
			if err != nil {
				f.err = client.Explain(err)
				return
			}
			f.open = false
			a.selectSession(list[0])
			a.openCompare(list[0].Group)
		})
	}()
}

// compareDialog is the Compare dialog's state. It belongs to the main
// thread.
type compareDialog struct {
	open  bool
	group string
	diffs map[string]proto.Diff // session ID → what it changed
	busy  bool
	err   string
}

func (a *App) openCompare(group string) {
	a.compare = compareDialog{open: true, group: group, diffs: map[string]proto.Diff{}}
	a.loadCompare()
}

// loadCompare asks what each session of the group changed.
func (a *App) loadCompare() {
	cl, group := a.client, a.compare.group
	if cl == nil {
		return
	}
	for _, s := range a.groupSessions(group) {
		go func() {
			d, err := cl.Diff(s.ID, "")
			if err != nil {
				return
			}
			a.onMain(func() {
				if a.compare.group == group {
					a.compare.diffs[s.ID] = d
				}
			})
		}()
	}
}

func (a *App) groupSessions(group string) []proto.SessionInfo {
	var out []proto.SessionInfo
	for _, s := range a.sessions {
		if group != "" && s.Group == group {
			out = append(out, s)
		}
	}
	return out
}

func (a *App) compareDialogView(c *ui.Context) {
	d := &a.compare
	if !d.open {
		return
	}
	list := a.groupSessions(d.group)
	widget.Dialog(c, &d.open, 640, 12, func() {
		widget.DialogHeader(c, "Compare", "One prompt, given to each of these agents on a branch of its own. Look at each, then keep the one you like: the others are discarded, with their worktrees and branches.", false)
		if d.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, d.err, "", nil)
		}
		if len(list) == 0 {
			ui.Text(c, "These sessions are gone.").FontSize(12).TextColor(theme.Muted)
		}
		for _, s := range list {
			a.compareRow(c.Key(s.ID), s, list)
		}
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Refresh").Clicked() {
				a.loadCompare()
			}
			if widget.GhostButton(c, "Close").Clicked() {
				d.open = false
			}
		})
	})
}

func (a *App) compareRow(c *ui.Context, s proto.SessionInfo, group []proto.SessionInfo) {
	d := &a.compare
	widget.Card(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			agentTile(c, s.Agent, 26)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, a.agentName(s.Agent)+" · "+s.Branch).FontSize(12.5).SingleLine()
				var detail []string
				if s.State != "" {
					detail = append(detail, string(s.State))
				}
				if diff, ok := d.diffs[s.ID]; ok {
					added, removed := 0, 0
					for _, f := range diff.Files {
						added, removed = added+f.Added, removed+f.Removed
					}
					detail = append(detail, fmt.Sprintf("%d %s, +%d −%d", len(diff.Files), textfmt.Plural(len(diff.Files), "file", "files"), added, removed))
				}
				if s.Note != "" {
					detail = append(detail, s.Note)
				}
				ui.Text(c, strings.Join(detail, " · ")).FontSize(11).TextColor(theme.Muted).SingleLine()
			})
			if widget.GhostButton(c, "Show").Clicked() {
				d.open = false
				a.selectSession(s)
			}
			if widget.PrimaryButton(c, icon.Check, "Keep", "").Disabled(d.busy).Clicked() && !d.busy {
				a.askKeep(s, group)
			}
		})
	})
}

// askKeep keeps s from group once the user agrees.
func (a *App) askKeep(s proto.SessionInfo, group []proto.SessionInfo) {
	n := len(group) - 1
	a.confirm("Keep “"+a.sessionName(s)+"”?", textfmt.Plural(n, "The other session", "The "+strconv.Itoa(n)+" other sessions")+" of this Compare, with their worktrees, branches and uncommitted changes, are discarded for good.", "Keep and discard", func() { a.keepFromGroup(s, group) })
}

// keepFromGroup discards the group's other sessions, with their worktrees
// and branches, leaving kept to review and merge as any other.
func (a *App) keepFromGroup(kept proto.SessionInfo, group []proto.SessionInfo) {
	cl, d := a.client, &a.compare
	if cl == nil {
		return
	}
	d.busy, d.err = true, ""
	go func() {
		var failed error
		for _, s := range group {
			if s.ID == kept.ID {
				continue
			}
			if err := cl.Discard(s.ID, true); err != nil && failed == nil {
				failed = err
			}
		}
		a.onMain(func() {
			d.busy = false
			if failed != nil {
				d.err = "Couldn't discard them all: " + client.Explain(failed)
				return
			}
			d.open = false
			a.selectSession(kept)
			a.notify("Kept " + a.agentName(kept.Agent) + "'s " + kept.Branch)
		})
	}()
}

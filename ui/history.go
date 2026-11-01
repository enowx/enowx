package ui

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Past conversations lists what Claude Code and Codex talked about in a
// project's folder and worktrees (agent.history, on its host), and goes
// on with one in a new session, in the folder it ran in.

// historyDialog is the dialog's state. It belongs to the main thread.
type historyDialog struct {
	open    bool
	project proto.ProjectInfo
	loading bool
	list    []proto.Conversation
	filter  string
	err     string
}

func (a *App) openHistory(p proto.ProjectInfo) {
	a.history = historyDialog{open: true, project: p, loading: true}
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		list, err := cl.History(p.Path)
		a.onMain(func() {
			if a.history.project.Path != p.Path {
				return
			}
			a.history.loading, a.history.list = false, list
			if err != nil {
				a.history.err = client.Explain(err)
			}
		})
	}()
}

func (a *App) historyDialogView(c *ui.Context) {
	h := &a.history
	if !h.open {
		return
	}
	widget.Dialog(c, &h.open, 640, 12, func() {
		widget.DialogHeader(c, "Past conversations in “"+h.project.Name+"”",
			"What Claude Code and Codex talked about in its folder and worktrees. Resume goes on with one in a new session, where it ran.", false)
		widget.SearchField(c, &h.filter, "Filter", nil)
		switch {
		case h.err != "":
			widget.Notice(c, nil, theme.Error, h.err, "", nil)
		case h.loading:
			ui.Row(c).Gap(8).Padding(8).Children(func() {
				widget.Spinner(c, 13)
				ui.Text(c, "Reading the conversations…").FontSize(theme.TextLabel).TextColor(theme.Muted)
			})
		case len(h.list) == 0:
			ui.Text(c, "No conversations here yet.").FontSize(theme.TextLabel).TextColor(theme.Muted).Padding(8)
		}
		ui.Scroll(c).FillWidth().MaxHeight(440).Children(func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				for _, conv := range h.list {
					if f := strings.ToLower(h.filter); f != "" && !strings.Contains(strings.ToLower(conv.Title+" "+conv.Branch), f) {
						continue
					}
					a.conversationRow(c.Key(conv.Agent+conv.ID), conv)
				}
			})
		})
	})
}

func (a *App) conversationRow(c *ui.Context, conv proto.Conversation) {
	widget.Card(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			agentTile(c, conv.Agent, 26)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				title := conv.Title
				if title == "" {
					title = "(no message)"
				}
				ui.Text(c, title).FontSize(theme.TextUI).SingleLine()
				var meta []string
				if conv.Branch != "" {
					meta = append(meta, conv.Branch)
				} else {
					meta = append(meta, path.Base(conv.Dir))
				}
				meta = append(meta, fmt.Sprintf("%d %s", conv.Messages, textfmt.Plural(conv.Messages, "message", "messages")))
				if conv.Model != "" {
					meta = append(meta, conv.Model)
				}
				meta = append(meta, textfmt.Age(conv.Updated, time.Now())+" ago")
				ui.Text(c, strings.Join(meta, " · ")).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			})
			if widget.GhostButton(c, "Resume").Clicked() {
				a.resumeConversation(conv)
			}
		})
	})
}

// resumeConversation starts a session of conv's agent going on with it,
// in the folder it ran in: the project's, or its branch's worktree.
func (a *App) resumeConversation(conv proto.Conversation) {
	cl, h := a.client, &a.history
	if cl == nil {
		return
	}
	p := proto.SessionCreateParams{
		Project: h.project.Path, Agent: conv.Agent, Resume: conv.ID, BranchMode: proto.BranchNone,
		SkipPermissions: a.skipPermissions, Cols: 120, Rows: 36,
	}
	if conv.Dir != h.project.Path && conv.Branch != "" {
		p.BranchMode, p.Branch = proto.BranchExisting, conv.Branch
	}
	h.open = false
	go func() {
		info, err := cl.CreateSession(p)
		a.onMain(func() {
			if err != nil {
				a.status = "Couldn't resume: " + client.Explain(err)
				return
			}
			a.upsert(info)
			a.selectSession(info)
		})
	}()
}

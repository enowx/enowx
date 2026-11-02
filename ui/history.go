package ui

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
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
	widget.FormSheet(c, &h.open, 480, "Past conversations in "+h.project.Name, "", func() {
		widget.SearchField(c, &h.filter, "Filter", icon.Search)
		switch {
		case h.err != "":
			widget.Notice(c, icon.Alert, theme.Error, h.err, "", nil)
		case h.loading:
			ui.Row(c).FillWidth().Gap(theme.S2).Padding(theme.S2, theme.S3).Children(func() {
				widget.Spinner(c, 13)
				ui.Text(c, "Reading the conversations…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			})
		case len(h.list) == 0:
			ui.Text(c, "No conversations here yet.").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S3)
		}
		ui.Column(c).FillWidth().Children(func() {
			for _, conv := range h.list {
				if f := strings.ToLower(h.filter); f != "" && !strings.Contains(strings.ToLower(conv.Title+" "+conv.Branch), f) {
					continue
				}
				a.conversationRow(c.Key(conv.Agent+conv.ID), conv)
			}
		})
	}, func() {})
}

// conversationRow is a conversation: its first message, whose it is and how
// it went, and the way back to it.
func (a *App) conversationRow(c *ui.Context, conv proto.Conversation) {
	ui.Row(c).FillWidth().Shrink(0).MinHeight(44).Padding(theme.S1, theme.S3).Gap(theme.S2).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, nonEmpty(conv.Title, "(no message)"), widget.RunStyle{})
			meta := []string{a.agentName(conv.Agent), nonEmpty(conv.Branch, path.Base(conv.Dir)),
				fmt.Sprintf("%d %s", conv.Messages, textfmt.Plural(conv.Messages, "message", "messages"))}
			if conv.Model != "" {
				meta = append(meta, conv.Model)
			}
			meta = append(meta, textfmt.Age(conv.Updated, time.Now())+" ago")
			widget.Running(c.Key("meta"), strings.Join(meta, " · "), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		if widget.Button(c, widget.Secondary, nil, "Resume", "").Clicked() {
			a.resumeConversation(conv)
		}
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

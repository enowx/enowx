package activityview

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Draw shows the summary, the files edited, then the turns, newest first.
func (v *View) Draw(c *ui.Context, o Options) {
	v.follow(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		switch {
		case v.err != "":
			note(c, v.err, theme.Error)
			return
		case !v.loaded:
			note(c, "Reading the conversation…", theme.Muted)
			return
		case !v.supported:
			note(c, v.reason, theme.Muted)
			return
		case len(v.turns) == 0:
			note(c, "Nothing yet: the agent's work shows here as it goes.", theme.Muted)
			return
		}
		v.summary(c, o)
		ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(4, 0, 12, 0).Children(func() {
				if len(v.files) > 0 {
					section(c, "FILES EDITED", len(v.files))
					for _, f := range v.files {
						v.fileRow(c.Key("file:"+f.path), o, f)
					}
				}
				section(c, "TURNS", len(v.turns))
				working := isWorking(o.Session)
				for i := len(v.turns) - 1; i >= 0; i-- {
					v.turnRow(c.Key(strconv.Itoa(v.turns[i].start)), v.turns[i], working && i == len(v.turns)-1, o.Session.Dir)
				}
			})
		})
	})
}

func note(c *ui.Context, text string, col ui.Color) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(24).Children(func() {
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(col).TextAlign(ui.Center)
	})
}

func section(c *ui.Context, label string, n int) {
	ui.Row(c).FillWidth().Height(30).PaddingX(14).Gap(6).Margin(6, 0, 0, 0).Children(func() {
		ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
		ui.Text(c, strconv.Itoa(n)).FontSize(theme.TextLabel).TextColor(theme.Idle).SingleLine()
	})
}

// isWorking reports whether the agent is on its last turn.
func isWorking(s proto.SessionInfo) bool {
	if s.Exited {
		return false
	}
	if s.State != "" {
		return s.State == proto.StateWorking
	}
	return s.Active
}

// summary is a line of totals.
func (v *View) summary(c *ui.Context, o Options) {
	tools, failed, edits := v.totals()
	ui.Row(c).Height(42).Shrink(0).FillWidth().Padding(0, 14).Gap(14).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		ui.Icon(c, icon.List).FontSize(13).TextColor(theme.Accent)
		stat := func(n int, one, many string, col ui.Color) {
			ui.Text(c, fmt.Sprintf("%d %s", n, textfmt.Plural(n, one, many))).FontSize(theme.TextLabel).TextColor(col).SingleLine().Shrink(1)
		}
		stat(len(v.turns), "turn", "turns", theme.Text)
		stat(tools, "tool call", "tool calls", theme.TextSoft)
		stat(edits, "edit", "edits", theme.TextSoft)
		if failed > 0 {
			stat(failed, "failed", "failed", theme.Error)
		}
	})
}

// fileRow is a file the agent edited; clicking it shows it in Files.
func (v *View) fileRow(c *ui.Context, o Options, f edited) {
	b := ui.ButtonBase(c).FillWidth().Height(26).PaddingX(14).Gap(8).Justify(ui.Start).Cursor(ui.CursorPointer).Label(f.path)
	if b.Hovered() {
		b.Background(theme.Hover)
	}
	b.Children(func() {
		ui.Icon(c, icon.Pencil).FontSize(11).TextColor(theme.Muted)
		dir, name := path.Split(f.path)
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			if dir != "" {
				ui.Text(c, dir).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
			}
			ui.Text(c, name).Font(theme.MonoFont).FontSize(theme.TextLabel).SingleLine().Shrink(0)
		})
		if f.added > 0 {
			ui.Text(c, "+"+strconv.Itoa(f.added)).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Done).SingleLine().Shrink(0)
		}
		if f.removed > 0 {
			ui.Text(c, "−"+strconv.Itoa(f.removed)).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Error).SingleLine().Shrink(0)
		}
	})
	if b.Clicked() && o.OpenFile != nil && !strings.HasPrefix(f.path, "/") {
		o.OpenFile(f.path)
	}
}

// turnRow is a turn: its prompt, when and how long, and what it ran,
// unfolding to each tool call.
func (v *View) turnRow(c *ui.Context, t turn, running bool, dir string) {
	open := v.open[t.start]
	ui.Column(c).FillWidth().BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		b := ui.ButtonBase(c).FillWidth().Padding(8, 14).Gap(8).Justify(ui.Start).AlignItems(ui.Start).Cursor(ui.CursorPointer).
			Label(textfmt.FirstLine(t.prompt, 80))
		if b.Hovered() {
			b.Background(theme.Hover)
		}
		b.Children(func() {
			chev := icon.ChevronRight
			if open {
				chev = icon.ChevronDown
			}
			ui.Icon(c, chev).FontSize(11).TextColor(theme.Muted).Margin(3, 0, 0, 0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				prompt := textfmt.FirstLine(t.prompt, 120)
				if prompt == "" {
					prompt = "(no prompt)"
				}
				ui.Text(c, prompt).FontSize(theme.TextUI).TextColor(theme.Text).SingleLine().Shrink(1)
				ui.Text(c, v.turnFacts(t, running)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
			})
			if running {
				widget.Spinner(c, 12)
			}
		})
		if b.Clicked() {
			if open {
				delete(v.open, t.start)
			} else {
				v.open[t.start] = true
			}
		}
		if open {
			ui.Column(c).FillWidth().Padding(0, 14, 8, 33).Gap(2).Children(func() {
				if len(t.tools) == 0 {
					ui.Text(c, "No tools: it answered.").FontSize(theme.TextLabel).TextColor(theme.Muted)
				}
				for i, e := range t.tools {
					toolRow(c.Key(i), e, dir)
				}
			})
		}
	})
}

// turnFacts is a turn's when, how long and what it ran.
func (v *View) turnFacts(t turn, running bool) string {
	var parts []string
	if !t.at.IsZero() {
		parts = append(parts, textfmt.Age(t.at, time.Now())+" ago")
		end := t.end
		if running {
			end = time.Now()
		}
		if d := end.Sub(t.at).Round(time.Second); d > 0 {
			parts = append(parts, "worked "+d.String())
		}
	}
	parts = append(parts, fmt.Sprintf("%d %s", len(t.tools), textfmt.Plural(len(t.tools), "tool", "tools")))
	if t.edits > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", t.edits, textfmt.Plural(t.edits, "edit", "edits")))
	}
	if t.failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", t.failed))
	}
	return strings.Join(parts, " · ")
}

// toolRow is a tool call: its kind, what it worked on (a path in the
// session's folder written from there), how it went.
func toolRow(c *ui.Context, e proto.TranscriptEntry, dir string) {
	col := theme.TextSoft
	if e.Failed {
		col = theme.Error
	}
	ui.Row(c).FillWidth().Height(22).Gap(8).Children(func() {
		ui.Icon(c, toolIcon(e.Text)).FontSize(11).TextColor(theme.Muted)
		ui.Text(c, e.Text).FontSize(theme.TextLabel).TextColor(col).SingleLine().Shrink(0)
		ui.Text(c, textfmt.FirstLine(relative(dir, e.Detail), 140)).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
		ui.Spacer(c)
		if e.Added > 0 || e.Removed > 0 {
			ui.Text(c, fmt.Sprintf("+%d −%d", e.Added, e.Removed)).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(0)
		}
	})
}

// toolIcon is the icon of a tool, by what it does.
func toolIcon(name string) *ui.SVG {
	switch name {
	case "Bash", "BashOutput", "KillShell":
		return icon.Terminal
	case "Edit", "MultiEdit", "Write", "NotebookEdit":
		return icon.Pencil
	case "Read", "NotebookRead":
		return icon.File
	case "Grep", "Glob":
		return icon.Search
	case "WebFetch", "WebSearch":
		return icon.Globe
	case "Task", "Agent":
		return icon.Bot
	case "TodoWrite":
		return icon.List
	}
	return icon.Slash
}

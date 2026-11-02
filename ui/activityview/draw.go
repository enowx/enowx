package activityview

import (
	"fmt"
	"slices"
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

// Draw shows the totals, the files edited, then the turns, newest first.
func (v *View) Draw(c *ui.Context, o Options) {
	v.follow(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(0, theme.S3, theme.S3, theme.S3).Gap(theme.S2).Children(func() {
		switch {
		case v.err != "":
			note(c, v.err, theme.Error)
			return
		case !v.loaded:
			note(c, "Reading the conversation…", theme.TextSoft)
			return
		case !v.supported:
			note(c, v.reason, theme.TextSoft)
			return
		case len(v.turns) == 0:
			note(c, "Nothing yet: the agent's work shows here as it goes.", theme.TextSoft)
			return
		}
		v.summary(c)
		ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				if len(v.files) > 0 {
					section(c, "Files edited", len(v.files))
					for _, f := range v.files {
						v.fileRow(c.Key("file:"+f.path), o, f)
					}
				}
				if len(v.files) > 0 {
					section(c, "Turns", len(v.turns))
				}
				working := isWorking(o.Session)
				for i := len(v.turns) - 1; i >= 0; i-- {
					v.turnRow(c.Key(strconv.Itoa(v.turns[i].start)), o, v.turns[i], working && i == len(v.turns)-1)
				}
			})
		})
	})
}

func note(c *ui.Context, text string, col ui.Color) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(theme.S5).Children(func() {
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(col).TextAlign(ui.Center)
	})
}

func section(c *ui.Context, label string, n int) {
	ui.Row(c.Key("section:" + label)).FillWidth().Height(28).Shrink(0).PaddingX(theme.S3).Gap(theme.S1).Children(func() {
		ui.Text(c, label).FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft).SingleLine()
		ui.Text(c, strconv.Itoa(n)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
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

// summary is the totals, a column each; failed ones stand out when there are any.
func (v *View) summary(c *ui.Context) {
	tools, failed, edits := v.totals()
	ui.Row(c.Key("totals")).FillWidth().Shrink(0).Gap(theme.S1).Children(func() {
		stat := func(label string, n int, col ui.Color) {
			ui.Column(c.Key(label)).Grow(1).Basis(0).MinWidth(0).Gap(theme.S1).Children(func() {
				widget.Running(c, label, widget.RunStyle{Color: theme.TextSoft})
				ui.Text(c, strconv.Itoa(n)).FontSize(theme.TextUI).FontWeight(500).TextColor(col).SingleLine()
			})
		}
		failedCol := theme.Text
		if failed > 0 {
			failedCol = theme.Error
		}
		stat("Turns", len(v.turns), theme.Text)
		stat("Tool calls", tools, theme.Text)
		stat("Edits", edits, theme.Text)
		stat("Failed", failed, failedCol)
	})
}

// rowFace is the look of a row that can be clicked: tinted under the pointer.
func rowFace(b ui.Element) {
	bg := theme.Fade(theme.Selected, 0)
	if b.Hovered() {
		bg = theme.Hover
	}
	b.Background(widget.QuickTint(b, "bg", bg))
}

// fileRow is a file the agent edited, in the folder or not; clicking one in the
// folder shows it in a tab.
func (v *View) fileRow(c *ui.Context, o Options, f edited) {
	b := ui.ButtonBase(c).FillWidth().MinHeight(28).PaddingX(theme.S3).Gap(theme.S2).Radius(theme.RadiusControl).Justify(ui.Start).Label(f.path)
	absolute := strings.HasPrefix(f.path, "/")
	if !absolute {
		b.Cursor(ui.CursorPointer)
		rowFace(b)
	}
	b.Children(func() {
		ui.Icon(c, icon.Pencil).FontSize(14).TextColor(theme.TextSoft)
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, f.path, widget.RunStyle{Mono: true})
		})
		counts(c, f.added, f.removed)
	})
	if b.Clicked() && o.OpenFile != nil && !absolute {
		o.OpenFile(f.path)
	}
}

// counts are the lines added and removed, as +n −n.
func counts(c *ui.Context, added, removed int) {
	if added > 0 {
		ui.Text(c, "+"+strconv.Itoa(added)).FontSize(theme.TextLabel).TextColor(theme.Add).SingleLine().Shrink(0)
	}
	if removed > 0 {
		ui.Text(c, "−"+strconv.Itoa(removed)).FontSize(theme.TextLabel).TextColor(theme.Del).SingleLine().Shrink(0)
	}
}

// turnRow is a turn: its prompt, when and how long, and what it ran, which run
// when too long, unfolding to each tool call.
func (v *View) turnRow(c *ui.Context, o Options, t turn, running bool) {
	open := v.open[t.start]
	prompt := textfmt.FirstLine(t.prompt, 120)
	if prompt == "" {
		prompt = "(no prompt)"
	}
	ui.Column(c).FillWidth().Children(func() {
		b := ui.ButtonBase(c).FillWidth().MinHeight(36).Padding(theme.S1, theme.S3).Gap(theme.S2).Radius(theme.RadiusControl).Justify(ui.Start).
			Cursor(ui.CursorPointer).Label(textfmt.FirstLine(t.prompt, 80))
		rowFace(b)
		if !t.at.IsZero() {
			b.Tooltip(textfmt.Age(t.at, time.Now()) + " ago")
		}
		b.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, prompt, widget.RunStyle{})
				widget.Running(c.Key("facts"), v.turnFacts(t, running), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			if running {
				widget.Spinner(c, 12)
			}
			chev := icon.ChevronRight
			if open {
				chev = icon.ChevronDown
			}
			ui.Icon(c, chev).FontSize(12).TextColor(theme.TextSoft)
		})
		if b.Clicked() {
			if open {
				delete(v.open, t.start)
			} else {
				v.open[t.start] = true
			}
		}
		if open {
			ui.Column(c).FillWidth().Padding(theme.S1, theme.S3, theme.S2, theme.S3).Children(func() {
				if len(t.tools) == 0 {
					ui.Text(c, "No tools: it answered.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
				}
				for i, e := range t.tools {
					toolRow(c.Key(i), o, e)
				}
			})
		}
	})
}

// turnFacts is how long a turn worked and what it ran.
func (v *View) turnFacts(t turn, running bool) string {
	var parts []string
	if !t.at.IsZero() {
		end := t.end
		if running {
			end = time.Now()
		}
		if d := end.Sub(t.at).Round(time.Second); d > 0 {
			parts = append(parts, spell(d))
		}
	}
	parts = append(parts, fmt.Sprintf("%d %s", len(t.tools), textfmt.Plural(len(t.tools), "tool", "tools")))
	if n := len(t.files); n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", n, textfmt.Plural(n, "file", "files")))
	}
	if t.failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", t.failed))
	}
	return strings.Join(parts, " · ")
}

// spell writes a duration with room to read it: 4m 12s, 1h 03m.
func spell(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// toolRow is a tool call: its kind and what it worked on (a path in the
// session's folder written from there), which runs when too long, and how it
// went. What it read opens as a file and what it edited as its changes.
func toolRow(c *ui.Context, o Options, e proto.TranscriptEntry) {
	col := theme.Text
	if e.Failed {
		col = theme.Error
	}
	detail := textfmt.FirstLine(relative(o.Session.Dir, e.Detail), 140)
	open := openOf(o, e, detail)
	b := ui.ButtonBase(c).FillWidth().Height(24).Gap(theme.S2).Radius(theme.RadiusSmall).Justify(ui.Start).Label(e.Text + " " + detail)
	if open != nil {
		b.Cursor(ui.CursorPointer)
		rowFace(b)
	}
	b.Children(func() {
		ui.Icon(c, toolIcon(e.Text)).FontSize(14).TextColor(theme.TextSoft)
		ui.Text(c, e.Text).FontSize(theme.TextUI).TextColor(col).SingleLine().Shrink(0)
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, detail, widget.RunStyle{Mono: true, Color: col})
		})
		counts(c, e.Added, e.Removed)
	})
	if open != nil && b.Clicked() {
		open()
	}
}

// openOf is what clicking a tool call opens, nil for nothing: a file of the
// session's folder it read or edited, not one it failed on.
func openOf(o Options, e proto.TranscriptEntry, p string) func() {
	if p == "" || strings.HasPrefix(p, "/") || e.Failed {
		return nil
	}
	switch {
	case slices.Contains(editTools, e.Text) && o.OpenDiff != nil:
		return func() { o.OpenDiff(p) }
	case slices.Contains(editTools, e.Text) || e.Text == "Read" || e.Text == "NotebookRead":
		if o.OpenFile != nil {
			return func() { o.OpenFile(p) }
		}
	}
	return nil
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

package diffview

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Review comments: the user clicks a line of the diff, writes what should
// change there, and sends every comment at once to the session's agent as
// one prompt, the way a reviewer leaves comments on a pull request.

// Comment is a review comment on a line of the diff.
type Comment struct {
	File string
	// Line is the line's number in the new file, or with Old in the file
	// as it was, for a removed line.
	Line int
	Old  bool
	Code string // the line commented on, as it reads
	Text string
}

// at reports whether c is on line l of file.
func (c Comment) at(file string, l proto.DiffLine) bool {
	old, n := lineAt(l)
	return c.File == file && c.Old == old && c.Line == n
}

// lineAt is where a line of the diff is: its number in the new file, or
// in the old one for a removed line.
func lineAt(l proto.DiffLine) (old bool, n int) {
	if l.Kind == '-' {
		return true, l.Old
	}
	return false, l.New
}

// draft is the comment being written, on a line.
type draft struct {
	file string
	line proto.DiffLine
	text string
}

// at reports whether d is on line l of file.
func (d *draft) at(file string, l proto.DiffLine) bool {
	old, n := lineAt(l)
	dOld, dn := lineAt(d.line)
	return d.file == file && old == dOld && n == dn
}

// startDraft opens a comment on line l of file, or closes the one open
// there.
func (v *View) startDraft(file string, l proto.DiffLine) {
	if d := v.draft; d != nil && d.at(file, l) {
		v.draft = nil
	} else {
		v.draft = &draft{file: file, line: l}
	}
	v.layoutRows()
}

// addDraft keeps the comment being written, if it says something.
func (v *View) addDraft() {
	d := v.draft
	if d == nil {
		return
	}
	if text := strings.TrimSpace(d.text); text != "" {
		old, n := lineAt(d.line)
		v.comments = append(v.comments, Comment{File: d.file, Line: n, Old: old, Code: d.line.Text, Text: text})
	}
	v.draft = nil
	v.layoutRows()
}

func (v *View) removeComment(i int) {
	if i >= 0 && i < len(v.comments) {
		v.comments = append(v.comments[:i:i], v.comments[i+1:]...)
	}
	v.layoutRows()
}

// commentsOn counts the comments on file.
func (v *View) commentsOn(file string) int {
	n := 0
	for _, c := range v.comments {
		if c.File == file {
			n++
		}
	}
	return n
}

// ReviewPrompt is the prompt that hands comments to the agent: what they
// are about, then each with the line it is on.
func ReviewPrompt(branch, base string, comments []Comment) string {
	var b strings.Builder
	what := "your changes"
	switch {
	case branch != "" && base != "":
		what = "your changes on " + branch + " (compared with " + base + ")"
	case branch != "":
		what = "your changes on " + branch
	}
	fmt.Fprintf(&b, "Review comments on %s. Address each one:\n", what)
	for i, c := range comments {
		where := fmt.Sprintf("%s:%d", c.File, c.Line)
		if c.Old {
			where = fmt.Sprintf("%s, removed line %d", c.File, c.Line)
		}
		fmt.Fprintf(&b, "\n%d. %s", i+1, where)
		if code := strings.TrimSpace(c.Code); code != "" {
			fmt.Fprintf(&b, " (`%s`)", code)
		}
		b.WriteString("\n")
		for _, line := range strings.Split(c.Text, "\n") {
			b.WriteString("   " + line + "\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// sendComments hands every comment to the agent and clears them.
func (v *View) sendComments(o Options) {
	if len(v.comments) == 0 || o.Send == nil {
		return
	}
	o.Send(ReviewPrompt(v.status.Branch, v.status.Base, v.comments))
	o.Notify(fmt.Sprintf("Sent %d review %s to the agent", len(v.comments), textfmt.Plural(len(v.comments), "comment", "comments")))
	v.comments, v.draft = nil, nil
	v.layoutRows()
}

// commentRow shows comment i under its line.
func (v *View) commentRow(c *ui.Context, i int) {
	cm := v.comments[i]
	ui.Row(c).FillWidth().Padding(6, 10, 6, 70).Gap(8).Background(theme.Fade(theme.Accent, 0.08)).
		BorderWidth(0, 0, 0, 2).BorderColor(theme.Accent).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, icon.Chat).FontSize(12).TextColor(theme.Accent).Margin(2, 0, 0, 0)
		ui.Text(c, cm.Text).FontSize(theme.TextUI).TextColor(theme.Text).Grow(1).Shrink(1)
		if widget.IconButton(c, icon.Close, "Remove the comment", false).Clicked() {
			v.removeComment(i)
		}
	})
}

// draftRow is the comment being written, under its line.
func (v *View) draftRow(c *ui.Context) {
	d := v.draft
	ui.Column(c).FillWidth().Padding(8, 10, 8, 70).Gap(8).Background(theme.Panel).
		BorderWidth(0, 0, 0, 2).BorderColor(theme.Accent).Children(func() {
		in := widget.Input(c, &d.text, "What should change here?").FillWidth().AutoFocus()
		if in.Submitted() {
			v.addDraft()
			return
		}
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, "The comments go to the agent together.").FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
			ui.Spacer(c)
			if widget.GhostButton(c, "Cancel").Clicked() {
				v.draft = nil
				v.layoutRows()
			}
			if widget.Button(c, widget.Primary, icon.Check, "Add comment", "").Clicked() {
				v.addDraft()
			}
		})
	})
}

// commentBar offers to send the comments, while there are any.
func (v *View) commentBar(c *ui.Context, o Options) {
	n := len(v.comments)
	if n == 0 {
		return
	}
	ui.Row(c).Shrink(0).FillWidth().Padding(8, 10, 8, 14).Gap(8).Background(theme.Panel).
		BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Icon(c, icon.Chat).FontSize(13).TextColor(theme.Accent)
		ui.Text(c, fmt.Sprintf("%d review %s", n, textfmt.Plural(n, "comment", "comments"))).FontSize(theme.TextUI).SingleLine().Shrink(1)
		ui.Spacer(c)
		if widget.GhostButton(c, "Discard").Clicked() {
			v.comments, v.draft = nil, nil
			v.layoutRows()
		}
		if widget.Button(c, widget.Primary, icon.ArrowUp, "Send to agent", "").Clicked() {
			v.sendComments(o)
		}
	})
}

package notesview

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

type imageRename struct {
	open, focused, focusRequested, submit bool
	ref, name, err                        string
}

// renameImage changes the display name and mentions while keeping the asset path.
func (e *editor) renameImage(ref, name string) error {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || utf8.RuneCountInString(name) > 80 || strings.ContainsAny(name, "[]\\`") {
		return fmt.Errorf("use 1–80 characters, without brackets, backslashes or backticks")
	}
	mention := regexp.MustCompile(`\[@[^\]]*\]\(` + regexp.QuoteMeta(ref) + `\)`)
	for i := range e.blocks {
		b := &e.blocks[i]
		if b.kind == kCode {
			continue
		}
		text := b.text
		matches := imageMatches(text)
		for j := len(matches) - 1; j >= 0; j-- {
			m := matches[j]
			if imageSource(text[m[4]:m[5]]) == ref {
				text = text[:m[2]] + name + text[m[3]:]
			}
		}
		mentions := markdownTokenMatches(text, mention, '[')
		for j := len(mentions) - 1; j >= 0; j-- {
			m := mentions[j]
			text = text[:m[0]] + imageReference(name, ref) + text[m[1]:]
		}
		if text != b.text {
			b.text, e.changed = text, true
		}
	}
	e.imageEditors = nil
	return nil
}

func (e *editor) imageRenameDialog(c *ui.Context) {
	r := e.rename
	if r == nil || !r.open {
		return
	}
	widget.Dialog(c, &r.open, 420, theme.S3, func() {
		if widget.DialogHeader(c, "Rename image", "Used under the image and in its mentions on this page.", true) {
			r.open = false
		}
		in := ui.TextInputBase(c, &r.name).FillWidth().Height(theme.ControlHeight).PaddingX(theme.S3).Radius(theme.RadiusControl).FontSize(theme.TextUI).
			TextColor(theme.Text).Placeholder("Image name").Label("Image name")
		widget.Well(in, in.Focused())
		if !r.focused {
			in.Focus()
			if r.focusRequested {
				in.SetTextSelection(0, utf8.RuneCountInString(r.name))
				r.focused = true
			}
			r.focusRequested = true
		}
		in.OnTextKey(func(k ui.TextKey) bool {
			if k.Key == ui.KeyEnter {
				r.name, r.submit = k.Text(), true
				return true
			}
			return false
		})
		if r.err != "" {
			ui.Text(c, r.err).FontSize(theme.TextLabel).TextColor(theme.Error).FillWidth()
		}
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				r.open = false
			}
			if widget.PrimaryButton(c, nil, "Rename", "").Disabled(strings.TrimSpace(r.name) == "").Clicked() {
				r.submit = true
			}
		})
		if r.submit {
			r.submit = false
			if err := e.renameImage(r.ref, r.name); err != nil {
				r.err = err.Error()
			} else {
				r.open = false
			}
		}
	})
}

package platformview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// promptState is the system prompt tab's. What is typed stays through closing the sheet
// or changing tabs, until it is saved, and until the host is another.
type promptState struct {
	host    string // the host it was read from
	loaded  bool
	loading bool
	saved   proto.PlatformPrompt
	draft   string
	busy    bool
	err     string
	note    string
}

// Editor's height: what the window leaves it, within these.
const (
	promptMostH  = 460
	promptLeastH = 140
	// promptAround is about what the sheet takes around the editor.
	promptAround = 330
)

// load reads the host's prompt, once for each host. A draft left typed for the same
// host is kept.
func (v *View) load(o Options) {
	p := &v.prompt
	if p.host != o.Host {
		*p = promptState{host: o.Host}
	}
	if p.loaded || p.loading || o.LoadPrompt == nil {
		return
	}
	p.loading, p.err = true, ""
	host := o.Host
	o.LoadPrompt(func(got proto.PlatformPrompt, err error) {
		if p.host != host {
			return
		}
		p.loading = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.loaded, p.saved, p.draft = true, got, got.Text
	})
}

// promptBody is the system prompt tab: what it does, where it is kept, the editor.
func (v *View) promptBody(c *ui.Context, o Options) {
	v.load(o)
	p := &v.prompt
	if !p.loaded {
		if p.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, "Couldn't read the prompt", p.err, func() {
				if widget.Button(c.Key("again"), widget.Secondary, nil, "Try again", "").Clicked() {
					v.load(o)
				}
			})
			return
		}
		ui.Row(c.Key("reading")).FillWidth().Justify(ui.Center).Padding(theme.S4).Children(func() { widget.Spinner(c, 14) })
		return
	}
	quiet(c, "Replaces Claude Platform's built-in prompt whole, for sessions started from now on on "+o.Host+". enowX's guide still goes after it.")
	if p.saved.Path != "" {
		ui.Row(c.Key("path")).FillWidth().MinWidth(0).Children(func() {
			widget.Running(c, p.saved.Path, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft, Mono: true})
		})
	}
	_, h := c.Size()
	height := min(max(h-promptAround, promptLeastH), promptMostH)
	widget.TextBox(c.Key("editor"), &p.draft, "Empty: sessions use the built-in prompt. Write your own to replace it.", height).
		FillWidth().Shrink(0).Font(theme.MonoFont).Label("System prompt")
	switch {
	case p.err != "":
		ui.Text(c, widget.LongWords(p.err)).FontSize(theme.TextLabel).TextColor(theme.Error)
	case p.draft != p.saved.Text:
		quiet(c, "Not saved yet.")
	case p.note != "":
		quiet(c, p.note)
	}
}

// promptButtons are the system prompt tab's foot: going back to the built-in prompt,
// which asks first, and saving.
func (v *View) promptButtons(c *ui.Context, o Options) {
	p := &v.prompt
	if !p.loaded {
		return
	}
	if p.saved.Custom && widget.Button(c.Key("reset"), widget.Secondary, nil, "Reset to built-in", "").Disabled(p.busy).Clicked() && !p.busy {
		o.Confirm("Reset the system prompt?",
			"Your prompt on "+o.Host+" is deleted, and what you typed since with it. Sessions started from now on use the built-in prompt.", "Reset", func() {
				v.resetPrompt(o)
			})
	}
	can := !p.busy && p.draft != p.saved.Text && strings.TrimSpace(p.draft) != ""
	b := widget.PrimaryButton(c.Key("save"), nil, "Save", "").Disabled(!can)
	if !can {
		b.Opacity(0.45)
	}
	if b.Clicked() && can {
		p.busy, p.err, p.note = true, "", ""
		o.SavePrompt(p.draft, func(got proto.PlatformPrompt, err error) {
			p.busy = false
			if err != nil {
				p.err = err.Error()
				return
			}
			p.saved, p.note = got, "Saved. Sessions started from now on use it."
		})
	}
}

// resetPrompt removes the host's prompt.
func (v *View) resetPrompt(o Options) {
	p := &v.prompt
	p.busy, p.err, p.note = true, "", ""
	o.ResetPrompt(func(got proto.PlatformPrompt, err error) {
		p.busy = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.saved, p.draft, p.note = got, "", "Back to the built-in prompt."
	})
}

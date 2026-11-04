package platformview

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Key is one of the vault's Anthropic credentials as the sheet shows it: no value.
type Key struct {
	ID, Name string
	Status   string   // how it fared when last checked ("Works", "Refused")
	Color    ui.Color // of Status
	LastUsed time.Time
	Default  bool
	Testing  bool
}

// Options are what the sheet shows and what it does; the app does it.
type Options struct {
	Host string // the host connected to, whose prompt shows
	// VaultOpen says the vault's keys show; otherwise Gate draws making or unlocking it.
	VaultOpen bool
	Gate      func()
	Keys      []Key
	// AddKey opens the vault's form for a new key.
	AddKey func()
	// TestKey asks Anthropic about it, done saying how that went.
	TestKey func(id string, done func(message string))
	// RemoveKey takes it out of the vault.
	RemoveKey func(id string)
	// SetDefault makes it the key sessions get.
	SetDefault func(id string)
	// OpenURL opens a page in the browser; nil leaves out what links to one.
	OpenURL func(url string)
	// Confirm asks before something that can't be undone: do runs on yes.
	Confirm func(title, message, action string, do func())
	// The host's system prompt: read, kept, or back to the built-in one. Each calls
	// done on the main thread.
	LoadPrompt  func(done func(proto.PlatformPrompt, error))
	SavePrompt  func(text string, done func(proto.PlatformPrompt, error))
	ResetPrompt func(done func(proto.PlatformPrompt, error))
}

// keysBody is the keys tab: the vault's Anthropic keys, or what has to come first.
func (v *View) keysBody(c *ui.Context, o Options) {
	if !o.VaultOpen {
		quiet(c, "Claude Platform's keys stay in your credential vault, sealed on this device.")
		ui.Row(c.Key("gate")).FillWidth().Justify(ui.Center).Children(func() {
			if o.Gate != nil {
				o.Gate()
			}
		})
		return
	}
	quiet(c, "Sessions borrow a key from your vault when they start, after you allow their host. It is never written to a file there.")
	if len(o.Keys) == 0 {
		widget.EmptyState(c, icon.Key, "No Anthropic API key yet", "Make one in the Claude Console and add it here. Until then Claude Platform can't answer, unless the host's own environment has one.", nil)
		return
	}
	now := time.Now()
	for _, k := range o.Keys {
		v.keyRow(c.Key(k.ID), o, k, now)
	}
}

// keysButtons are the keys tab's foot.
func (v *View) keysButtons(c *ui.Context, o Options) {
	if o.OpenURL != nil && widget.Button(c.Key("console"), widget.Secondary, icon.Globe, "Open the Claude Console", "").Clicked() {
		o.OpenURL(ConsoleURL)
	}
	if o.VaultOpen && o.AddKey != nil && widget.PrimaryButton(c.Key("add"), icon.Plus, "Add API key", "").Clicked() {
		o.AddKey()
	}
}

// keyRow is a key: its name, which runs inside the row, how it fares and when it was
// last used, and what can be done with it.
func (v *View) keyRow(c *ui.Context, o Options, k Key, now time.Time) {
	row := ui.Row(c).FillWidth().Shrink(0).MinHeight(theme.RowHeight).Padding(theme.S2, theme.S2, theme.S2, theme.S3).Gap(theme.S2).
		AlignItems(ui.Center).Radius(theme.RadiusCard).Background(theme.Sunken)
	row.Children(func() {
		widget.Dot(c, k.Color, false, 8)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			ui.Row(c).FillWidth().MinWidth(0).Children(func() { widget.Running(c, k.Name, widget.RunStyle{Weight: 500, Color: theme.Text}) })
			ui.Row(c.Key("sub")).FillWidth().MinWidth(0).Children(func() {
				widget.Running(c, keyLine(k, now), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			if said := v.tested[k.ID]; said != "" {
				ui.Row(c.Key("said")).FillWidth().MinWidth(0).Children(func() {
					widget.Running(c, said, widget.RunStyle{Size: theme.TextLabel, Color: k.Color})
				})
			}
		})
		ui.Row(c.Key("acts")).Shrink(0).Gap(theme.S1).AlignItems(ui.Center).Children(func() {
			if k.Testing {
				widget.Spinner(c, 14)
			} else if widget.GhostButton(c.Key("test"), "Test").Label("Test "+k.Name).Tooltip("Ask Anthropic whether it still works").Clicked() && o.TestKey != nil {
				id := k.ID
				o.TestKey(id, func(message string) {
					if v.tested == nil {
						v.tested = map[string]string{}
					}
					v.tested[id] = message
				})
			}
			if !k.Default && len(o.Keys) > 1 && o.SetDefault != nil &&
				widget.GhostButton(c.Key("default"), "Use").Label("Use "+k.Name).Tooltip("Make it the key sessions get").Clicked() {
				o.SetDefault(k.ID)
			}
			if widget.IconButton(c.Key("remove"), icon.Trash, "Remove "+k.Name, false).Clicked() {
				v.askRemove(o, k)
			}
		})
	})
}

// askRemove confirms removing a key, saying what stops working.
func (v *View) askRemove(o Options, k Key) {
	do := func() {
		if o.RemoveKey != nil {
			o.RemoveKey(k.ID)
		}
		delete(v.tested, k.ID)
	}
	msg := widget.LongWords(k.Name) + " leaves your vault on every device. Claude Platform sessions on it stop being able to answer when they next start, and whatever else borrows it loses it too. Anthropic still takes the key: delete it in the Claude Console to end it."
	if len(o.Keys) > 1 {
		msg += " Sessions move to your other key."
	} else {
		msg += " It is your only one: Claude Platform has no key without it."
	}
	if o.Confirm != nil {
		o.Confirm("Remove this API key?", msg, "Remove", do)
	}
}

// keyLine says how a key fares and when it was last used.
func keyLine(k Key, now time.Time) string {
	line := k.Status
	if k.Default {
		line += " · sessions get this one"
	}
	if !k.LastUsed.IsZero() {
		line += " · used " + strings.ToLower(textfmt.Ago(k.LastUsed, now))
	}
	return line
}

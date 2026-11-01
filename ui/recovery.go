package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The account's recovery key (docs/cloud.md, Keys), in Settings › Cloud's
// Devices tab: made and shown once, to keep away from every device; used
// on a new device to get the teams' keys back when the others are lost.

type recoveryState struct {
	shown   string // the key just made, shown once
	copied  widget.CopyFeedback
	using   bool
	typed   string
	err     string
	done    string
	working bool
}

func (a *App) recoverySection(c *ui.Context, cs *cloudSettings) {
	rs := &cs.recovery
	set := false
	for _, d := range cs.devices {
		set = set || d.Recovery
	}
	widget.Field(c, "Recovery key", func() {
		msg := "No recovery key yet. With one, a new device gets your teams' keys back when all of yours are lost; without, a teammate's device shares them."
		if set {
			msg = "Set. Keep it away from your devices (a password manager, paper). Making a new one replaces it."
		}
		ui.Text(c, msg).FontSize(theme.TextLabel).TextColor(theme.Muted)
		if rs.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, rs.err, "", nil)
		}
		if rs.done != "" {
			ui.Text(c, rs.done).FontSize(theme.TextLabel).TextColor(theme.Done)
		}
		ui.Row(c).Gap(8).Children(func() {
			label := "Make a recovery key"
			if set {
				label = "Make a new one"
			}
			if widget.Button(c, widget.Secondary, icon.Key, label, "").Clicked() && !rs.working {
				make := func() { a.makeRecovery(rs) }
				if set {
					a.confirm("Replace your recovery key?", "The one you kept stops working: only the new one gets your teams' keys back.", "Replace it", make)
				} else {
					make()
				}
			}
			if widget.GhostButton(c, "Use a recovery key…").Clicked() {
				rs.using, rs.typed, rs.err = true, "", ""
			}
		})
	})
	if rs.shown != "" {
		open := true
		widget.FormSheet(c, &open, 520, "Your recovery key", "Shown this once. Keep it away from your devices: whoever has it, and signs in as you, gets your teams' keys.", func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Padding(10, 12).Radius(theme.RadiusControl).Background(theme.Sunken).Border(1, theme.Border).Children(func() {
				ui.Text(c, rs.shown).Font(theme.MonoFont).FontSize(theme.TextUI).Grow(1).Shrink(1).MinWidth(0).Selectable()
				widget.CopyIcon(c.Key("rk-copy"), &rs.copied, icon.Copy, "Copy the key", "Copy the recovery key", rs.shown)
			})
		}, func() {
			if widget.PrimaryButton(c, nil, "I kept it", "").Clicked() {
				rs.shown = ""
			}
		})
		if !open {
			rs.shown = ""
		}
	}
	if rs.using {
		widget.FormSheet(c, &rs.using, 520, "Use a recovery key", "Gets your teams' keys back on "+a.host.Name+", for when your other devices are lost.", func() {
			widget.Input(c, &rs.typed, "enxr-XXXX-XXXX-…").FillWidth().Font(theme.MonoFont)
		}, func() {
			b := widget.PrimaryButton(c, icon.Key, "Recover", "")
			if strings.TrimSpace(rs.typed) == "" || rs.working {
				b.Disabled(true)
			} else if b.Clicked() {
				a.useRecovery(rs)
			}
		})
	}
}

func (a *App) makeRecovery(rs *recoveryState) {
	cl := a.client
	rs.working, rs.err, rs.done = true, "", ""
	go func() {
		out, err := cl.NewRecoveryKey()
		a.onMain(func() {
			rs.working = false
			if err != nil {
				rs.err = client.Explain(err)
				return
			}
			rs.shown = out.Key
			a.loadCloud()
		})
	}()
}

func (a *App) useRecovery(rs *recoveryState) {
	cl, key := a.client, strings.TrimSpace(rs.typed)
	rs.working, rs.err = true, ""
	go func() {
		out, err := cl.UseRecoveryKey(key)
		a.onMain(func() {
			rs.working = false
			if err != nil {
				rs.err = strings.TrimPrefix(client.Explain(err), "cloud: ")
				return
			}
			rs.using, rs.typed = false, ""
			rs.done = "Got back " + textfmt.Plural(out.Recovered, "1 key", fmt.Sprintf("%d keys", out.Recovered)) + " of your teams: their spaces sync again."
		})
	}()
}

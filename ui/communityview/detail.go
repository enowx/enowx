package communityview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// detail is the sheet for one design.
type detail struct {
	open   bool
	item   proto.LibraryListing
	prompt string
	busy   bool
	err    string
	copied widget.CopyFeedback
}

// detailPicture is the picture's height in the sheet.
const detailPicture = 260

// detailSheet says what a design is and what its license allows, as the cloud
// answered: a view-only design offers nothing to copy. Trying it is on its page,
// in the browser: its HTML doesn't run here.
func (v *View) detailSheet(c *ui.Context, o Options) {
	d := v.detail
	if d == nil {
		return
	}
	if !d.open {
		v.detail = nil
		return
	}
	it := d.item
	widget.Sheet(c.Key("detail"), &d.open, 720, func() {
		// A title is anyone's and as long as they like: it runs, the frame stays.
		ui.Column(c.Key("head")).Grow(1).Basis(0).MinWidth(0).Gap(theme.S1).Children(func() {
			ui.Row(c.Key("title")).FillWidth().MinWidth(0).Children(func() {
				widget.Running(c, it.Title, widget.RunStyle{Size: theme.TextTitle, Weight: 600, Always: true})
			})
			ui.Row(c.Key("by")).FillWidth().MinWidth(0).Children(func() {
				widget.Running(c, "By "+it.Handle+", "+kindName(it.Kind), widget.RunStyle{Color: theme.TextSoft})
			})
		})
		if widget.IconButton(c, icon.Close, "Close", false).Clicked() {
			d.open = false
		}
	}, func() {
		if d.err != "" {
			widget.Banner(c.Key("detail-error"), theme.Error, d.err, nil)
		}
		v.pictureBox(c, o, it, detailPicture, ui.Contain)
		if it.Description != "" {
			ui.Text(c, it.Description).FontSize(theme.TextUI).TextColor(theme.Text).FillWidth()
		}
		ui.Column(c.Key("facts")).FillWidth().Gap(theme.S1).Children(func() {
			widget.KV(c.Key("license"), "License", licenseName(it.License), false)
			if it.Category != "" {
				widget.KV(c.Key("category"), "Category", categoryName(it.Category), false)
			}
			if len(it.Tags) > 0 {
				widget.KV(c.Key("tags"), "Tags", strings.Join(it.Tags, ", "), false)
			}
			if it.HasMotion {
				widget.KV(c.Key("motion"), "Motion", "It moves: its page shows it", false)
			}
		})
		widget.SectionLabel(c.Key("allows-label"), "What this design allows", theme.TextSoft)
		ui.Column(c.Key("allows")).FillWidth().Gap(theme.S1).Children(func() {
			allows(c, it.Actions)
		})
		ui.Text(c, "Other people's pages don't run in the app: try this one on its page, in your browser.").FontSize(theme.TextLabel).TextColor(theme.TextSoft).FillWidth()
	}, func() {
		if widget.GhostButton(c, "Close").Clicked() {
			d.open = false
		}
		v.detailButtons(c, o, d)
	})
}

// allows lists what the item allows, or says it may only be looked at.
func allows(c *ui.Context, a proto.LibraryActions) {
	line := func(text string) {
		ui.Row(c.Key(text)).FillWidth().Gap(theme.S2).Children(func() {
			ui.Icon(c, icon.Check).FontSize(14).TextColor(theme.TextSoft)
			ui.Text(c, text).FontSize(theme.TextUI).TextColor(theme.Text).Grow(1).MinWidth(0)
		})
	}
	if a == (proto.LibraryActions{}) {
		ui.Text(c, "View only: look at it, nothing can be copied or opened from it.").FontSize(theme.TextUI).TextColor(theme.Text).FillWidth()
		return
	}
	if a.CopyPrompt {
		line("Copy its prompt, to rebuild it with an agent")
	}
	if a.UseComponent {
		line("Copy the component's HTML and CSS, on its page")
	}
	if a.UseReference {
		line("Use it as a reference or a skill")
	}
	if a.OpenInEnowX {
		line("Open a copy as a canvas")
	}
}

// detailButtons are Copy prompt, when the design allows it, and its page. The
// prompt is asked for once, when it's wanted.
func (v *View) detailButtons(c *ui.Context, o Options, d *detail) {
	if d.item.Actions.CopyPrompt && o.Prompt != nil {
		if d.prompt != "" {
			widget.CopyButton(c, &d.copied, "Copy prompt", d.prompt)
		} else {
			b := widget.Button(c, widget.Secondary, icon.Copy, "Get prompt", "")
			if d.busy {
				b.Disabled(true)
			} else if b.Clicked() {
				d.busy, d.err = true, ""
				o.Prompt(d.item.ItemID, func(text string, err error) {
					d.busy = false
					if err != nil {
						d.err = err.Error()
						return
					}
					d.prompt = text
				})
			}
		}
	}
	if d.item.URL != "" && o.OpenURL != nil {
		if widget.PrimaryButton(c, icon.External, "Open in browser", "").Clicked() {
			o.OpenURL(d.item.URL)
		}
	}
}

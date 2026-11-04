package communityview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A card's picture and the lines under it: fixed, so a card never grows for what
// it holds; a long title runs instead.
const (
	pictureHeight = 176
	cardFoot      = 64
)

// grid is the results, scrolling under the search.
func (v *View) grid(c *ui.Context, o Options) {
	ui.Scroll(c.Key("grid-scroll")).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(theme.S1, theme.S4, theme.S4, theme.S4).Gap(theme.S3).Children(func() {
			if v.err != "" {
				widget.Banner(c.Key("error"), theme.Error, v.err, func() {
					if widget.GhostButton(c, "Try again").Clicked() {
						v.search(o)
					}
				})
			}
			switch {
			case len(v.items) == 0 && v.busy:
				ui.Text(c, "Looking in the library…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
				return
			case len(v.items) == 0 && v.known && v.err == "":
				ui.Text(c, "No designs match this").FontSize(theme.TextUI).TextColor(theme.TextSoft)
				return
			}
			widget.FitGrid(c.Key("grid"), cellWidth, mostColumns).Gap(theme.S3).Children(func() {
				for _, it := range v.items {
					v.card(c, o, it)
				}
			})
			if v.next != "" {
				ui.Row(c.Key("more")).FillWidth().Justify(ui.Center).Children(func() {
					b := widget.Button(c, widget.Secondary, nil, "Show more", "")
					if v.more {
						b.Disabled(true)
					} else if b.Clicked() {
						v.showMore(o)
					}
				})
			}
		})
	})
}

// card is a design in the grid: its picture, title, who made it and its license.
func (v *View) card(c *ui.Context, o Options, it proto.LibraryListing) {
	b := ui.ButtonBase(c.Key("card:"+it.ItemID)).MinWidth(0).Height(pictureHeight+cardFoot).Radius(theme.RadiusCard).Clip().
		Label("Open "+it.Title).Cursor(ui.CursorPointer).Background(theme.Raised).InsetShadow(0, theme.Rule, 0, 0, theme.Border)
	if b.Clicked() {
		v.detail = &detail{open: true, item: it}
	}
	b.Children(func() {
		ui.Column(c.Key("inside")).Grow(1).Basis(0).MinWidth(0).FillHeight().Children(func() {
			v.pictureBox(c, o, it, pictureHeight, ui.Cover)
			ui.Column(c.Key("foot")).FillWidth().Height(cardFoot).Shrink(0).Padding(theme.S2, theme.S3).Gap(theme.S1).Children(func() {
				ui.Row(c.Key("title")).FillWidth().MinWidth(0).Children(func() {
					widget.Running(c, it.Title, widget.RunStyle{Weight: 500})
				})
				ui.Row(c.Key("by")).FillWidth().MinWidth(0).Gap(theme.S2).Children(func() {
					ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
						widget.Running(c, it.Handle+" · "+kindName(it.Kind), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
					})
					widget.Tag(c, licenseName(it.License), theme.Text)
				})
			})
		})
	})
}

// pictureBox is a design's picture filling a box of height, its frame the same
// before it comes and if it never does.
func (v *View) pictureBox(c *ui.Context, o Options, it proto.LibraryListing, height float32, fit ui.Fit) {
	box := ui.Box(c.Key("picture")).FillWidth().Height(height).Shrink(0).Clip().Background(theme.Fade(theme.TextStrong, 0.06)).Center()
	box.Children(func() {
		if bmp := v.pictures.of(o, it); bmp != nil {
			ui.Image(c, bmp).FillWidth().FillHeight().Fit(fit).Label("Picture of " + it.Title)
			return
		}
		ui.Icon(c, icon.Image).FontSize(20).TextColor(theme.TextSoft)
	})
}

func kindName(kind string) string {
	if kind == "component" {
		return "Component"
	}
	return "Full page"
}

func licenseName(license string) string {
	switch license {
	case "mit":
		return "MIT"
	case "cc-by-4.0":
		return "CC BY 4.0"
	case "view-only":
		return "View only"
	}
	return license
}

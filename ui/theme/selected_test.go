package theme

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// In every style, appearance, palette and accent, words and icons read on what is
// selected, picked or hovered, on every surface it may lie on, secondary ones
// included, whatever their accent: bright, dark or a mid-tone.
func TestSelectedStatesRead(t *testing.T) {
	oldStyle, oldPalette := CurrentStyle(), Current()
	defer func() {
		UseStyle(oldStyle)
		Apply(oldPalette)
	}()
	accents := append([]ui.Color{{}}, Accents...)
	accents = append(accents, ui.Hex("#FFFFFF"), ui.Hex("#000000"), ui.Hex("#808080"), ui.Hex("#FFFF00"), ui.Hex("#FF00AA"), ui.Hex("#7F7F00"))
	for _, s := range Styles {
		UseStyle(s)
		for _, light := range []bool{false, true} {
			for _, palette := range Palettes {
				for _, accent := range accents {
					p := LookupAppearance(palette.ID, accent, light)
					Apply(p)
					q := styled(p)
					where := s.ID + " " + palette.ID + " " + Hex(accent)
					if light {
						where += " light"
					}
					read := func(what string, fg, bg ui.Color) {
						t.Helper()
						if r := contrast(fg, bg); r < 4.5 {
							t.Errorf("%s: %s: %.2f", where, what, r)
						}
					}
					read("on the accent", OnAccent, Accent)
					read("on what is picked", OnPicked, opaqueOver(Picked, q))
					for name, surface := range map[string]ui.Color{"panel": q.Panel, "frame": q.Frame, "menu": q.Popover, "dialog": q.Dialog} {
						ground := over(surface, q.Ground)
						selected, hovered := over(q.Selected, ground), over(q.Hover, ground)
						read("words on selected, "+name, q.Text, selected)
						read("strong words on selected, "+name, q.TextStrong, selected)
						read("secondary words on selected, "+name, SoftOn(true), selected)
						read("words under the pointer, "+name, q.Text, hovered)
						read("secondary words under the pointer, "+name, q.TextSoft, hovered)
						read("secondary words, "+name, q.TextSoft, ground)
					}
				}
			}
		}
	}
}

// opaqueOver is what is picked as it reads: a solid color, or a tint on the frame.
func opaqueOver(c ui.Color, p Palette) ui.Color {
	if c.A == 255 {
		return c
	}
	return over(c, over(p.Frame, p.Ground))
}

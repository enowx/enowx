package widget

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// SwatchColors are the colors a badge picks from, as #RRGGBB.
var SwatchColors = []string{"#7C6CF2", "#3B82F6", "#14B8A6", "#22A35A", "#E0A100", "#F97316", "#E5484D", "#D946EF"}

// ParseHex reads a #RRGGBB color; false for anything else.
func ParseHex(s string) (ui.Color, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return ui.Color{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return ui.Color{}, false
	}
	return ui.RGB(uint8(v>>16), uint8(v>>8), uint8(v)), true
}

// Swatches picks one of colors (#RRGGBB) into value, "" for none.
func Swatches(c *ui.Context, colors []string, value *string) {
	ui.Row(c).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
		swatch := func(key, hex string) {
			col, ok := ParseHex(hex)
			on := strings.EqualFold(*value, hex)
			b := ui.ButtonBase(c.Key("swatch:" + key)).Width(22).Height(22).Radius(theme.RadiusSmall).Cursor(ui.CursorPointer).Label("Color " + key)
			if on {
				b.Border(2, theme.Text)
			} else {
				b.Border(1, theme.Border)
			}
			b.Center().Children(func() {
				if ok {
					ui.Box(c).Width(14).Height(14).Radius(theme.RadiusSmall).Background(col)
				} else {
					ui.Text(c, "–").FontSize(theme.TextLabel).TextColor(theme.Muted)
				}
			})
			if Pressable(b).Clicked() {
				*value = hex
			}
		}
		swatch("none", "")
		for _, hex := range colors {
			swatch(hex, hex)
		}
	})
}

// BadgeTag is a Tag led by a dot of hex's color (#RRGGBB), for what a
// session or an item runs as.
func BadgeTag(c *ui.Context, label, hex string) {
	col, ok := ParseHex(hex)
	if !ok {
		col = theme.Accent
	}
	ui.Row(c).PaddingX(theme.S2).Height(18).Gap(theme.S1).Radius(theme.RadiusSmall).Border(1, theme.Fade(col, 0.45)).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Box(c).Width(6).Height(6).Radius(theme.RadiusTail).Background(col)
		ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
	})
}

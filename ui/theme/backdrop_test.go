package theme

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// However low the user sets the panels' opacity, words read on them over the
// lightest and the darkest image, in every palette and appearance; asked for
// more, they get it; and what terminals sit on stays opaque.
func TestBackdropKeepsWordsReadable(t *testing.T) {
	for _, preset := range Palettes {
		for _, light := range []bool{false, true} {
			base := LookupAppearance(preset.ID, ui.Color{}, light)
			low := WithBackdrop(base, 0)
			for _, image := range []ui.Color{ui.Hex("#FFFFFF"), ui.Hex("#000000"), ui.Hex("#808080")} {
				under := over(Fade(low.Ground, BackdropScrim), image)
				for name, surface := range map[string]ui.Color{"panel": low.Panel, "frame": low.Frame} {
					bg := over(surface, under)
					for _, fg := range []ui.Color{low.Text, low.TextStrong, low.TextSoft, low.Muted} {
						if r := contrast(fg, bg); r < 4.5 {
							t.Errorf("%s light=%v %s over %s: %s on %s: %.2f", preset.ID, light, name, Hex(image), Hex(fg), Hex(bg), r)
						}
					}
				}
			}
			if high := WithBackdrop(base, 1); high.Panel.A != 255 || high.Frame.A != 255 {
				t.Errorf("%s light=%v: fully opaque gave %d, %d", preset.ID, light, high.Panel.A, high.Frame.A)
			}
			if low.Background != base.Background || low.Dialog != base.Dialog || low.Popover != base.Popover {
				t.Errorf("%s light=%v: changed what isn't a panel", preset.ID, light)
			}
		}
	}
}

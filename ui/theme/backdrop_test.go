package theme

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The app's default for a new image (backgroundDefaultOpacity in package ui).
const defaultBackdropOpacityForTest = 0.8

// At the default opacity words read on the panels over the lightest and the
// darkest image, in every palette and appearance. Lower is the user's choice.
func TestDefaultBackdropKeepsWordsReadable(t *testing.T) {
	for _, preset := range Palettes {
		for _, light := range []bool{false, true} {
			p := WithBackdrop(LookupAppearance(preset.ID, ui.Color{}, light), defaultBackdropOpacityForTest)
			for _, image := range []ui.Color{ui.Hex("#FFFFFF"), ui.Hex("#000000")} {
				under := over(Fade(p.Ground, BackdropScrim*defaultBackdropOpacityForTest), image)
				for name, surface := range map[string]ui.Color{"panel": p.Panel, "frame": p.Frame} {
					bg := over(surface, under)
					for _, fg := range []ui.Color{p.Text, p.TextStrong, p.TextSoft, p.Muted} {
						if r := contrast(fg, bg); r < 4.5 {
							t.Errorf("%s light=%v %s over %s: %s on %s: %.2f", preset.ID, light, name, Hex(image), Hex(fg), Hex(bg), r)
						}
					}
					if r := contrast(p.Text, over(p.Selected, bg)); r < 3 {
						t.Errorf("%s light=%v %s over %s: selected row %.2f", preset.ID, light, name, Hex(image), r)
					}
				}
			}
		}
	}
}

// The slider reaches every main surface, down to nothing and up to opaque; what
// floats stays, and so do the terminal's own colors, which its text needs.
func TestBackdropReachesTheMainSurfaces(t *testing.T) {
	old := Current()
	defer Apply(old)
	base := LookupAppearance(Default.ID, ui.Color{}, false)
	low := WithBackdrop(base, 0)
	if low.Panel.A != 0 || low.Frame.A != 0 || low.Raised.A != 0 || low.Sunken.A != 0 {
		t.Fatalf("at 0: %d %d %d %d", low.Panel.A, low.Frame.A, low.Raised.A, low.Sunken.A)
	}
	if low.Dialog != base.Dialog || low.Popover != base.Popover || low.Background != base.Background {
		t.Fatal("changed what floats, or the terminal's own colors")
	}
	Apply(low)
	if CodeGround.A != 0 || TermGround.A != 0 || Surface(ui.Hex("#123456")).A != 0 {
		t.Fatalf("code %d, terminal %d", CodeGround.A, TermGround.A)
	}
	if high := WithBackdrop(base, 1); high.Panel.A != 255 || high.Frame.A != 255 {
		t.Fatalf("at 1: %d, %d", high.Panel.A, high.Frame.A)
	}
	Apply(base)
	if SurfaceOpacity != 1 || TermGround.A != 255 || Surface(ui.Hex("#123456")).A != 255 {
		t.Fatalf("with no image: %v, %d", SurfaceOpacity, TermGround.A)
	}
}

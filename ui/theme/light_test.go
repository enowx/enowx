package theme

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLightPalettesKeepReadableSurfacesAndStatus(t *testing.T) {
	old := Current()
	defer Apply(old)
	accents := append([]ui.Color{{}}, Accents...)
	accents = append(accents, ui.Hex("#FFFFFF"), ui.Hex("#000000"), ui.Hex("#FF0000"))
	for _, preset := range Palettes {
		for _, accent := range accents {
			p := LookupAppearance(preset.ID, accent, true)
			Apply(p)
			if Window().Dark || p.ID != preset.ID {
				t.Fatalf("%s did not keep its palette in light appearance", preset.ID)
			}
			for _, bg := range []ui.Color{p.Background, p.Panel, p.Raised, p.Sunken} {
				if contrast(p.BorderStrong, bg) < 3 {
					t.Fatalf("%s: control outlines lost contrast", preset.ID)
				}
				for _, fg := range []ui.Color{p.Text, p.TextStrong, p.TextSoft, p.Muted, p.Accent, p.Working, p.Done, p.Warning, p.Error} {
					if ratio := contrast(fg, bg); ratio < 4.5 {
						t.Fatalf("%s: %s on %s contrast %.2f", preset.ID, Hex(fg), Hex(bg), ratio)
					}
				}
			}
			if contrast(p.Accent, p.Selected) < 4.5 || contrast(p.OnAccent, AccentHigh) < 4.5 || contrast(p.OnAccent, p.Accent) < 4.5 {
				t.Fatalf("%s: selected/primary labels lost contrast", preset.ID)
			}
			terminal := Terminal()
			if terminal.Background != preset.Background || contrast(terminal.Foreground, terminal.Background) < 4.5 || contrast(ui.Hex("#E0E0E0"), terminal.Background) < 4.5 {
				t.Fatal("light appearance made terminal input unreadable")
			}
			if int(Ground.R)+int(Ground.G)+int(Ground.B) < 600 {
				t.Fatal("light mode still has a dark app ground")
			}
		}
	}
}

func TestSwitchingBackToDarkRestoresPalette(t *testing.T) {
	old := Current()
	defer Apply(old)
	for _, preset := range Palettes {
		Apply(LookupAppearance(preset.ID, ui.Color{}, true))
		Apply(LookupAppearance(preset.ID, ui.Color{}, false))
		if Current() != preset || !Window().Dark {
			t.Fatalf("%s did not restore its original dark palette", preset.ID)
		}
	}
}

// Who saved a palette keeps it, and who didn't gets the default.
func TestSavedPalettesStillLoad(t *testing.T) {
	for _, id := range []string{"champagne", "obsidian", "midnight", "copper", "graphite"} {
		if got := Lookup(id, ui.Color{}).ID; got != id {
			t.Errorf("saved palette %s loads as %s", id, got)
		}
	}
	if got := Lookup("", ui.Color{}).ID; got != "teal" {
		t.Errorf("no saved palette loads as %s", got)
	}
}

// Teal's words read where its wallpaper shines brightest through the glass, and
// on the opaque surfaces, in dark and light. The surfaces are composed as drawn.
func TestTealWordsRead(t *testing.T) {
	for _, light := range []bool{false, true} {
		p := LookupAppearance("teal", ui.Color{}, light)
		lit := p.Ground
		for _, w := range p.Wall {
			if on := over(w, p.Ground); contrast(on, ui.Hex("#000000")) > contrast(lit, ui.Hex("#000000")) {
				lit = on
			}
		}
		panel := over(p.Panel, lit)
		if light { // the ground is pale: its darkest light is the hardest for dark words
			panel = over(p.Panel, over(p.Wall[2], p.Ground))
		}
		frame := over(p.Frame, lit)
		surfaces := map[string]ui.Color{"panel": panel, "frame": frame, "dialog": over(p.Dialog, p.Ground), "popover": over(p.Popover, p.Ground)}
		for name, bg := range surfaces {
			for _, under := range []ui.Color{bg, over(p.Raised, bg), over(p.Sunken, bg), over(p.Hover, bg)} {
				for _, fg := range []ui.Color{p.Text, p.TextStrong, p.TextSoft, p.Muted, p.Accent} {
					if r := contrast(fg, under); r < 4.5 {
						t.Errorf("light=%v %s: %s on %s: %.2f", light, name, Hex(fg), Hex(under), r)
					}
				}
			}
			selected := over(p.Selected, bg)
			for _, fg := range []ui.Color{p.Text, p.TextStrong} {
				if r := contrast(fg, selected); r < 4.5 {
					t.Errorf("light=%v %s selected: %s on %s: %.2f", light, name, Hex(fg), Hex(selected), r)
				}
			}
		}
		if r := contrast(p.OnAccent, p.Accent); r < 4.5 {
			t.Errorf("light=%v: on accent %.2f", light, r)
		}
	}
}

// Status colors read on the surfaces they sit on, the frame and the session
// list's glass, in every palette dark and light. Hue alone doesn't carry them:
// the list groups by state in words, and tabs and rows have tooltips.
func TestStatusColorsReadOnTheirSurfaces(t *testing.T) {
	old := Current()
	defer Apply(old)
	for _, preset := range Palettes {
		for _, light := range []bool{false, true} {
			p := LookupAppearance(preset.ID, ui.Color{}, light)
			Apply(p)
			for _, bg := range []ui.Color{Solid(p.Frame), Solid(p.Panel)} {
				for _, fg := range []ui.Color{p.Working, p.Done, p.Warning, p.Error, p.Idle} {
					if r := contrast(fg, bg); r < 3 {
						t.Errorf("%s light=%v: %s on %s: %.2f", p.ID, light, Hex(fg), Hex(bg), r)
					}
				}
			}
		}
	}
}

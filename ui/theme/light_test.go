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

// Words read on what they really sit on: the opaque surfaces and, over those,
// the translucent cards, wells and selections, in every palette, dark and
// light, with every accent. On white, as contrast reads a translucent color,
// wouldn't say so.
func TestWordsReadOnComposedSurfaces(t *testing.T) {
	old := Current()
	defer Apply(old)
	accents := append([]ui.Color{{}}, Accents...)
	accents = append(accents, ui.Hex("#FFFFFF"), ui.Hex("#000000"), ui.Hex("#FF0000"), ui.Hex("#0A6FE0"))
	for _, preset := range Palettes {
		for _, light := range []bool{false, true} {
			for _, accent := range accents {
				p := LookupAppearance(preset.ID, accent, light)
				for _, under := range []ui.Color{p.Panel, p.Frame, p.Dialog, p.Popover} {
					for _, bg := range []ui.Color{under, over(p.Raised, under), over(p.Sunken, under), over(p.Hover, under)} {
						for _, fg := range []ui.Color{p.Text, p.TextStrong, p.TextSoft, p.Muted} {
							if r := contrast(fg, bg); r < 4.5 {
								t.Errorf("%s light=%v: %s on %s: %.2f", p.ID, light, Hex(fg), Hex(bg), r)
							}
						}
					}
					selected := over(p.Selected, under)
					for _, fg := range []ui.Color{p.Text, p.TextStrong, p.Accent} {
						if r := contrast(fg, selected); r < 4.5 {
							t.Errorf("%s light=%v accent=%s: %s on selected %s: %.2f", p.ID, light, Hex(accent), Hex(fg), Hex(selected), r)
						}
					}
				}
				if r := contrast(p.OnAccent, p.Accent); r < 4.5 {
					t.Errorf("%s light=%v: on accent %.2f", p.ID, light, r)
				}
				if p.Panel.A != 255 || p.Frame.A != 255 || p.Dialog.A != 255 || p.Popover.A != 255 {
					t.Errorf("%s light=%v: a surface is still translucent", p.ID, light)
				}
			}
		}
	}
}

// Who saved a palette keeps it, and who didn't gets the default.
func TestSavedPaletteIDsStillLoad(t *testing.T) {
	for _, id := range []string{"champagne", "obsidian", "midnight", "copper", "graphite"} {
		if got := Lookup(id, ui.Color{}).ID; got != id {
			t.Errorf("saved palette %s loads as %s", id, got)
		}
	}
	if got := Lookup("", ui.Color{}).ID; got != Default.ID {
		t.Errorf("no saved palette loads as %s", got)
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

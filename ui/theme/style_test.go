package theme

import "testing"

// Liquid Glass keeps today's numbers, and a setting that names no style loads it.
func TestLiquidGlassKeepsItsValues(t *testing.T) {
	want := map[string][2]float32{
		"full": {RadiusFull, 999}, "control": {RadiusControl, 16}, "tail": {RadiusTail, 4}, "small": {RadiusSmall, 12},
		"card": {RadiusCard, 20}, "panel": {RadiusPanel, 28}, "bento": {RadiusBento, 28}, "gap": {BentoGap, 8},
		"inset": {WindowInset, 12}, "height": {ControlHeight, 32}, "bar": {BarHeight, 40}, "scroll": {ScrollWidth, 8},
		"s1": {S1, 4}, "s2": {S2, 8}, "s3": {S3, 12}, "s4": {S4, 16}, "s5": {S5, 24}, "s6": {S6, 32},
		"label": {TextLabel, 12}, "ui": {TextUI, 13}, "read": {TextRead, 15}, "title": {TextTitle, 17},
	}
	for name, v := range want {
		if v[0] != v[1] {
			t.Errorf("%s: %v, want %v", name, v[0], v[1])
		}
	}
	for _, id := range []string{"", "gone-in-a-later-version"} {
		if got := StyleByID(id).ID; got != LiquidGlass.ID {
			t.Errorf("%q loads %q", id, got)
		}
	}
}

// The sample's colors leave the app's own alone.
func TestStyleColorsKeepTheAppsPalette(t *testing.T) {
	before := Panel
	for _, s := range Styles {
		StyleColors(s, Default)
	}
	if Panel != before {
		t.Fatal("a sample changed the app's colors")
	}
}

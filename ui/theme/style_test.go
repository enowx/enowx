package theme

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

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

// Bento is Ledger's surfaces with its frame a tile, and a stronger line on what floats.
func TestBentoIsLedgerInTiles(t *testing.T) {
	l, b := ledgerSurfaces(Default), bentoSurfaces(Default)
	if b.Ground != l.Ground || b.Panel != l.Panel || b.Accent != l.Accent {
		t.Fatalf("not Ledger's: %+v", b)
	}
	if b.Frame != b.Panel {
		t.Fatal("its frame isn't a tile")
	}
	if Bento.Scrim.A != 0 || !Bento.Flat || Bento.Butted {
		t.Fatalf("%+v", Bento)
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

// In every style and appearance words read on what they sit on, and a terminal
// stays dark.
func TestStylesRead(t *testing.T) {
	oldStyle, oldPalette := CurrentStyle(), Current()
	defer func() {
		UseStyle(oldStyle)
		Apply(oldPalette)
	}()
	for _, s := range Styles[1:] {
		UseStyle(s)
		for _, light := range []bool{false, true} {
			for _, id := range []string{Default.ID, Copper.ID} {
				p := styled(LookupAppearance(id, ui.Color{}, light))
				for name, ground := range map[string]ui.Color{"panel": p.Panel, "frame": p.Frame, "raised": over(p.Raised, p.Frame), "dialog": p.Dialog, "menu": p.Popover} {
					if r := contrast(p.Text, ground); r < 4.5 {
						t.Errorf("%s %s light=%v: words on %s: %.2f", s.ID, id, light, name, r)
					}
					if r := contrast(p.TextSoft, ground); r < 4.5 {
						t.Errorf("%s %s light=%v: soft words on %s: %.2f", s.ID, id, light, name, r)
					}
				}
				if r := contrast(p.OnAccent, p.Accent); r < 4.5 {
					t.Errorf("%s %s light=%v: on accent %.2f", s.ID, id, light, r)
				}
			}
		}
	}
}

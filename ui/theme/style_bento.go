package theme

import "github.com/egoist/mygo/ui"

// Bento is Ledger in a compartment tray: the same black working tool, with every
// region and page section in a fixed tile, the same gap around each.
var Bento = Style{
	ID: "bento", Name: "Bento",
	RadiusFull: 4, RadiusControl: 4, RadiusTail: 4, RadiusSmall: 4, RadiusCard: 6, RadiusPanel: 6,
	BentoGap: 8, WindowInset: 8, ControlHeight: 26, BarHeight: 36, ScrollWidth: 6,
	BarRadius: 6, RailWidth: 40, ReadWidth: 800, HeaderHeight: 40,
	S1: 4, S2: 6, S3: 8, S4: 12, S5: 16, S6: 24,
	TextLabel: 12, TextUI: 12, TextRead: 13, TextTitle: 14,
	Flat: true, Hairline: 1, Scrim: ui.RGBA(0, 0, 0, 0),
	FullDock: true, DockedComposer: true, Tiled: true,
	Family:   "IBM Plex Sans, system-ui",
	surfaces: bentoSurfaces,
}

// bentoSurfaces is Ledger's, with the frame a tile like the others. With no dimming
// under a dialog, what floats has the stronger line.
func bentoSurfaces(p Palette) Palette {
	p = ledgerSurfaces(p)
	p.Frame = p.Panel
	p.floatLine = Mix(p.Ground, p.Text, 0.34)
	return p
}

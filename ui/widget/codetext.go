package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// CodeDim is quieter text on CodeGround.
var CodeDim = ui.Hex("#9A9EB2")

// CodeWarn and CodeError stand out on CodeGround.
var (
	CodeWarn  = ui.Hex("#F7C948")
	CodeError = ui.Hex("#FF8A80")
)

// codeColor is a token's color on CodeGround: fixed, since the ground stays
// dark in light mode, where the theme's own would sink into it.
func codeColor(k spanKind) ui.Color {
	switch k {
	case commentSpan:
		return CodeDim
	case keywordSpan:
		return ui.Hex("#6FC3FF")
	case stringSpan, insertedSpan:
		return ui.Hex("#7EE2A0")
	case numberSpan, attrSpan:
		return CodeWarn
	case nameSpan:
		return ui.Hex("#9DB8FF")
	case punctSpan:
		return ui.Hex("#B7BBCB")
	case deletedSpan:
		return CodeError
	}
	return CodeText
}

// CodeOnGround is code in lang, highlighted to sit on CodeGround; a lang it
// doesn't know shows plain.
func CodeOnGround(c *ui.Context, lang, code string) {
	ui.RichText(c).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(CodeText).Selectable().Children(func() {
		lines := HighlightLines("", lang, code)
		if lines == nil {
			ui.Text(c, code)
			return
		}
		n := 0
		for i, line := range lines {
			if i > 0 {
				ui.Text(c.Key(n), "\n")
				n++
			}
			for _, s := range line {
				ui.Text(c.Key(n), s.Text).TextColor(codeColor(s.kind))
				n++
			}
		}
	})
}

// CodeWell is a scrolling well of CodeGround that fills what its parent has
// left: what fn draws scrolls inside, the well keeps its frame.
func CodeWell(c *ui.Context, fn func()) {
	ui.Scroll(c.Key("code-well")).Grow(1).FillWidth().MinHeight(0).Radius(theme.RadiusCard).Background(CodeGround).
		InsetShadow(0, 0, 0, 0.5, theme.Border).Children(func() {
		ui.Column(c).FillWidth().MinWidth(0).Padding(theme.S3, theme.S4).Gap(theme.S2).TextColor(CodeText).Children(fn)
	})
}

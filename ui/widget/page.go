package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// The parts every page is made of, so one look is defined once.

// PageHeader is a frame's top: its title, what it is about, which runs when
// it can't fit, and its actions at the right. The frame keeps its height.
func PageHeader(c *ui.Context, title, sub string, actions func()) {
	ui.Row(c.Key("page-header")).MinHeight(theme.HeaderHeight).Shrink(0).FillWidth().Padding(theme.S2, theme.S4, theme.S2, theme.S5).Gap(theme.S3).
		BorderWidth(0, 0, 0.5, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(theme.S3).Children(func() {
			ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
				Running(c, title, RunStyle{Size: theme.TextTitle, Weight: 600, Always: true})
			})
			if sub != "" {
				ui.Row(c).MinWidth(0).Shrink(100).Children(func() {
					Running(c.Key("sub"), sub, RunStyle{Color: theme.TextSoft})
				})
			}
		})
		if actions != nil {
			ui.Row(c).Shrink(0).Gap(theme.S2).Children(actions)
		}
	})
}

// PageBody is what scrolls under a PageHeader; wide leaves it the frame's
// width, else its column is at most 760 and centered.
func PageBody(c *ui.Context, wide bool, fn func()) {
	ui.Scroll(c.Key("page-body")).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(theme.S4, theme.S5, theme.S5, theme.S5).AlignItems(ui.Center).Children(func() {
			col := ui.Column(c).FillWidth().Gap(theme.S3)
			if !wide {
				col.MaxWidth(760)
			}
			col.Children(fn)
		})
	})
}

// SidePanel is a page's own list beside its frame: glass, as the session list.
func SidePanel(c *ui.Context, width float32, fn func()) {
	Glass(ui.Column(c.Key("page-side")).Width(width).Shrink(0).FillHeight().Radius(theme.RadiusPanel).Clip()).Children(func() {
		ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(theme.S2, ScrollGutter, theme.S2, theme.S2).Gap(theme.S1).Children(fn)
		})
	})
}

// SideLabel is the label over a group of a SidePanel's entries.
func SideLabel(c *ui.Context, label string) {
	SectionLabel(c.Key("side:"+label), label, theme.TextSoft).Padding(theme.S3, theme.S3, theme.S1, theme.S3)
}

// SideEntry is an entry of a SidePanel: a capsule, lit while on.
func SideEntry(c *ui.Context, svg *ui.SVG, label string, on bool) ui.Element {
	b := ui.ButtonBase(c.Key("entry:" + label)).FillWidth().MinHeight(theme.ControlHeight).PaddingX(theme.S3).Gap(theme.S2).Radius(theme.RadiusFull).
		Justify(ui.Start).Label(label).Cursor(ui.CursorPointer).Shrink(0)
	bg, fg, weight := theme.Fade(theme.Selected, 0), theme.Text, 400
	switch {
	case on:
		bg, fg, weight = theme.Selected, theme.TextStrong, 500
	case b.Hovered():
		bg = theme.Hover
	}
	b.Background(QuickTint(b, "bg", bg)).TextColor(fg).Children(func() {
		if svg != nil {
			ui.Icon(c, svg).FontSize(14)
		}
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { Running(c, label, RunStyle{Weight: weight, Always: on}) })
	})
	return b
}

// Fig is one figure of a Figs row. Clickable ones return true from Clicked
// through FigsNav.
type Fig struct {
	Label, Value string
	Tone         ui.Color // of a value that stands out; zero is the text's
}

// Figs is a row of figures on one well, divided by hairlines.
func Figs(c *ui.Context, figs []Fig) {
	ui.Row(c.Key("figs")).FillWidth().Shrink(0).Radius(theme.RadiusCard).Clip().Background(figGround()).AlignItems(ui.Stretch).Children(func() {
		for i, f := range figs {
			cell := ui.Column(c.Key(f.Label)).Grow(1).Basis(0).MinWidth(0).Padding(theme.S3, theme.S4)
			if i > 0 {
				cell.BorderWidth(0, 0, 0, 0.5).BorderColor(theme.Fade(theme.Border, 0.14))
			}
			cell.Children(func() { figFace(c, f) })
		}
	})
}

// FigsNav is Figs whose figures pick what shows; it returns the one clicked,
// or -1.
func FigsNav(c *ui.Context, figs []Fig, current int) int {
	clicked := -1
	ui.Row(c.Key("figs")).FillWidth().Shrink(0).Radius(theme.RadiusCard).Clip().Background(figGround()).AlignItems(ui.Stretch).Children(func() {
		for i, f := range figs {
			b := ui.ButtonBase(c.Key(f.Label)).Grow(1).Basis(0).MinWidth(0).Padding(theme.S3, theme.S4).Justify(ui.Start).AlignItems(ui.Start).
				Label(f.Label).Cursor(ui.CursorPointer)
			if i > 0 {
				b.BorderWidth(0, 0, 0, 0.5).BorderColor(theme.Fade(theme.Border, 0.14))
			}
			switch {
			case i == current:
				b.Background(theme.Selected)
			case b.Hovered():
				b.Background(theme.Hover)
			}
			b.Children(func() {
				ui.Column(c).FillWidth().MinWidth(0).Children(func() { figFace(c, f) })
			})
			if b.Clicked() {
				clicked = i
			}
		}
	})
	return clicked
}

// figGround is the well figures sit in, lighter than an input's.
func figGround() ui.Color {
	if theme.Current().Light {
		return theme.Sunken
	}
	return theme.Surface(ui.RGBA(20, 22, 32, 0.34))
}

func figFace(c *ui.Context, f Fig) {
	Running(c.Key("l"), f.Label, RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
	Running(c.Key("v"), f.Value, RunStyle{Size: 24, Weight: 500, Color: f.Tone})
}

// KV is a fact: its name and its value, in one row of a card. A long value
// wraps inside the frame.
func KV(c *ui.Context, key, value string, mono bool) {
	ui.Row(c.Key("kv:" + key)).FillWidth().MinHeight(24).Gap(theme.S3).AlignItems(ui.Start).Children(func() {
		ui.Text(c, key).FontSize(theme.TextUI).TextColor(theme.TextSoft).Width(108).Shrink(0)
		v := ui.Text(c, value).FontSize(theme.TextUI).Grow(1).Basis(0).MinWidth(0)
		if mono {
			v.Font(theme.MonoFont)
		}
	})
}

// ToggleRow is a setting that is on or off: what it is, a line about it, and
// its switch. It reports whether it changed.
func ToggleRow(c *ui.Context, title, about string, on *bool) bool {
	changed := false
	ui.Row(c.Key("toggle:" + title)).FillWidth().MinHeight(44).Gap(theme.S3).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(theme.TextUI)
			if about != "" {
				ui.Text(c, about).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			}
		})
		changed = Switch(c, on, title)
	})
	return changed
}

// Banner says what stands out, a capsule tinted by col, with what to do about
// it after its words, which run when they can't fit.
func Banner(c *ui.Context, col ui.Color, text string, actions func()) {
	ui.Row(c.Key("banner")).FillWidth().Shrink(0).MinHeight(40).Padding(0, theme.S1, 0, theme.S4).Gap(theme.S2).Radius(theme.RadiusFull).
		Background(theme.Fade(col, 0.14)).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { Running(c, text, RunStyle{}) })
		if actions != nil {
			ui.Row(c).Shrink(0).Gap(theme.S2).Children(actions)
		}
	})
}

// Chip is a small fact in a well.
func Chip(c *ui.Context, label string, col ui.Color) {
	if col.A == 0 {
		col = theme.Text
	}
	ui.Row(c.Key("chip:" + label)).Height(24).Shrink(0).PaddingX(theme.S2).Radius(theme.RadiusFull).Background(theme.Sunken).Children(func() {
		ui.Text(c, label).FontSize(theme.TextLabel).TextColor(col).SingleLine()
	})
}

// CodeText is their color.
var CodeText = ui.Hex("#E4E6EE")

// CodeBlockText is output or a file's text on its ground, in a rounded well.
func CodeBlockText(c *ui.Context, text string) ui.Element {
	box := ui.Column(c).FillWidth().MinWidth(0).Padding(theme.S3, theme.S4).Radius(theme.RadiusCard).Background(theme.CodeGround).
		InsetShadow(0, 0, 0, 0.5, theme.Border)
	box.Children(func() {
		ui.Text(c, text).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(CodeText)
	})
	return box
}

// ViewerBar heads a viewer in a center tab: its icon, its path, which runs
// inside the room it has, then what can be done with it.
func ViewerBar(c *ui.Context, svg *ui.SVG, path string, actions func()) {
	ui.Row(c.Key("viewer-bar")).Height(44).Shrink(0).FillWidth().Padding(0, theme.S2, 0, theme.S4).Gap(theme.S2).Clip().
		BorderWidth(0, 0, 0.5, 0).BorderColor(theme.Border).Children(func() {
		ui.Icon(c, svg).FontSize(14).TextColor(theme.TextSoft)
		ui.Row(c).MinWidth(64).Shrink(1).Children(func() {
			Running(c, path, RunStyle{Size: theme.TextLabel, Mono: true, Always: true})
		})
		ui.Spacer(c)
		if actions != nil {
			actions()
		}
	})
}

// ToolHeader is the row of a side panel's tool under its title: what it says,
// then its actions.
func ToolHeader(c *ui.Context, fn func()) {
	ui.Row(c.Key("tool-bar")).FillWidth().MinHeight(theme.ControlHeight).Shrink(0).Gap(theme.S2).Children(fn)
}

// ListItem is a row of a side panel's list: a dot, a title and a quieter line,
// which run inside the row, and what follows at its right. It's lit while on.
func ListItem(c *ui.Context, dot ui.Color, title, sub string, on bool, right func()) ui.Element {
	b := ui.ButtonBase(c.Key("item:"+title+sub)).FillWidth().MinHeight(36).Padding(theme.S1, theme.S3).Gap(theme.S2).Radius(theme.RadiusCard).
		Justify(ui.Start).Label(title).Cursor(ui.CursorPointer).Shrink(0)
	bg := theme.Fade(theme.Selected, 0)
	switch {
	case on:
		bg = theme.Selected
	case b.Hovered():
		bg = theme.Hover
	}
	b.Background(QuickTint(b, "bg", bg)).Children(func() {
		if dot.A != 0 {
			Dot(c, dot, false, 8)
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			Running(c, title, RunStyle{Always: on})
			if sub != "" {
				Running(c.Key("sub"), sub, RunStyle{Size: theme.TextLabel, Color: theme.TextSoft, Always: on})
			}
		})
		if right != nil {
			ui.Row(c).Shrink(0).Gap(theme.S2).Children(right)
		}
	})
	return b
}

// CloseButton is the × of a frame.
func CloseButton(c *ui.Context, label string) ui.Element {
	return IconButton(c.Key("close"), icon.Close, label, false)
}

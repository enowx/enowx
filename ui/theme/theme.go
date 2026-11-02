// Package theme is enowX's look: the color tokens every view reads, the
// palettes that set them, fonts, and the themes of windows and terminals.
//
// The tokens are package variables read while drawing, and the UI is drawn
// every frame, so Apply changes the whole app at once. They belong to the
// main thread.
package theme

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// The color tokens. Status colors differ in lightness as well as hue.
var (
	Background   ui.Color // window, terminals
	Ground       ui.Color // the wallpaper the glass sits on
	Panel        ui.Color // glass: the session list, the bars' capsules, the tools
	Frame        ui.Color // the one bright frame the user works in
	CodeGround   ui.Color // code, logs and output: dark in every palette
	TermGround   ui.Color // what a terminal shows on: dark in every palette
	Raised       ui.Color // cards on a frame
	Sunken       ui.Color // wells: inputs, tracks, segmented controls
	Hover        ui.Color
	Selected     ui.Color
	Picked       ui.Color // what is selected, in the main lists, bars and tabs
	OnPicked     ui.Color // words on Picked
	DockGround   ui.Color // the tools dock
	Border       ui.Color // hairlines between areas
	BorderStrong ui.Color // outlines of controls
	BorderFloat  ui.Color // the line of what floats, in a flat style
	Dialog       ui.Color // dialogs
	Popover      ui.Color // menus, pop-overs
	Text         ui.Color
	TextStrong   ui.Color
	TextSoft     ui.Color
	Muted        ui.Color
	Accent       ui.Color
	AccentHigh   ui.Color // the light end of the accent's gradient
	AccentSoft   ui.Color // accent tint behind selected options
	AccentLine   ui.Color // accent outline of the selected item
	OnAccent     ui.Color
	Highlight    ui.Color // the faint light along a raised surface's top

	Working ui.Color
	Done    ui.Color
	Warning ui.Color
	Error   ui.Color
	Idle    ui.Color

	Add ui.Color // lines added
	Del ui.Color // lines removed, destructive words
	Now ui.Color // what happens now, the focus ring
)

// SurfaceOpacity is how opaque the main surfaces are over the user's own
// background image, 1 with none. See Surface.
var SurfaceOpacity float64 = 1

// MonoFont is the font of paths, code and command output.
const MonoFont = "monospace"

// Shape, spacing and type are the style's (style.go), the same in every palette.
// They are read while drawing, as the colors are, and belong to the main thread.
var (
	RadiusFull    float32 // capsules: half the height, whatever the height is
	RadiusControl float32 // buttons, inputs: a capsule at ControlHeight
	RadiusTail    float32 // a bubble's corner toward who said it
	RadiusSmall   float32 // thumbnails, swatches
	RadiusCard    float32 // cards, bubbles, wells, menus
	RadiusPanel   float32 // the window's panels, dialogs
	RadiusBento   float32 // the window's panels
	BentoGap      float32 // between the window's panels
	WindowInset   float32 // around them
	ControlHeight float32
	BarHeight     float32 // the title bar's pieces
	ScrollWidth   float32 // a scroll bar's thickness
	BarRadius     float32 // the title bar's pieces' corners
	RailWidth     float32 // the tools dock's region
	ReadWidth     float32 // the column a conversation reads in
	HeaderHeight  float32 // a page's header

	// Spacing: the steps of every gap and padding.
	S1, S2, S3, S4, S5, S6 float32

	// Effects: see Style.
	Flat, Butted, FullDock, DockedComposer bool
	Hairline                               float32
	Scrim                                  ui.Color
	Family                                 string

	// Type: the steps of every text size.
	TextLabel float32 // labels, details
	TextUI    float32 // controls, lists: the default
	TextRead  float32 // what is read: conversations, notes
	TextTitle float32 // the title of a frame or dialog
)

// current is the palette applied last.
var current Palette

// Current is the palette in use.
func Current() Palette { return current }

func init() {
	UseStyle(LiquidGlass)
	Apply(Default)
}

// Apply sets every token from p.
func Apply(p Palette) {
	current = p
	term := styled(darkPalette(p)).Background
	p = styled(p)
	Background, Panel, Raised, Sunken = p.Background, p.Panel, p.Raised, p.Sunken
	Ground, Frame, Popover = p.Ground, p.Frame, p.Popover
	Hover, Selected, Dialog = p.Hover, p.Selected, p.Dialog
	Border, BorderStrong = p.Border, p.BorderStrong
	Text, TextStrong, TextSoft, Muted = p.Text, p.TextStrong, p.TextSoft, p.Muted
	Accent, OnAccent = p.Accent, p.OnAccent
	AccentHigh = Mix(p.Accent, ui.Hex("#FFFFFF"), 0.12)
	AccentSoft = Fade(p.Accent, 0.16)
	AccentLine = p.Now
	Highlight = ui.RGBA(255, 255, 255, 0.55)
	if p.Light {
		AccentHigh = Mix(p.Accent, ui.Hex("#000000"), 0.08)
		AccentLine = Fade(p.Accent, 0.75)
		Highlight = ui.RGBA(255, 255, 255, 0.85)
	}
	SurfaceOpacity = 1
	if p.backdrop {
		SurfaceOpacity = p.opacity
	}
	CodeGround = Surface(ui.RGBA(10, 11, 18, 0.94))
	if p.code.A != 0 {
		CodeGround = p.code
	}
	TermGround = Surface(term)
	Picked, OnPicked, DockGround = p.Selected, p.TextStrong, p.Panel
	if p.picked.A != 0 {
		Picked, OnPicked = p.picked, p.onPicked
	}
	if p.dock.A != 0 {
		DockGround = p.dock
	}
	BorderFloat = p.Border
	if p.floatLine.A != 0 {
		BorderFloat = p.floatLine
	}
	Add, Del, Now = p.Add, p.Del, p.Now
	Working, Done, Warning, Error, Idle = p.Working, p.Done, p.Warning, p.Error, p.Idle
}

// Window is the window theme MyGo's own controls draw with: the tokens of
// the current palette.
func Window() *ui.Theme {
	t := *ui.DarkTheme()
	if current.Light {
		t = *ui.LightTheme()
	}
	t.Background = Solid(Dialog) // what the toolkit's own panels fill with
	t.Surface = Selected
	t.SurfaceHover = Hover
	t.SurfacePressed = Selected
	t.Border = BorderStrong
	t.Text = Text
	t.TextMuted = Muted
	t.Accent = Accent
	t.AccentHover = AccentHigh
	t.AccentPressed = Mix(Accent, Background, 0.15)
	t.AccentText = OnAccent
	t.Danger = Error
	t.Warning = Warning
	t.Success = Done
	t.Selection = Fade(Accent, 0.28)
	t.Focus = Now
	t.Radius = RadiusControl
	t.Inverse, t.InverseText = Popover, Text
	t.Scrollbar = Selected
	t.ScrollbarWidth = ScrollWidth
	t.FontSize = TextUI
	return &t
}

// Mix is a blended toward b by t, 0 to 1.
func Mix(a, b ui.Color, t float64) ui.Color {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return ui.Color{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B), A: m(a.A, b.A)}
}

// Fade is c with alpha a, 0 to 1.
func Fade(c ui.Color, a float64) ui.Color {
	c.A = uint8(a*255 + 0.5)
	return c
}

// Solid is translucent c as it reads on the wallpaper's ground, opaque: for
// what has no ground under it.
func Solid(c ui.Color) ui.Color {
	at := func(v, g uint8) uint8 {
		return uint8(float64(g) + (float64(v)-float64(g))*float64(c.A)/255 + 0.5)
	}
	return ui.Color{R: at(c.R, Ground.R), G: at(c.G, Ground.G), B: at(c.B, Ground.B), A: 255}
}

// Hex writes c as #RRGGBB, as ui.Hex reads it back.
func Hex(c ui.Color) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

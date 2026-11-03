// Package widget holds the controls every view shares, so one look and
// behavior is defined once: buttons, inputs, status marks, lists, cards,
// menus and dialogs. They draw with the tokens of package theme, so they
// follow the palette the user picks.
package widget

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// ButtonKind is the role of a button.
type ButtonKind int

const (
	Primary   ButtonKind = iota // the one action a view is for
	Secondary                   // the others
	Danger                      // destructive ones
)

// Button is a capsule with its label, an optional icon and shortcut hint.
func Button(c *ui.Context, kind ButtonKind, svg *ui.SVG, label, hint string) ui.Element {
	b := ui.ButtonBase(c).Gap(theme.S1).PaddingX(theme.S3).Height(theme.ControlHeight).Radius(theme.RadiusFull).
		Cursor(ui.CursorPointer).Label(label).Shrink(0)
	hover := b.Hovered() && !b.IsDisabled()
	weight := 400
	switch kind {
	case Primary:
		bg := theme.Accent
		if hover {
			bg = theme.AccentHigh
		}
		b.Background(QuickTint(b, "bg", bg)).TextColor(theme.OnAccent)
		weight = 500
	case Secondary:
		bg := theme.Fade(theme.TextStrong, 0.1)
		if hover {
			bg = theme.Fade(theme.TextStrong, 0.18)
		}
		if theme.Current().Light {
			bg = theme.Hover
			if hover {
				bg = theme.Selected
			}
		}
		b.Background(QuickTint(b, "bg", bg)).TextColor(theme.Text)
	case Danger:
		bg := theme.Fade(theme.Error, 0.16)
		if hover {
			bg = theme.Fade(theme.Error, 0.26)
		}
		b.Background(QuickTint(b, "bg", bg)).TextColor(theme.Del)
	}
	if b.IsDisabled() {
		b.Opacity(0.45)
	}
	b.Children(func() {
		if svg != nil {
			ui.Icon(c, svg).FontSize(14)
		}
		ui.Text(c, label).FontSize(theme.TextUI).FontWeight(weight).SingleLine()
		if hint != "" {
			ui.Text(c, hint).FontSize(theme.TextLabel).Opacity(0.6).SingleLine()
		}
	})
	return Pressable(b)
}

// PrimaryButton is the accent button with an icon and a shortcut hint.
func PrimaryButton(c *ui.Context, svg *ui.SVG, label, hint string) ui.Element {
	return Button(c, Primary, svg, label, hint)
}

// GhostButton is a secondary capsule.
func GhostButton(c *ui.Context, label string) ui.Element {
	return Button(c, Secondary, nil, label, "")
}

// DangerButton is a capsule for destructive actions.
func DangerButton(c *ui.Context, label string) ui.Element {
	return Button(c, Danger, nil, label, "")
}

// IconButton is a round button showing only an icon; on marks a toggle
// that is on.
func IconButton(c *ui.Context, svg *ui.SVG, label string, on bool) ui.Element {
	b := ui.ButtonBase(c).Size(theme.ControlHeight, theme.ControlHeight).Radius(theme.RadiusFull).Center().Shrink(0).
		Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	bg, fg := theme.Fade(theme.Selected, 0), theme.TextSoft
	if on || (b.Hovered() && !b.IsDisabled()) {
		bg, fg = theme.Selected, theme.Text
	}
	if b.IsDisabled() {
		b.Opacity(0.45)
	}
	b.Background(QuickTint(b, "bg", bg)).TextColor(QuickTint(b, "fg", fg))
	b.Children(func() { ui.Icon(c, svg).FontSize(14) })
	return Pressable(b)
}

// Well is what inputs and drop-downs are made of: sunk under the surface
// with a hairline around it, and the focus ring while focused.
func Well(e ui.Element, focused bool) ui.Element {
	Edge(e.Background(theme.Sunken))
	if focused {
		e.Border(2, theme.Now)
	}
	return e
}

// Input is a text input: a well as a capsule.
func Input(c *ui.Context, value *string, placeholder string) ui.Element {
	in := ui.TextInput(c, value).Height(theme.ControlHeight).PaddingX(theme.S3).Radius(theme.RadiusFull).
		FontSize(theme.TextUI).Placeholder(placeholder).Label(placeholder)
	return Well(in, in.Focused())
}

// SearchField is a filter input with a search icon in front.
func SearchField(c *ui.Context, value *string, placeholder string, svg *ui.SVG) {
	box := ui.Row(c).FillWidth().Height(theme.ControlHeight).PaddingX(theme.S3).Gap(theme.S2).Radius(theme.RadiusFull)
	Well(box, box.FocusWithin()).Children(func() {
		ui.Icon(c, svg).FontSize(14).TextColor(theme.TextSoft)
		ui.TextInputBase(c, value).Grow(1).FontSize(theme.TextUI).Placeholder(placeholder).Label(placeholder)
	})
}

// Trigger styles the face of a drop-down, open while its pop-up shows.
func Trigger(t ui.Element, open bool) ui.Element {
	t.FillWidth().Height(theme.ControlHeight).PaddingX(theme.S3).Gap(theme.S2).Radius(theme.RadiusFull).Cursor(ui.CursorPointer)
	Well(t, open)
	if t.Hovered() {
		t.Background(theme.Fade(theme.TextStrong, 0.1))
	}
	return t
}

// Switch is an on/off toggle; it reports whether it changed.
func Switch(c *ui.Context, on *bool, label string) bool {
	// The knob sits on the left, centered top to bottom, and slides right.
	b := ui.ButtonBase(c).Size(40, 24).Radius(theme.RadiusFull).Shrink(0).Label(label).Cursor(ui.CursorPointer).
		Justify(ui.Start).AlignItems(ui.Center)
	track := theme.Fade(theme.TextStrong, 0.18)
	if theme.Current().Light {
		track = theme.Fade(theme.TextStrong, 0.25)
	}
	if *on {
		track = theme.Accent
	}
	x := b.AnimateTo("x", map[bool]float32{false: 0, true: 16}[*on], Press)
	b.Background(QuickTint(b, "track", track)).Children(func() {
		DropShadow(ui.Box(c).Size(20, 20).Radius(theme.RadiusFull).Shrink(0).Margin(0, 0, 0, 2+x).Background(ui.Hex("#FFFFFF")), 1, 3, ui.RGBA(0, 0, 0, 0.4))
	})
	if b.Clicked() {
		*on = !*on
		return true
	}
	return false
}

// Checkbox is a box with its label; it reports whether it changed.
func Checkbox(c *ui.Context, checked *bool, label string) bool {
	b := ui.ButtonBase(c).Gap(theme.S2).Cursor(ui.CursorPointer).Label(label).Shrink(0).Justify(ui.Start)
	b.Children(func() {
		box := ui.Box(c).Size(16, 16).Radius(theme.RadiusTail).Center().TextColor(theme.OnAccent)
		if *checked {
			box.Background(theme.Accent)
		} else {
			Well(box, false)
			if b.Hovered() {
				box.Background(theme.Fade(theme.TextStrong, 0.1))
			}
		}
		box.Children(func() {
			if *checked {
				ui.Icon(c, icon.Check).FontSize(12)
			}
		})
		ui.Text(c, label).FontSize(theme.TextUI).TextColor(theme.Text).SingleLine()
	})
	if b.Clicked() {
		*checked = !*checked
		return true
	}
	return false
}

// CheckRow is a checkbox filling the width it is given, for a column of them:
// its label runs when too long for it.
func CheckRow(c *ui.Context, checked *bool, label string) bool {
	b := ui.ButtonBase(c).FillWidth().MinWidth(0).Height(theme.ControlHeight).Gap(theme.S2).Cursor(ui.CursorPointer).Label(label).Justify(ui.Start)
	b.Children(func() {
		box := ui.Box(c).Size(16, 16).Shrink(0).Radius(theme.RadiusTail).Center().TextColor(theme.OnAccent)
		if *checked {
			box.Background(theme.Accent)
		} else {
			Well(box, false)
			if b.Hovered() {
				box.Background(theme.Fade(theme.TextStrong, 0.1))
			}
		}
		box.Children(func() {
			if *checked {
				ui.Icon(c, icon.Check).FontSize(12)
			}
		})
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { Running(c, label, RunStyle{Color: theme.Text}) })
	})
	if b.Clicked() {
		*checked = !*checked
		return true
	}
	return false
}

// Field is a form field: a label over its input.
func Field(c *ui.Context, label string, fn func()) {
	ui.Column(c).FillWidth().MinWidth(0).Gap(theme.S1).Children(func() {
		ui.Text(c, label).FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft)
		fn()
	})
}

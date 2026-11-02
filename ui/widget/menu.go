package widget

import (
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// ContextMenu is the open right-click menu of a window: which element it
// belongs to and where, relative to that element, the pointer was. The
// zero value has no menu open. It belongs to the main thread.
type ContextMenu struct {
	open *menuAt
}

type menuAt struct {
	key  string
	x, y float32
}

// Open opens key's menu at the pointer when anchor is right-clicked.
func (m *ContextMenu) Open(anchor ui.Element, key string) {
	if !anchor.RightClicked() {
		return
	}
	x, y, _ := anchor.PointerPosition()
	m.open = &menuAt{key: key, x: x, y: y}
}

// OpenBelow opens key's menu under anchor when anchor is clicked, for
// buttons that show a menu; height is anchor's height.
func (m *ContextMenu) OpenBelow(anchor ui.Element, key string, height float32) {
	if anchor.Clicked() {
		m.open = &menuAt{key: key, y: height}
	}
}

// Show draws key's menu, if open, at the pointer over anchor; items builds
// its entries with MenuItem and MenuSeparator, and calls done to close it.
func (m *ContextMenu) Show(c *ui.Context, anchor ui.Element, key string, items func(done func())) {
	at := m.open
	if at == nil || at.key != key {
		return
	}
	open := true
	closeMenu := func() { open = false }
	ui.PopoverBase(c, anchor, &open, func(panel ui.Element) {
		Popup(panel.AttachTo(anchor, ui.AnchorTopStart, ui.AnchorTopStart).Margin(at.y+4, 0, 0, at.x+4).MinWidth(190))
		items(closeMenu)
	})
	if !open && m.open == at {
		m.open = nil
	}
}

// ShowBeside is Show for a menu whose anchor is at the window's edge: it opens
// up and to the left of it, clear of what sits under the anchor.
func (m *ContextMenu) ShowBeside(c *ui.Context, anchor ui.Element, key string, items func(done func())) {
	at := m.open
	if at == nil || at.key != key {
		return
	}
	open := true
	closeMenu := func() { open = false }
	ui.PopoverBase(c, anchor, &open, func(panel ui.Element) {
		Popup(panel.AttachTo(anchor, ui.AnchorTopLeft, ui.AnchorBottomRight).Margin(0, theme.S1, theme.S1, 0).MinWidth(190))
		items(closeMenu)
	})
	if !open && m.open == at {
		m.open = nil
	}
}

// MenuItem is an entry of a context menu; danger colors destructive ones.
func MenuItem(c *ui.Context, svg *ui.SVG, label string, danger bool) bool {
	tip := ""
	if utf8.RuneCountInString(label) > menuFits {
		tip = label // cut short in the menu: whole in its tooltip
	}
	return MenuItemTip(c, svg, label, tip, danger)
}

// menuFits is about how many characters a menu shows on one line.
const menuFits = 24

// MenuItemTip is a menu item with a short label and, on hover, tip: what
// it does in full. Keep labels to a few words; say the rest in tip.
func MenuItemTip(c *ui.Context, svg *ui.SVG, label, tip string, danger bool) bool {
	color := theme.Text
	if danger {
		color = theme.Del
	}
	b := ui.ButtonBase(c).Gap(theme.S2).Padding(theme.S1, theme.S3).MinHeight(theme.ControlHeight).Radius(theme.RadiusSmall).
		Cursor(ui.CursorPointer).Label(label).FillWidth().Justify(ui.Start)
	if tip != "" {
		b.Tooltip(tip)
	}
	bg := theme.Fade(theme.Selected, 0)
	if b.Hovered() {
		bg = theme.Selected
	}
	b.Background(QuickTint(b, "bg", bg)).TextColor(color)
	b.Children(func() {
		if svg != nil {
			ui.Icon(c, svg).FontSize(14)
		}
		ui.Text(c, label).FontSize(theme.TextUI).SingleLine().Shrink(1).MinWidth(0)
	})
	return b.Clicked()
}

// MenuItemSub is a menu item with a quieter line under its label.
func MenuItemSub(c *ui.Context, svg *ui.SVG, label, sub string, danger bool) bool {
	color := theme.Text
	if danger {
		color = theme.Del
	}
	height := float32(theme.ControlHeight)
	if sub != "" {
		height = menuItemSubHeight
	}
	b := ui.ButtonBase(c).Gap(theme.S2).Padding(theme.S1, theme.S3).MinHeight(height).Radius(theme.RadiusControl).
		Cursor(ui.CursorPointer).Label(label).FillWidth().Justify(ui.Start)
	bg := theme.Fade(theme.Selected, 0)
	if b.Hovered() {
		bg = theme.Selected
	}
	b.Background(QuickTint(b, "bg", bg)).TextColor(color)
	b.Children(func() {
		if svg != nil {
			ui.Icon(c, svg).FontSize(14)
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, label).FontSize(theme.TextUI).SingleLine().Shrink(1)
			if sub != "" {
				ui.Text(c, sub).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(1)
			}
		})
	})
	return b.Clicked()
}

// menuItemSubHeight is an entry with a line under its name.
const menuItemSubHeight = 44

// MenuSeparator separates groups of menu items.
func MenuSeparator(c *ui.Context) { Divider(c) }

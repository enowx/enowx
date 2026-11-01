package widget

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// Dialog draws a modal dialog of the given width while *open; gap spaces
// its children. Closing it (Escape, a click outside) clears *open.
func Dialog(c *ui.Context, open *bool, width, gap float32, body func()) {
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		dialogMotion(backdrop, panel)
		// It fits the window: narrower in a narrow one, and what is
		// taller than the window scrolls.
		w, h := c.Size()
		Float(panel.Width(min(width, max(280, w-32))).MaxHeight(max(0, h-32)).Radius(theme.RadiusPanel).Background(theme.Dialog).
			Border(1, theme.BorderStrong))
		ui.Scroll(c).FillWidth().Shrink(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(20).Gap(gap).Children(body)
		})
	})
}

// dialogMotion fades the backdrop in and grows the panel in over it.
func dialogMotion(backdrop, panel ui.Element) {
	fade := &ui.Motion{Opacity: 0}
	backdrop.Background(Backdrop).Transition(ui.ElementTransition{Duration: 240 * time.Millisecond, Ease: Out, Enter: fade, Exit: fade})
	panel.Transition(OverlayMotion)
}

// Sheet draws a modal dialog in three parts while *open: a header row,
// a scrollable body and a footer row, split by hairlines. The header and
// footer stay visible when the body is taller than the window.
func Sheet(c *ui.Context, open *bool, width float32, header, body, footer func()) {
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		dialogMotion(backdrop, panel)
		_, height := c.Size()
		w, _ := c.Size()
		Float(panel.Width(min(width, max(280, w-32))).MaxHeight(max(0, height-32)).Radius(theme.RadiusPanel).Background(theme.Dialog).Border(1, theme.BorderStrong))
		ui.Row(c).FillWidth().Shrink(0).Padding(16, 20, 14, 20).Gap(10).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(header)
		ui.Scroll(c).FillWidth().Shrink(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(18, 20, 20, 20).Gap(18).Children(body)
		})
		// Buttons that don't fit go to another line, rather than past the frame.
		ui.Row(c).FillWidth().Shrink(0).Padding(14, 20).Gap(8).Wrap().Justify(ui.End).BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(footer)
	})
}

// DialogHeader is a dialog's breadcrumb, title and description, with a
// close button when closeable. It returns whether the close button was
// clicked.
func DialogHeader(c *ui.Context, title, description string, closeable bool, crumbs ...string) (closed bool) {
	ui.Row(c).FillWidth().AlignItems(ui.Start).Children(func() {
		ui.Column(c).Gap(4).Grow(1).Children(func() {
			if len(crumbs) > 0 {
				ui.Box(c).Margin(0, 0, 4, 0).Children(func() { Breadcrumb(c, crumbs...) })
			}
			ui.Text(c, title).FontSize(16)
			if description != "" {
				ui.Text(c, description).FontSize(12).TextColor(theme.Muted)
			}
		})
		if closeable && IconButton(c, icon.Close, "Close", false).Clicked() {
			closed = true
		}
	})
	return closed
}

// DialogFooter puts a dialog's buttons at its bottom right, after an
// optional note on the left.
func DialogFooter(c *ui.Context, note string, buttons func()) {
	ui.Row(c).FillWidth().Gap(8).Wrap().Margin(4, 0, 0, 0).Children(func() {
		if note != "" {
			ui.Text(c, note).FontSize(11.5).TextColor(theme.Muted).Shrink(1)
		}
		ui.Spacer(c)
		buttons()
	})
}

// FormSheet is a tool's form as a Sheet: a title and a close button, the
// form, then Cancel and its buttons. Escape or a click outside closes it,
// as Cancel does. It narrows to fit a small window.
func FormSheet(c *ui.Context, open *bool, width float32, title, description string, body, buttons func()) {
	w, _ := c.Size()
	width = min(width, max(280, w-32))
	Sheet(c, open, width, func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, title).FontSize(15)
			if description != "" {
				ui.Text(c, description).FontSize(11.5).TextColor(theme.Muted)
			}
		})
		if IconButton(c, icon.Close, "Close", false).Clicked() {
			*open = false
		}
	}, body, func() {
		if GhostButton(c, "Cancel").Clicked() {
			*open = false
		}
		buttons()
	})
}

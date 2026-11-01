package chatview

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// attachment is an image or a file going with the next message: shown
// over the message as a small preview, sent as its path once the app has
// it where the agent runs.
type attachment struct {
	id      int
	name    string
	preview ui.ImageSource // nil for a file that isn't an image
	w, h    int            // the image's size, in pixels
	path    string         // as the prompt takes it; "" while uploading
	failed  bool
}

// Attach adds an image or a file to the next message, with preview (an
// image's bytes, or nil) to show while it waits. done gives its path, as
// the agent's prompt takes it, or the error that dropped it; call it on
// the main thread.
func (ch *Chat) Attach(name string, preview []byte) (done func(path string, err error)) {
	ch.nextAttach++
	at := &attachment{id: ch.nextAttach, name: name}
	if len(preview) > 0 {
		if bmp, err := ui.DecodeBitmap(preview); err == nil {
			at.preview = bmp
			at.w, at.h = bmp.Size()
		}
	}
	ch.attachments = append(ch.attachments, at)
	ch.focus = true
	return func(path string, err error) {
		if err != nil {
			at.failed = true
			ch.dropAttachment(at.id)
			return
		}
		at.path = path
	}
}

func (ch *Chat) dropAttachment(id int) {
	if ch.viewing != nil && ch.viewing.id == id {
		ch.viewing = nil
	}
	for i, at := range ch.attachments {
		if at.id == id {
			ch.attachments = append(ch.attachments[:i:i], ch.attachments[i+1:]...)
			return
		}
	}
}

// attaching reports whether an attachment still uploads: the message
// waits for it.
func (ch *Chat) attaching() bool {
	for _, at := range ch.attachments {
		if at.path == "" {
			return true
		}
	}
	return false
}

// takeAttachments returns the attachments' paths, emptying the list.
func (ch *Chat) takeAttachments() []string {
	var paths []string
	for _, at := range ch.attachments {
		if at.path != "" {
			paths = append(paths, at.path)
		}
	}
	ch.attachments = nil
	return paths
}

// send sends text with the attachments, unless one still uploads.
func (ch *Chat) send(o Options, text string) bool {
	text = strings.TrimRight(text, "\n")
	if ch.attaching() || strings.TrimSpace(text) == "" && len(ch.attachments) == 0 {
		return false
	}
	if ch.editing != nil && o.Edit != nil {
		if ch.editBusy {
			return false
		}
		ch.editBusy, ch.editErr = true, ""
		o.Edit(ch.editBack(), ch.editing.Text, text, ch.takeAttachments(), func(err error) {
			ch.editBusy = false
			if err != nil {
				ch.editErr, ch.input = err.Error(), text // to try again, or stop
				return
			}
			ch.editing = nil
		})
		return true
	}
	o.Send(text, ch.takeAttachments())
	return true
}

// attachmentRow shows the attachments over the message: previews of
// images, names of files, each with an × that drops it.
func (ch *Chat) attachmentRow(c *ui.Context) {
	if len(ch.attachments) == 0 {
		return
	}
	ui.Row(c.Key("attachments")).FillWidth().Gap(8).Padding(10, 12, 0, 12).Children(func() {
		for _, at := range ch.attachments {
			ch.attachmentChip(c.Key(at.id), at)
		}
	})
}

func (ch *Chat) attachmentChip(c *ui.Context, at *attachment) {
	chip := ui.ButtonBase(c).Size(56, 56).Radius(theme.RadiusSmall).Clip().Background(theme.Sunken).Border(1, theme.Border).
		Tooltip(at.name).Label(at.name)
	if at.preview == nil {
		chip.Width(140)
	} else {
		chip.Cursor(ui.CursorPointer)
		if chip.Hovered() {
			chip.Border(1, theme.AccentLine)
		}
	}
	removed := false
	chip.Children(func() {
		if at.preview != nil {
			img := ui.Image(c, at.preview).Size(56, 56).Fit(ui.Cover)
			if at.path == "" {
				img.Opacity(0.45)
			}
		} else {
			ui.Row(c).Fill().Gap(6).PaddingX(8).Children(func() {
				ui.Icon(c, icon.File).FontSize(13).TextColor(theme.Muted)
				ui.Text(c, at.name).FontSize(theme.TextLabel).SingleLine().Shrink(1)
			})
		}
		if at.path == "" {
			ui.Box(c).Absolute().Fill().Center().Children(func() { widget.Spinner(c, 14) })
		}
		x := ui.ButtonBase(c).Absolute().Top(3).Right(3).Size(18, 18).Radius(theme.RadiusSmall).Center().
			Background(theme.Fade(theme.Ground, 0.8)).Cursor(ui.CursorPointer).Label("Remove " + at.name)
		x.Children(func() { ui.Icon(c, icon.Close).FontSize(10).TextColor(theme.Text) })
		if x.Clicked() {
			removed = true
			ch.dropAttachment(at.id)
			ch.focus = true
		}
	})
	if !removed && chip.Clicked() && at.preview != nil {
		ch.viewing = at
	}
}

// Room an image gets in its preview, at most, in points.
const (
	viewerWidth  = 960
	viewerHeight = 620
)

// imageViewer shows the attachment clicked, large, over everything; a
// click outside it or Escape closes it.
func (ch *Chat) imageViewer(c *ui.Context) {
	at := ch.viewing
	if at == nil {
		return
	}
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(widget.Backdrop)
		panel.Padding(10).Gap(8).Radius(theme.RadiusPanel).Background(theme.Dialog).Border(1, theme.BorderStrong).Transition(widget.OverlayMotion)
		w, h := float32(max(at.w, 1)), float32(max(at.h, 1))
		scale := min(1, viewerWidth/w, viewerHeight/h)
		ui.Image(c, at.preview).Size(w*scale, h*scale).Fit(ui.Contain).Radius(theme.RadiusSmall)
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, at.name).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(1)
			ui.Text(c, fmt.Sprintf("%d × %d", at.w, at.h)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			ui.Spacer(c)
			if widget.GhostButton(c, "Remove").Clicked() {
				ch.dropAttachment(at.id)
				open = false
			}
			if widget.GhostButton(c, "Close").Clicked() {
				open = false
			}
		})
	})
	if !open {
		ch.viewing, ch.focus = nil, true
	}
}

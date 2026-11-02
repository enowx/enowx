package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Images in a team's chat: picked, pasted or dropped on the chat, each is
// uploaded at once (the host makes it WebP and small, and seals it), shown
// above the composer until sent. In a message they show in its bubble,
// fetched once; a click opens one large.

const (
	maxPickedImage = 12 << 20 // as the host takes one (daemon: maxUpload)
	maxChatImages  = 4
	chatImageW     = 240 // a message's image, at most
)

// pendingImage is an image on its way into the draft.
type pendingImage struct {
	key     int // tells it apart while uploading
	name    string
	data    []byte // kept until it is up, to try again
	preview *ui.Bitmap
	att     *proto.ChatAttachment // once uploaded
	err     string
}

// chatImage is an attachment's image as fetched.
type chatImage struct {
	bmp   *ui.Bitmap
	data  []byte
	err   string
	asked bool
}

// addChatImage uploads data, named name, for the draft.
func (a *App) addChatImage(team, name string, data []byte) {
	hc, cl := a.chatsOf(), a.client
	if cl == nil {
		return
	}
	if len(hc.pending) >= maxChatImages {
		hc.err = fmt.Sprintf("%d images a message at most", maxChatImages)
		return
	}
	if len(data) > maxPickedImage {
		hc.err = fmt.Sprintf("%s is over %d MB", nonEmptyStr(name, "the image"), maxPickedImage>>20)
		return
	}
	hc.nextKey++
	p := &pendingImage{key: hc.nextKey, name: name, data: data}
	p.preview, _ = ui.DecodeBitmap(data)
	hc.pending = append(hc.pending, p)
	a.uploadChatImage(team, p)
}

// uploadChatImage uploads p, in the draft or in a message on its way, which then
// goes on.
func (a *App) uploadChatImage(team string, p *pendingImage) {
	hc, cl := a.chatsOf(), a.client
	if cl == nil {
		p.err = "Not connected"
		return
	}
	name, data := p.name, p.data
	go func() {
		att, err := cl.UploadChatImage(proto.ChatUploadParams{Team: team, Name: name, Data: data})
		a.onMain(func() {
			if err != nil {
				p.err = strings.TrimPrefix(client.Explain(err), "teams: ")
			} else {
				p.att, p.data = &att, nil
				// Shown from here on without fetching it back.
				if p.preview != nil {
					hc.images[att.ID] = &chatImage{bmp: p.preview, asked: true}
				}
			}
			a.outboxStep(team)
		})
	}()
}

// pickChatImages adds images from files the user picks.
func (a *App) pickChatImages(team string) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "Send images to the team", Multiple: true,
			Filters: []mygo.FileFilter{{Name: "Images", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp"}}}})
		if err != nil || len(paths) == 0 {
			return
		}
		a.onMain(func() { a.addChatImageFiles(team, paths) })
	}()
}

// addChatImageFiles adds the images among files.
func (a *App) addChatImageFiles(team string, files []string) {
	for _, f := range files {
		switch strings.ToLower(filepath.Ext(f)) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		default:
			continue
		}
		data, err := readChatImage(f)
		if err != nil {
			a.chatsOf().err = err.Error()
			continue
		}
		a.addChatImage(team, filepath.Base(f), data)
	}
}

func readChatImage(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPickedImage+1))
	if err == nil && len(data) > maxPickedImage {
		err = fmt.Errorf("%s is over %d MB", filepath.Base(path), maxPickedImage>>20)
	}
	return data, err
}

// pasteChatImage adds the clipboard's image, when it holds one.
func (a *App) pasteChatImage(team string) bool {
	data := mygo.Clipboard.ReadImage()
	if len(data) == 0 {
		return false
	}
	a.addChatImage(team, "pasted-image.png", data)
	return true
}

// imagesSendable are the draft's images uploaded or uploading: one that failed says
// so on its own, and can't make a message alone.
func (a *App) imagesSendable() []*pendingImage {
	var out []*pendingImage
	for _, p := range a.chatsOf().pending {
		if p.err == "" {
			out = append(out, p)
		}
	}
	return out
}

// pendingImages shows the draft's images above the composer.
func (a *App) pendingImages(c *ui.Context) {
	hc := a.chatsOf()
	if len(hc.pending) == 0 {
		return
	}
	ui.Row(c).FillWidth().Gap(theme.S2).PaddingX(theme.S1).Wrap().Children(func() {
		for i, p := range hc.pending {
			box := ui.Box(c.Key(p.key)).Size(64, 64).Radius(theme.RadiusSmall).Background(theme.Sunken).Border(1, theme.Border).Center()
			if p.err != "" {
				box.BorderColor(theme.Error).Tooltip(p.err)
			}
			box.Children(func() {
				switch {
				case p.preview != nil:
					ui.Image(c, p.preview).Size(64, 64).Radius(theme.RadiusSmall).Fit(ui.Cover)
				default:
					ui.Icon(c, icon.Image).FontSize(18).TextColor(theme.Idle)
				}
				if p.att == nil && p.err == "" {
					ui.Box(c).Absolute().Left(0).Top(0).Size(64, 64).Radius(theme.RadiusSmall).Background(theme.Fade(theme.Ground, 0.55)).Center().Children(func() {
						widget.Spinner(c, 14)
					})
				}
				x := ui.ButtonBase(c).Absolute().Right(-6).Top(-6).Size(20, 20).Radius(theme.RadiusSmall).Center().Background(theme.Dialog).
					Border(1, theme.BorderStrong).Label("Remove " + nonEmptyStr(p.name, "the image")).Cursor(ui.CursorPointer)
				x.Children(func() { ui.Icon(c, icon.Close).FontSize(10).TextColor(theme.TextSoft) })
				if x.Clicked() {
					hc.pending = slices.Delete(slices.Clone(hc.pending), i, i+1)
				}
			})
		}
	})
}

// imageOf is attachment att's image of team, fetched the first time.
func (a *App) imageOf(team string, att proto.ChatAttachment) *chatImage {
	hc, cl := a.chatsOf(), a.client
	im := hc.images[att.ID]
	if im == nil {
		im = &chatImage{}
		hc.images[att.ID] = im
	}
	if !im.asked && cl != nil {
		im.asked = true
		go func() {
			data, err := cl.ChatImage(proto.ChatImageParams{Team: team, ID: att.ID, Epoch: att.Epoch})
			var bmp *ui.Bitmap
			if err == nil {
				bmp, err = ui.DecodeBitmap(data)
			}
			a.onMain(func() {
				if err != nil {
					im.err = strings.TrimPrefix(client.Explain(err), "teams: ")
					return
				}
				im.bmp, im.data = bmp, data
			})
		}()
	}
	return im
}

// imageSize is how large att shows in a message: within w, its shape kept.
func imageSize(att proto.ChatAttachment, w float32) (float32, float32) {
	if att.Width <= 0 || att.Height <= 0 {
		return w, w * 0.6
	}
	iw, ih := float32(att.Width), float32(att.Height)
	scale := min(w/iw, 1)
	// A tall one is held to a square's height.
	if ih*scale > w*1.2 {
		scale = w * 1.2 / ih
	}
	return max(iw*scale, 40), max(ih*scale, 40)
}

// messageImages are a message's images in its bubble: a click opens one.
func (a *App) messageImages(c *ui.Context, team string, m proto.ChatMessage) {
	if len(m.Attachments) == 0 {
		return
	}
	w := float32(chatImageW)
	if len(m.Attachments) > 1 {
		w = (chatImageW - theme.S2) / 2 // two a row, with the row's gap between
	}
	ui.Row(c).Gap(theme.S2).Wrap().MaxWidth(chatImageW).Children(func() {
		for _, att := range m.Attachments {
			im, up := a.imageOf(team, att), a.uploadOf(att.ID)
			iw, ih := imageSize(att, w)
			if len(m.Attachments) > 1 {
				iw, ih = w, w
			}
			b := ui.ButtonBase(c.Key("img:"+att.ID)).Size(iw, ih).Radius(theme.RadiusSmall).Background(theme.Sunken).Center().
				Label("Image " + att.Name).Cursor(ui.CursorPointer)
			b.Children(func() {
				switch {
				case im.bmp != nil:
					ui.Image(c, im.bmp).Size(iw, ih).Radius(theme.RadiusSmall).Fit(ui.Cover)
				case im.err != "":
					ui.Column(c).Gap(theme.S1).Padding(theme.S2).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, icon.Image).FontSize(16).TextColor(theme.Idle)
						ui.Text(c, im.err).FontSize(theme.TextLabel).TextColor(theme.Muted).TextAlign(ui.Center).MaxLines(3)
					})
				default:
					widget.Spinner(c, 14)
				}
				uploadState(c, up)
			})
			if b.Clicked() && im.bmp != nil && up == nil {
				hc := a.chatsOf()
				hc.viewing, hc.viewingOf = &att, m
			}
		}
	})
}

// uploadState is what lies over a picture on its way: a small spinner in its corner
// while it uploads, a mark if that failed; nothing once it is up, or for a real one.
func uploadState(c *ui.Context, up *pendingImage) {
	if up == nil || up.att != nil {
		return
	}
	face := ui.Box(c.Key("upload")).Absolute().Right(theme.S1).Top(theme.S1).Size(20, 20).Radius(theme.RadiusFull).Background(theme.Dialog).Center()
	face.Children(func() {
		if up.err != "" {
			face.Tooltip(up.err)
			ui.Icon(c, icon.Alert).FontSize(12).TextColor(theme.Error)
			return
		}
		widget.Spinner(c, 12)
	})
}

// imageViewer is the image opened from a message, large over the app.
func (a *App) imageViewer(c *ui.Context, team string) {
	hc := a.chatsOf()
	if hc.viewing == nil {
		return
	}
	att, m := *hc.viewing, hc.viewingOf
	im := hc.images[att.ID]
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.78))
		w, h := c.Size()
		maxW, maxH := max(w-64, 200), max(h-120, 200)
		iw, ih := float32(att.Width), float32(att.Height)
		if iw <= 0 || ih <= 0 {
			iw, ih = maxW, maxH
		}
		scale := min(maxW/iw, maxH/ih, 1)
		panel.Gap(theme.S2).AlignItems(ui.Center)
		if im != nil && im.bmp != nil {
			ui.Image(c, im.bmp).Size(iw*scale, ih*scale).Radius(theme.RadiusSmall)
		}
		ui.Row(c).Gap(theme.S2).AlignItems(ui.Center).Padding(theme.S2, theme.S2, theme.S2, theme.S3).Radius(theme.RadiusControl+4).Background(theme.Dialog).Border(1, theme.BorderStrong).Children(func() {
			ui.Column(c).Gap(1).Shrink(1).MinWidth(0).Children(func() {
				ui.Text(c, nonEmptyStr(att.Name, "Image")).FontSize(theme.TextLabel).FontWeight(600).SingleLine()
				detail := m.Name + " · " + m.At.Local().Format("Mon 2 Jan 15:04")
				if att.Width > 0 {
					detail += fmt.Sprintf(" · %d×%d · %s", att.Width, att.Height, textfmt.Bytes(uint64(att.Size)))
				}
				ui.Text(c, detail).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			})
			if im != nil && len(im.data) > 0 && widget.IconButton(c, icon.Download, "Save to Downloads", false).Clicked() {
				name := strings.TrimSuffix(nonEmptyStr(att.Name, "image"), filepath.Ext(att.Name)) + imageExt(att.Type)
				if path, err := saveDownload(name, im.data); err != nil {
					a.notify(err.Error())
				} else {
					a.notify("Saved to " + path)
				}
			}
			if widget.IconButton(c, icon.Close, "Close (Esc)", false).Clicked() {
				open = false
			}
		})
	})
	if !open {
		hc.viewing = nil
	}
}

func imageExt(kind string) string {
	switch kind {
	case "image/gif":
		return ".gif"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	}
	return ".webp"
}

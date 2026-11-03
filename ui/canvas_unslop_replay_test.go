package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// A redesign's steps show in its panel; Replay lists them with what each
// changed, and leaves one out in a copy; the passes to run are picked
// before it starts.
func TestUnslopReplay(t *testing.T) {
	h := newCanvasHarness(t)
	cv, _ := h.cl.CreateCanvas(proto.CanvasCreateParams{Title: "Shop", Kind: proto.CanvasMockup})
	set := func(frame, html string) {
		h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLSet, Frame: frame, HTML: html})
	}
	set(cv.Frames[0].ID, `<style>.c{padding:13px}</style><h1 id="t">Welcome back!!</h1>`)
	h.settle("the canvas open", func() bool { return h.app.canvases.Selected() == cv.ID })
	h.app.chooseUnslop(cv.Frames[0].ID)
	if len(h.app.unslopPick.on) != 12 {
		t.Fatal("the passes to pick")
	}
	h.cl.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpBrief, Brief: &proto.DesignBrief{Product: "A shop", Users: "Owners", Tone: "Calm", FirstScreen: "Orders"}})
	u, err := h.cl.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Start: true, Only: []string{"3", "8"}})
	if err != nil {
		t.Fatal(err)
	}
	h.app.canvases.Goto(h.app.canvasOptions(), cv.ID, u.Frame)
	h.cl.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Frame: u.Frame, Pass: "3"})
	set(u.Frame, `<style>.c{padding:16px}</style><h1 id="t">Welcome back!!</h1>`)
	h.cl.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Frame: u.Frame, Pass: "8"})
	set(u.Frame, `<style>.c{padding:16px}</style><h1 id="t">Orders this week</h1>`)
	h.cl.CanvasUnslop(proto.CanvasUnslopParams{ID: cv.ID, Frame: u.Frame, Pass: "done"})
	h.settle("the steps in the panel", func() bool { return h.tt.HasText("Replay") && h.tt.HasText("08 Copy") })
	shot(t, h.tt, "unslop-steps.png")

	h.app.openReplay(u.Frame, 1)
	dlg := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(1180).Height(760).Background(theme.Background).TextColor(theme.Text).Children(func() { h.app.replayDialog(c) })
	}, 1180, 760)
	pump := func() {
		time.Sleep(10 * time.Millisecond)
		h.later.run()
	}
	for range 300 {
		pump()
		dlg.Frame()
		if h.app.replay.loaded || h.app.replay.err != "" {
			break
		}
	}
	dlg.Frame()
	if !dlg.HasText("Step 1 · 03 Spacing") || !dlg.HasText("Original") || !dlg.HasText("Leave this step out") {
		t.Fatalf("replay: %q", dlg.Texts())
	}
	shot(t, dlg, "unslop-replay.png")
	if err := dlg.Click("Leave this step out"); err != nil {
		t.Fatal(err)
	}
	for range 300 {
		pump()
		if !h.app.replay.open {
			break
		}
	}
	got, _ := h.cl.Canvas(cv.ID)
	last := got.Frames[len(got.Frames)-1]
	if !strings.Contains(last.Name, "without 03 spacing") || !strings.Contains(last.HTML, "padding:13px") || !strings.Contains(last.HTML, "Orders this week") {
		t.Fatalf("left out: %q %s", last.Name, last.HTML)
	}
}

// Frames make a looping GIF, and an MP4 where ffmpeg is.
func TestVideoEncoding(t *testing.T) {
	var frames [][]byte
	for i := range 3 {
		img := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for p := range img.Pix {
			img.Pix[p] = uint8(i * 80)
		}
		img.Set(1, 1, color.White)
		var b bytes.Buffer
		png.Encode(&b, img)
		frames = append(frames, b.Bytes())
	}
	data, err := encodeGIF(frames)
	if err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil || len(g.Image) != 3 {
		t.Fatalf("gif: %v", err)
	}
	if ffmpegPath() == "" {
		return
	}
	mp4, err := encodeMP4(frames)
	if err != nil || len(mp4) < 12 || string(mp4[4:8]) != "ftyp" {
		t.Fatalf("mp4: %d bytes, %v", len(mp4), err)
	}
}

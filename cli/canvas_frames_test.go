package cli

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

func solidPNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// frames asks the host for each moment, keeps each as a file and all on one
// sheet, says what the app found, and says when nothing moved between two.
func TestCanvasFrames(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	dial := func(on func(proto.Event)) *client.Client {
		a, b := net.Pipe()
		go srv.ServeConn(a)
		c, err := client.New(b, on)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	red, blue := solidPNG(t, 800, 500, color.RGBA{200, 0, 0, 255}), solidPNG(t, 800, 500, color.RGBA{0, 0, 200, 255})
	var app *client.Client
	seen := make(chan string, 8)
	motion := &proto.CanvasMotion{Duration: 8000, At: 4000}
	app = dial(func(e proto.Event) {
		var req proto.CanvasRenderRequest
		if e.Name != proto.EventCanvasRender || json.Unmarshal(e.Data, &req) != nil {
			return
		}
		at := ""
		if i := strings.Index(req.HTML, "__enowxMotionAt="); i >= 0 {
			at = req.HTML[i : i+strings.Index(req.HTML[i:], "<")]
		}
		seen <- at
		data := red
		if strings.Contains(at, "pct:100") {
			data = blue
		}
		go func() { _ = app.CanvasRenderReply(proto.CanvasRenderReply{ID: req.ID, Data: data, Motion: motion}) }()
	})
	if err := app.ServeBrowsers(); err != nil {
		t.Fatal(err)
	}
	c := dial(nil)
	var out bytes.Buffer
	run := runner{c: c, out: &out}
	if err := run.canvasCmd([]string{"new", "Logo", "--kind", "mockup"}); err != nil {
		t.Fatal(err)
	}
	if err := run.canvasCmd([]string{"write", "Logo", "--html", `<style>@keyframes a{}</style>`}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	dir := t.TempDir()
	if err := run.canvasCmd([]string{"frames", "Logo", "--at", "0,50,100", "--out", dir}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"{pct:0}", "{pct:50}", "{pct:100}"} {
		if got := <-seen; !strings.Contains(got, want) {
			t.Errorf("asked for %s, the page was told %q", want, got)
		}
	}
	text := out.String()
	for _, want := range []string{"4000 ms of 8000", "0 and 50 are the same picture", "sheet, in the order above"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in %s", want, text)
		}
	}
	if strings.Contains(text, "50 and 100 are the same") {
		t.Errorf("different pictures called the same: %s", text)
	}
	raw, err := os.ReadFile(dir + "/sheet.png")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 640 || cfg.Width > 2*640+3*sheetGap {
		t.Fatalf("sheet: %+v %v", cfg, err)
	}
	if _, err := os.Stat(dir + "/frame-50.png"); err != nil {
		t.Fatal(err)
	}

	// As a tool: the sheet as an image, with what it says beside it.
	var tool mcpTool
	for _, tl := range canvasTools {
		if tl.name == "canvas_frames" {
			tool = tl
		}
	}
	res := run.callTool(tool, runner.canvasCmd, map[string]any{"canvas": "Logo", "at": "0,100"})
	got, _ := json.Marshal(res)
	if !strings.Contains(string(got), `"type":"image"`) || !strings.Contains(string(got), "ms of 8000") {
		t.Fatalf("tool answered %.300s", got)
	}

	// A still says it caught one frame.
	out.Reset()
	if err := run.canvasCmd([]string{"screenshot", "Logo", dir + "/still.png"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "one frame") {
		t.Errorf("a still of a moving design said only: %s", out.String())
	}
}

func TestSplitMoments(t *testing.T) {
	if got := splitMoments(" 0, 25 ,,1500ms "); strings.Join(got, "|") != "0|25|1500ms" {
		t.Fatalf("%q", got)
	}
}

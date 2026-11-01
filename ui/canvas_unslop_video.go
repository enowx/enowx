package ui

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	colorpalette "image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"

	"github.com/enowx/enowx/core/unslop"
	"github.com/enowx/enowx/internal/quietcmd"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A redesign's play exported as a video to post: its steps merged into
// at most five, played in a hidden window at the video's size (a square
// cropped close on what changes most, or wide), captured frame by frame
// and made an MP4 with ffmpeg, when there is one, or a GIF; kept in the
// Downloads folder.

const (
	videoFPS      = 15
	videoMaxSteps = 5
)

// videoExport is an export under way. It belongs to the main thread.
type videoExport struct {
	running  bool
	progress string
}

// ffmpegPath is ffmpeg's program, "" when there is none.
func ffmpegPath() string {
	p, _ := exec.LookPath("ffmpeg")
	return p
}

func (a *App) videoButtons(c *ui.Context, r *unslopReplay) {
	if len(r.story.Steps) == 0 || !webViewAvailable() {
		return
	}
	ui.Column(c.Key("video")).FillWidth().Gap(theme.S2).Children(func() {
		ui.Text(c, "Export the play").FontSize(theme.TextUI).FontWeight(500).TextColor(theme.Text)
		if r.video.running {
			ui.Row(c.Key("running")).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
				widget.Spinner(c, 14)
				ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, r.video.progress, widget.RunStyle{Color: theme.TextSoft}) })
			})
			return
		}
		formats := []string{"gif"}
		if ffmpegPath() != "" {
			formats = []string{"mp4", "gif"}
		}
		ui.Row(c.Key("formats")).FillWidth().Gap(theme.S1).Wrap().Children(func() {
			for _, f := range formats {
				for _, square := range []bool{true, false} {
					shape := "wide"
					if square {
						shape = "square"
					}
					label := strings.ToUpper(f) + " · " + shape
					if widget.Button(c.Key(label), widget.Secondary, nil, label, "").Clicked() {
						a.exportReplay(r, f, square)
					}
				}
			}
		})
		if ffmpegPath() == "" {
			ui.Text(c.Key("no-ffmpeg"), "Install ffmpeg for an MP4.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
	})
}

// exportReplay records r's play as format (mp4, gif), square or wide.
func (a *App) exportReplay(r *unslopReplay, format string, square bool) {
	st := unslop.StoryGroup(r.story, videoMaxSteps)
	r.video = videoExport{running: true, progress: "Playing it…"}
	name := strings.TrimSpace(r.story.Name)
	if name == "" {
		name = "design"
	}
	progress := func(s string) { a.onMain(func() { r.video.progress = s }) }
	go func() {
		frames, err := recordStory(st, square, progress)
		var path string
		if err == nil {
			progress("Making the " + strings.ToUpper(format) + "…")
			var data []byte
			if format == "mp4" {
				data, err = encodeMP4(frames)
			} else {
				data, err = encodeGIF(frames)
			}
			if err == nil {
				path, err = saveDownload(name+" unslop."+format, data)
			}
		}
		a.onMain(func() {
			r.video = videoExport{}
			if err != nil {
				a.status = "Couldn't export the video: " + err.Error()
				return
			}
			a.notify("Saved the video: " + path)
		})
	}()
}

// recordStory plays st in a hidden window, square (1080) or wide
// (1920×1080, shown at half), and captures it at videoFPS.
func recordStory(st proto.UnslopStory, square bool, progress func(string)) ([][]byte, error) {
	renders.Lock()
	defer renders.Unlock()
	w, h := 960, 540
	if square {
		w, h = 720, 720
	}
	win := mygo.NewWindow(mygo.WindowOptions{Hidden: true, Width: w, Height: h, UseContentSize: true,
		Frameless: true, SkipTaskbar: true, BackgroundColor: "#0d0d0f"})
	defer win.Destroy()
	r := renderIn(win.Page(), func() ([]byte, error) { return nil, nil },
		proto.CanvasRenderRequest{HTML: unslop.StoryHTML(st, ""), Width: w, Height: h, Script: "1"})
	if r.err != nil {
		return nil, r.err
	}
	pg := win.Page()
	time.Sleep(800 * time.Millisecond) // the designs' frames load
	if square {
		if x, y, cw := unslop.Crop(st); cw > 0 {
			_, _ = pg.Eval(fmt.Sprintf("__story.crop(%d,%d,%d)", x, y, cw))
		}
	}
	total := (len(st.Steps)*unslop.StoryStepMs + 1600)
	n := total * videoFPS / 1000
	var frames [][]byte
	for i := 0; i <= n; i++ {
		if _, err := pg.Eval(fmt.Sprintf("__story.at(%d)", i*1000/videoFPS)); err != nil {
			return nil, err
		}
		time.Sleep(25 * time.Millisecond) // painted
		data, err := win.CapturePage()
		if err != nil || len(data) == 0 {
			return nil, errors.New("the hidden window gave no picture")
		}
		frames = append(frames, data)
		if i%15 == 0 {
			progress(fmt.Sprintf("Recording… %d%%", i*100/max(n, 1)))
		}
	}
	return frames, nil
}

// encodeMP4 makes frames (PNGs) an H.264 MP4 with ffmpeg.
func encodeMP4(frames [][]byte) ([]byte, error) {
	ff := ffmpegPath()
	if ff == "" {
		return nil, errors.New("ffmpeg isn't installed")
	}
	dir, err := os.MkdirTemp("", "enowx-video")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	for i, f := range frames {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%04d.png", i)), f, 0o600); err != nil {
			return nil, err
		}
	}
	out := filepath.Join(dir, "out.mp4")
	cmd := quietcmd.Command(ff, "-y", "-loglevel", "error", "-framerate", fmt.Sprint(videoFPS), "-i", filepath.Join(dir, "f%04d.png"),
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", out)
	if msg, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %v: %s", err, bytes.TrimSpace(msg))
	}
	return os.ReadFile(out)
}

// gifWidth is a GIF's width: smaller than an MP4's, a GIF's colors costly.
const gifWidth = 540

// encodeGIF makes frames (PNGs) a looping GIF, gifWidth wide.
func encodeGIF(frames [][]byte) ([]byte, error) {
	out := &gif.GIF{LoopCount: 0}
	for _, f := range frames {
		src, err := png.Decode(bytes.NewReader(f))
		if err != nil {
			return nil, err
		}
		b := src.Bounds()
		w := min(gifWidth, b.Dx())
		h := b.Dy() * w / max(b.Dx(), 1)
		small := image.NewRGBA(image.Rect(0, 0, w, h))
		xdraw.ApproxBiLinear.Scale(small, small.Bounds(), src, b, draw.Src, nil)
		pal := image.NewPaletted(small.Bounds(), colorpalette.Plan9)
		draw.FloydSteinberg.Draw(pal, small.Bounds(), small, image.Point{})
		out.Image = append(out.Image, pal)
		out.Delay = append(out.Delay, 100/videoFPS)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

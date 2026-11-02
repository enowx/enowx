package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/internal/quietcmd"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A moving design rendered to an MP4 (docs/canvas.md): the page in a hidden window, its
// clock moved to each frame's time, each captured and piped to ffmpeg. One at a time,
// in the background, a card in the window's corner saying how far.

const (
	// videoCarried is the largest file that goes back to the one who asked over the
	// host's connection, under what one message holds.
	videoCarried = 12 << 20
	videoThreads = "2"
	// videoNoFFmpeg is what's said without an encoder.
	videoNoFFmpeg = "MP4 needs ffmpeg, which isn't installed here. Install it with your package manager (brew, winget or apt), and try again."
)

// videoJob is the video being made, and then made, until the user dismisses it. Its
// counts belong to the main thread; cancelled is read from the one making it.
type videoJob struct {
	name        string
	done, total int
	started     time.Time
	cancelled   atomic.Bool
	running     bool
	path, err   string
	warning     string
}

// videoEncoders are H.264 encoders in the order taken: the first gives the same file
// each time; the systems' own, the others, may not.
var videoEncoders = []string{"libx264", "h264_videotoolbox", "h264_mf", "libopenh264"}

// pickEncoder is the first of videoEncoders in what `ffmpeg -encoders` lists.
func pickEncoder(list string) string {
	for _, e := range videoEncoders {
		for _, line := range strings.Split(list, "\n") {
			if f := strings.Fields(line); len(f) >= 2 && f[1] == e {
				return e
			}
		}
	}
	return ""
}

// videoArgs are ffmpeg's arguments for PNGs on its stdin at fps, to path at out×h.
func videoArgs(encoder string, fps, w, h int, path string) []string {
	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-f", "image2pipe", "-framerate", fmt.Sprint(fps), "-c:v", "png", "-i", "-",
		"-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos", w, h), "-an", "-c:v", encoder, "-pix_fmt", "yuv420p", "-threads", videoThreads, "-movflags", "+faststart"}
	if encoder == "libx264" {
		args = append(args, "-preset", "medium", "-crf", "18")
	} else {
		args = append(args, "-b:v", "12M")
	}
	return append(args, "-f", "mp4", path)
}

// videoWhyNot says why this device can't make MP4s, "" when it can.
func videoWhyNot() string {
	if !webViewAvailable() {
		return "This device has no web view to render a design."
	}
	if ffmpegPath() == "" {
		return videoNoFFmpeg
	}
	return ""
}

// startCanvasVideo takes on the video req, one at a time.
func (a *App) startCanvasVideo(req proto.CanvasVideoRequest) {
	cl := a.client
	if cl == nil {
		return
	}
	answer := func(res proto.CanvasVideoResult, err error) {
		r := proto.CanvasVideoReply{ID: req.ID, Result: res}
		if err != nil {
			r.Error = err.Error()
		}
		_ = cl.CanvasVideoReply(r)
	}
	if a.video != nil && a.video.running {
		go answer(proto.CanvasVideoResult{}, errors.New("canvas: a video is being made already: one at a time"))
		return
	}
	job := &videoJob{name: req.Name, started: time.Now(), running: true, warning: req.Warning}
	a.video = job
	go func() {
		res, err := a.makeVideo(cl, job, req)
		a.onMain(func() {
			job.running = false
			switch {
			case err != nil && !job.cancelled.Load():
				job.err = err.Error()
			case err == nil:
				job.path = res.Path
			default:
				if a.video == job {
					a.video = nil // cancelled: nothing to say, and no file
				}
			}
		})
		answer(res, err)
	}()
}

// makeVideo loads the page and walks its frames, off the main thread.
func (a *App) makeVideo(cl *client.Client, job *videoJob, req proto.CanvasVideoRequest) (proto.CanvasVideoResult, error) {
	_ = cl.CanvasVideoProgress(proto.CanvasVideoProgress{ID: req.ID}) // taken on
	if why := videoWhyNot(); why != "" {
		return proto.CanvasVideoResult{}, errors.New(why)
	}
	ff := ffmpegPath()
	list, err := quietcmd.Command(ff, "-hide_banner", "-encoders").Output()
	if err != nil {
		return proto.CanvasVideoResult{}, fmt.Errorf("ffmpeg: %w", err)
	}
	encoder := pickEncoder(string(list))
	if encoder == "" {
		return proto.CanvasVideoResult{}, errors.New("this ffmpeg has no H.264 encoder: install a full build of it")
	}
	win := mygo.NewWindow(mygo.WindowOptions{Hidden: true, Width: req.Width, Height: req.Height, UseContentSize: true,
		Frameless: true, SkipTaskbar: true, BackgroundColor: "#ffffff"})
	defer win.Destroy()
	pg := win.Page()
	if err := loadRenderPage(pg, req.HTML); err != nil {
		return proto.CanvasVideoResult{}, err
	}
	// Fonts and images before the first frame, or it wouldn't match the others.
	for try := 0; try < 50; try++ {
		if v, err := pg.Eval(canvas.VideoReadyScript); err == nil && v == true {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	v, err := pg.Eval(canvas.VideoStartScript)
	if err != nil {
		return proto.CanvasVideoResult{}, err
	}
	duration, loop, ok := canvas.ParseVideoMotion(evalText(v))
	if !ok {
		return proto.CanvasVideoResult{}, errors.New("canvas: the page reported no motion to render")
	}
	times, err := canvas.VideoFramesAt(duration, loop, req.From, req.To, req.FPS)
	if err != nil {
		return proto.CanvasVideoResult{}, err
	}
	w, h := canvas.VideoSize(req.Width, req.Height, req.OutWidth)
	final, err := downloadPath(videoFileName(req.Name))
	if err != nil {
		return proto.CanvasVideoResult{}, err
	}
	// Beside where it ends up, renamed only when whole: a cancel leaves nothing.
	part := final + ".part"
	defer os.Remove(part)
	cmd := quietcmd.Command(ff, videoArgs(encoder, req.FPS, w, h, part)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return proto.CanvasVideoResult{}, err
	}
	if err := cmd.Start(); err != nil {
		return proto.CanvasVideoResult{}, fmt.Errorf("ffmpeg: %w", err)
	}
	if err := videoFrames(a, cl, job, req, win, stdin, times); err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return proto.CanvasVideoResult{}, err
	}
	if err := stdin.Close(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return proto.CanvasVideoResult{}, err
	}
	if err := cmd.Wait(); err != nil {
		return proto.CanvasVideoResult{}, fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	if err := os.Rename(part, final); err != nil {
		return proto.CanvasVideoResult{}, err
	}
	res := proto.CanvasVideoResult{Path: final, Width: w, Height: h, Frames: len(times), Seconds: float64(len(times)) / float64(req.FPS), Warning: req.Warning}
	if info, err := os.Stat(final); err == nil && info.Size() <= videoCarried {
		res.Data, _ = os.ReadFile(final)
	}
	return res, nil
}

// videoFrames moves the page to each time, captures it and gives it to the encoder.
func videoFrames(a *App, cl *client.Client, job *videoJob, req proto.CanvasVideoRequest, win *mygo.Window, out io.Writer, times []float64) error {
	pg := win.Page()
	said := time.Now()
	return videoWalk(times, job.cancelled.Load, func(at float64) ([]byte, error) {
		if _, err := pg.Eval(canvas.VideoSeekScript(at)); err != nil {
			return nil, err
		}
		time.Sleep(motionPaint)
		return capturePainted(win.CapturePage)
	}, out, func(done, total int) {
		if done < total && time.Since(said) < 500*time.Millisecond {
			return
		}
		said = time.Now()
		a.onMain(func() { job.done, job.total = done, total })
		_ = cl.CanvasVideoProgress(proto.CanvasVideoProgress{ID: req.ID, Done: done, Total: total})
	})
}

// errVideoCancelled says the user cancelled a video.
var errVideoCancelled = errors.New("canvas: the video was cancelled")

// videoWalk takes each time's frame to out, in order, until done or cancelled.
func videoWalk(times []float64, cancelled func() bool, frame func(at float64) ([]byte, error), out io.Writer, progress func(done, total int)) error {
	for i, at := range times {
		if cancelled() {
			return errVideoCancelled
		}
		data, err := frame(at)
		if err != nil {
			return err
		}
		if _, err := out.Write(data); err != nil {
			return fmt.Errorf("ffmpeg stopped taking frames: %w", err)
		}
		progress(i+1, len(times))
	}
	return nil
}

// videoFileName is a video's file, from what it's of.
func videoFileName(name string) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	if clean == "" {
		clean = "canvas"
	}
	return clean + ".mp4"
}

// timeLeft says how long a job at done of total, begun at started, has to go.
func timeLeft(done, total int, elapsed time.Duration) string {
	if done <= 0 || total <= done {
		return ""
	}
	left := time.Duration(float64(elapsed) / float64(done) * float64(total-done)).Round(time.Second)
	return left.String() + " left"
}

// showsVideoCard says the corner has a video to speak of.
func (a *App) showsVideoCard() bool { return a.video != nil }

// videoCard is how far the video has got, or what became of it. It takes no focus and
// moves nobody.
func (a *App) videoCard(c *ui.Context) {
	job := a.video
	if job == nil {
		return
	}
	title, detail := "Rendering "+job.name, "Starting…"
	switch {
	case job.err != "":
		title, detail = "Couldn't render "+job.name, job.err
	case job.path != "":
		title, detail = "Rendered "+job.name, job.path
	case job.total > 0:
		detail = fmt.Sprintf("%d%% · %d of %d frames", job.done*100/job.total, job.done, job.total)
		if left := timeLeft(job.done, job.total, time.Since(job.started)); left != "" {
			detail += " · " + left
		}
	}
	widget.Float(ui.Column(c).FillWidth().Padding(theme.S3).Gap(theme.S2).Radius(theme.RadiusCard).
		Background(theme.Dialog).Border(1, theme.BorderStrong)).Transition(widget.OverlayMotion).Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.Download).FontSize(14).TextColor(theme.Now).Shrink(0)
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, title, widget.RunStyle{Weight: 500}) })
		})
		widget.Running(c.Key("detail"), detail, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		if job.warning != "" && !job.running {
			ui.Text(c.Key("warning"), job.warning).FontSize(theme.TextLabel).TextColor(theme.Warning)
		}
		if job.running && job.total > 0 {
			widget.Meter(c, widget.Segment{Fraction: float64(job.done) / float64(job.total), Color: theme.Now})
			c.After(time.Second)
		}
		ui.Row(c.Key("actions")).FillWidth().Gap(theme.S2).Justify(ui.End).Children(func() {
			switch {
			case job.running:
				if widget.GhostButton(c.Key("cancel"), "Cancel").Clicked() {
					job.cancelled.Store(true)
				}
			default:
				if job.path != "" {
					if widget.GhostButton(c.Key("folder"), "Show in folder").Clicked() {
						go mygo.Shell.ShowItemInFolder(job.path)
					}
					if widget.GhostButton(c.Key("open"), "Open").Clicked() {
						go func() { _ = mygo.Shell.OpenPath(job.path) }()
					}
				}
				if widget.GhostButton(c.Key("dismiss"), "Dismiss").Clicked() {
					a.video = nil
				}
			}
		})
	})
}

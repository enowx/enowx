package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/enowx/enowx/proto"
)

// A still shows one moment of a moving design, so agents check motion from
// several: each a render of the design moved there, and a sheet of them to read
// at once.

const framesUsage = `motion:
  frames CANVAS [DESIGN] [--at 0,25,50,75,100] [--out DIR] [--sheet FILE]
                                   moments of a design's animation as PNGs (percent of its length, or 1500ms),
                                   and one sheet of them in order, left to right, to read at once`

const (
	defaultMoments = "0,25,50,75,100"
	maxMoments     = 24
	sheetCell      = 640 // a frame's width on a sheet, at most
	sheetGap       = 8
)

func (r runner) canvasFrames(args []string) error {
	fs := flag.NewFlagSet("frames", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	frame := fs.String("frame", "", "")
	at := fs.String("at", defaultMoments, "")
	dir := fs.String("out", "", "")
	sheet := fs.String("sheet", "", "")
	pos, err := parseAll(fs, args)
	if err != nil || len(pos) == 0 {
		return errors.New(framesUsage)
	}
	design := *frame
	if len(pos) > 1 {
		design = pos[1]
	}
	moments := splitMoments(*at)
	if len(moments) == 0 || len(moments) > maxMoments {
		return fmt.Errorf("canvas: frames takes 1 to %d moments", maxMoments)
	}
	c, err := r.findCanvas(pos[0])
	if err != nil {
		return err
	}
	if *dir == "" {
		if *dir, err = os.MkdirTemp("", "enowx-frames-"); err != nil {
			return err
		}
	} else if err := os.MkdirAll(*dir, 0o755); err != nil {
		return err
	}
	var pictures [][]byte
	// One page for all of them: loading a large design for each moment ran past what a
	// tool call may take.
	run := fmt.Sprintf("frames-%d", time.Now().UnixNano())
	for _, m := range moments {
		out, err := r.c.RenderCanvas(proto.CanvasRenderParams{ID: c.ID, Frame: design, Format: proto.RenderPNG, At: m, Keep: run})
		if err != nil {
			return err
		}
		if out.Motion == nil {
			return errors.New("canvas: this design has no motion to sample: the page reported none (no enowx.motion.add, CSS animation or running anime.js), or the app rendering it is older; canvas screenshot shows it as it is")
		}
		path := filepath.Join(*dir, "frame-"+strings.NewReplacer("%", "", ".", "_").Replace(m)+".png")
		if err := os.WriteFile(path, out.Data, 0o644); err != nil {
			return err
		}
		pictures = append(pictures, out.Data)
		fmt.Fprintf(r.out, "%6s  %6.0f ms of %.0f  %s\n", m, out.Motion.At, out.Motion.Duration, path)
	}
	for i := 1; i < len(pictures); i++ {
		if bytes.Equal(pictures[i-1], pictures[i]) {
			fmt.Fprintf(r.out, "note: %s and %s are the same picture: nothing moves between them\n", moments[i-1], moments[i])
		}
	}
	if *sheet == "" {
		*sheet = filepath.Join(*dir, "sheet.png")
	}
	data, err := contactSheet(pictures)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*sheet, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "sheet, in the order above: %s\n", *sheet)
	return nil
}

// splitMoments reads "0, 25,50" as its moments.
func splitMoments(list string) []string {
	var out []string
	for _, m := range strings.Split(list, ",") {
		if m = strings.TrimSpace(m); m != "" {
			out = append(out, m)
		}
	}
	return out
}

// contactSheet lays PNGs out in order, left to right, in a roughly square grid,
// shrunk so that an agent can read them at once.
func contactSheet(pictures [][]byte) ([]byte, error) {
	if len(pictures) == 0 {
		return nil, errors.New("canvas: no frames")
	}
	var imgs []image.Image
	for _, p := range pictures {
		img, err := png.Decode(bytes.NewReader(p))
		if err != nil {
			return nil, fmt.Errorf("canvas: read a frame: %w", err)
		}
		imgs = append(imgs, img)
	}
	first := imgs[0].Bounds()
	cw := min(first.Dx(), sheetCell)
	ch := max(first.Dy()*cw/max(first.Dx(), 1), 1)
	cols := int(math.Ceil(math.Sqrt(float64(len(imgs)))))
	rows := (len(imgs) + cols - 1) / cols
	sheet := image.NewRGBA(image.Rect(0, 0, cols*cw+(cols+1)*sheetGap, rows*ch+(rows+1)*sheetGap))
	xdraw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{0x1a, 0x1a, 0x1d, 0xff}), image.Point{}, xdraw.Src)
	for i, img := range imgs {
		x := sheetGap + (i%cols)*(cw+sheetGap)
		y := sheetGap + (i/cols)*(ch+sheetGap)
		xdraw.ApproxBiLinear.Scale(sheet, image.Rect(x, y, x+cw, y+ch), img, img.Bounds(), xdraw.Src, nil)
	}
	var b bytes.Buffer
	err := png.Encode(&b, sheet)
	return b.Bytes(), err
}

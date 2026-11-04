package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/core/designshare"
	"github.com/enowx/enowx/proto"
)

// A canvas holds many designs (its frames), numbered from 1: variations
// of a page, the pages of a flow, or both, which the user browses, compares
// and picks from in the Canvas page.

const designsUsage = `designs of a canvas (N: a design's number, name or ID):
  designs CANVAS                   its designs: number, name, page × variant, favorite, winner, link
  new-design CANVAS --title T [--description D] [--page P] [--variant V] [--size WxH] [--from N]
                                   a new design (a copy of design N with --from), its number printed
  design CANVAS N [--title T] [--description D] [--page P] [--variant V]   describe a design
  goto CANVAS N                    show design N in the app
  compare CANVAS N M [...]         show 2 to 4 designs side by side in the app, and what differs
  pick CANVAS N                    design N is the winner (pick CANVAS none: no winner)
  favorite CANVAS N [--off]        star a design
  move-design CANVAS N --before M  reorder (without --before: last)
  rm-design CANVAS N               delete a design`

var designCommands = []string{"designs", "new-design", "design", "goto", "compare", "pick", "favorite", "move-design", "rm-design"}

func (r runner) canvasDesigns(cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	title := fs.String("title", "", "")
	desc := fs.String("description", "", "")
	page := fs.String("page", "", "")
	variant := fs.String("variant", "", "")
	size := fs.String("size", "", "")
	from := fs.String("from", "", "")
	before := fs.String("before", "", "")
	off := fs.Bool("off", false, "")
	pos, err := parseAll(fs, args)
	if err != nil || len(pos) == 0 {
		return errors.New(designsUsage)
	}
	c, err := r.findCanvas(pos[0])
	if err != nil {
		return err
	}
	pos = pos[1:]
	need := func(n int) error {
		if len(pos) < n {
			return errors.New(designsUsage)
		}
		return nil
	}
	edit := func(e proto.CanvasEdit) (proto.Canvas, error) {
		e.ID, e.Session = c.ID, os.Getenv("ENOWX_SESSION")
		return r.c.EditCanvas(e)
	}
	set := func(name string) bool { return isSet(fs, name) }
	str := func(name string, v *string) *string {
		if set(name) {
			return v
		}
		return nil
	}
	switch cmd {
	case "designs":
		return r.listDesigns(c)
	case "new-design":
		var got proto.Canvas
		if *from != "" {
			name := *title
			got, err = edit(proto.CanvasEdit{Op: proto.CanvasOpFrameCopy, Frame: *from, Name: &name, Description: str("description", desc),
				Page: str("page", page), Variant: str("variant", variant)})
		} else {
			if *title == "" {
				return errors.New("usage: enowx-app canvas new-design CANVAS --title T [--description D] [--page P] [--variant V] [--from N]")
			}
			f := proto.CanvasFrame{Name: *title, Description: *desc, Page: *page, Variant: *variant}
			if *size != "" {
				if f.Width, f.Height, err = sizeOf(*size); err != nil {
					return err
				}
			} else if len(c.Frames) > 0 && canvasSized(c) {
				f.Width, f.Height = c.Frames[0].Width, c.Frames[0].Height
			}
			got, err = edit(proto.CanvasEdit{Op: proto.CanvasOpFrameAdd, FrameInfo: &f})
		}
		if err != nil {
			return err
		}
		n := len(got.Frames)
		if *from != "" {
			i, _ := canvas.FrameIndex(c, *from)
			n = i + 2
		}
		fmt.Fprintf(r.out, "Design %d: %s\nWrite it with --frame %d (canvas write/patch/shape).\n", n, got.Frames[n-1].Name, n)
		return nil
	case "design":
		if err := need(1); err != nil {
			return err
		}
		_, err = edit(proto.CanvasEdit{Op: proto.CanvasOpFrameMeta, Frame: pos[0], Name: str("title", title), Description: str("description", desc),
			Page: str("page", page), Variant: str("variant", variant)})
		return err
	case "goto":
		if err := need(1); err != nil {
			return err
		}
		_, err = r.c.ShowCanvas(proto.CanvasControl{ID: c.ID, Action: proto.CanvasOpen, Frame: pos[0]})
		return err
	case "compare":
		if len(pos) < 2 || len(pos) > 4 {
			return errors.New("usage: enowx-app canvas compare CANVAS N M [N2 N3] (2 to 4 designs)")
		}
		if _, err := r.c.ShowCanvas(proto.CanvasControl{ID: c.ID, Action: proto.CanvasCompare, Frames: pos}); err != nil {
			return err
		}
		return r.compareDesigns(c, pos)
	case "pick":
		if err := need(1); err != nil {
			return err
		}
		ref := pos[0]
		if strings.EqualFold(ref, "none") {
			ref = ""
		}
		_, err = edit(proto.CanvasEdit{Op: proto.CanvasOpPick, Frame: ref})
		return err
	case "favorite":
		if err := need(1); err != nil {
			return err
		}
		on := !*off
		_, err = edit(proto.CanvasEdit{Op: proto.CanvasOpFrameMeta, Frame: pos[0], Favorite: &on})
		return err
	case "move-design":
		if err := need(1); err != nil {
			return err
		}
		_, err = edit(proto.CanvasEdit{Op: proto.CanvasOpFrameMove, Frame: pos[0], Before: *before})
		return err
	case "rm-design":
		if err := need(1); err != nil {
			return err
		}
		_, err = edit(proto.CanvasEdit{Op: proto.CanvasOpFrameRemove, Frame: pos[0]})
		return err
	}
	return errors.New(designsUsage)
}

// canvasSized says c's designs share a size, which a new one takes.
func canvasSized(c proto.Canvas) bool {
	for _, f := range c.Frames[1:] {
		if f.Width != c.Frames[0].Width || f.Height != c.Frames[0].Height {
			return false
		}
	}
	return true
}

func (r runner) listDesigns(c proto.Canvas) error {
	return r.labelledIfCopied(c, func(r runner) error { return r.printDesigns(c) })
}

func (r runner) printDesigns(c proto.Canvas) error {
	fmt.Fprintf(r.out, "%s  %s: %d designs\n", c.ID, c.Title, len(c.Frames))
	for i, f := range c.Frames {
		marks := ""
		if f.ID == c.Winner {
			marks += " [winner]"
		}
		if f.Favorite {
			marks += " [favorite]"
		}
		group := strings.TrimSpace(strings.Join([]string{f.Page, f.Variant}, " × "))
		group = strings.Trim(group, " ×")
		if group != "" {
			group = "  (" + group + ")"
		}
		fmt.Fprintf(r.out, "%3d  %s%s  %d×%d%s  %s\n", i+1, designshare.OneLine(f.Name), group, f.Width, f.Height, marks, canvas.Link(c.ID, i+1))
		if f.Description != "" {
			fmt.Fprintf(r.out, "     %s\n", f.Description)
		}
	}
	return nil
}

// compareDesigns prints what tells designs refs apart: each one's
// elements, the first against the others.
func (r runner) compareDesigns(c proto.Canvas, refs []string) error {
	var frames []proto.CanvasFrame
	for _, ref := range refs {
		i, err := canvas.FrameIndex(c, ref)
		if err != nil {
			return err
		}
		frames = append(frames, c.Frames[i])
	}
	fmt.Fprintf(r.out, "Shown side by side in the app.\n")
	for _, f := range frames[1:] {
		a, b := frames[0], f
		b.ID, b.Name = a.ID, a.Name // the same frame, so its elements compare
		changes := canvas.Compare(proto.Canvas{Frames: []proto.CanvasFrame{a}}, proto.Canvas{Frames: []proto.CanvasFrame{b}})
		fmt.Fprintf(r.out, "\n%s → %s: %d differences\n", frames[0].Name, f.Name, len(changes))
		for _, ch := range changes {
			fmt.Fprintf(r.out, "  %s %s\n", ch.Kind, strings.TrimSpace(ch.Element+" "+ch.About))
		}
	}
	return nil
}

package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/core/designshare"
	"github.com/enowx/enowx/proto"
)

func (r runner) canvasList() error {
	list, err := r.canvasesSeen()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Fprintln(r.out, "No canvases yet: \"enowx-app canvas new TITLE\" makes one.")
	}
	names := map[string]string{}
	if spaces, _ := r.c.Spaces(); len(spaces) > 0 {
		for _, sp := range spaces {
			names[sp.Team] = sp.Name
		}
	}
	var copied strings.Builder // their titles are the library's creators' words
	for _, c := range list {
		state := ""
		if c.Team != "" {
			state = "  [team " + names[c.Team] + "]"
		}
		if c.Paused {
			state += "  (paused by the user)"
		}
		if c.Source != nil {
			fmt.Fprintf(&copied, "%s  %-9s  %s%s\n", c.ID, c.Kind, designshare.OneLine(c.Title), state)
			continue
		}
		fmt.Fprintf(r.out, "%s  %-9s  %s%s\n", c.ID, c.Kind, c.Title, state)
	}
	if copied.Len() > 0 {
		fmt.Fprintf(r.out, "Copied from the community library:\n%s\n", designshare.ForAgents(strings.TrimRight(copied.String(), "\n")))
	}
	return nil
}

// sizeOf reads WxH.
func sizeOf(s string) (w, h int, err error) {
	a, b, ok := strings.Cut(strings.ToLower(s), "x")
	if ok {
		w, err = strconv.Atoi(a)
		if err == nil {
			h, err = strconv.Atoi(b)
		}
	}
	if !ok || err != nil || w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("size %q: write WIDTHxHEIGHT, as 1440x900", s)
	}
	return w, h, nil
}

func (r runner) canvasNew(title string, f *canvasFlags) error {
	if title == "" {
		return errors.New(canvasUsage)
	}
	// In a team's project a canvas is the team's, as the session sees
	// only the team's canvases.
	team, _ := sessionTeam()
	p := proto.CanvasCreateParams{Title: title, Kind: *f.kind, Group: *f.group, Team: team}
	if *f.project != "" {
		abs, _ := filepath.Abs(*f.project)
		p.Projects = []string{abs}
	}
	for _, fr := range f.frames {
		name, size, sized := strings.Cut(fr, ":")
		frame := proto.CanvasFrame{Name: name}
		if sized {
			w, h, err := sizeOf(size)
			if err != nil {
				return err
			}
			frame.Width, frame.Height = w, h
		}
		p.Frames = append(p.Frames, frame)
	}
	c, err := r.c.CreateCanvas(p)
	if err != nil {
		return err
	}
	fmt.Fprintln(r.out, c.ID)
	return nil
}

// canvasShow prints what an agent works from: frames, their elements by
// id, and the comments still open.
func (r runner) canvasShow(c proto.Canvas, f *canvasFlags) error {
	return r.labelledIfCopied(c, func(r runner) error { return r.printCanvas(c, f) })
}

func (r runner) printCanvas(c proto.Canvas, f *canvasFlags) error {
	if *f.showHTML {
		i, err := canvas.FrameIndex(c, *f.frame)
		if err != nil {
			return err
		}
		fmt.Fprintln(r.out, c.Frames[i].HTML)
		return nil
	}
	fmt.Fprintf(r.out, "%s  %s (%s), version %d\n", c.ID, c.Title, c.Kind, c.Version)
	if c.Paused {
		fmt.Fprintln(r.out, "Paused by the user: changes wait until they resume it.")
	}
	if len(c.Projects) > 0 {
		fmt.Fprintf(r.out, "Projects: %s\n", strings.Join(c.Projects, ", "))
	}
	for _, fr := range c.Frames {
		fmt.Fprintf(r.out, "\nFrame %s (%s) %d×%d\n", designshare.OneLine(fr.Name), fr.ID, fr.Width, fr.Height)
		if g := fr.Grid; g != nil {
			var names []string
			for _, a := range g.Areas {
				names = append(names, "#"+a.Name)
			}
			fmt.Fprintf(r.out, "  Grid: %d columns × %d rows, gaps %g × %g, margin %g %s (canvas grid tells its CSS)\n", g.Cols, g.Rows, g.ColGap, g.RowGap, g.Margin, strings.Join(names, " "))
		}
		if out := canvas.Outline(fr.HTML); len(out) > 0 {
			fmt.Fprintln(r.out, "  HTML elements:")
			for _, h := range out {
				fmt.Fprintf(r.out, "  %s#%s <%s> %s\n", strings.Repeat("  ", h.Depth+1), h.ID, h.Tag, h.Text)
			}
		} else if strings.TrimSpace(fr.HTML) != "" {
			fmt.Fprintln(r.out, "  HTML without ids: give elements id attributes to change them one at a time")
		}
		if len(fr.Elements) > 0 {
			fmt.Fprintln(r.out, "  Shapes:")
		}
		for _, el := range fr.Elements {
			fmt.Fprintf(r.out, "    %s %s at %g,%g %g×%g", el.ID, el.Type, el.X, el.Y, el.W, el.H)
			if el.From != "" || el.To != "" {
				fmt.Fprintf(r.out, " %s→%s", el.From, el.To)
			}
			if el.Text != "" {
				fmt.Fprintf(r.out, " %q", el.Text)
			}
			fmt.Fprintln(r.out)
		}
	}
	comments, err := r.c.CanvasComments(c.ID)
	if err == nil {
		open := 0
		for _, cm := range comments {
			if !cm.Resolved {
				open++
			}
		}
		if open > 0 {
			fmt.Fprintf(r.out, "\n%d open comments: \"enowx-app canvas comments %s\"\n", open, c.ID)
		}
	}
	return nil
}

// shapeFrom is the shape the flags say: a new one, or shape --id changed
// as far as they go.
func (r runner) shapeFrom(c proto.Canvas, f *canvasFlags) (proto.CanvasElement, error) {
	el, ok := shapeIn(c, *f.frame, *f.id)
	if !ok {
		el = proto.CanvasElement{ID: *f.id, Type: *f.typ, X: 40, Y: 40, W: 160, H: 80}
		if el.Type == "" {
			return el, errors.New("usage: enowx-app canvas shape CANVAS --type rect|ellipse|diamond|text|sticky|line|arrow|image …")
		}
	}
	set := map[string]bool{}
	f.fs.Visit(func(fl *flag.Flag) { set[fl.Name] = true })
	strs := map[string]*string{"type": &el.Type, "text": &el.Text, "fill": &el.Fill, "stroke": &el.Stroke, "color": &el.Color,
		"from": &el.From, "to": &el.To, "src": &el.Src}
	vals := map[string]*string{"type": f.typ, "text": f.text, "fill": f.fill, "stroke": f.stroke, "color": f.color, "from": f.from, "to": f.to, "src": f.src}
	for k, p := range strs {
		if set[k] {
			*p = *vals[k]
		}
	}
	nums := map[string][2]*float64{"x": {&el.X, f.x}, "y": {&el.Y, f.y}, "w": {&el.W, f.w}, "h": {&el.H, f.h},
		"size-text": {&el.FontSize, f.fontSize}, "radius": {&el.Radius, f.radius}, "stroke-width": {&el.StrokeWidth, f.strokeW}}
	for k, p := range nums {
		if set[k] {
			*p[0] = *p[1]
		}
	}
	if set["bold"] {
		el.Bold = *f.bold
	}
	if el.Type == proto.ShapeText && !ok && !set["fill"] {
		el.H = 32
	}
	return el, nil
}

func (r runner) canvasFrame(c proto.Canvas, pos []string, f *canvasFlags, edit func(proto.CanvasEdit) error) error {
	usage := errors.New("usage: enowx-app canvas frame CANVAS add NAME [--size WxH] | set F [--name N] [--size WxH] | rm F")
	if len(pos) < 2 {
		return usage
	}
	info := proto.CanvasFrame{Name: *f.name, Background: *f.background}
	if *f.size != "" {
		w, h, err := sizeOf(*f.size)
		if err != nil {
			return err
		}
		info.Width, info.Height = w, h
	}
	switch pos[0] {
	case "add":
		info.Name = pos[1]
		return edit(proto.CanvasEdit{Op: proto.CanvasOpFrameAdd, FrameInfo: &info})
	case "set":
		return edit(proto.CanvasEdit{Op: proto.CanvasOpFrameSet, Frame: pos[1], FrameInfo: &info})
	case "rm":
		return edit(proto.CanvasEdit{Op: proto.CanvasOpFrameRemove, Frame: pos[1]})
	}
	return usage
}

// canvasExport writes a frame, or the whole canvas, as --format says.
func (r runner) canvasExport(c proto.Canvas, f *canvasFlags) error {
	format := strings.ToLower(*f.format)
	if format == "" {
		return errors.New("usage: enowx-app canvas export CANVAS --format png|pdf|svg|html|mp4 [--frame F] [--out PATH]")
	}
	if format == "mp4" {
		return r.canvasExportVideo(c, f)
	}
	out, err := r.c.RenderCanvas(proto.CanvasRenderParams{ID: c.ID, Frame: *f.frame, Format: format})
	if err != nil {
		return err
	}
	path := *f.out
	if path == "" {
		path = safeName(c.Title) + "." + format
	}
	if err := os.WriteFile(path, out.Data, 0o644); err != nil {
		return err
	}
	abs, _ := filepath.Abs(path)
	fmt.Fprintf(r.out, "Wrote %s", abs)
	if out.Width > 0 {
		fmt.Fprintf(r.out, " (%d×%d)", out.Width, out.Height)
	}
	fmt.Fprintln(r.out)
	if out.Wider > 0 {
		fmt.Fprintf(r.out, "warning: the page is %d px wider than the frame, so it scrolls sideways: find what sticks out (fixed widths, rows that don't wrap, 100vw with padding, wide images) and fix it, then look again\n", out.Wider)
	}
	if out.Motion != nil {
		fmt.Fprintf(r.out, "note: this design moves, and a %s catches one frame of it, at %.1f s of %.1f s; canvas frames samples it, and HTML keeps the motion\n", format, out.Motion.At/1000, out.Motion.Duration/1000)
	}
	return nil
}

// canvasExportVideo has the app render a moving design to an MP4. The file is kept where
// the app runs, and written to --out too when it's small enough to carry here.
func (r runner) canvasExportVideo(c proto.Canvas, f *canvasFlags) error {
	out, err := r.c.CanvasVideo(proto.CanvasVideoParams{ID: c.ID, Frame: *f.frame, FPS: *f.fps, Width: *f.width, From: *f.fromMs, To: *f.toMs})
	if err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Rendered %d frames, %.1f s, %d×%d, by the app: %s\n", out.Frames, out.Seconds, out.Width, out.Height, out.Path)
	if *f.out != "" {
		if len(out.Data) == 0 {
			fmt.Fprintln(r.out, "note: it's too large to carry here, so it stays where the app runs")
		} else if err := os.WriteFile(*f.out, out.Data, 0o644); err != nil {
			return err
		} else {
			abs, _ := filepath.Abs(*f.out)
			fmt.Fprintf(r.out, "Wrote %s\n", abs)
		}
	}
	if out.Warning != "" {
		fmt.Fprintln(r.out, "warning: "+out.Warning)
	}
	return nil
}

// safeName is title as a file's name.
func safeName(title string) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(title))
	if name == "" {
		return "canvas"
	}
	return name
}

func (r runner) canvasComments(c proto.Canvas, all bool) error {
	list, err := r.c.CanvasComments(c.ID)
	if err != nil {
		return err
	}
	shown := 0
	for _, cm := range list {
		if cm.Resolved && !all {
			continue
		}
		shown++
		on := "the frame"
		if cm.Element != "" {
			on = "#" + cm.Element
		}
		if cm.Frame != "" {
			if i, err := canvas.FrameIndex(c, cm.Frame); err == nil {
				on += fmt.Sprintf(" of design %d (%s)", i+1, c.Frames[i].Name)
			} else {
				on += " of " + cm.Frame
			}
		}
		by := "the user"
		if cm.Author != "" {
			by = "an agent"
		}
		state := ""
		if cm.Resolved {
			state = " [resolved]"
		}
		fmt.Fprintf(r.out, "%s  on %s, by %s, %s ago%s: %s\n", cm.ID, on, by, time.Since(cm.Created).Round(time.Minute), state, cm.Text)
	}
	if shown == 0 {
		fmt.Fprintln(r.out, "No open comments.")
	} else if !all {
		fmt.Fprintf(r.out, "Done with one? \"enowx-app canvas resolve %s COMMENT\"\n", c.ID)
	}
	return nil
}

func (r runner) canvasHistory(c proto.Canvas) error {
	h, err := r.c.CanvasHistory(c.ID)
	if err != nil {
		return err
	}
	for _, v := range h.Versions {
		mark := " "
		if v.N == h.Head {
			mark = "*"
		}
		by := "user"
		if v.Session != "" {
			by = "agent"
		}
		what := v.Op
		if v.Element != "" {
			what += " " + v.Element
		}
		fmt.Fprintf(r.out, "%s %3d  %s  %-5s  %s\n", mark, v.N, v.Time.Local().Format("Jan 2 15:04:05"), by, what)
	}
	return nil
}

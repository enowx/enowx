package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
)

const canvasUsage = `usage: enowx-app canvas COMMAND (CANVAS: its ID or title; F: a frame's ID or name, the first without)
  list                             the host's canvases: ID, kind, title
  new TITLE [--kind mockup|wireframe|diagram|board] [--frame NAME[:WxH]]... [--group G] [--project PATH]
                                   a new canvas, its ID printed; frames named desktop, tablet or mobile take their size
  open CANVAS                      tell the user in the app it's there, with a card to click; it doesn't move them
  show CANVAS [--frame F] [--html] its frames, elements and open comments; --html prints a frame's HTML
  write CANVAS [--frame F] --html HTML|-          a frame's whole HTML (a mockup), elements with id attributes
  patch CANVAS --element E [--frame F] --html HTML|- [--into PARENT]
                                   put HTML in place of element E (by id), or add it at the end of PARENT (the body)
  patch CANVAS --element E --text TEXT            what element E says (HTML inside it, or a shape's text)
  shape CANVAS [--frame F] [--id E] --type rect|ellipse|diamond|text|sticky|line|arrow|image
        [--x N --y N --w N --h N] [--text T] [--fill #RRGGBB] [--stroke #RRGGBB] [--color #RRGGBB]
        [--size N] [--bold] [--radius N] [--from E --to E] [--src ASSET]
                                   add a shape, or change shape E (only what is given)
  remove CANVAS --element E [--frame F]           remove an HTML element or a shape
  frame CANVAS add NAME [--size WxH] | set F [--name N] [--size WxH] [--background #RRGGBB] | rm F
  screenshot CANVAS FILE [--frame F|desktop|tablet|mobile]   a PNG of a frame, to look at what you made (a moving design's end; frames samples it)
  export CANVAS --format png|pdf|svg|html [--frame F] [--out PATH]   (html without --frame: all of it, read-only, to share)
  comments CANVAS [--all]          the user's comments on its elements, to work on; --all the resolved too
  comment CANVAS TEXT [--element E] [--frame F] | resolve CANVAS COMMENT
  history CANVAS | undo CANVAS | redo CANVAS | restore CANVAS N
  asset CANVAS FILE                keep an image: shapes show it as --src NAME, HTML as assets/NAME
  rm CANVAS                        delete a canvas, its versions and comments
  space CANVAS --to TEAM|personal [--copy]
                                   move a canvas to a team's space or your own (copied with --copy)
  guide [motion]                   how to design a canvas here: rules, components, layouts; motion: animation in depth
  ` + unslopUsage + `
  ` + briefUsage + `
  ` + buildUsage + `
  ` + imageUsage + `
` + framesUsage + "\n" + shareUsage + "\n" + designsUsage + "\n" + gridUsage + "\n" + canvasChatUsage

// parseAll parses args' flags wherever they are, and returns the rest.
func parseAll(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return rest, nil
		}
		rest = append(rest, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// canvasFlags are every command's flags.
type canvasFlags struct {
	fs                                    *flag.FlagSet
	kind, group, project, frame, html     *string
	element, into, text, typ, id          *string
	fill, stroke, color, from, to, src    *string
	format, out, name, size, background   *string
	x, y, w, h, fontSize, radius, strokeW *float64
	bold, showHTML, all, copy             *bool
	frames                                []string
}

func newCanvasFlags(cmd string) *canvasFlags {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	f := &canvasFlags{fs: fs}
	for p, name := range map[**string]string{&f.kind: "kind", &f.group: "group", &f.project: "project", &f.element: "element",
		&f.into: "into", &f.text: "text", &f.typ: "type", &f.id: "id", &f.fill: "fill", &f.stroke: "stroke", &f.color: "color",
		&f.from: "from", &f.to: "to", &f.src: "src", &f.format: "format", &f.out: "out", &f.name: "name", &f.size: "size",
		&f.background: "background"} {
		*p = fs.String(name, "", "")
	}
	f.frame = fs.String("frame-ref", "", "")
	f.html = fs.String("html", "", "")
	for p, name := range map[**float64]string{&f.x: "x", &f.y: "y", &f.w: "w", &f.h: "h", &f.fontSize: "size-text", &f.radius: "radius", &f.strokeW: "stroke-width"} {
		*p = fs.Float64(name, 0, "")
	}
	f.bold = fs.Bool("bold", false, "")
	f.all = fs.Bool("all", false, "")
	f.copy = fs.Bool("copy", false, "")
	f.showHTML = fs.Bool("show-html", false, "")
	// --frame names a frame, or, with new, adds one each time.
	fs.Func("frame", "", func(v string) error { *f.frame = v; f.frames = append(f.frames, v); return nil })
	return f
}

// canvasCmd reads and changes the host's canvases.
func (r runner) canvasCmd(args []string) error {
	if len(args) == 0 {
		return errors.New(canvasUsage)
	}
	cmd := args[0]
	if slices.Contains(designCommands, cmd) {
		return r.canvasDesigns(cmd, args[1:])
	}
	if cmd == "unslop" {
		return r.canvasUnslop(args[1:])
	}
	if cmd == "brief" {
		return r.canvasBrief(args[1:])
	}
	if cmd == "frames" {
		return r.canvasFrames(args[1:])
	}
	if cmd == "image" {
		return r.canvasImage(args[1:])
	}
	if cmd == "build" || cmd == "direction" || cmd == "match" {
		return r.canvasBuild(cmd, args[1:])
	}
	if slices.Contains(chatCommands, cmd) {
		return r.canvasChat(cmd, args[1:])
	}
	if slices.Contains(gridCommands, cmd) {
		return r.canvasGrid(cmd, args[1:])
	}
	if slices.Contains(shareCommands, cmd) {
		return r.canvasShare(cmd, args[1:])
	}
	f := newCanvasFlags(cmd)
	// --html alone on show prints the HTML; elsewhere it takes a value.
	rest := slices.Clone(args[1:])
	if cmd == "show" {
		for i, a := range rest {
			if a == "--html" || a == "-html" {
				rest[i] = "--show-html"
			}
		}
	}
	if cmd == "shape" {
		for i, a := range rest {
			if a == "--size" || a == "-size" {
				rest[i] = "--size-text"
			}
		}
	}
	pos, err := parseAll(f.fs, rest)
	if err != nil {
		return errors.New(canvasUsage)
	}
	if *f.html == "-" || *f.text == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		if *f.html == "-" {
			*f.html = string(data)
		} else {
			*f.text = string(data)
		}
	}
	arg := func(i int) string {
		if i < len(pos) {
			return pos[i]
		}
		return ""
	}
	switch cmd {
	case "help":
		fmt.Fprintln(r.out, canvasUsage)
		return nil
	case "guide":
		if arg(0) == "motion" {
			fmt.Fprint(r.out, agent.CanvasMotionGuide)
			return nil
		}
		fmt.Fprint(r.out, agent.CanvasGuide)
		return nil
	case "list", "ls":
		return r.canvasList()
	case "new":
		return r.canvasNew(arg(0), f)
	}
	if arg(0) == "" {
		return errors.New(canvasUsage)
	}
	c, err := r.findCanvas(arg(0))
	if err != nil {
		return err
	}
	return r.canvasOn(cmd, c, pos[1:], f)
}

// canvasOn runs command cmd on canvas c.
func (r runner) canvasOn(cmd string, c proto.Canvas, pos []string, f *canvasFlags) error {
	session := os.Getenv("ENOWX_SESSION")
	edit := func(e proto.CanvasEdit) error {
		e.ID, e.Session = c.ID, session
		if e.Frame == "" {
			e.Frame = *f.frame
		}
		got, err := r.c.EditCanvas(e)
		if err != nil {
			return err
		}
		fmt.Fprintf(r.out, "Version %d of %s.\n", got.Version, got.Title)
		return nil
	}
	switch cmd {
	case "open":
		_, err := r.c.ControlCanvas(c.ID, proto.CanvasOpen)
		return err
	case "show":
		return r.canvasShow(c, f)
	case "write":
		if !isSet(f.fs, "html") {
			return errors.New("usage: enowx-app canvas write CANVAS [--frame F] --html HTML|-")
		}
		return edit(proto.CanvasEdit{Op: proto.CanvasOpHTMLSet, HTML: *f.html})
	case "patch":
		if *f.element == "" && !isSet(f.fs, "html") {
			return errors.New("usage: enowx-app canvas patch CANVAS --element E (--html HTML|- | --text TEXT)")
		}
		if isSet(f.fs, "text") {
			if el, ok := shapeIn(c, *f.frame, *f.element); ok {
				el.Text = *f.text
				return edit(proto.CanvasEdit{Op: proto.CanvasOpElementSet, Element: &el})
			}
			return edit(proto.CanvasEdit{Op: proto.CanvasOpHTMLText, Target: *f.element, HTML: *f.text})
		}
		return edit(proto.CanvasEdit{Op: proto.CanvasOpHTMLPatch, Target: *f.element, Into: *f.into, HTML: *f.html})
	case "shape":
		el, err := r.shapeFrom(c, f)
		if err != nil {
			return err
		}
		return edit(proto.CanvasEdit{Op: proto.CanvasOpElementSet, Element: &el})
	case "remove":
		if *f.element == "" {
			return errors.New("usage: enowx-app canvas remove CANVAS --element E")
		}
		if _, ok := shapeIn(c, *f.frame, *f.element); ok {
			return edit(proto.CanvasEdit{Op: proto.CanvasOpElementDel, Target: *f.element})
		}
		return edit(proto.CanvasEdit{Op: proto.CanvasOpHTMLRemove, Target: *f.element})
	case "frame":
		return r.canvasFrame(c, pos, f, edit)
	case "screenshot":
		if len(pos) == 0 {
			return errors.New("usage: enowx-app canvas screenshot CANVAS FILE [--frame F|mobile]")
		}
		*f.format, *f.out = proto.RenderPNG, pos[0]
		return r.canvasExport(c, f)
	case "export":
		return r.canvasExport(c, f)
	case "comments":
		return r.canvasComments(c, *f.all)
	case "comment":
		if len(pos) == 0 {
			return errors.New("usage: enowx-app canvas comment CANVAS TEXT [--element E]")
		}
		_, err := r.c.CommentCanvas(proto.CanvasCommentParams{ID: c.ID, Op: proto.CanvasCommentAdd,
			Comment: proto.CanvasComment{Text: strings.Join(pos, " "), Element: *f.element, Frame: *f.frame, Author: session}})
		return err
	case "resolve":
		if len(pos) == 0 {
			return errors.New("usage: enowx-app canvas resolve CANVAS COMMENT")
		}
		_, err := r.c.CommentCanvas(proto.CanvasCommentParams{ID: c.ID, Op: proto.CanvasCommentResolve, Comment: proto.CanvasComment{ID: pos[0]}})
		return err
	case "history":
		return r.canvasHistory(c)
	case "undo", "redo", "restore":
		var got proto.Canvas
		var err error
		switch cmd {
		case "undo":
			got, err = r.c.UndoCanvas(c.ID)
		case "redo":
			got, err = r.c.RedoCanvas(c.ID)
		default:
			n, perr := strconv.Atoi(strings.Join(pos, ""))
			if perr != nil {
				return errors.New("usage: enowx-app canvas restore CANVAS N (a version, from history)")
			}
			got, err = r.c.RestoreCanvas(c.ID, n)
		}
		if err == nil {
			fmt.Fprintf(r.out, "Shows version %d.\n", got.Version)
		}
		return err
	case "asset":
		if len(pos) == 0 {
			return errors.New("usage: enowx-app canvas asset CANVAS FILE")
		}
		data, err := os.ReadFile(pos[0])
		if err != nil {
			return err
		}
		name, err := r.c.AddCanvasAsset(c.ID, filepath.Base(pos[0]), data)
		if err == nil {
			fmt.Fprintf(r.out, "Kept as %s: a shape's --src %s, or assets/%s in HTML.\n", name, name, name)
		}
		return err
	case "rm":
		return r.c.DeleteCanvas(c.ID)
	case "space":
		if *f.to == "" {
			return errors.New("usage: enowx-app canvas space CANVAS --to TEAM|personal [--copy]")
		}
		team, err := r.findSpace(*f.to)
		if err != nil {
			return err
		}
		info, err := r.c.MoveCanvasToSpace(c.ID, team, *f.copy)
		if err != nil {
			return err
		}
		fmt.Fprintln(r.out, info.ID)
		return nil
	}
	return errors.New(canvasUsage)
}

// shapeIn is shape id of c's frame ref, if it has one.
func shapeIn(c proto.Canvas, ref, id string) (proto.CanvasElement, bool) {
	i, err := canvas.FrameIndex(c, ref)
	if err != nil {
		return proto.CanvasElement{}, false
	}
	j := slices.IndexFunc(c.Frames[i].Elements, func(e proto.CanvasElement) bool { return e.ID == id })
	if j < 0 {
		return proto.CanvasElement{}, false
	}
	return c.Frames[i].Elements[j], true
}

// findCanvas is the canvas whose ID or title is ref.
func (r runner) findCanvas(ref string) (proto.Canvas, error) {
	list, err := r.canvasesSeen()
	if err != nil {
		return proto.Canvas{}, err
	}
	var found []proto.CanvasInfo
	for _, c := range list {
		if c.ID == ref {
			return r.c.Canvas(c.ID)
		}
		if strings.EqualFold(c.Title, ref) {
			found = append(found, c)
		}
	}
	switch len(found) {
	case 0:
		return proto.Canvas{}, fmt.Errorf("no canvas %q: \"enowx-app canvas list\" lists them", ref)
	case 1:
		return r.c.Canvas(found[0].ID)
	}
	return proto.Canvas{}, fmt.Errorf("%d canvases are titled %q: use the ID (\"enowx-app canvas list\")", len(found), ref)
}

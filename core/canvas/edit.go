package canvas

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// Edit makes change e to its canvas, a new version, and returns the
// canvas, with the frame and element it changed. Each edit reads the
// canvas as it is now, so the user's and an agent's changes to different
// elements never undo one another.
func (s *Store) Edit(e proto.CanvasEdit) (c proto.Canvas, frame, element string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, err = s.Get(e.ID); err != nil {
		return c, "", "", err
	}
	if e.Session != "" {
		c.Agent, c.AgentAt = e.Session, time.Now().UTC()
	}
	if frame, element, err = apply(&c, e); err != nil {
		return c, "", "", err
	}
	err = s.commit(&c, proto.CanvasVersion{Op: e.Op, Frame: frame, Element: element, Session: e.Session})
	return c, frame, element, err
}

// apply makes e to c, saying the frame and element it changed.
func apply(c *proto.Canvas, e proto.CanvasEdit) (frame, element string, err error) {
	if e.Op == proto.CanvasOpMeta {
		if e.Title != nil {
			if t := strings.Join(strings.Fields(*e.Title), " "); t != "" {
				c.Title = t
			}
		}
		if e.Kind != nil {
			switch *e.Kind {
			case proto.CanvasMockup, proto.CanvasWireframe, proto.CanvasDiagram, proto.CanvasBoard:
				c.Kind = *e.Kind
			default:
				return "", "", fmt.Errorf("canvas: no kind %q", *e.Kind)
			}
		}
		if e.Group != nil {
			c.Group = strings.TrimSpace(*e.Group)
		}
		if e.Projects != nil {
			c.Projects = slices.Compact(slices.Sorted(slices.Values(*e.Projects)))
		}
		return "", "", nil
	}
	if e.Op == proto.CanvasOpFrameAdd {
		f := proto.CanvasFrame{}
		if e.FrameInfo != nil {
			f = *e.FrameInfo
			f.ID = ""
		}
		f = newFrame(f, c.Kind)
		c.Frames = append(c.Frames, f)
		return f.ID, "", nil
	}
	if e.Op == proto.CanvasOpReference {
		if strings.TrimSpace(e.Reference) == "" {
			return "", "", errors.New("canvas: a reference needs its credit")
		}
		c.References = append(c.References, e.Reference)
		return "", "", nil
	}
	if e.Op == proto.CanvasOpBrief {
		c.Brief = e.Brief
		return "", "", nil
	}
	if e.Op == proto.CanvasOpPick {
		if e.Frame == "" {
			c.Winner = ""
			return "", "", nil
		}
		i, err := FrameIndex(*c, e.Frame)
		if err != nil {
			return "", "", err
		}
		c.Winner = c.Frames[i].ID
		return c.Winner, "", nil
	}
	i, err := FrameIndex(*c, e.Frame)
	if err != nil {
		return "", "", err
	}
	f := &c.Frames[i]
	frame = f.ID
	switch e.Op {
	case proto.CanvasOpFrameSet:
		if e.FrameInfo == nil {
			return "", "", errors.New("canvas: frame.set takes the frame's name, size or background")
		}
		in := *e.FrameInfo
		if n := strings.TrimSpace(in.Name); n != "" {
			f.Name = n
		}
		if in.Width > 0 && in.Height > 0 {
			f.Width, f.Height = in.Width, in.Height
		}
		if in.Background != "" {
			f.Background = in.Background
		}
	case proto.CanvasOpDirection:
		f.Direction = e.Direction
	case proto.CanvasOpFrameRemove:
		if len(c.Frames) == 1 {
			return "", "", errors.New("canvas: a canvas keeps a frame at least")
		}
		if c.Winner == f.ID {
			c.Winner = ""
		}
		c.Frames = slices.Delete(c.Frames, i, i+1)
	case proto.CanvasOpFrameMeta:
		setDesign(f, e)
	case proto.CanvasOpFrameGrid:
		if e.Grid == nil {
			f.Grid = nil
			break
		}
		g := *e.Grid
		g.Areas = slices.Clone(g.Areas)
		if err := CheckGrid(&g, *f); err != nil {
			return "", "", err
		}
		f.Grid = &g
	case proto.CanvasOpFrameMove:
		moved := *f
		c.Frames = slices.Delete(c.Frames, i, i+1)
		to := len(c.Frames)
		if e.Before != "" {
			j, err := FrameIndex(*c, e.Before)
			if err != nil {
				return "", "", err
			}
			to = j
		}
		c.Frames = slices.Insert(c.Frames, to, moved)
	case proto.CanvasOpFrameCopy:
		cp := *f
		cp.ID, cp.Favorite = "", false
		cp.Elements = slices.Clone(f.Elements)
		if f.Grid != nil {
			g := *f.Grid
			g.Areas = slices.Clone(g.Areas)
			cp.Grid = &g
		}
		cp.Name = copyName(*c, f.Name)
		setDesign(&cp, e)
		cp = newFrame(cp, c.Kind)
		c.Frames = slices.Insert(c.Frames, i+1, cp)
		frame = cp.ID
	case proto.CanvasOpHTMLSet:
		f.HTML = e.HTML
	case proto.CanvasOpHTMLPatch:
		if f.HTML, err = patchHTML(f.HTML, e.Target, e.Into, e.HTML); err != nil {
			return "", "", err
		}
		element = e.Target
		if element == "" || !slices.ContainsFunc(Outline(f.HTML), func(h HTMLElement) bool { return h.ID == element }) {
			element = firstID(e.HTML)
		}
	case proto.CanvasOpHTMLRemove:
		f.HTML, err = removeHTML(f.HTML, e.Target)
		element = e.Target
	case proto.CanvasOpHTMLText:
		f.HTML, err = innerHTML(f.HTML, e.Target, e.HTML)
		element = e.Target
	case proto.CanvasOpElementSet:
		if e.Element == nil {
			return "", "", errors.New("canvas: el.set takes an element")
		}
		el := *e.Element
		if err := checkElement(&el, f); err != nil {
			return "", "", err
		}
		snapShape(*f, &el)
		if j := slices.IndexFunc(f.Elements, func(x proto.CanvasElement) bool { return x.ID == el.ID }); j >= 0 {
			f.Elements[j] = el
		} else {
			f.Elements = append(f.Elements, el)
		}
		element = el.ID
	case proto.CanvasOpElementDel:
		j := slices.IndexFunc(f.Elements, func(x proto.CanvasElement) bool { return x.ID == e.Target })
		if j < 0 {
			return "", "", fmt.Errorf("canvas: no element %q in frame %s", e.Target, f.Name)
		}
		f.Elements = slices.Delete(f.Elements, j, j+1)
		// Arrows to it go with it.
		f.Elements = slices.DeleteFunc(f.Elements, func(x proto.CanvasElement) bool { return x.From == e.Target || x.To == e.Target })
		element = e.Target
	default:
		return "", "", fmt.Errorf("canvas: no edit %q", e.Op)
	}
	return frame, element, err
}

// shapes are the element types a frame takes.
var shapes = []string{proto.ShapeRect, proto.ShapeEllipse, proto.ShapeDiamond, proto.ShapeText, proto.ShapeSticky,
	proto.ShapeLine, proto.ShapeArrow, proto.ShapeImage}

// checkElement checks el and gives it an ID when it has none.
func checkElement(el *proto.CanvasElement, f *proto.CanvasFrame) error {
	if el.Type == "" {
		el.Type = proto.ShapeRect
	}
	if !slices.Contains(shapes, el.Type) {
		return fmt.Errorf("canvas: no shape %q: %s", el.Type, strings.Join(shapes, ", "))
	}
	if el.ID == "" {
		for n := len(f.Elements) + 1; ; n++ {
			id := fmt.Sprintf("%s-%d", el.Type, n)
			if !slices.ContainsFunc(f.Elements, func(x proto.CanvasElement) bool { return x.ID == id }) {
				el.ID = id
				break
			}
		}
	}
	for _, end := range []string{el.From, el.To} {
		if end != "" && !slices.ContainsFunc(f.Elements, func(x proto.CanvasElement) bool { return x.ID == end }) {
			return fmt.Errorf("canvas: no element %q to connect", end)
		}
	}
	for _, col := range []string{el.Fill, el.Stroke, el.Color} {
		if col != "" && col != "none" {
			if _, ok := ParseColor(col); !ok {
				return fmt.Errorf("canvas: color %q: write #rrggbb", col)
			}
		}
	}
	return nil
}

// firstID is the id of the first element of src that has one.
func firstID(src string) string {
	if list := Outline(src); len(list) > 0 {
		return list[0].ID
	}
	return ""
}

// setDesign sets what e gives of design f: its name, description, page,
// variant, favorite.
func setDesign(f *proto.CanvasFrame, e proto.CanvasEdit) {
	if e.Name != nil {
		if n := strings.Join(strings.Fields(*e.Name), " "); n != "" {
			f.Name = n
		}
	}
	if e.Description != nil {
		f.Description = strings.TrimSpace(*e.Description)
	}
	if e.Page != nil {
		f.Page = strings.TrimSpace(*e.Page)
	}
	if e.Variant != nil {
		f.Variant = strings.TrimSpace(*e.Variant)
	}
	if e.Favorite != nil {
		f.Favorite = *e.Favorite
	}
}

// copyName is a name for a copy of design name: "Hero (2)", the first
// free.
func copyName(c proto.Canvas, name string) string {
	base := strings.TrimSpace(name)
	if i := strings.LastIndex(base, " ("); i > 0 && strings.HasSuffix(base, ")") {
		base = base[:i]
	}
	for n := 2; ; n++ {
		try := fmt.Sprintf("%s (%d)", base, n)
		if !slices.ContainsFunc(c.Frames, func(f proto.CanvasFrame) bool { return strings.EqualFold(f.Name, try) }) {
			return try
		}
	}
}

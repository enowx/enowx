package canvasview

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/designcompare"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Compare: the designs picked side by side at one scale, the current one
// (a design named Current) first as the baseline; all at a desktop's or a
// phone's width at once, fitted or at their real size; arrow keys move
// between them. A build's directions show their cards and how distinct
// each pair is, and Pick sends the one chosen, with what to keep and
// change, to the build's agent.

// phoneWidth is the width designs are compared at as on a phone.
const phoneWidth = 390

// designCompare is how Compare shows.
type designCompare struct {
	phone, actual bool
	focus         int
	pick          *pickForm
}

// pickForm is a design being picked, with what to keep and change.
type pickForm struct {
	open         bool
	frame        string
	name         string
	keep, change string
}

// compareList is the designs picked, the current one first.
func (v *View) compareList(cv proto.Canvas) []proto.CanvasFrame {
	list := v.comparing(cv)
	slices.SortStableFunc(list, func(a, b proto.CanvasFrame) int {
		return boolInt(isBaseline(b)) - boolInt(isBaseline(a))
	})
	return list
}

func isBaseline(f proto.CanvasFrame) bool {
	return strings.EqualFold(strings.TrimSpace(f.Name), "Current")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// compareView is the designs picked, side by side.
func (v *View) compareView(c *ui.Context, o Options, cv proto.Canvas) {
	list := v.compareList(cv)
	if len(list) < 2 {
		v.view = viewGrid
		return
	}
	st := &v.cmp
	st.focus = min(max(st.focus, 0), len(list)-1)
	if c.Shortcut(0, ui.KeyRight) {
		st.focus = min(st.focus+1, len(list)-1)
	}
	if c.Shortcut(0, ui.KeyLeft) {
		st.focus = max(st.focus-1, 0)
	}
	if c.Shortcut(0, ui.KeyEnter) {
		v.frame, v.view = list[st.focus].ID, viewDesign
		return
	}
	ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
		v.compareBar(c, o, cv, list)
		area := ui.Scroll(c.Key("compare")).Grow(1).MinWidth(0).FillHeight().Background(theme.Sunken)
		b := area.Bounds()
		widest := 1
		for _, f := range list {
			widest = max(widest, v.compareWidth(f))
		}
		w := float32(360)
		if b.W > 0 {
			w = float32(math.Max(float64(b.W-40-float32(len(list)-1)*16)/float64(len(list)), 160))
		}
		// Too narrow side by side, or at their real size, they go one
		// above the other.
		stacked := b.W > 0 && w < 240 || st.actual
		if stacked {
			w = b.W - 40
		}
		area.Children(func() {
			box := ui.Row(c)
			if stacked {
				box = ui.Column(c)
			}
			box.Padding(20).Gap(16).AlignItems(ui.Start).Children(func() {
				for k, f := range list {
					fw := w * float32(v.compareWidth(f)) / float32(widest)
					if st.actual {
						fw = float32(v.compareWidth(f))
					}
					v.compareColumn(c, o, cv, f, k, w, fw)
				}
			})
		})
	})
	v.pickDialog(c, o, cv)
}

// compareWidth is the width design f is compared at.
func (v *View) compareWidth(f proto.CanvasFrame) int {
	if v.cmp.phone {
		return phoneWidth
	}
	return f.Width
}

// compareBar is Compare's controls: the width, fitted or real size, and a
// build's pairs.
func (v *View) compareBar(c *ui.Context, o Options, cv proto.Canvas, list []proto.CanvasFrame) {
	st := &v.cmp
	ui.Row(c).FillWidth().Gap(10).Padding(8, 14).AlignItems(ui.Center).Wrap().BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		dev := 0
		if st.phone {
			dev = 1
		}
		st.phone = chosen(widget.Segmented(c.Key("dev"), []string{"Desktop", "Phone"}, dev, func(i int) bool { return i == 1 && o.RenderWidth == nil }), dev) == 1
		size := 0
		if st.actual {
			size = 1
		}
		st.actual = chosen(widget.Segmented(c.Key("size"), []string{"Fit", "Real size"}, size, nil), size) == 1
		ui.Text(c, "← → move between them, Enter opens one").FontSize(theme.TextLabel).TextColor(theme.Muted)
		ui.Spacer(c)
		if run := o.BuildRun; run != nil && run.ID == cv.ID {
			for _, p := range run.Pairs {
				if !slices.ContainsFunc(list, func(f proto.CanvasFrame) bool { return f.ID == p.A }) || !slices.ContainsFunc(list, func(f proto.CanvasFrame) bool { return f.ID == p.B }) {
					continue
				}
				col := theme.Done
				if designcompare.TooClose(p) {
					col = theme.Warning
				}
				tip := "How distinct they are: what of skeleton, type, palette and hero they share, and how alike their first screens look squinted at"
				if len(p.Shared) > 0 {
					tip += " (they share " + strings.Join(p.Shared, ", ") + ")"
				}
				ui.Row(c.Key(p.A + p.B)).Tooltip(tip).Children(func() {
					widget.Tag(c, fmt.Sprintf("%s ↔ %s %d", short(frameName(cv, p.A)), short(frameName(cv, p.B)), p.Distinct), col)
				})
			}
		}
	})
}

func frameName(cv proto.Canvas, id string) string {
	if i := slices.IndexFunc(cv.Frames, func(f proto.CanvasFrame) bool { return f.ID == id }); i >= 0 {
		return cv.Frames[i].Name
	}
	return "?"
}

// compareColumn is design f in Compare, the k-th, in a column w wide, its
// picture fw wide.
func (v *View) compareColumn(c *ui.Context, o Options, cv proto.Canvas, f proto.CanvasFrame, k int, w, fw float32) {
	i := slices.IndexFunc(cv.Frames, func(x proto.CanvasFrame) bool { return x.ID == f.ID })
	col := ui.Column(c.Key(f.ID)).Width(max(w, fw)).Gap(8).Padding(6).Radius(theme.RadiusSmall)
	if v.cmp.focus == k {
		col.Border(1, theme.AccentLine)
	}
	col.Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			label := fmt.Sprintf("%d · %s", i+1, f.Name)
			if isBaseline(f) {
				label += " · baseline"
			}
			ui.Text(c, label).FontSize(theme.TextUI).FontWeight(600).SingleLine().Grow(1).Shrink(1).MinWidth(0)
			v.starButton(c, o, f)
			v.pickButton(c, o, cv, f)
		})
		open := ui.ButtonBase(c.Key("open")).Cursor(ui.CursorPointer).Label(fmt.Sprintf("Open design %d", i+1))
		open.Children(func() { v.comparePreview(c.Key("pv"), o, f, fw-14) })
		if open.Clicked() {
			v.frame, v.view = f.ID, viewDesign
		}
		if f.Direction != nil {
			directionCard(c, *f.Direction)
		} else if f.Description != "" {
			ui.Text(c, f.Description).FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
	})
}

// comparePreview is design f's picture fw wide, at a phone's width when
// Compare shows phones.
func (v *View) comparePreview(c *ui.Context, o Options, f proto.CanvasFrame, fw float32) ui.Element {
	if !v.cmp.phone || strings.TrimSpace(f.HTML) == "" {
		return v.designPreview(c, o, f, fw)
	}
	h := fw * float32(max(844, f.Height*phoneWidth/max(f.Width, 1))) / phoneWidth
	box := ui.Box(c).Size(fw, h).Shrink(0).Radius(theme.RadiusTail).Clip().Background(color(f.Background, ui.RGB(255, 255, 255)))
	if img := v.pictureAt(c, o, f, phoneWidth); img != nil {
		box.Children(func() { ui.Image(c, img).Width(fw).Fit(ui.Contain) })
	} else {
		box.Children(func() {
			ui.Row(c).FillWidth().FillHeight().Justify(ui.Center).AlignItems(ui.Center).Children(func() { widget.Spinner(c, 14) })
		})
	}
	return box
}

// directionCard is a direction's card under its design.
func directionCard(c *ui.Context, d proto.DesignDirection) {
	ui.Column(c.Key("card")).FillWidth().Gap(4).Children(func() {
		ui.Text(c, d.NorthStar).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		ui.Text(c, "From "+d.Source).FontSize(theme.TextLabel).TextColor(theme.Muted)
		ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
			for _, hex := range d.Palette {
				ui.Box(c.Key(hex)).Size(14, 14).Radius(theme.RadiusTail).Background(ui.Hex(hex)).Border(1, theme.Border).Tooltip(hex)
			}
			ui.Text(c, d.Type).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1).MinWidth(0).Margin(0, 0, 0, 4)
		})
		if d.Moment != "" {
			ui.Text(c, "Moment: "+d.Moment).FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
	})
}

// pickButton picks design f: for a build, with what to keep and change,
// sent to its agent; else as the canvas's winner.
func (v *View) pickButton(c *ui.Context, o Options, cv proto.Canvas, f proto.CanvasFrame) {
	if run := o.BuildRun; run != nil && run.ID == cv.ID && o.PickDirection != nil {
		if widget.Button(c.Key("pick"), widget.Primary, icon.Award, "Pick", "").Height(26).PaddingX(10).Label("Pick " + f.Name).Clicked() {
			v.cmp.pick = &pickForm{open: true, frame: f.ID, name: f.Name}
		}
		return
	}
	if widget.IconButton(c.Key("pick"), icon.Award, "Pick "+f.Name+" as the winner", cv.Winner == f.ID).Clicked() {
		v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpPick, Frame: f.ID})
	}
}

func (v *View) pickDialog(c *ui.Context, o Options, cv proto.Canvas) {
	p := v.cmp.pick
	if p == nil {
		return
	}
	if !p.open {
		v.cmp.pick = nil
		return
	}
	widget.FormSheet(c, &p.open, 480, "Pick "+p.name, "The build's agent refines it around what you say; the others stay as designs to go back to.", func() {
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			widget.Field(c, "What to keep", func() {
				ui.TextArea(c, &p.keep).FillWidth().TextColor(theme.Text).Height(56).Padding(8, 10).Radius(theme.RadiusControl).FontSize(theme.TextUI).
					Background(theme.Sunken).Border(1, theme.BorderStrong).Label("What to keep").Placeholder("The type, the ledger lines")
			})
			widget.Field(c, "What to change", func() {
				ui.TextArea(c, &p.change).FillWidth().TextColor(theme.Text).Height(56).Padding(8, 10).Radius(theme.RadiusControl).FontSize(theme.TextUI).
					Background(theme.Sunken).Border(1, theme.BorderStrong).Label("What to change").Placeholder("A warmer accent; the plans easier to tell apart")
			})
		})
	}, func() {
		if widget.PrimaryButton(c, icon.Award, "Pick", "").Label("Pick it, for the agent to refine").Clicked() {
			v.edit(o, proto.CanvasEdit{Op: proto.CanvasOpPick, Frame: p.frame})
			o.PickDirection(cv, p.frame, strings.TrimSpace(p.keep), strings.TrimSpace(p.change))
			p.open = false
		}
	})
}

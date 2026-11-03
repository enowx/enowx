package canvasview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A component of a mockup picked on the board: a click on the page picks
// the element with an id under it, and a bar over it asks the agent about
// it, has it changed or redesigned, or keeps a note on it.

// componentSel is the component picked: its id and box in the page (CSS
// pixels), and what the user writes in its bar.
type componentSel struct {
	id   string
	box  ui.Rect
	text string
	note bool   // the bar writes a note, not a message to the agent
	said string // what the bar last did, a moment
}

// pickAt picks the component at x, y of the page (CSS pixels), or nothing
// when no element with an id is there.
func (v *View) pickAt(o Options, x, y float32) {
	if o.Web == nil {
		return
	}
	o.Web.pick(float64(x), float64(y), func(id string, box ui.Rect) {
		if id == "" {
			v.deselect(o)
			v.comp = nil
			return
		}
		v.sel = id
		o.Web.outline(id)
		if v.comp == nil || v.comp.id != id {
			v.comp = &componentSel{id: id}
		}
		v.comp.box = box
	})
}

// componentBar is the bar over the component picked, on a board w×h at
// zoom s.
func (v *View) componentBar(c *ui.Context, o Options, s float64, w, h float32) {
	cp := v.comp
	if cp == nil || v.sel != cp.id || v.mode != modeDesign {
		v.comp = nil
		return
	}
	if c.Shortcut(0, ui.KeyEscape) {
		v.deselect(o)
		v.comp = nil
		return
	}
	const barW, barH = float32(460), float32(48)
	bw := min(barW, w)
	x := min(max(cp.box.X*float32(s), 0), max(w-bw, 0))
	y := cp.box.Y*float32(s) - barH - 6
	if y < 0 {
		y = min((cp.box.Y+cp.box.H)*float32(s)+6, h-barH)
	}
	bar := raised(ui.Row(c.Key("component-bar")).Absolute().Left(x).Top(max(y, 0)).Width(bw).Height(barH).PaddingX(theme.S2).Gap(theme.S1).
		AlignItems(ui.Center).Radius(theme.RadiusFull))
	bar.Children(func() {
		ui.Row(c.Key("id")).MaxWidth(110).MinWidth(0).Shrink(1).Padding(0, 0, 0, theme.S2).Children(func() {
			widget.Running(c, "#"+cp.id, widget.RunStyle{Size: theme.TextLabel, Color: theme.Accent})
		})
		hint := "Ask the AI, or say what to change"
		if cp.note {
			hint = "A note on it, for later"
		}
		if cp.said != "" && cp.text == "" {
			hint = cp.said
		}
		in := widget.Input(c.Key("ask"), &cp.text, hint).Grow(1).Shrink(1).MinWidth(0).Label("About #" + cp.id)
		if in.Submitted() && strings.TrimSpace(cp.text) != "" {
			v.componentSend(o, cp, strings.TrimSpace(cp.text))
		}
		if !cp.note && o.SendAgent != nil {
			rd := widget.GhostButton(c.Key("redesign"), "Redesign").
				Tooltip("Have the AI redesign this component alone, in the design's style")
			if rd.Clicked() {
				v.componentSend(o, cp, "Redesign this component: a better take that keeps the design's style, colors and type. Change only it, then look at it.")
			}
		}
		tip := "Write a note instead of asking the AI"
		if cp.note {
			tip = "Ask the AI instead of writing a note"
		}
		if widget.IconButton(c.Key("note"), icon.Comment, tip, cp.note).Clicked() {
			cp.note = !cp.note
		}
		if widget.IconButton(c.Key("close"), icon.Close, "Done with #"+cp.id+" (Esc)", false).Clicked() {
			v.deselect(o)
			v.comp = nil
		}
	})
}

// componentSend sends the bar's message about component cp to the agent,
// or keeps it as a note on it.
func (v *View) componentSend(o Options, cp *componentSel, text string) {
	cp.text = ""
	if cp.note {
		if v.canvas == nil || o.Comment == nil {
			return
		}
		cm := proto.CanvasComment{Text: text, Element: cp.id, Frame: v.frameOf().ID}
		o.Comment(proto.CanvasCommentParams{ID: v.canvas.ID, Op: proto.CanvasCommentAdd, Comment: cm}, v.tookComments)
		cp.said = "Noted on #" + cp.id
		return
	}
	if o.SendAgent == nil {
		return
	}
	o.SendAgent("@#" + cp.id + " " + text)
	v.OpenAssistant() // the agent's answer shows beside
	cp.said = "Sent to the AI"
}

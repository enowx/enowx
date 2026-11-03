package canvasview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// What a design's whole HTML costs (hashing it, searching it, parsing it) happens once for
// each page, not each frame: ten megabytes of it would be felt in every one. A page read
// anew compares whole once, and is kept, so that from then on it compares at once.

const (
	// outlineInline is how long a page may be to be parsed where it's asked for.
	outlineInline = 256 << 10
	// htmlEditMost is how long an element's HTML may be to be edited in the inspector,
	// whose text area lays all of it out.
	htmlEditMost = 64 << 10
)

// htmlWork is what was last worked out of a page. It belongs to the main thread.
type htmlWork struct {
	motionPage string
	motion     bool
	motionDone bool

	elementPage, elementID, elementSrc string
	elementErr                         error
	elementDone                        bool

	outlineAsked  string // the page being parsed off the main thread
	outlineAsking bool
}

// htmlKeyed is a design's key, and the page it's of.
type htmlKeyed struct {
	html string
	w, h int
	key  string
}

// keyOf is f's htmlKey.
func (v *View) keyOf(f proto.CanvasFrame) string {
	k := v.htmlKeys[f.ID]
	if k == nil {
		if v.htmlKeys == nil {
			v.htmlKeys = map[string]*htmlKeyed{}
		}
		k = &htmlKeyed{}
		v.htmlKeys[f.ID] = k
	}
	if k.key == "" || k.html != f.HTML || k.w != f.Width || k.h != f.Height {
		k.key = htmlKey(f)
	}
	k.html, k.w, k.h = f.HTML, f.Width, f.Height
	return k.key
}

// usesMotion is canvas.UsesMotion of page.
func (v *View) usesMotion(page string) bool {
	w := &v.htmlWork
	if !w.motionDone || w.motionPage != page {
		w.motion = canvas.UsesMotion(page)
	}
	w.motionPage, w.motionDone = page, true
	return w.motion
}

// elementHTML is canvas.ElementHTML of page and id.
func (v *View) elementHTML(page, id string) (string, error) {
	w := &v.htmlWork
	if !w.elementDone || w.elementID != id || w.elementPage != page {
		w.elementSrc, w.elementErr = canvas.ElementHTML(page, id)
	}
	w.elementPage, w.elementID, w.elementDone = page, id, true
	return w.elementSrc, w.elementErr
}

// outlineOf is page's elements. A long page is parsed off the main thread, what was read
// last showing until it's done.
func (v *View) outlineOf(o Options, page string) []canvas.HTMLElement {
	if v.outlineDone && page == v.outlinePage {
		v.outlinePage = page
		return v.outline
	}
	if len(page) <= outlineInline || o.OffMain == nil {
		v.outlinePage, v.outline, v.outlineDone = page, canvas.Outline(page), true
		return v.outline
	}
	w := &v.htmlWork
	if w.outlineAsking && w.outlineAsked == page {
		w.outlineAsked = page
		return v.outline
	}
	w.outlineAsking, w.outlineAsked = true, page
	var out []canvas.HTMLElement
	o.OffMain(func() { out = canvas.Outline(page) }, func() {
		if w.outlineAsked == page {
			v.outlinePage, v.outline, v.outlineDone, w.outlineAsking = page, out, true, false
		}
	})
	return v.outline
}

// longElement stands in for the inspector's HTML of an element too long to edit there.
func (v *View) longElement(c *ui.Context, o Options) {
	widget.Running(c.Key("html-id"), "#"+v.sel, widget.RunStyle{Weight: 500})
	widget.Notice(c.Key("html-long"), icon.Alert, theme.Warning, "Too long to edit here", "Ask an agent to change this element.", nil)
	ui.Row(c.Key("html-actions")).FillWidth().Gap(theme.S1).Wrap().AlignItems(ui.Center).Children(func() {
		v.selectionActions(c, o, v.sel)
	})
}

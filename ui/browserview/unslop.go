package browserview

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/unslopview"
	"github.com/enowx/enowx/ui/widget"
)

// Unslop in the Browser: the toolbar's button has the session's agent
// redesign the page's code where it looks AI-made, for a page of the
// project running on localhost; the panel shows the run over the page,
// and the page each read's marks, drawn again as the page reloads.

// localURL says u is on this device: localhost or a loopback address.
func localURL(u string) bool {
	p, err := url.Parse(u)
	if err != nil {
		return false
	}
	h := strings.ToLower(p.Hostname())
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// unslopButton starts an unslop of the page shown.
func (v *View) unslopButton(c *ui.Context, o Options, page Page) {
	if o.Unslop == nil {
		return
	}
	tip := "Find what makes this page look AI-made, and have the session's agent fix it in the code, pass by pass"
	can := page != nil && !o.Unslopping && localURL(page.URL())
	if page != nil && !localURL(page.URL()) {
		tip = "Unslop works on the project's pages running here (localhost)"
	}
	if o.Unslopping {
		widget.Spinner(c, 12)
		return
	}
	b := widget.Button(c.Key("unslop"), widget.Secondary, icon.Wand, "Unslop", "").Height(28).Tooltip(tip)
	if !can {
		b.Opacity(0.35).Label(tip)
		return
	}
	if b.Clicked() {
		o.Unslop()
	}
}

// unslopOverlay draws o.UnslopRun's panel over the page, and its latest
// read's marks in the page.
func (v *View) unslopOverlay(c *ui.Context, o Options) {
	steps := o.UnslopRun != nil && o.UnslopRun.Scans == 0 && len(o.UnslopRun.Steps) > 0
	switch {
	case o.Unslopping && !steps:
		// The page swept at once, while it is read.
		if v.sweeping == "" && v.page != nil {
			v.sweeping = v.page.URL()
			if ev, ok := v.page.(Evaluator); ok {
				go unslopview.DrawScan(func(code string) (any, error) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					return ev.Eval(ctx, code)
				}, "[]")
			}
		}
		unslopview.DrawStarting(c, "Reading the page, making a branch…")
		return
	case o.UnslopErr != "":
		v.sweeping = ""
		unslopview.DrawError(c, o.UnslopErr, o.UnslopDismiss)
		return
	}
	v.sweeping = ""
	run := o.UnslopRun
	if run == nil || v.page == nil {
		v.scanned = ""
		return
	}
	// A page done loading (the dev server reloaded it) lost its marks.
	if loading := v.page.Loading(); loading != v.wasLoading {
		if v.wasLoading = loading; !loading {
			v.scanned = ""
		}
	}
	// Each read, and each step before the first (the page at another
	// width), swept anew.
	if key := strconv.Itoa(run.Scans) + "/" + strconv.Itoa(run.Step) + "/" + v.page.URL(); !run.Done && v.scanned != key && !v.page.Loading() {
		v.scanned = key
		if ev, ok := v.page.(Evaluator); ok {
			go unslopview.DrawScan(func(code string) (any, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				return ev.Eval(ctx, code)
			}, unslopview.ScanItems(run))
		}
	}
	if run.Done || o.Unslopping {
		c.After(time.Second)
	}
	po := unslopview.PanelOptions{Reading: "Reading the page…", AgentName: o.AgentName, OpenReviewer: o.OpenSession}
	if run.Shots > 0 && o.UnslopCompare != nil {
		po.Compare = func() { o.UnslopCompare(*run) }
	}
	v.panel.Draw(c, run, po)
}

// Width is the device width the page shows at, 0 for the room's.
func (v *View) Width() int { return v.width }

// SetWidth shows the page at n CSS pixels wide, 0 for the room's, as an
// unslop reads it at each device's size.
func (v *View) SetWidth(n int) { v.width = max(n, 0) }

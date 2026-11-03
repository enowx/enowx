package browserview

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Draw shows the toolbar, then the page or why there is none, for a center tab.
func (v *View) Draw(c *ui.Context, o Options) {
	v.follow(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		v.toolbar(c, o)
		if v.err != "" {
			ui.Row(c).FillWidth().Shrink(0).Padding(theme.S2, theme.S3, 0, theme.S3).Children(func() {
				widget.Notice(c, nil, theme.Del, v.err, "", nil)
			})
		}
		switch {
		case !o.Supported:
			v.note(c, o, "This device has no web view to show pages in. Open them in the browser instead.")
		case v.page != nil && v.external:
			v.showPage(c, o)
		case !v.loaded:
			v.note(c, o, "Looking for the session's dev server…")
		case v.port == 0:
			// Something to look at until a dev server starts, which then
			// takes its place.
			v.openHome(o)
			v.note(c, o, "Waiting for a dev server: start one in the session (npm run dev, say) and it shows here.")
		case v.stopped:
			v.note(c, o, "The dev server on "+portLabel(v.port)+" stopped. The page comes back when it does.")
		case v.page == nil:
			v.note(c, o, "Opening "+portLabel(v.port)+"…")
		default:
			v.showPage(c, o)
		}
	})
}

// showPage places the page, as wide as the room or a device, an
// unslop's panel over it.
func (v *View) showPage(c *ui.Context, o Options) {
	ui.Box(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		if v.width == 0 {
			v.page.Show(c).Grow(1).FillWidth().FillHeight().Background(theme.Background)
		} else {
			// A device's width, centered, the rest of the room dimmed.
			ui.Row(c).Grow(1).FillWidth().FillHeight().Justify(ui.Center).AlignItems(ui.Stretch).Background(theme.Sunken).Children(func() {
				v.page.Show(c).Grow(1).MaxWidth(float32(v.width)).Background(theme.Background).
					BorderWidth(0, 1, 0, 1).BorderColor(theme.Border)
			})
		}
		v.unslopOverlay(c, o)
	})
}

func (v *View) note(c *ui.Context, o Options, text string) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(theme.S5).Gap(theme.S3).Children(func() {
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(theme.TextSoft).TextAlign(ui.Center)
		if !o.Supported && v.port != 0 && widget.Button(c, widget.Secondary, icon.External, "Open in browser", "").Clicked() {
			o.OpenURL(v.origin + "/")
		}
	})
}

// toolbar is back, forward, reload, the address, the port, the page's width,
// and what to do with the page. What can't fit gives way: the width's frame
// shrinks and the rest is in its menu.
func (v *View) toolbar(c *ui.Context, o Options) {
	ui.Row(c.Key("page-bar")).Height(44).Shrink(0).FillWidth().PaddingX(theme.S2).Gap(theme.S1).Clip().
		BorderWidth(0, 0, theme.Rule, 0).BorderColor(theme.Border).Children(func() {
		page := v.page
		if navButton(c, icon.ArrowLeft, "Back", page != nil && page.CanBack()) {
			page.Back()
		}
		if navButton(c, icon.ArrowRight, "Forward", page != nil && page.CanForward()) {
			page.Forward()
		}
		if navButton(c, icon.Sync, "Reload", page != nil) {
			page.Reload()
		}
		v.addressField(c, o)
		if page != nil && page.Loading() {
			widget.Spinner(c, 12)
		}
		if len(v.ports) > 1 {
			v.portPicker(c, o)
		}
		v.sizeMenu(c)
		v.pickButton(c, o, page)
		if navButton(c, icon.Camera, "Screenshot", page != nil && o.Attach != nil) {
			if png, err := page.Capture(); err != nil {
				v.err = "Couldn't take a screenshot: " + err.Error()
			} else {
				o.Attach(png)
			}
		}
		if o.ToCanvas != nil && navButton(c, icon.Canvas, "Put on a canvas", page != nil) {
			if png, err := page.Capture(); err != nil {
				v.err = "Couldn't take a screenshot: " + err.Error()
			} else {
				go func(u string) {
					w := 0
					if ev, ok := page.(Evaluator); ok {
						ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
						if n, err := ev.Eval(ctx, "window.innerWidth"); err == nil {
							if f, ok := n.(float64); ok {
								w = int(f)
							}
						}
						cancel()
					}
					o.ToCanvas(png, u, w)
				}(page.URL())
			}
		}
		v.unslopButton(c, o, page)
		more := widget.IconButton(c.Key("more"), icon.More, "More for the page", false)
		v.more.OpenBelow(more, "more", theme.ControlHeight)
		v.more.Show(c, more, "more", func(done func()) {
			if page != nil && widget.MenuItem(c, icon.Code, "DevTools", false) {
				page.ToggleDevTools()
				done()
			}
			if (page != nil || v.port != 0) && widget.MenuItem(c, icon.External, "Open in browser", false) {
				u := v.shown()
				if u == "" {
					u = v.origin + "/"
				}
				o.OpenURL(u)
				done()
			}
		})
	})
}

// navButton is a toolbar button, dimmed when it can't act; it reports a
// click when it can.
func navButton(c *ui.Context, svg *ui.SVG, label string, can bool) bool {
	b := widget.IconButton(c.Key(label), svg, label, false)
	if !can {
		b.Opacity(0.35)
		return false
	}
	return b.Clicked()
}

// addressField shows the page's path, which the user edits to go
// elsewhere on the port.
func (v *View) addressField(c *ui.Context, o Options) {
	if !v.editing {
		v.address = addressShown(v.shown())
		if v.home {
			v.address = "" // as a new tab's: what is typed goes in clean
		}
	}
	placeholder := "A URL, a host, or a path of the dev server"
	if v.port != 0 {
		placeholder = "localhost" + portLabel(v.port) + "/"
	}
	in := widget.Input(c.Key("address"), &v.address, placeholder).Height(28).Grow(1).Basis(0).MinWidth(80).Label("Address")
	submitted := in.Submitted()
	v.editing = in.Focused() && !submitted
	if submitted {
		v.navigate(o)
	}
}

// addressShown is a page's URL as the field reads: this machine's without its
// scheme and bare root, which target() puts back.
func addressShown(u string) string {
	rest, ok := strings.CutPrefix(u, "http://")
	if !ok || !strings.HasPrefix(rest, "localhost") && !strings.HasPrefix(rest, "127.0.0.1") {
		return u
	}
	return strings.TrimSuffix(rest, "/")
}

// sizeMenu picks the page's width, as a phone's, a tablet's or a desktop's.
func (v *View) sizeMenu(c *ui.Context) {
	label := widths[0].name
	for _, w := range widths {
		if w.width == v.width {
			label = w.name
		}
	}
	b := widget.Trigger(ui.ButtonBase(c.Key("size")), false).Width(116).Height(28).MinWidth(48).Shrink(1).Justify(ui.Start).
		Label("Page width: " + label).Tooltip("Page width")
	b.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, label, widget.RunStyle{}) })
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	v.sizes.OpenBelow(b, "sizes", 28)
	v.sizes.Show(c, b, "sizes", func(done func()) {
		for _, w := range widths {
			var svg *ui.SVG
			if w.width == v.width {
				svg = icon.Check
			}
			if widget.MenuItem(c, svg, w.name, false) {
				v.width = w.width
				done()
			}
		}
	})
}

// portPicker switches among the session's ports, when it has more than one.
func (v *View) portPicker(c *ui.Context, o Options) {
	b := widget.IconButton(c.Key("ports"), icon.ChevronsUD, "Ports", false)
	v.portsAt.OpenBelow(b, "ports", theme.ControlHeight)
	v.portsAt.Show(c, b, "ports", func(done func()) {
		for _, p := range v.ports {
			label := "localhost" + portLabel(p.Port)
			if p.Process != "" {
				label += " · " + p.Process
			}
			var svg *ui.SVG
			if p.Port == v.port && !v.external {
				svg = icon.Check
			}
			if widget.MenuItem(c.Key(p.Port), svg, label, false) {
				v.pick(o, p)
				done()
			}
		}
	})
}

// pick shows port p from its start.
func (v *View) pick(o Options, p proto.Port) {
	v.show(o, p, "/")
	if o.Remember != nil {
		o.Remember(p.Port, "/")
	}
}

// Show shows port p of the session, as the port offer and the Ports tool
// ask; it waits for the session's ports when they aren't read yet.
func (v *View) Show(o Options, p proto.Port) {
	v.started = true
	if !slices.ContainsFunc(v.ports, func(q proto.Port) bool { return q.Port == p.Port }) {
		v.ports = append(v.ports, p)
	}
	v.pick(o, p)
}

// pickButton turns design mode on: the next element clicked in the page
// goes to the agent, described and pictured; clicked again, or Escape in
// the page, it stops.
func (v *View) pickButton(c *ui.Context, o Options, page Page) {
	picker, ok := page.(Picker)
	if !ok || o.AttachElement == nil {
		return
	}
	tip := "Pick an element"
	if v.picking != nil {
		tip = "Stop picking"
	}
	b := widget.IconButton(c.Key("pick"), icon.Target, tip, v.picking != nil)
	if !b.Clicked() {
		return
	}
	if v.picking != nil {
		picker.StopPick()
		return // Pick returns nil, and the view settles
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	v.picking, v.err = cancel, ""
	go func() {
		picked, err := picker.Pick(ctx)
		var shot []byte
		if err == nil && picked != nil {
			if capture, cerr := page.Capture(); cerr == nil {
				shot, err = picked.Crop(capture)
			} else {
				err = cerr
			}
		}
		o.Update(func() {
			// Asked before cancel: its end is what says the time ran out.
			timedOut := ctx.Err() != nil
			cancel()
			v.picking = nil
			switch {
			case err != nil && !timedOut:
				v.err = "Couldn't pick the element: " + err.Error()
			case picked != nil:
				o.AttachElement(picked.Describe(), shot)
			}
		})
	}()
}

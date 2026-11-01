package browserview

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Draw shows the toolbar, then the page or why there is none.
func (v *View) Draw(c *ui.Context, o Options) {
	v.follow(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		v.toolbar(c, o)
		if v.err != "" {
			ui.Row(c).FillWidth().Padding(8, 14).Background(theme.Fade(theme.Error, 0.1)).Children(func() {
				ui.Text(c, v.err).FontSize(theme.TextLabel).TextColor(theme.Error).Shrink(1)
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
	ui.Column(c).Grow(1).FillWidth().Center().Padding(24).Gap(12).Children(func() {
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(theme.Muted).TextAlign(ui.Center)
		if !o.Supported && v.port != 0 && widget.Button(c, widget.Secondary, icon.External, "Open in browser", "").Clicked() {
			o.OpenURL(v.origin + "/")
		}
	})
}

// toolbar is back, forward, reload, the address, the port, and what to do
// with the page.
func (v *View) toolbar(c *ui.Context, o Options) {
	ui.Column(c).Shrink(0).FillWidth().Padding(6, 10, 6, 10).Gap(6).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c).FillWidth().Height(30).Gap(4).Children(func() {
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
			v.sizeMenu(c)
			if navButton(c, icon.Camera, "Screenshot to the agent", page != nil && o.Attach != nil) {
				if png, err := page.Capture(); err != nil {
					v.err = "Couldn't take a screenshot: " + err.Error()
				} else {
					o.Attach(png)
				}
			}
			if o.ToCanvas != nil && navButton(c, icon.Canvas, "Put this page on a canvas, as a design to compare others with", page != nil) {
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
			v.pickButton(c, o, page)
			v.unslopButton(c, o, page)
			if navButton(c, icon.Code, "DevTools", page != nil) {
				page.ToggleDevTools()
			}
			if navButton(c, icon.External, "Open in browser", page != nil || v.port != 0) {
				u := v.shown()
				if u == "" {
					u = v.origin + "/"
				}
				o.OpenURL(u)
			}
		})
		if len(v.ports) > 1 {
			v.portPicker(c, o)
		}
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
		v.address = v.shown()
		if v.home {
			v.address = "" // as a new tab's: what is typed goes in clean
		}
	}
	ui.Row(c).Grow(1).MinWidth(0).Height(28).PaddingX(10).Gap(4).Radius(theme.RadiusControl).
		Background(theme.Sunken).Border(1, theme.BorderStrong).Children(func() {
		placeholder := "A URL, a host, or a path of the dev server"
		if v.port != 0 {
			placeholder = "localhost" + portLabel(v.port) + "/"
		}
		in := ui.TextInputBase(c, &v.address).Grow(1).MinWidth(0).Font(theme.MonoFont).FontSize(theme.TextLabel).
			Label("Address").Placeholder(placeholder)
		submitted := in.Submitted()
		v.editing = in.Focused() && !submitted
		if submitted {
			v.navigate(o)
		}
	})
}

// sizeMenu picks the page's width, as a phone's, a tablet's or a laptop's.
func (v *View) sizeMenu(c *ui.Context) {
	label := "Fit"
	for _, w := range widths {
		if w.width == v.width && w.width != 0 {
			label = strconv.Itoa(w.width)
		}
	}
	b := ui.ButtonBase(c.Key("size")).Height(26).PaddingX(7).Gap(4).Radius(theme.RadiusControl).Shrink(0).
		Label("Page width: " + label).Tooltip("Page width").Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(theme.Hover)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
		ui.Icon(c, icon.ChevronDown).FontSize(10).TextColor(theme.Muted)
	})
	v.sizes.OpenBelow(b, "sizes", 26)
	v.sizes.Show(c, b, "sizes", func(done func()) {
		for _, w := range widths {
			svg := icon.Browser
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

// portPicker switches among the session's ports.
func (v *View) portPicker(c *ui.Context, o Options) {
	labels := make([]string, len(v.ports))
	cur := -1
	for i, p := range v.ports {
		labels[i] = portLabel(p.Port)
		if p.Process != "" {
			labels[i] += " " + p.Process
		}
		if p.Port == v.port {
			cur = i
		}
	}
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		ui.Text(c, strconv.Itoa(len(v.ports))+" ports").FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
		if i := widget.Segmented(c.Key("ports"), labels, cur, nil); i >= 0 {
			v.pick(o, v.ports[i])
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
	tip := "Pick an element for the agent"
	if v.picking != nil {
		tip = "Stop picking"
	}
	b := widget.IconButton(c, icon.Target, tip, false)
	if v.picking != nil {
		b.Background(theme.Selected).TextColor(theme.Accent)
	}
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
			cancel()
			v.picking = nil
			switch {
			case err != nil && ctx.Err() == nil:
				v.err = "Couldn't pick the element: " + err.Error()
			case picked != nil:
				o.AttachElement(picked.Describe(), shot)
			}
		})
	}()
}

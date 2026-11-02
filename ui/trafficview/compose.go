package trafficview

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// composeForm is a request written by hand.
type composeForm struct {
	method  string
	url     string
	headers string
	body    string
	direct  bool
	open    bool
	sending bool
	err     string
	history []composeForm // the last ones sent, newest first
}

const composeHistory = 20

var methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// fill fills the form from a request caught, to change and send again.
func (f *composeForm) fill(ex proto.TrafficExchange, req proto.TrafficBody) {
	f.method, f.url, f.err = ex.Method, ex.URL, ""
	var hs []proto.Header
	for _, h := range ex.ReqHeaders {
		switch strings.ToLower(h.Name) {
		case "content-length", "host", "accept-encoding":
			continue
		}
		hs = append(hs, h)
	}
	f.headers = headerLines(hs)
	f.body = ""
	if utf8.Valid(req.Data) {
		f.body = string(req.Data)
	}
}

// composeSheet is a request written by hand, or from one caught, as a
// dialog over the tool.
func (v *View) composeSheet(c *ui.Context, o Options) {
	f := &v.compose
	if !f.open {
		return
	}
	if f.method == "" {
		f.method = "GET"
	}
	sheet(c, &f.open, 600, "Send a request", "Sent from "+o.HostName+" and recorded with the rest.", func() {
		ui.Column(c).FillWidth().Gap(theme.S3).Children(func() {
			i := slices.Index(methods, f.method)
			ui.Row(c).FillWidth().Children(func() {
				if k := widget.Segmented(c, methods, i, nil); k >= 0 {
					f.method = methods[k]
				}
			})
			widget.Field(c, "URL", func() { widget.Input(c, &f.url, "https://api.example.com/v1/users").FillWidth() })
			codeArea(c, "Headers (Name: value per line)", &f.headers, 60)
			codeArea(c, "Body", &f.body, 110)
			check(c, &f.direct, "Around the rules (no mocks or breakpoints)")
			errorText(c, f.err)
		})
		if len(f.history) > 0 {
			ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
				widget.SectionLabel(c, "Sent before", theme.TextSoft)
				for i, h := range f.history {
					b := widget.ItemRow(c.Key(fmt.Sprint(i)), widget.Item{Title: h.url, Sub: h.method, Mono: true, Label: h.method + " " + h.url}, nil)
					if b.Clicked() {
						hist := f.history
						*f = h
						f.history, f.open = hist, true
					}
				}
			})
		}
	}, func() {
		label := "Send"
		if f.sending {
			label = "Sending…"
		}
		if widget.PrimaryButton(c, nil, label, "").Disabled(f.sending || strings.TrimSpace(f.url) == "").Clicked() {
			v.send(o)
		}
	})
}

func (v *View) send(o Options) {
	f := &v.compose
	p := proto.TrafficSendParams{Method: f.method, URL: strings.TrimSpace(f.url), Headers: parseHeaderLines(f.headers), Direct: f.direct,
		Project: o.Session.Project, Session: o.Session.ID}
	if f.body != "" {
		p.Body = []byte(f.body)
	}
	f.sending, f.err = true, ""
	sent := *f
	sent.history, sent.sending, sent.open = nil, false, false
	f.history = append([]composeForm{sent}, f.history...)
	if len(f.history) > composeHistory {
		f.history = f.history[:composeHistory]
	}
	h := o.Host
	go func() {
		sum, err := h.TrafficSend(p)
		o.Update(func() {
			f.sending = false
			if err != nil {
				f.err = client.Explain(err)
				return
			}
			v.Apply(sum)
			f.open = false
			v.page, v.all, v.filter = pageRequests, true, ""
			v.open(o, sum)
		})
	}()
}

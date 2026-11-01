package trafficview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/intercept"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// bodyShown bounds a body read for the viewer; Save keeps all of it.
const (
	bodyShown    = 1 << 20
	highlightMax = 200 << 10
	hexShown     = 4 << 10
)

// detail is the exchange picked, as read from the host.
type detail struct {
	id      int64
	state   string
	stale   bool // it finished since it was read
	loading bool
	err     string
	ex      proto.TrafficExchange
	req     proto.TrafficBody
	res     proto.TrafficBody
	msgs    []proto.TrafficMessage
	tab     int
	reveal  bool
	images  map[string]*ui.Bitmap
	menu    widget.ContextMenu
}

const (
	tabRequest = iota
	tabResponse
	tabMessages
	tabTiming
)

// open reads exchange id for the detail below the list.
func (v *View) open(o Options, id int64) {
	d := &detail{id: id, tab: tabResponse, images: map[string]*ui.Bitmap{}}
	if v.detail != nil && v.detail.id != id {
		d.tab, d.reveal = v.detail.tab, v.detail.reveal
	} else if v.detail != nil {
		d = v.detail
	}
	v.detail = d
	v.read(o, d)
}

func (v *View) read(o Options, d *detail) {
	if d.loading || o.Host == nil {
		return
	}
	d.loading, d.stale = true, false
	h, id, reveal := o.Host, d.id, d.reveal
	go func() {
		ex, err := h.TrafficGet(id, reveal)
		var req, res proto.TrafficBody
		var msgs []proto.TrafficMessage
		if err == nil {
			req, _ = h.TrafficBody(proto.TrafficBodyParams{ID: id, Which: "request", Size: bodyShown, Reveal: reveal})
			res, _ = h.TrafficBody(proto.TrafficBodyParams{ID: id, Which: "response", Size: bodyShown, Reveal: reveal})
			if ex.Messages > 0 {
				msgs, _ = h.TrafficMessages(proto.TrafficMessagesParams{ID: id, Limit: 2000})
			}
		}
		o.Update(func() {
			d.loading = false
			if err != nil {
				d.err = client.Explain(err)
				return
			}
			d.err, d.ex, d.req, d.res, d.msgs, d.state = "", ex, req, res, msgs, ex.State
			if ex.Messages > 0 && d.tab == tabResponse && len(res.Data) == 0 {
				d.tab = tabMessages
			}
		})
	}()
}

func (v *View) detailView(c *ui.Context, o Options) {
	d := v.detail
	if d.stale {
		v.read(o, d)
	}
	ui.Column(c).Grow(1.2).FillWidth().MinHeight(160).BorderWidth(1, 0, 0, 0).BorderColor(theme.BorderStrong).Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Padding(8, 8, 6, 12).Gap(6).Children(func() {
			ex := d.ex
			ui.Text(c, ex.Method).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Shrink(0)
			ui.Text(c, ex.URL).FontSize(theme.TextLabel).SingleLine().Grow(1).Shrink(1).MinWidth(0).Tooltip(ex.URL)
			if widget.IconButton(c, icon.Close, "Close", false).Clicked() {
				v.detail, v.selected = nil, -1
			}
		})
		if d.err != "" {
			note(c, d.err, theme.Error)
			return
		}
		if d.ex.ID == 0 {
			note(c, "Reading…", theme.Muted)
			return
		}
		v.detailActions(c, o, d)
		tabs := []string{"Request", "Response"}
		if d.ex.Messages > 0 {
			tabs = append(tabs, fmt.Sprintf("Messages (%d)", d.ex.Messages))
		} else {
			tabs = append(tabs, "Messages")
		}
		tabs = append(tabs, "Timing")
		ui.Row(c).FillWidth().Shrink(0).Padding(4, 12).Children(func() {
			if i := widget.Segmented(c, tabs, d.tab, func(i int) bool { return i == tabMessages && d.ex.Messages == 0 }); i >= 0 {
				d.tab = i
			}
		})
		ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(6, 12, 12, 12).Gap(10).Children(func() {
				switch d.tab {
				case tabRequest:
					headersView(c, d.ex.ReqHeaders)
					bodyView(c, d, "request", d.req)
				case tabResponse:
					if d.ex.Error != "" {
						ui.Text(c, d.ex.Error).FontSize(theme.TextLabel).TextColor(theme.Error)
					}
					if d.ex.Status != 0 {
						ui.Text(c, fmt.Sprintf("%d %s", d.ex.Status, d.ex.Proto)).Font(theme.MonoFont).FontSize(theme.TextLabel)
					}
					headersView(c, d.ex.ResHeaders)
					bodyView(c, d, "response", d.res)
					if len(d.ex.Trailers) > 0 {
						ui.Text(c, "Trailers").FontSize(theme.TextLabel).TextColor(theme.Muted)
						headersView(c, d.ex.Trailers)
					}
				case tabMessages:
					messagesView(c, d.msgs)
				case tabTiming:
					timingView(c, d.ex)
				}
			})
		})
	})
}

// detailActions are what can be done with the exchange.
func (v *View) detailActions(c *ui.Context, o Options, d *detail) {
	ex := d.ex
	ui.Row(c).FillWidth().Shrink(0).Padding(0, 10, 4, 12).Gap(6).Children(func() {
		if widget.Button(c, widget.Secondary, icon.Plus, "Mock this", "").Tooltip("Answer requests like this from a mock").Clicked() {
			v.rules.startNew(ruleFromExchange(ex, d.res, proto.ActionMock), o)
		}
		if widget.Button(c, widget.Secondary, icon.Send, "Edit and send", "").Tooltip("Change this request and send it again").Clicked() {
			v.compose.fill(ex, d.req)
			v.compose.open = true
		}
		if o.SendToAgent != nil && widget.Button(c, widget.Secondary, icon.Chat, "To the agent", "").Tooltip("Send this exchange to the session's agent, secrets masked").Clicked() {
			o.SendToAgent(exchangeText(ex, d.req, d.res))
			v.notice = "Sent to the agent, secrets masked."
		}
		ui.Spacer(c)
		ui.Text(c, "Secrets").FontSize(theme.TextLabel).TextColor(theme.Muted).Shrink(0)
		if widget.Switch(c, &d.reveal, "Show secrets") {
			v.read(o, d)
		}
		more := widget.IconButton(c.Key("more:exchange"), icon.More, "More for this request", false)
		d.menu.OpenBelow(more, "exchange", 24)
		d.menu.Show(c, more, "exchange", func(done func()) {
			if widget.MenuItemTip(c, icon.Stop, "Breakpoint…", "Pause requests like this one, to change them before they go", false) {
				v.rules.startNew(ruleFromExchange(ex, d.res, proto.ActionBreakpoint), o)
				done()
			}
			pin := "Pin (kept when cleared)"
			if ex.Pinned {
				pin = "Unpin"
			}
			if widget.MenuItem(c, icon.Target, pin, false) {
				id, pinned := ex.ID, !ex.Pinned
				run(o, func() error { return o.Host.TrafficPin([]int64{id}, pinned) }, func(error) {})
				d.ex.Pinned = pinned
				done()
			}
			widget.MenuSeparator(c)
			widget.PopupLabel(c, "Copy as")
			for _, lang := range intercept.SnippetLanguages {
				if widget.MenuItem(c, icon.Copy, lang, false) {
					code, err := intercept.Snippet(lang, ex.Method, ex.URL, ex.ReqHeaders, d.req.Data)
					if err == nil && o.CopyText != nil {
						o.CopyText(code)
						v.notice = "Copied as " + lang + "."
					}
					done()
				}
			}
			widget.MenuSeparator(c)
			if o.OpenInBrowser != nil && ex.Method == "GET" && widget.MenuItemTip(c, icon.Browser, "Open in Browser", "Open the URL in the Browser tool", false) {
				o.OpenInBrowser(ex.URL)
				done()
			}
			if widget.MenuItem(c, icon.Copy, "Copy URL", false) {
				if o.CopyText != nil {
					o.CopyText(ex.URL)
				}
				done()
			}
			if widget.MenuItem(c, icon.Copy, "Copy response body", false) {
				if o.CopyText != nil {
					o.CopyText(string(d.res.Data))
				}
				done()
			}
			if widget.MenuItem(c, icon.ArrowUp, "Export as HAR…", false) {
				v.export(o, []int64{ex.ID})
				done()
			}
		})
	})
}

func headersView(c *ui.Context, hs []proto.Header) {
	if len(hs) == 0 {
		return
	}
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		for i, h := range hs {
			ui.Row(c.Key(fmt.Sprint(i))).FillWidth().Gap(8).AlignItems(ui.Start).Children(func() {
				ui.Text(c, h.Name).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).Width(150).Shrink(0).SingleLine().Tooltip(h.Name)
				ui.Text(c, h.Value).Font(theme.MonoFont).FontSize(theme.TextLabel).Grow(1).Shrink(1).MinWidth(0).Selectable()
			})
		}
	})
}

// bodyView shows a body by its kind: JSON pretty, code highlighted,
// images, forms as fields, other binaries as hex.
func bodyView(c *ui.Context, d *detail, which string, b proto.TrafficBody) {
	if b.Total == 0 {
		if b.ContentType != "" {
			ui.Text(c, "No body").FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		return
	}
	kind := kindOf(b.ContentType)
	ui.Row(c).Gap(6).Children(func() {
		ui.Text(c, strings.TrimSpace(b.ContentType+" · "+textfmt.Bytes(uint64(b.Total)))).FontSize(theme.TextLabel).TextColor(theme.Muted)
		if b.Encoding != "" {
			widget.Tag(c, b.Encoding+" (not decoded)", theme.Warning)
		}
		if b.Truncated {
			widget.Tag(c, "only the first part was kept", theme.Warning)
		}
	})
	data := b.Data
	switch {
	case kind == "image":
		bm, ok := d.images[which]
		if !ok {
			bm, _ = ui.DecodeBitmap(data)
			d.images[which] = bm
		}
		if bm != nil {
			ui.Image(c, bm).MaxWidth(360).MaxHeight(360)
			return
		}
	case kind == "form":
		if q, err := url.ParseQuery(string(data)); err == nil {
			var hs []proto.Header
			for k, vs := range q {
				for _, x := range vs {
					hs = append(hs, proto.Header{Name: k, Value: x})
				}
			}
			headersView(c, hs)
			return
		}
	}
	if strings.Contains(b.ContentType, "protobuf") {
		if pb, ok := intercept.DecodeProtobuf(data); ok {
			ui.Text(c, pb).Font(theme.MonoFont).FontSize(theme.TextLabel).Selectable()
			return
		}
	}
	if !utf8.Valid(data) {
		ui.Text(c, hexDump(data)).Font(theme.MonoFont).FontSize(theme.TextLabel).Selectable()
		return
	}
	text, lang := string(data), ""
	switch kind {
	case "json":
		var buf bytes.Buffer
		if json.Indent(&buf, data, "", "  ") == nil {
			text = buf.String()
		}
		lang = "json"
	case "html":
		lang = "html"
	case "xml":
		lang = "xml"
	case "js":
		lang = "javascript"
	case "css":
		lang = "css"
	case "graphql":
		lang = "graphql"
	}
	if lang != "" && len(text) <= highlightMax {
		widget.CodeBlock(c, lang, text)
		return
	}
	ui.Text(c, text).Font(theme.MonoFont).FontSize(theme.TextLabel).Selectable()
	if b.Total > int64(len(data)) {
		ui.Text(c, "… "+textfmt.Bytes(uint64(b.Total-int64(len(data))))+" more: export it to see all").FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
}

func hexDump(b []byte) string {
	more := len(b) > hexShown
	if more {
		b = b[:hexShown]
	}
	var s strings.Builder
	for i := 0; i < len(b); i += 16 {
		end := min(i+16, len(b))
		fmt.Fprintf(&s, "%06x  % x", i, b[i:end])
		s.WriteString(strings.Repeat("   ", 16-(end-i)))
		s.WriteString("  ")
		for _, ch := range b[i:end] {
			if ch < 32 || ch > 126 {
				ch = '.'
			}
			s.WriteByte(ch)
		}
		s.WriteByte('\n')
	}
	if more {
		s.WriteString("…")
	}
	return s.String()
}

func messagesView(c *ui.Context, msgs []proto.TrafficMessage) {
	if len(msgs) == 0 {
		ui.Text(c, "No messages yet.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		return
	}
	for i, m := range msgs {
		ui.Row(c.Key(fmt.Sprint(i))).FillWidth().Gap(8).AlignItems(ui.Start).Children(func() {
			dir, col := "↓", theme.Done
			if m.Sent {
				dir, col = "↑", theme.Accent
			}
			ui.Text(c, dir).FontSize(theme.TextLabel).TextColor(col).Width(12).Shrink(0)
			ui.Text(c, m.Time.Format("15:04:05.000")).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).Shrink(0)
			label := m.Kind
			if m.Event != "" {
				label = m.Event
			}
			if m.Mocked {
				label += " · mock"
			}
			ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.Muted).Width(70).Shrink(0).SingleLine()
			text := messageText(m)
			ui.Text(c, text).Font(theme.MonoFont).FontSize(theme.TextLabel).Grow(1).Shrink(1).MinWidth(0).Selectable()
		})
	}
}

func timingView(c *ui.Context, ex proto.TrafficExchange) {
	t := ex.Timings
	rows := []struct {
		label string
		ms    int64
	}{{"Connected", t.Connect}, {"TLS done", t.TLS}, {"Request sent", t.Sent}, {"First byte", t.FirstByte}, {"Done", t.Done}}
	total := max(t.Done, ex.Duration, 1)
	for i, r := range rows {
		if r.ms == 0 {
			continue
		}
		ui.Row(c.Key(fmt.Sprint(i))).FillWidth().Gap(8).Children(func() {
			ui.Text(c, r.label).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Width(100).Shrink(0)
			ui.Row(c).Grow(1).Height(6).Radius(theme.RadiusTail).Background(theme.Sunken).Children(func() {
				ui.Box(c).Height(6).Radius(theme.RadiusTail).Background(theme.Accent).Width(float32(r.ms) / float32(total) * 220)
			})
			ui.Text(c, fmt.Sprintf("%d ms", r.ms)).Font(theme.MonoFont).FontSize(theme.TextLabel).Width(64).Shrink(0)
		})
	}
	if ex.TLS != nil {
		ui.Text(c, fmt.Sprintf("Client TLS: %s, %s, %s", ex.TLS.Version, ex.TLS.Cipher, ex.TLS.ALPN)).FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
	if ex.Remote != "" {
		ui.Text(c, "Server: "+ex.Remote).FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
	if ex.Rule != "" {
		ui.Text(c, "Rule: "+ex.Rule).FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
}

// exchangeText is an exchange for an agent to read, secrets masked.
func exchangeText(ex proto.TrafficExchange, req, res proto.TrafficBody) string {
	var s strings.Builder
	fmt.Fprintf(&s, "This HTTP exchange was caught by enowX (secrets masked):\n\n%s %s %s\n", ex.Method, intercept.MaskURL(ex.URL), ex.Proto)
	for _, h := range intercept.MaskHeaders(ex.ReqHeaders) {
		fmt.Fprintf(&s, "%s: %s\n", h.Name, h.Value)
	}
	if body := textOf(req, 8<<10); body != "" {
		s.WriteString("\n" + body + "\n")
	}
	if ex.Error != "" {
		fmt.Fprintf(&s, "\nIt failed: %s\n", ex.Error)
	} else {
		fmt.Fprintf(&s, "\nResponse %d\n", ex.Status)
		for _, h := range intercept.MaskHeaders(ex.ResHeaders) {
			fmt.Fprintf(&s, "%s: %s\n", h.Name, h.Value)
		}
		if body := textOf(res, 16<<10); body != "" {
			s.WriteString("\n" + body + "\n")
		}
	}
	return s.String()
}

func textOf(b proto.TrafficBody, limit int) string {
	if len(b.Data) == 0 || !utf8.Valid(b.Data) {
		return ""
	}
	data := intercept.MaskBody(kindOf(b.ContentType), b.Data)
	if len(data) > limit {
		return string(data[:limit]) + "\n… (cut)"
	}
	return string(data)
}

// kindOf is a body's kind by its content type, as the host names them.
func kindOf(ctype string) string { return intercept.KindOf(ctype) }

// messageText is a message as text: gRPC's decoded as protobuf, text as
// it is, other bytes by their size.
func messageText(m proto.TrafficMessage) string {
	if m.Kind == "message" {
		if pb, ok := intercept.DecodeProtobuf(m.Data); ok {
			return strings.TrimRight(pb, "\n")
		}
	}
	if utf8.Valid(m.Data) {
		return string(m.Data)
	}
	if pb, ok := intercept.DecodeProtobuf(m.Data); ok {
		return strings.TrimRight(pb, "\n")
	}
	return fmt.Sprintf("(%s binary)", textfmt.Bytes(uint64(m.Size)))
}

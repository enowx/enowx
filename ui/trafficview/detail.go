package trafficview

import (
	"fmt"
	"net/url"
	"slices"
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

// bodyShown is how much of a body is read for the viewer at first, and bodyMost the most
// Show more reads: past it, Save keeps all of it.
const (
	bodyShown = 1 << 20
	bodyMost  = 16 << 20
	hexShown  = 4 << 10
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
	reqText widget.LongText
	resText widget.LongText
	msgList ui.ListState
	shown   int // how much of its bodies to read
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

// detailOf is exchange id as read for its viewer, read the first time; a new
// viewer opens the way the last one was left.
func (v *View) detailOf(o Options, id int64) *detail {
	d := v.details[id]
	if d != nil {
		return d
	}
	d = &detail{id: id, tab: tabResponse, images: map[string]*ui.Bitmap{}, shown: bodyShown}
	if v.lastRead != nil {
		d.tab, d.reveal = v.lastRead.tab, v.lastRead.reveal
	}
	v.details[id], v.lastRead = d, d
	v.read(o, d)
	return d
}

// ForgetExchange drops what was read for exchange id, whose viewer closed.
func (v *View) ForgetExchange(id int64) {
	if v.lastRead != nil && v.lastRead.id == id {
		last := *v.lastRead
		v.lastRead = &detail{tab: last.tab, reveal: last.reveal}
	}
	delete(v.details, id)
}

// ExchangeColor is the color of how exchange id went, for its tab's dot.
func (v *View) ExchangeColor(id int64) ui.Color {
	if i, ok := v.index[id]; ok {
		_, col := statusText(v.list[i])
		return col
	}
	return theme.Idle
}

func (v *View) read(o Options, d *detail) {
	if d.loading || o.Host == nil {
		return
	}
	d.loading, d.stale = true, false
	h, id, reveal, shown := o.Host, d.id, d.reveal, max(d.shown, bodyShown)
	go func() {
		ex, err := h.TrafficGet(id, reveal)
		var req, res proto.TrafficBody
		var msgs []proto.TrafficMessage
		if err == nil {
			req, _ = h.TrafficBody(proto.TrafficBodyParams{ID: id, Which: "request", Size: shown, Reveal: reveal})
			res, _ = h.TrafficBody(proto.TrafficBodyParams{ID: id, Which: "response", Size: shown, Reveal: reveal})
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

// DrawExchange is the viewer of exchange id, in the center: a bar with where it
// went and how it went, its tabs, and what can be done with it.
func (v *View) DrawExchange(c *ui.Context, o Options, id int64) {
	v.load(o)
	d := v.detailOf(o, id)
	if d.stale {
		v.read(o, d)
	}
	title, facts := d.ex.URL, ""
	if i, ok := v.index[id]; ok {
		title, facts = hostPath(v.list[i]), exchangeFacts(v.list[i])
	}
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		widget.ViewerBar(c, icon.Traffic, title, func() {
			if facts != "" {
				ui.Text(c, facts).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
			}
		})
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(theme.S3, theme.S4).Gap(theme.S2).Children(func() {
			v.noticeBanner(c)
			switch {
			case d.err != "":
				note(c, d.err, theme.Error)
				return
			case d.ex.ID == 0:
				note(c, "Reading…", theme.TextSoft)
				return
			}
			tabs := []string{"Request", "Response", "Messages", "Timing"}
			if d.ex.Messages > 0 {
				tabs[tabMessages] = fmt.Sprintf("Messages (%d)", d.ex.Messages)
			}
			ui.Row(c).FillWidth().Shrink(0).Children(func() {
				if i := widget.Segmented(c, tabs, d.tab, func(i int) bool { return i == tabMessages && d.ex.Messages == 0 }); i >= 0 {
					d.tab = i
				}
			})
			switch d.tab {
			case tabRequest:
				v.bodyPane(c, o, d, "request", d.req, &d.reqText, func() {
					codeText(c, d.ex.Method+" "+d.ex.URL+"\n"+headerText(d.ex.ReqHeaders))
				}, nil)
			case tabResponse:
				v.bodyPane(c, o, d, "response", d.res, &d.resText, func() { responseHead(c, d) }, func() {
					if len(d.ex.Trailers) > 0 {
						ui.Text(c.Key("trailers"), "Trailers").FontSize(theme.TextLabel).TextColor(widget.CodeDim)
						codeText(c.Key("trailer-text"), headerText(d.ex.Trailers))
					}
				})
			case tabMessages:
				messagesView(c, d)
			case tabTiming:
				timingView(c, d.ex)
			}
			v.detailActions(c, o, d)
		})
	})
	if !o.ToolShown {
		v.sheets(c, o)
	}
}

// responseHead is how an exchange was answered: its error, status, protocol and headers.
func responseHead(c *ui.Context, d *detail) {
	if d.ex.Error != "" {
		codeText(c, d.ex.Error).TextColor(widget.CodeError)
	}
	var head []string
	if d.ex.Status != 0 {
		head = append(head, fmt.Sprintf("%s %d", d.ex.Proto, d.ex.Status))
	}
	if h := headerText(d.ex.ResHeaders); h != "" {
		head = append(head, h)
	}
	if len(head) > 0 {
		codeText(c, strings.Join(head, "\n"))
	}
}

// codeText is text on the code's ground, to select.
func codeText(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).Font(theme.MonoFont).FontSize(theme.TextUI).Selectable()
}

// detailActions are what can be done with the exchange.
func (v *View) detailActions(c *ui.Context, o Options, d *detail) {
	ex := d.ex
	ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Wrap().Gap(theme.S2).Children(func() {
			if widget.Button(c, widget.Secondary, nil, "Mock", "").Tooltip("Answer requests like this from a mock").Clicked() {
				v.rules.startNew(ruleFromExchange(ex, d.res, proto.ActionMock), o)
			}
			if widget.Button(c, widget.Secondary, nil, "Replay", "").Tooltip("Change this request and send it again").Clicked() {
				v.compose.fill(ex, d.req)
				v.compose.open = true
			}
			if slices.ContainsFunc(v.paused, func(p proto.TrafficPaused) bool { return p.ID == ex.ID }) &&
				widget.Button(c, widget.Secondary, nil, "Resume", "").Tooltip("Let it go on as it is").Clicked() {
				v.resume(o, proto.TrafficResumeParams{ID: ex.ID})
			}
			if o.SendToAgent != nil && widget.Button(c, widget.Secondary, nil, "To the agent", "").Tooltip("Send this exchange to the session's agent, secrets masked").Clicked() {
				o.SendToAgent(exchangeText(ex, d.req, d.res))
				v.notice = "Sent to the agent, secrets masked."
			}
		})
		ui.Row(c).Shrink(0).Gap(theme.S2).Children(func() {
			ui.Text(c, "Secrets").FontSize(theme.TextLabel).TextColor(theme.TextSoft).Shrink(0)
			if widget.Switch(c, &d.reveal, "Show secrets") {
				v.read(o, d)
			}
			v.exchangeMenu(c, o, d)
		})
	})
}

// exchangeMenu is the rest of what can be done with the exchange.
func (v *View) exchangeMenu(c *ui.Context, o Options, d *detail) {
	ex := d.ex
	more := widget.IconButton(c.Key("more:exchange"), icon.More, "More for this request", false)
	d.menu.OpenBelow(more, "exchange", theme.ControlHeight)
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
}

// headerText is headers as a request or a response writes them.
func headerText(hs []proto.Header) string {
	lines := make([]string, len(hs))
	for i, h := range hs {
		lines[i] = h.Name + ": " + h.Value
	}
	return strings.Join(lines, "\n")
}

// bodyPane is a body in its well: what comes before it scrolls with it, and all that's
// slow about it happens once, off the main thread, in its LongText.
func (v *View) bodyPane(c *ui.Context, o Options, d *detail, which string, b proto.TrafficBody, long *widget.LongText, head, foot func()) {
	long.Load(widget.LongSource{ID: fmt.Sprint(d.id, which, len(b.Data), b.Total, d.reveal), Size: len(b.Data),
		Make: func() (string, string) { return bodyText(b) }}, o.Update)
	if len(b.Data) > bodyFindFrom {
		ui.Row(c.Key("find")).FillWidth().Shrink(0).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			long.FindBar(c, o.Update)
			widget.CopyIcon(c.Key("copy"), &long.Copied, icon.Copy, "Copy the body", "Copy all of what's shown", long.Text())
		})
	}
	widget.CodeFrame(c, func() {
		long.Draw(c.Key(which), widget.LongOptions{Post: o.Update, OnCode: true,
			Head: func() {
				head()
				bodyHead(c, d, which, b)
			},
			Foot: func() {
				if b.Total > int64(len(b.Data)) {
					ui.Row(c.Key("more")).FillWidth().Wrap().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "… "+textfmt.Bytes(uint64(b.Total-int64(len(b.Data))))+" more: export it to see all").FontSize(theme.TextLabel).TextColor(widget.CodeDim)
						if d.shown < bodyMost && !d.loading && widget.Button(c.Key("show-more"), widget.Secondary, nil, "Show more", "").Height(26).Clicked() {
							d.shown = min(max(d.shown, bodyShown)*4, bodyMost)
							v.read(o, d)
						}
					})
				}
				if foot != nil {
					foot()
				}
			}})
	})
}

// bodyFindFrom is how long a body is to be worth a search and a Copy of its own.
const bodyFindFrom = 4 << 10

// bodyHead says what a body is: its type and size, what's odd about it, and its picture.
func bodyHead(c *ui.Context, d *detail, which string, b proto.TrafficBody) {
	if b.Total == 0 {
		if b.ContentType != "" {
			ui.Text(c.Key("none"), "No body").FontSize(theme.TextLabel).TextColor(widget.CodeDim)
		}
		return
	}
	ui.Row(c.Key("what")).FillWidth().Wrap().Gap(theme.S2).Children(func() {
		ui.Text(c, strings.TrimSpace(b.ContentType+" · "+textfmt.Bytes(uint64(b.Total)))).FontSize(theme.TextLabel).TextColor(widget.CodeDim)
		if b.Encoding != "" {
			widget.Tag(c, b.Encoding+" (not decoded)", widget.CodeWarn)
		}
		if b.Truncated {
			widget.Tag(c, "only the first part was kept", widget.CodeWarn)
		}
	})
	if kindOf(b.ContentType) != "image" {
		return
	}
	bm, ok := d.images[which]
	if !ok {
		bm, _ = ui.DecodeBitmap(b.Data)
		d.images[which] = bm
	}
	if bm != nil {
		ui.Image(c.Key("picture"), bm).MaxWidth(360).MaxHeight(360)
	}
}

// bodyText is a body as text to read, and the language it's in. It runs off the main thread
// for a long one.
func bodyText(b proto.TrafficBody) (text, lang string) {
	data := b.Data
	kind := kindOf(b.ContentType)
	switch {
	case b.Total == 0:
		return "", ""
	case kind == "image":
		if bm, _ := ui.DecodeBitmap(data); bm != nil {
			return "", ""
		}
	case kind == "form":
		if q, err := url.ParseQuery(string(data)); err == nil {
			var hs []proto.Header
			for k, vs := range q {
				for _, x := range vs {
					hs = append(hs, proto.Header{Name: k, Value: x})
				}
			}
			return headerText(hs), ""
		}
	}
	if strings.Contains(b.ContentType, "protobuf") {
		if pb, ok := intercept.DecodeProtobuf(data); ok {
			return pb, ""
		}
	}
	// What was read may end partway through a character, which isn't binary.
	whole := data
	for i := 0; i < utf8.UTFMax && len(whole) > 0 && !utf8.Valid(whole); i++ {
		whole = whole[:len(whole)-1]
	}
	if !utf8.Valid(whole) {
		return hexDump(data), ""
	}
	switch kind {
	case "json":
		return widget.IndentJSON(whole), "json"
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
	return string(whole), lang
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

func messagesView(c *ui.Context, d *detail) {
	msgs := d.msgs
	if len(msgs) == 0 {
		note(c, "No messages yet.", theme.TextSoft)
		return
	}
	widget.CodeFrame(c, func() {
		ui.List(c.Key("messages"), &d.msgList, len(msgs), func(i int) {
			m := msgs[i]
			ui.Row(c).FillWidth().Gap(theme.S2).Padding(0, 0, theme.S2, 0).AlignItems(ui.Start).Children(func() {
				dir, col := "↓", widget.CodeText
				if m.Sent {
					dir, col = "↑", widget.CodeDim
				}
				ui.Text(c, dir).FontSize(theme.TextUI).TextColor(col).Width(12).Shrink(0)
				ui.Text(c, m.Time.Format("15:04:05.000")).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(widget.CodeDim).Shrink(0)
				label := m.Kind
				if m.Event != "" {
					label = m.Event
				}
				if m.Mocked {
					label += " · mock"
				}
				ui.Row(c).Width(72).Shrink(0).Children(func() {
					widget.Running(c, label, widget.RunStyle{Size: theme.TextLabel, Color: widget.CodeDim})
				})
				codeText(c, widget.LongWords(messageText(m))).Grow(1).Basis(0).MinWidth(0)
			})
		}).Grow(1).FillWidth().MinHeight(0).Padding(0, widget.ScrollGutter, 0, 0)
	})
}

func timingView(c *ui.Context, ex proto.TrafficExchange) {
	t := ex.Timings
	rows := []struct {
		label string
		ms    int64
	}{{"Connect", t.Connect}, {"TLS", t.TLS}, {"Sent", t.Sent}, {"First byte", t.FirstByte}, {"Total", t.Done}}
	widget.Scroll(c.Key("timing")).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Gap(theme.S1).Padding(theme.S2, 0).Children(func() {
			for _, r := range rows {
				if r.ms != 0 {
					widget.KV(c, r.label, millis(r.ms), false)
				}
			}
			if ex.TLS != nil {
				widget.KV(c, "Client TLS", fmt.Sprintf("%s, %s, %s", ex.TLS.Version, ex.TLS.Cipher, ex.TLS.ALPN), false)
			}
			if ex.Remote != "" {
				widget.KV(c, "Server", ex.Remote, false)
			}
			if ex.Rule != "" {
				widget.KV(c, "Rule", ex.Rule, false)
			}
		})
	})
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

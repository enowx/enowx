package trafficview

import (
	"bytes"
	"image"
	pngenc "image/png"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/wrapcheck"
)

// sampleExchanges cover how an exchange can stand in the list.
func sampleExchanges() []proto.TrafficSummary {
	at := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	row := func(id int64, state string, status int, method, host, path string, ms int64, session string) proto.TrafficSummary {
		return proto.TrafficSummary{ID: id, State: state, Status: status, Method: method, Host: host, Path: path, URL: "https://" + host + path,
			Duration: ms, Session: session, Kind: proto.TrafficHTTP, Started: at.Add(time.Duration(id) * time.Minute)}
	}
	mock := row(6, proto.TrafficDone, 200, "GET", "api.example.com", "/mocked", 1, "s1")
	mock.Mocked = true
	rule := row(7, proto.TrafficDone, 200, "GET", "api.example.com", "/ruled", 12, "s1")
	rule.Rule = "r1"
	ws := row(8, proto.TrafficDone, 101, "GET", "stream.example.com", "/socket/a/rather/long/path/that/runs/in/its/frame/of/the/list", 4000, "s1")
	ws.Kind = "websocket"
	return []proto.TrafficSummary{
		row(1, proto.TrafficDone, 200, "GET", "api.github.com", "/repos/enowx/enowx", 142, "s1"),
		row(2, proto.TrafficDone, 500, "POST", "api.anthropic.com", "/v1/messages", 2100, "s1"),
		row(3, proto.TrafficDone, 404, "GET", "registry.npmjs.org", "/@enowx/x", 88, "s1"),
		row(4, proto.TrafficHeld, 0, "POST", "api.voyageai.com", "/v1/embeddings", 0, "s1"),
		row(5, proto.TrafficFailed, 0, "GET", "localhost:5173", "/assets/app.js", 9, "s1"),
		mock, rule, ws,
		row(9, proto.TrafficPending, 0, "GET", "api.github.com", "/user", 0, "s1"),
		row(10, proto.TrafficDone, 302, "GET", "github.com", "/login", 31, "s1"),
	}
}

// statesView is a tool as the host would have left it, reading nothing: with no
// Host, what the state says is all it draws.
func statesView() *View {
	v := New()
	v.loaded, v.capture.loaded = true, true
	v.capture.st = proto.TrafficCapture{State: proto.CaptureRecording}
	v.rules.loaded, v.rules.project, v.settings.loaded = true, "/work/enowx", true
	for _, s := range sampleExchanges() {
		v.Apply(s)
	}
	return v
}

// TestTrafficStates draws each state of the tool the design lists: what it must
// say, and that nothing sticks out of what wraps it, in the panel's 320.
func TestTrafficStates(t *testing.T) {
	session := proto.SessionInfo{ID: "s1", Project: "/work/enowx"}
	exch := int64(1)
	bodyExchange := func(v *View, tab int, ex proto.TrafficExchange, res proto.TrafficBody) {
		ex.ID, ex.URL, ex.Method = 1, "https://api.github.com/repos/enowx/enowx", "GET"
		v.details[1] = &detail{id: 1, tab: tab, ex: ex, res: res, req: proto.TrafficBody{}, images: map[string]*ui.Bitmap{}}
	}
	states := []struct {
		name   string
		set    func(*View)
		click  string
		center bool
		opts   func(*Options)
		wants  []string
	}{
		{name: "loading", set: func(v *View) { v.loaded = false }, wants: []string{"Starting the interceptor…"}},
		{name: "error", set: func(v *View) { v.err = "The interceptor couldn't start: address in use" }, wants: []string{"The interceptor couldn't start: address in use"}},
		{name: "list", wants: []string{"Recording", "Filter by URL, method or status", "api.github.com/repos/enowx/enowx", "GET · 200 · 142 ms", "POST · … · paused", "GET · ERR · 9 ms", "GET · …", "mock", "rule", "websocket"}},
		{name: "empty", set: func(v *View) { v.list, v.index = nil, map[int64]int{} }, wants: []string{"Nothing caught yet. Intercept attaches a terminal, a browser or the session; what they send shows here."}},
		{name: "no-match", set: func(v *View) { v.filter, v.applied, v.matched = "nothing", "nothing", map[int64]bool{} }, wants: []string{"Nothing matches the filter."}},
		{name: "filter-error", set: func(v *View) {
			v.filter, v.applied, v.matched, v.filterErr = "status:", "status:", map[int64]bool{}, "status: wants a number"
		}, wants: []string{"status: wants a number"}},
		{name: "notice", set: func(v *View) {
			v.notice = "Saved as /Users/someone/Downloads/traffic-20261102-090000.har. Secrets are masked."
		}, wants: []string{"Saved as /Users/someone/Downloads/traffic-20261102-090000.har. Secrets are masked."}},
		{name: "paused", set: func(v *View) {
			v.paused = []proto.TrafficPaused{{ID: 4, Stage: "request", Method: "POST", URL: "https://api.voyageai.com/v1/embeddings"}, {ID: 2, Stage: "response", Status: 200, Method: "GET", URL: "https://api.github.com/user"}}
		}, wants: []string{"2 paused", "paused request", "paused response", "Drop", "Change", "Resume"}},
		{name: "paused-from-rules", set: func(v *View) {
			v.page = pageRules
			v.paused = []proto.TrafficPaused{{ID: 4, Stage: "request", Method: "POST", URL: "https://api.voyageai.com/v1/embeddings"}}
		}, click: "1 paused", wants: []string{"paused request"}},
		{name: "partly-paused", set: func(v *View) {
			v.capture.st = proto.TrafficCapture{State: proto.CapturePaused, Filter: "domain:github.com", Skipped: 12, Since: time.Date(2026, 11, 2, 9, 20, 0, 0, time.UTC)}
		}, wants: []string{"Partly paused · 12 not recorded", "until now", "12 not recorded"}},
		{name: "stopped", set: func(v *View) {
			v.capture.st = proto.TrafficCapture{State: proto.CaptureStopped, Since: time.Date(2026, 11, 2, 9, 20, 0, 0, time.UTC)}
		}, wants: []string{"Stopped"}},
		{name: "pause-menu", click: "Pause recording (⌘⌥P)", set: func(v *View) { v.filter, v.applied, v.matched = "api", "api", nil },
			wants: []string{"Pause everything", "Pause this session's", "Pause browsers'", "Pause matches", "Pause rules too"}},
		{name: "domains-menu", click: "Domains", wants: []string{"github.com (3)"}},
		{name: "domains-empty", set: func(v *View) { v.list, v.index = nil, map[int64]int{} }, click: "Domains", wants: []string{"No domains yet"}},
		{name: "more-menu", set: func(v *View) { v.filter, v.applied, v.matched = "api", "api", nil }, click: "More", wants: []string{"Save filter", "Export as HAR…"}},

		{name: "rules-empty", set: func(v *View) { v.page = pageRules }, wants: []string{"New rule", "No rules: everything passes through. Mock on a request, or New rule, answers, changes, slows or pauses requests."}},
		{name: "rules", set: func(v *View) {
			v.page = pageRules
			v.rules.project = "/work/enowx"
			off := proto.TrafficRule{ID: "b", Match: proto.TrafficMatch{Host: "registry.npmjs.org"}, Action: proto.TrafficAction{Kind: proto.ActionFail, Fail: "status"}}
			bad := proto.TrafficRule{ID: "c", Enabled: true, Match: proto.TrafficMatch{Path: "(["}, Problem: "the path isn't a regular expression", Action: proto.TrafficAction{Kind: proto.ActionPass}}
			v.rules.list = []proto.TrafficRule{{ID: "a", Enabled: true, Hits: 3, Match: proto.TrafficMatch{Host: "api.voyageai.com"}, Action: proto.TrafficAction{Kind: proto.ActionBreakpoint, Break: "both"}}, off, bad}
		}, wants: []string{"Rules of enowx (.enowx/traffic.json)", "3 hits", "the path isn't a regular expression"}},
		{name: "rule-mock", set: func(v *View) { v.page = pageRules; v.rules.project = "/work/enowx" }, click: "New rule", wants: []string{"When a request matches", "Methods", "Save rule", "Headers (Name: value per line)"}},
		{name: "rule-forward", set: func(v *View) {
			v.rules.edit = formOf(proto.TrafficRule{Action: proto.TrafficAction{Kind: proto.ActionForward}})
			v.rules.edit.index = -1
		}, wants: []string{"To the origin", "Keep the Host header"}},
		{name: "rule-transform", set: func(v *View) {
			v.rules.edit = formOf(proto.TrafficRule{Action: proto.TrafficAction{Kind: proto.ActionTransform}})
			v.rules.edit.index = -1
		}, wants: []string{"Request: set headers", "Response: replace", "with", "Response JSON: field = value per line"}},
		{name: "rule-fail", set: func(v *View) {
			v.rules.edit = formOf(proto.TrafficRule{Action: proto.TrafficAction{Kind: proto.ActionFail}})
			v.rules.edit.index = 0
		}, wants: []string{"Edit rule", "503", "Time out"}},
		{name: "rule-pause", set: func(v *View) {
			v.rules.edit = formOf(proto.TrafficRule{Action: proto.TrafficAction{Kind: proto.ActionBreakpoint}})
			v.rules.edit.index = -1
			v.rules.saving = true
			v.rules.edit.err = "say what it matches: a host, a path, a method or a body"
		}, wants: []string{"Pause the", "Saving…", "say what it matches: a host, a path, a method or a body"}},

		{name: "intercept", set: func(v *View) {
			v.page = pageIntercept
			v.inter.err = "attach: no such session"
			v.targets = []proto.TrafficTarget{{Source: "t1", Kind: proto.TargetTerminal, Label: "npm run dev", Proxy: "http://127.0.0.1:8899", Session: "s1"}}
			v.status.Browsers = []proto.TrafficProgram{{ID: "chrome", Name: "Chrome"}}
		}, wants: []string{"Attached", "Detach", "Terminals", "New terminal", "Browsers", "Chrome", "Devices and containers", "Docker containers", "Agents", "attach: no such session"}},
		{name: "intercept-no-browser", set: func(v *View) { v.page = pageIntercept }, wants: []string{"No Chrome, Chromium, Edge, Brave or Firefox found on This Mac.", "No device online: start an emulator or connect a phone in the Android tool."}},
		{name: "intercept-no-session", set: func(v *View) { v.page = pageIntercept }, opts: func(o *Options) { o.Session = proto.SessionInfo{} }, wants: []string{"Select a session to attach to it."}},
		{name: "intercept-caught", set: func(v *View) {
			v.page = pageIntercept
			v.targets = []proto.TrafficTarget{{Source: "t2", Kind: proto.TargetSession, Label: "This session", Session: "s1"}}
			v.status.Display = true
			v.status.Browsers = []proto.TrafficProgram{{ID: "chrome", Name: "Chrome"}}
		}, opts: func(o *Options) {
			o.BrowserCaptured = true
			o.Tabs = []proto.SessionInfo{{ID: "tab1", Name: "npm run dev", Agent: "shell"}, {ID: "tab2", Name: "gone", Agent: "shell", Exited: true}, {ID: "tab3", Name: "the agent", Agent: "claude"}}
			o.Android = []AndroidDevice{{Key: "a", Label: "Pixel 8 · this Mac", Caught: true}, {Key: "b", Label: "Galaxy S23 · this Mac"}}
			o.CatchAndroid = func(string, bool) {}
			o.AndroidNote = "Install enowX's CA from the phone's Settings to read its HTTPS."
		}, wants: []string{"Stop catching", "“npm run dev”", "The Browser tool: caught", "the session's display", "Pixel 8 · this Mac: caught", "Catch", "Install enowX's CA from the phone's Settings to read its HTTPS."}},
		{name: "domain-hidden", set: func(v *View) { v.filter, v.applied, v.matched = "-domain:github.com", "-domain:github.com", nil }, click: "Domains", wants: []string{"Show github.com (3)"}},
		{name: "rule-host", set: func(v *View) {
			v.rules.project = ""
			v.rules.edit = formOf(proto.TrafficRule{Action: proto.TrafficAction{Kind: proto.ActionMock}})
			v.rules.edit.index = -1
		}, opts: func(o *Options) { o.Session.Project = "" }, wants: []string{"Saved in this host's rules."}},
		{name: "electron", set: func(v *View) { v.page, v.inter.electronOpen = pageIntercept, true }, wants: []string{"Start an Electron app", "Start"}},
		{name: "ca-reading", set: func(v *View) { v.inter.caOpen = true }, wants: []string{"enowX's certificate authority", "Reading…"}},
		{name: "ca", set: func(v *View) {
			v.inter.caOpen = true
			v.inter.ca = &proto.TrafficCA{Fingerprint: strings.Repeat("ab:", 31) + "ab", File: "/home/someone/.enowx-app/traffic/ca.pem", Bundle: "/home/someone/.enowx-app/traffic/bundle.pem"}
		}, wants: []string{"SHA-256", "Copy the CA (PEM)"}},
		{name: "compose", set: func(v *View) {
			v.compose.open, v.compose.url = true, "https://api.github.com/user"
			v.compose.history = []composeForm{{method: "GET", url: "https://api.github.com/user"}}
			v.compose.err = "no such host"
		}, wants: []string{"Send a request", "URL", "Sent before", "no such host", "Send"}},
		{name: "compose-sending", set: func(v *View) { v.compose.open, v.compose.sending = true, true }, wants: []string{"Sending…"}},
		{name: "settings", set: func(v *View) {
			v.settings.open = true
			v.capture.prefsLoaded = true
			v.status.Fingerprint = strings.Repeat("ab:", 31) + "ab"
			v.settings.err = "Exchanges is a number"
		}, wants: []string{"Traffic settings", "Keep at most (exchanges)", "Pausing", "Certificate authority", "SHA-256 ", "Exchanges is a number", "Done"}},
		{name: "settings-saving", set: func(v *View) { v.settings.open, v.settings.saving = true, true }, wants: []string{"Saving…"}},
		{name: "paused-sheet", set: func(v *View) {
			v.pausedEdit = &pausedEdit{paused: proto.TrafficPaused{ID: 2, Stage: "response", Method: "GET", URL: "https://api.github.com/user", Status: 200}, status: "200"}
		}, wants: []string{"Change, then resume", "Status", "Headers (Name: value per line)", "Resume changed"}},

		{name: "detail-reading", center: true, set: func(v *View) { v.details[1] = &detail{id: 1, images: map[string]*ui.Bitmap{}} }, wants: []string{"Reading…"}},
		{name: "detail-error", center: true, set: func(v *View) { v.details[1] = &detail{id: 1, err: "The host said no", images: map[string]*ui.Bitmap{}} }, wants: []string{"The host said no"}},
		{name: "detail-request", center: true, set: func(v *View) {
			bodyExchange(v, tabRequest, proto.TrafficExchange{ReqHeaders: []proto.Header{{Name: "accept", Value: "application/json"}}}, proto.TrafficBody{})
		}, wants: []string{"GET · 200 · 142 ms", "Request", "Response", "Timing", "Mock", "Replay", "To the agent", "Secrets"}},
		{name: "detail-response", center: true, set: func(v *View) {
			bodyExchange(v, tabResponse, proto.TrafficExchange{Proto: "HTTP/2", Status: 200, ResHeaders: []proto.Header{{Name: "content-type", Value: "application/json"}}, Trailers: []proto.Header{{Name: "grpc-status", Value: "0"}}},
				proto.TrafficBody{ContentType: "application/json", Total: 16, Data: []byte(`{"ok":true}`), Encoding: "br", Truncated: true})
		}, wants: []string{"HTTP/2 200", "br (not decoded)", "only the first part was kept", "Trailers"}},
		{name: "detail-failed", center: true, set: func(v *View) {
			bodyExchange(v, tabResponse, proto.TrafficExchange{Error: "dial tcp: connection refused"}, proto.TrafficBody{ContentType: "text/plain"})
		}, wants: []string{"dial tcp: connection refused", "No body"}},
		{name: "detail-binary", center: true, set: func(v *View) {
			bodyExchange(v, tabResponse, proto.TrafficExchange{Status: 200, Proto: "HTTP/1.1"}, proto.TrafficBody{ContentType: "application/octet-stream", Total: 32, Data: []byte{0xff, 0xfe, 0, 1, 2, 3}})
		}, wants: []string{"000000"}},
		{name: "detail-image", center: true, set: func(v *View) {
			var png bytes.Buffer
			picture := image.NewRGBA(image.Rect(0, 0, 900, 40))
			for i := range picture.Pix {
				picture.Pix[i] = 0xc0
			}
			pngenc.Encode(&png, picture)
			bodyExchange(v, tabResponse, proto.TrafficExchange{Status: 200, Proto: "HTTP/1.1"}, proto.TrafficBody{ContentType: "image/png", Total: int64(png.Len()), Data: png.Bytes()})
		}, wants: []string{"image/png"}},
		{name: "detail-protobuf", center: true, set: func(v *View) {
			bodyExchange(v, tabResponse, proto.TrafficExchange{Status: 200, Proto: "HTTP/2"}, proto.TrafficBody{ContentType: "application/x-protobuf", Total: 2, Data: []byte{0x08, 0x96, 0x01}})
		}, wants: []string{"150"}},
		{name: "detail-form", center: true, set: func(v *View) {
			bodyExchange(v, tabResponse, proto.TrafficExchange{Status: 200, Proto: "HTTP/1.1"}, proto.TrafficBody{ContentType: "application/x-www-form-urlencoded", Total: 7, Data: []byte("name=ada")})
		}, wants: []string{"name: ada"}},
		{name: "detail-more-to-read", center: true, set: func(v *View) {
			bodyExchange(v, tabResponse, proto.TrafficExchange{Status: 200, Proto: "HTTP/1.1"}, proto.TrafficBody{ContentType: "text/plain", Total: 2 << 20, Data: []byte("the first part")})
		}, wants: []string{"more: export it to see all"}},
		{name: "detail-messages-empty", center: true, set: func(v *View) {
			bodyExchange(v, tabMessages, proto.TrafficExchange{Status: 101, Messages: 2}, proto.TrafficBody{})
		}, wants: []string{"Messages (2)", "No messages yet."}},
		{name: "detail-messages", center: true, set: func(v *View) {
			bodyExchange(v, tabMessages, proto.TrafficExchange{Status: 101, Messages: 2}, proto.TrafficBody{})
			v.details[1].msgs = []proto.TrafficMessage{{Sent: true, Kind: "text", Data: []byte("hello")}, {Kind: "text", Data: []byte("a mock's answer"), Mocked: true}}
		}, wants: []string{"hello", "a mock's answer", "text · mock"}},
		{name: "detail-timing", center: true, set: func(v *View) {
			bodyExchange(v, tabTiming, proto.TrafficExchange{Status: 200, Timings: proto.TrafficTimings{Connect: 21, FirstByte: 98, Done: 2100}}, proto.TrafficBody{})
		}, wants: []string{"Connect", "21 ms", "First byte", "2.1 s"}},
		{name: "detail-more", center: true, set: func(v *View) {
			bodyExchange(v, tabRequest, proto.TrafficExchange{Status: 200, Pinned: true}, proto.TrafficBody{})
		},
			click: "More for this request", wants: []string{"Breakpoint…", "Unpin", "Copy URL", "Open in Browser", "Copy response body", "Export as HAR…"}},
	}
	for _, st := range states {
		t.Run(st.name, func(t *testing.T) {
			v := statesView()
			if st.set != nil {
				st.set(v)
			}
			o := Options{HostName: "This Mac", Session: session, Update: func(fn func()) { fn() }, SendToAgent: func(string) {}, OpenInBrowser: func(string) {},
				CopyText: func(string) {}, Android: nil, ToolShown: true, Current: func() int64 {
					if st.center {
						return exch
					}
					return 0
				},
				SetBrowserCapture: func(bool) {}}
			if st.opts != nil {
				st.opts(&o)
			}
			if st.name == "intercept-no-browser" {
				o.SetBrowserCapture = nil
			}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(theme.Window())
				ui.Row(c).Width(1200).Height(720).Background(theme.Frame).TextColor(theme.Text).AlignItems(ui.Stretch).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
						if st.center {
							v.DrawExchange(c, o, exch)
						}
					})
					ui.Column(c).Width(320).Shrink(0).Background(theme.Panel).Children(func() { v.Draw(c, o) })
				})
			}, 1200, 720)
			for range 4 {
				tt.Frame()
			}
			if st.click != "" {
				if err := tt.Click(st.click); err != nil {
					t.Fatalf("%v (%q)", err, tt.Texts())
				}
				for range 4 {
					tt.Frame()
				}
			}
			for _, want := range st.wants {
				if !hasTextPart(tt, want) {
					t.Errorf("no %q in %q", want, tt.Texts())
				}
			}
			if out := wrapcheck.OutOfParents(tt); len(out) > 0 {
				t.Errorf("out of their parents:\n%s", strings.Join(out, "\n"))
			}
			shot(t, tt, "traffic-"+st.name+".png")
		})
	}
}

// hasTextPart says whether some text drawn holds part.
func hasTextPart(tt *ui.Tester, part string) bool {
	for _, text := range tt.Texts() {
		if strings.Contains(text, part) {
			return true
		}
	}
	return false
}

package trafficview

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

func shot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, tt.Image())
}

// The Traffic tool lists what a session's programs sent, shows one, makes
// a mock from it that then answers, and sends a request of its own.
func TestTrafficTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	t.Setenv("HOME", t.TempDir())
	up := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"users":[{"id":1,"name":"Ada"}],"path":%q}`, r.URL.Path)
	}))
	defer up.Close()
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	queue := make(chan func(), 256)
	a, b := net.Pipe()
	go srv.ServeConn(a)
	var v *View
	cl, err := client.New(b, func(ev proto.Event) {
		if ev.Name == proto.EventTrafficExchange {
			var s proto.TrafficSummary
			if json.Unmarshal(ev.Data, &s) == nil {
				queue <- func() { v.Apply(s) }
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	project := t.TempDir()
	if _, err := cl.AddProject(project); err != nil {
		t.Fatal(err)
	}
	sess, err := cl.CreateSession(proto.SessionCreateParams{Project: project, Agent: "shell", BranchMode: proto.BranchNone, Name: "app"})
	if err != nil {
		t.Fatal(err)
	}
	env, err := cl.TrafficEnv(proto.TrafficEnvParams{Session: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	var proxy string
	for _, kv := range env.Env {
		if p, ok := strings.CutPrefix(kv, "HTTPS_PROXY="); ok {
			proxy = p
		}
	}
	pu, _ := url.Parse(proxy)
	hc := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	get := func(path string) (int, string) {
		resp, err := hc.Get(up.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	v = New()
	o := Options{Host: cl, HostName: "This PC", Session: sess, Update: func(fn func()) { queue <- fn }, CopyText: func(string) {}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(520).Height(760).Background(theme.Background).TextColor(theme.Text).Children(func() { v.Draw(c, o) })
	}, 520, 760)
	drain := func(until func() bool, what string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for !until() {
			select {
			case fn := <-queue:
				fn()
			case <-time.After(30 * time.Millisecond):
			}
			tt.Frame()
			if time.Now().After(deadline) {
				t.Fatalf("never: %s (%q)", what, tt.Texts())
			}
		}
	}
	drain(func() bool { return v.loaded }, "loaded")
	for _, p := range []string{"/api/users", "/api/orders", "/health"} {
		get(p)
	}
	drain(func() bool { return tt.HasText(strings.TrimPrefix(up.URL, "https://") + "/api/orders") }, "the list")
	shot(t, tt, "traffic-list.png")

	// Filter: path:/api/* hides /health.
	v.filter = "path:/api/*"
	v.queryAt = time.Now().Add(-time.Second)
	drain(func() bool {
		return v.applied == v.filter && !tt.HasText(strings.TrimPrefix(up.URL, "https://")+"/health")
	}, "the filter")
	v.filter = ""

	// Pick one: its response shows.
	drain(func() bool { return len(v.shown) == 3 }, "unfiltered")
	v.open(o, v.list[v.shown[0]].ID)
	drain(func() bool { return tt.HasText(`"name": "Ada"`) || tt.HasText("Ada") }, "the response body")
	shot(t, tt, "traffic-detail.png")

	// Its menu opens a GET in the Browser tool.
	opened := ""
	o.OpenInBrowser = func(url string) { opened = url }
	tt.Frame()
	if err := tt.Click("More for this request"); err != nil {
		t.Fatal(err)
	}
	drain(func() bool { return tt.HasText("Open in Browser") }, "the menu's item")
	if err := tt.Click("Open in Browser"); err != nil {
		t.Fatal(err)
	}
	drain(func() bool { return strings.Contains(opened, "/api/") || strings.Contains(opened, "/health") }, "opened")
	tt.Frame()

	// Making the CA anew asks first; not agreeing leaves it.
	var askedCA string
	o.Confirm = func(title, _, _ string, _ func()) { askedCA = title }
	before, _ := cl.TrafficStatus()
	if err := tt.Click("Traffic settings"); err != nil {
		t.Fatal(err)
	}
	drain(func() bool { return tt.HasText("Certificate authority") }, "the settings")
	tt.Scroll(260, 380, 0, 800)
	tt.Frame()
	if err := tt.Click("Make the CA anew"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if after, _ := cl.TrafficStatus(); askedCA == "" || after.Fingerprint != before.Fingerprint {
		t.Fatalf("the CA: asked %q, %q then %q", askedCA, before.Fingerprint, after.Fingerprint)
	}
	v.settings.open = false
	tt.Frame()

	// Mock this, then save the rule: the next request is mocked.
	if err := tt.Click("Mock this"); err != nil {
		t.Fatal(err)
	}
	drain(func() bool { return v.rules.loaded && v.rules.edit != nil }, "the rule form")
	v.rules.edit.status, v.rules.edit.resBody = "500", `{"error":"down"}`
	tt.Frame()
	shot(t, tt, "traffic-rule.png")
	if err := tt.Click("Save rule"); err != nil {
		t.Fatal(err)
	}
	drain(func() bool { return v.rules.edit == nil && len(v.rules.list) == 1 }, "the rule saved")
	shot(t, tt, "traffic-rules.png")
	if st, body := get("/api/users"); st != 500 || body != `{"error":"down"}` {
		t.Fatalf("mocked: %d %s", st, body)
	}

	// Compose sends from the host.
	v.compose.open = true
	v.compose.url = up.URL + "/compose"
	tt.Frame()
	shot(t, tt, "traffic-compose.png")
	if err := tt.Click("Send"); err != nil {
		t.Fatal(err)
	}
	drain(func() bool { return !v.compose.open && v.detail != nil && v.detail.ex.Status == 200 }, "the composed request")

	v.settings.open = true
	drain(func() bool { return tt.HasText("Don't check the certificates of") && v.settings.loaded }, "the settings")
	shot(t, tt, "traffic-settings.png")
	v.settings.open = false

	v.page = pageIntercept
	drain(func() bool { return tt.HasText("New terminal") }, "the intercept page")
	shot(t, tt, "traffic-intercept.png")
}

func TestDomainOf(t *testing.T) {
	for host, want := range map[string]string{"api.example.com": "example.com", "example.com": "example.com", "a.b.example.co.uk": "example.co.uk", "127.0.0.1": "127.0.0.1", "localhost": "localhost"} {
		if got := domainOf(host); got != want {
			t.Errorf("%s: %s", host, got)
		}
	}
}

package portview

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// lookShot keeps what tt shows as name under $SHOTS, to compare with the design.
func lookShot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = png.Encode(f, tt.Image())
}

// The tool's states read in a panel's width, its rows and switches inside it.
func TestPortsStatesInAPanel(t *testing.T) {
	ports := proto.SessionPorts{Supported: true, Ports: []proto.Port{
		{Port: 5173, Addr: "127.0.0.1", PID: 8123, Process: "node"},
		{Port: 8080, Addr: "*", PID: 43, Process: "a-process-with-a-rather-long-name"},
		{Port: 6010, Addr: "127.0.0.1", PID: 44, Process: "display"},
	}}
	full := Options{OpenURL: func(string) {}, Preview: func(proto.Port) {}, OpenWindow: func(proto.Port) {}, SetOffers: func(bool) {}, Capture: func(bool) {}, Offers: true}
	draw := func(name string, v *View, o Options, want ...string) {
		t.Helper()
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(theme.Window())
			ui.Column(c).Width(320).Height(640).Background(theme.Panel).TextColor(theme.Text).Children(func() { v.Draw(c, o) })
		}, 320, 640)
		tt.Frame()
		tt.Frame()
		for _, w := range want {
			if !tt.HasText(w) {
				t.Errorf("%s: no %q in %q", name, w, tt.Texts())
			}
		}
		lookShot(t, tt, "ports-"+name)
	}
	v := New()
	v.loaded, v.ports = true, ports
	draw("list", v, full, "Listening on this machine", ":5173", "only this host", "every address", "Open", "Through Traffic", "Pop up when a session opens a port")
	draw("plain", v, Options{OpenURL: func(string) {}, SetOffers: func(bool) {}, Offers: true}, "Pop up when a session opens a port")
	draw("loading", New(), Options{}, "Looking for ports…")
	v = New()
	v.loaded, v.err = true, "not connected to the host"
	draw("error", v, Options{}, "not connected to the host")
	v = New()
	v.loaded = true
	draw("unsupported", v, Options{}, "This host can't tell which ports a session listens on.")
	v = New()
	v.loaded, v.ports = true, proto.SessionPorts{Supported: true}
	draw("none", v, full, "No ports open. A dev server the session starts shows here.")
}

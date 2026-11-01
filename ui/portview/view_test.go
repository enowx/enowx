package portview

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

func TestPortsOfThisMachineOpen(t *testing.T) {
	v := New()
	v.loaded = true
	v.ports = proto.SessionPorts{Supported: true, Ports: []proto.Port{
		{Port: 5173, Addr: "127.0.0.1", PID: 42, Process: "node"},
		{Port: 8080, Addr: "*", PID: 43, Process: "python3"},
	}}
	var opened string
	tt := ui.NewTester(func(c *ui.Context) {
		v.Draw(c, Options{OpenURL: func(url string) { opened = url }})
	}, 480, 400)
	tt.Frame()
	for _, want := range []string{":5173", "node · pid 42 · ", "only this host", ":8080", "every address"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	if opened != "http://localhost:5173" {
		t.Fatalf("opened %q", opened)
	}
}

func TestNoPortsSaysWhereTheyWillShow(t *testing.T) {
	v := New()
	v.loaded, v.ports = true, proto.SessionPorts{Supported: true}
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, Options{}) }, 480, 300)
	tt.Frame()
	if !tt.HasText("No ports open. A dev server the session starts shows here.") {
		t.Fatalf("%q", tt.Texts())
	}
}

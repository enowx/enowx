package activityview

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

// The tool's states read in a panel's width, a turn showing how long it worked
// and what it ran, its calls when open.
func TestActivityStatesInAPanel(t *testing.T) {
	o := Options{Session: proto.SessionInfo{Dir: "/work/app", State: proto.StateWorking}, OpenFile: func(string) {}, OpenDiff: func(string) {}}
	draw := func(name string, v *View, want ...string) {
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
		lookShot(t, tt, "activity-"+name)
	}
	v := New()
	conversation(v)
	v.open[0], v.open[5] = true, true
	draw("turns", v, "Turns", "Tool calls", "Edits", "Failed", "Files edited", "Add search ranking", "42s · 3 tools · 1 file", "1 failed")
	v = New()
	v.apply(proto.Transcript{Supported: true}, nil, "")
	draw("nothing", v, "Nothing yet: the agent's work shows here as it goes.")
	draw("reading", New(), "Reading the conversation…")
	v = New()
	v.apply(proto.Transcript{Reason: "Codex has no chat view yet; it shows in the terminal."}, nil, "")
	draw("unsupported", v, "Codex has no chat view yet; it shows in the terminal.")
	v = New()
	v.err = "not connected to the host"
	draw("error", v, "not connected to the host")
	v = New()
	v.apply(proto.Transcript{Supported: true, Entries: []proto.TranscriptEntry{{Index: 0, Kind: proto.TranscriptUser, Text: "Just answer"}, {Index: 1, Kind: proto.TranscriptAssistant, Text: "Ok."}}}, nil, "")
	v.open[0] = true
	draw("no-tools", v, "No tools: it answered.")
}

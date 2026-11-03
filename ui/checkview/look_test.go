package checkview

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// The tool's states read in a panel's width, its dialog and a check's output
// beside it.
func TestChecksStatesInAPanel(t *testing.T) {
	info := proto.ChecksInfo{
		Checks: []proto.Check{{Name: "lint", Command: "make lint"}, {Name: "build", Command: "go build ./..."}, {Name: "test", Command: "go test ./ui"}, {Name: "vet", Command: "go vet ./ui"}, {Name: "e2e", Command: "npm run e2e and a command long enough to have to run in its row"}, {Name: "q", Command: "make q"}, {Name: "s", Command: "make s"}},
		Runs: []proto.CheckRun{
			{Name: "lint", State: proto.CheckPassed, Duration: 12 * time.Second},
			{Name: "build", State: proto.CheckPassed, Duration: 8 * time.Second, Output: ""},
			{Name: "test", State: proto.CheckRunning, Started: time.Now().Add(-41 * time.Second)},
			{Name: "vet", State: proto.CheckFailed, ExitCode: 1, Duration: 3 * time.Second, Output: "ui/goa_tabs.go:88: unreachable code\n", Judged: proto.VerdictFlaky, JudgedWhy: "dial tcp: i/o timeout, not the code"},
			{Name: "q", State: proto.CheckQueued},
			{Name: "s", State: proto.CheckStopped},
		},
	}
	draw := func(name string, v *View, output string, want ...string) {
		t.Helper()
		o := Options{OpenOutput: func(string) {}}
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(theme.Window())
			ui.Row(c).Width(1040).Height(640).Background(theme.Frame).TextColor(theme.Text).Children(func() {
				ui.Column(c).Width(720).FillHeight().Children(func() {
					if output != "" {
						v.DrawOutput(c, o, output)
					}
				})
				ui.Column(c).Width(320).FillHeight().Children(func() { v.Draw(c, o) })
			})
		}, 1040, 640)
		tt.Frame()
		tt.Frame()
		for _, w := range want {
			if !tt.HasText(w) {
				t.Errorf("%s: no %q in %q", name, w, tt.Texts())
			}
		}
		lookShot(t, tt, "checks-"+name)
	}
	v := New()
	v.loaded, v.info = true, info
	draw("running", v, "vet", "Stop", "passed · 12s", "failed (exit 1) · 3s", "queued", "stopped", "not run", "flaky", "Found in the project's files", "exit status 1")
	draw("building", v, "build", "(no output)")
	v = New()
	v.loaded, v.info = true, proto.ChecksInfo{Custom: true, Checks: info.Checks[:2], Runs: info.Runs[:1]}
	draw("custom", v, "build", "Run all", "Set for this project", "Run", "Not run yet.")
	v.edit = &editForm{open: true, rows: append([]proto.Check(nil), info.Checks[:3]...)}
	draw("edit", v, "", "Checks of this project", "Use those found", "Add a check")
	v.edit = nil
	draw("loading", New(), "", "Looking for checks…")
	v = New()
	v.loaded, v.err = true, "the host hung up"
	draw("none", v, "", "the host hung up", "No checks found in this project. Edit them to add the commands that test it.")
}

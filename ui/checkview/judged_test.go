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

// A failed check shows the judge's word: flaky with why and Run again, a
// real failure with why, or that the judge is reading it.
func TestFailedChecksShowTheJudge(t *testing.T) {
	v := New()
	v.loaded = true
	v.info = proto.ChecksInfo{
		Checks: []proto.Check{{Name: "e2e", Command: "npm run e2e"}, {Name: "unit", Command: "go test ./..."}, {Name: "lint", Command: "make lint"}},
		Runs: []proto.CheckRun{
			{Name: "e2e", State: proto.CheckFailed, ExitCode: 1, Duration: 9 * time.Second, Judged: proto.VerdictFlaky, JudgedWhy: "dial tcp 127.0.0.1:5432: i/o timeout, not the code"},
			{Name: "unit", State: proto.CheckFailed, ExitCode: 1, Duration: 2 * time.Second, Judged: proto.VerdictReal, JudgedWhy: "TestTotal got 9, want 10"},
			{Name: "lint", State: proto.CheckFailed, ExitCode: 2, Duration: time.Second, Judging: true},
		},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(520).Height(400).Background(theme.Background).TextColor(theme.Text).Children(func() { v.Draw(c, Options{}) })
	}, 520, 400)
	tt.Frame()
	tt.Frame()
	for _, want := range []string{"flaky", "dial tcp 127.0.0.1:5432: i/o timeout, not the code", "Run again", "real failure", "TestTotal got 9, want 10", "The judge reads the failure…"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	if dir := os.Getenv("SHOTS"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "checks-judged.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		png.Encode(f, tt.Image())
	}
}

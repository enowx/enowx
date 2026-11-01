package checkview

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

func TestChecksShowHowTheyWent(t *testing.T) {
	v := New()
	v.loaded = true
	v.info = proto.ChecksInfo{
		Checks: []proto.Check{{Name: "lint", Command: "make lint"}, {Name: "test", Command: "make test"}, {Name: "build", Command: "make build"}},
		Runs: []proto.CheckRun{
			{Name: "lint", State: proto.CheckPassed, Duration: 1200 * time.Millisecond, Output: "ok\n"},
			{Name: "test", State: proto.CheckFailed, ExitCode: 2, Duration: 3 * time.Second, Output: "--- FAIL: TestX\nFAIL\n"},
		},
	}
	var opened string
	tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, Options{OpenOutput: func(name string) { opened = name }}) }, 520, 600)
	tt.Frame()
	for _, want := range []string{"Found in the project's files", "Run all", "passed · 1.2s", "failed (exit 2) · 3s", "not run", "make test"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText("--- FAIL: TestX\nFAIL") {
		t.Error("the panel's list shows a check's output")
	}
	// A check opens in the center, where its output shows.
	if err := tt.Click("test"); err != nil {
		t.Fatal(err)
	}
	if opened != "test" {
		t.Fatalf("opened %q", opened)
	}
	output := ui.NewTester(func(c *ui.Context) { v.DrawOutput(c, Options{}, "test") }, 700, 400)
	output.Frame()
	for _, want := range []string{"$ make test", "--- FAIL: TestX\nFAIL", "exit status 2", "failed (exit 2) · 3s", "Run again"} {
		if !output.HasText(want) {
			t.Errorf("no %q in %q", want, output.Texts())
		}
	}

	// While one runs, Stop replaces Run all, and no check starts on its own.
	v.info.Runs[1] = proto.CheckRun{Name: "test", State: proto.CheckRunning, Started: time.Now()}
	tt.Frame()
	if !tt.HasText("Stop") || tt.HasText("Run all") {
		t.Fatalf("running: %q", tt.Texts())
	}
	output.Frame()
	if output.HasText("Run again") {
		t.Fatalf("running, it offers to run: %q", output.Texts())
	}

	// Edit opens the project's checks to change.
	if err := tt.Click("Edit"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Checks of this project") || !tt.HasText("Add a check") {
		t.Fatalf("edit: %q", tt.Texts())
	}
}

func TestLastLines(t *testing.T) {
	if got := lastLines("a\nb\nc\n", 2); got != "…\nb\nc" {
		t.Fatalf("%q", got)
	}
	if got := lastLines("FAIL\tpkg\n\x1b[31mred\x1b[0m", 5); got != "FAIL    pkg\nred" {
		t.Fatalf("%q", got)
	}
}

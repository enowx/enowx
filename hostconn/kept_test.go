package hostconn

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

// Real daemons, of another build than this test, in a home of their own. One running
// a command stays, its session alive, until the user says otherwise; one with nothing
// to lose is replaced, as before; replacing it says what it ended.
func TestUpdateKeepsABusyDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	home, err := os.MkdirTemp("", "enowx-kept")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	bin := filepath.Join(home, "enowxd")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/enowxd").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	t.Setenv("ENOWX_HOME", filepath.Join(home, "data"))
	t.Setenv("SHELL", "/bin/sh")
	t.Cleanup(func() { _ = exec.Command(bin, "stop").Run() })
	// In the background of a shell that ends, as a daemon outlives the app that started
	// it: a child of this test would stay a zombie when stopped, and never seem gone.
	opts := Options{DaemonCommand: []string{"/bin/sh", "-c", bin + " serve >/dev/null 2>&1 &"}}
	connect := func(o Options) *client.Client {
		t.Helper()
		c, err := connectLocal(o)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	first := connect(opts)
	a, err := first.Info()
	if err != nil {
		t.Fatal(err)
	}
	// Nothing runs: replaced.
	second := connect(opts)
	b, err := second.Info()
	if err != nil {
		t.Fatal(err)
	}
	if a.PID == b.PID {
		t.Fatalf("a daemon with nothing to lose wasn't replaced: %d", a.PID)
	}

	info, err := second.CreateSession(proto.SessionCreateParams{Agent: "shell", Dir: t.TempDir(), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	if err := second.Input(info.ID, "sleep 3333", false, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		plan, err := second.RestartPlan()
		if err == nil && len(plan.Commands) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the command wasn't seen: %+v, %v", plan, err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Busy: it stays, with its command, and says so.
	var kept proto.RestartPlan
	withKept := opts
	withKept.Kept = func(_ proto.DaemonInfo, plan proto.RestartPlan) { kept = plan }
	third := connect(withKept)
	c, err := third.Info()
	if err != nil {
		t.Fatal(err)
	}
	if c.PID != b.PID {
		t.Fatalf("a busy daemon was replaced: %d, then %d", b.PID, c.PID)
	}
	if len(kept.Commands) != 1 || !strings.Contains(kept.Commands[0].What, "sleep 3333") {
		t.Fatalf("kept: %+v", kept)
	}

	// The user says so: replaced, and what ended is listed, not run again.
	asked := opts
	asked.ReplaceBusy = true
	fourth := connect(asked)
	d, err := fourth.Info()
	if err != nil {
		t.Fatal(err)
	}
	if d.PID == c.PID {
		t.Fatal("not replaced when asked")
	}
	plan, err := fourth.RestartPlan()
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Lost) != 1 || !strings.Contains(plan.Lost[0].What, "sleep 3333") || len(plan.Commands) != 0 {
		t.Fatalf("after: %+v", plan)
	}
}

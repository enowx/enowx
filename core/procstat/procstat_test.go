package procstat

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestTreeCountsABusyChild(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("not measured on " + runtime.GOOS)
	}
	// A shell whose child spins: the tree is the shell and the spinner.
	cmd := exec.Command("sh", "-c", "while :; do :; done & wait")
	cmd.Env = append(os.Environ(), "ENOWX_SESSION=test-1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command("pkill", "-P", strconv.Itoa(cmd.Process.Pid)).Run()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	time.Sleep(200 * time.Millisecond) // the spinner starts
	var s Sampler
	if err := s.Sample(); err != nil {
		t.Fatal(err)
	}
	// A machine busy with other tests can starve the spinner for a moment: it shows as a
	// core at some look.
	u := s.Tree(cmd.Process.Pid)
	for deadline := time.Now().Add(30 * time.Second); u.Procs < 2 || u.RSS == 0 || u.CPU < 50; {
		if time.Now().After(deadline) {
			t.Fatalf("tree = %+v, want 2+ processes using a core", u)
		}
		time.Sleep(time.Second)
		if err := s.Sample(); err != nil {
			t.Fatal(err)
		}
		u = s.Tree(cmd.Process.Pid)
	}
	if one := s.One(cmd.Process.Pid); one.CPU > u.CPU || one.Procs != 1 {
		t.Fatalf("one = %+v, tree = %+v", one, u)
	}
	if got := Args(cmd.Process.Pid); len(got) != 3 || got[1] != "-c" || got[2] != "while :; do :; done & wait" {
		t.Fatalf("args = %q", got)
	}
	// Recent macOS doesn't show other processes' environment.
	if got := Env(cmd.Process.Pid, "ENOWX_SESSION"); got != "test-1" && runtime.GOOS == "linux" {
		t.Fatalf("env = %q", got)
	}
	if h := s.Host(); h.MemTotal == 0 || h.MemUsed == 0 || h.MemUsed > h.MemTotal || h.CPUs == 0 {
		t.Fatalf("host = %+v", h)
	}
}

func TestDisk(t *testing.T) {
	total, used := Disk(t.TempDir())
	if total == 0 || used == 0 || used > total {
		t.Fatalf("disk %d used of %d", used, total)
	}
}

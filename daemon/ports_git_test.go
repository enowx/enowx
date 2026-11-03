package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// inspectRuns puts a git first on PATH that keeps what it is asked, and gives how many
// of its runs were to describe a project: its root, its branch, its branches. Others
// go on in the background (a catalog reads a project's remote) and aren't this.
func inspectRuns(t *testing.T) func() int {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git here")
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "runs")
	script := fmt.Sprintf("#!/bin/sh\necho \"$*\" >> %q\nexec %q \"$@\"\n", log, real)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() int {
		data, err := os.ReadFile(log)
		if err != nil {
			return 0
		}
		n := 0
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "--show-toplevel") || strings.Contains(line, "symbolic-ref") || strings.Contains(line, "for-each-ref") {
				n++
			}
		}
		return n
	}
}

// settled waits until runs stops moving, what adding a project set off being over.
func settled(runs func() int) int {
	n := runs()
	for {
		time.Sleep(200 * time.Millisecond)
		now := runs()
		if now == n {
			return n
		}
		n = now
	}
}

// Looking for ports every few seconds used to run git three times for each project on
// every tick, to answer a question about hooks that git has no part in.
func TestPortTicksRunNoGit(t *testing.T) {
	if !portsSupported {
		t.Skip("ports aren't read here")
	}
	runs := inspectRuns(t)
	every := portsEvery
	portsEvery = 20 * time.Millisecond
	t.Cleanup(func() { portsEvery = every })
	srv := startServer(t)
	c := connect(t, srv, nil)
	repos := []string{gitRepo(t), gitRepo(t), gitRepo(t)}
	for _, repo := range repos {
		if _, err := c.AddProject(repo); err != nil {
			t.Fatal(err)
		}
	}
	srv.projects.List()
	before := settled(runs)
	if before == 0 {
		t.Fatal("adding projects ran no git that was counted: this test counts nothing")
	}

	// watchPorts ticks on its own meanwhile, and a tick's question is asked here too,
	// so that some certainly were.
	for range 10 {
		if srv.hostWants(proto.HookPort) {
			t.Fatal("no hook wants ports yet")
		}
		time.Sleep(portsEvery)
	}
	if ran := runs() - before; ran != 0 {
		t.Fatalf("about 10 ticks over %d projects ran git %d times to describe them", len(repos), ran)
	}

	// Lists on a timer of their own, as the tokens' scan and the schedules have, stay quiet too.
	for range 10 {
		if got := srv.projects.List(); len(got) != len(repos) {
			t.Fatalf("projects: %+v", got)
		}
	}
	if ran := runs() - before; ran != 0 {
		t.Fatalf("10 lists of %d projects ran git %d times", len(repos), ran)
	}
}

// hostWants still answers as it did, at once: a project's hook wants the event from
// the moment it's there, and stops when its project is gone.
func TestHostWantsFollowsProjectsHooks(t *testing.T) {
	srv := startServer(t)
	c := connect(t, srv, nil)
	repo := gitRepo(t)
	if _, err := c.AddProject(repo); err != nil {
		t.Fatal(err)
	}
	if srv.hostWants(proto.HookPort) {
		t.Fatal("wanted with no hook at all")
	}
	hook := proto.Hook{On: true, Event: proto.HookPort, Action: proto.HookActionNotify}
	if err := srv.saveProjectHooks(proto.ProjectHooks{Dir: repo, Hooks: []proto.Hook{hook}}); err != nil {
		t.Fatal(err)
	}
	if !srv.hostWants(proto.HookPort) {
		t.Fatal("a project's hook on the event isn't wanted")
	}
	if srv.hostWants(proto.HookCommit) {
		t.Fatal("a hook on another event counted")
	}
	if err := c.RemoveProject(repo); err != nil {
		t.Fatal(err)
	}
	if srv.hostWants(proto.HookPort) {
		t.Fatal("the hook of a project no longer on the list counted")
	}
}

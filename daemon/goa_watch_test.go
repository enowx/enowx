package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/session"
)

// A GOA on an agent that can't be held to enowX's tools is watched: a
// file it changes in a project while no session works there is flagged,
// and put back as it was when GOA started; a file it adds, removed.
func TestGOAWatchedFlagsAndReverts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh and git")
	}
	srv := startServer(t)
	proj := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := git.Command(proj, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q")
	os.WriteFile(filepath.Join(proj, "totals.go"), []byte("package shop // 1\n"), 0o644)
	run("add", ".")
	run("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "totals")
	if _, err := srv.projects.Add(proj); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(srv.goaDir(), 0o700)
	g, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: srv.goaDir(), Meta: session.Meta{Agent: "aider", GOA: true}})
	if err != nil {
		t.Fatal(err)
	}
	srv.goaWatchSeen(g.Info())
	waitFor(t, "the projects' state kept", func() bool {
		srv.goa.mu.Lock()
		defer srv.goa.mu.Unlock()
		w := srv.goa.watch[g.ID()]
		return w != nil && w.base != nil
	})
	os.WriteFile(filepath.Join(proj, "totals.go"), []byte("package shop // changed by GOA\n"), 0o644)
	os.WriteFile(filepath.Join(proj, "new.go"), []byte("package shop\n"), 0o644)
	files := func() (n int) {
		for _, f := range srv.goaFlags() {
			if f.File != "" {
				n++
			}
		}
		return n
	}
	waitFor(t, "both files flagged", func() bool { return files() == 2 })
	for _, f := range srv.goaFlags() {
		if f.File == "" {
			continue
		}
		if !f.Revertable {
			t.Fatalf("not revertable: %+v", f)
		}
		if _, err := srv.goaRevert(f.ID); err != nil {
			t.Fatal(err)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(proj, "totals.go")); string(data) != "package shop // 1\n" {
		t.Fatalf("totals.go not put back: %q", data)
	}
	if _, err := os.Stat(filepath.Join(proj, "new.go")); err == nil {
		t.Fatal("the file GOA added is still there")
	}
}

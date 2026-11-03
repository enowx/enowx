package daemon

import (
	"testing"

	"github.com/enowx/enowx/core/procstat"
)

// A session's tree goes on the list; what left it stays, marked; what ended goes off;
// a process given a PID freed meanwhile isn't taken for the one before; and nothing a
// session didn't start ever joins.
func TestMergeTrees(t *testing.T) {
	const self = 100
	root := func(pid int) string {
		if pid == 200 {
			return "s1"
		}
		return ""
	}
	list := []procstat.Proc{
		{PID: self, PPID: 1, Start: "d"},
		{PID: 200, PPID: self, Start: "a", Name: "sh"},
		{PID: 201, PPID: 200, Start: "b", Name: "omp"},
		{PID: 202, PPID: 201, Start: "c", Name: "lsp"},
		{PID: 300, PPID: self, Start: "e", Name: "git"},
		{PID: 400, PPID: 1, Start: "f", Name: "someone-elses"},
	}
	known, changed := mergeTrees(nil, list, self, root, nil)
	if !changed || len(known) != 3 || known[202].Session != "s1" || known[202].Name != "lsp" {
		t.Fatalf("first look: %v", known)
	}
	if _, changed := mergeTrees(known, list, self, root, nil); changed {
		t.Fatal("nothing changed, yet it says so")
	}
	// The agent ended: its server was taken up by init, and the program's wrapper ended.
	later := []procstat.Proc{
		{PID: self, PPID: 1, Start: "d"},
		{PID: 202, PPID: 1, Start: "c", Name: "lsp"},
		{PID: 201, PPID: 1, Start: "z", Name: "reused"},
	}
	known, _ = mergeTrees(known, later, self, root, nil)
	if len(known) != 1 || !known[202].Left {
		t.Fatalf("later: %v", known)
	}
}

// A daemon a session started isn't stopped with that session, nor is what's above it.
func TestMergeTreesSparesTheDaemon(t *testing.T) {
	list := []procstat.Proc{
		{PID: 100, PPID: 201, Start: "d"},
		{PID: 200, PPID: 100, Start: "a"},
		{PID: 201, PPID: 200, Start: "b"},
	}
	spare := ancestors(list, 100)
	known, _ := mergeTrees(nil, list, 100, func(int) string { return "s1" }, spare)
	if _, took := known[100]; took {
		t.Fatalf("took the daemon: %v", known)
	}
	if _, took := known[201]; took {
		t.Fatalf("took what's above it: %v", known)
	}
}

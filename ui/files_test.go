package ui

import (
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

func TestFileTabsBelongToTheirSession(t *testing.T) {
	a := &App{client: &client.Client{}}
	s1, s2 := proto.SessionInfo{ID: "s1"}, proto.SessionInfo{ID: "s2"}
	a.selected = "s1"
	a.openFileTab(s1, "src/main.go")
	a.openFileTab(s1, "README.md")
	a.openFileTab(s1, "src/main.go") // already open: shown, not opened twice
	if got := a.centerFiles("s1"); len(got) != 2 {
		t.Fatalf("tabs = %q", got)
	}
	if a.centerFront(s1) != "file:src/main.go" {
		t.Fatalf("in front: %q", a.centerFront(s1))
	}
	// Another session hasn't that file open.
	if got := a.centerFront(s2); got != "" {
		t.Fatalf("for s2 in front: %q", got)
	}
	a.closeCenter("s1", "file:src/main.go")
	if got := a.centerFront(s1); got != "" || len(a.centerFiles("s1")) != 1 {
		t.Fatalf("after closing the tab in front: %q, tabs %q", got, a.centerFiles("s1"))
	}
	a.closeCenter("s1", "file:README.md")
	a.openFileTab(s1, "a.go")
	a.openFileTab(s1, "b.go")
	a.closeCenter("s1", "file:b.go")
	if got := a.centerFront(s1); got != "file:a.go" {
		t.Fatalf("the tab before it should come to the front: %q", got)
	}
}

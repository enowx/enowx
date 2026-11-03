package fileview

import (
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// What changes on disk comes in while nothing is changed here, and waits for the user when
// something is; a file that can't be edited says why.
func TestEditorFromDisk(t *testing.T) {
	at := time.Now()
	d := NewDoc("a.go")
	d.fromDisk(&proto.FileContent{Path: "a.go", Text: "one\r\ntwo\r\n", Modified: at})
	e := d.edit
	if e == nil || !e.crlf || e.buf.String() != "one\ntwo\n" || d.Dirty() {
		t.Fatalf("opened: %+v", e)
	}
	d.fromDisk(&proto.FileContent{Path: "a.go", Text: "three\n", Modified: at.Add(time.Second)})
	if e.buf.String() != "three\n" || d.Dirty() || e.conflict != nil {
		t.Fatalf("a change on disk with none here: %q", e.buf.String())
	}
	e.change(0, 0, "x", 1, 1)
	if !d.Dirty() {
		t.Fatal("a change isn't unsaved")
	}
	theirs := at.Add(2 * time.Second)
	d.fromDisk(&proto.FileContent{Path: "a.go", Text: "four\n", Modified: theirs})
	if e.conflict == nil || e.buf.String() != "xthree\n" {
		t.Fatalf("overwrote changes: %q", e.buf.String())
	}
	e.stepBack()
	if d.Dirty() || e.buf.String() != "three\n" {
		t.Fatalf("undo: %q, dirty %v", e.buf.String(), d.Dirty())
	}
	e.stepForward()
	if !d.Dirty() || e.buf.String() != "xthree\n" {
		t.Fatalf("redo: %q", e.buf.String())
	}
	for why, f := range map[string]proto.FileContent{
		"Read only": {ReadOnly: true},
		"over":      {Truncated: true},
		"a line":    {Text: strings.Repeat("y", editLineMost+1)},
	} {
		if got := editingBlocked(&f); !strings.Contains(got, why) {
			t.Errorf("%q: %q", why, got)
		}
	}
	if got := editingBlocked(&proto.FileContent{Text: "fine\n"}); got != "" {
		t.Errorf("%q", got)
	}
}

func TestEditKeys(t *testing.T) {
	d := NewDoc("a.go")
	d.fromDisk(&proto.FileContent{Text: "func f() {\n\tx()\n}\n"})
	e := d.edit
	e.selA, e.selB = 15, 15 // at the end of the second line
	e.newLine()
	if got := e.buf.String(); got != "func f() {\n\tx()\n\t\n}\n" {
		t.Fatalf("new line: %q", got)
	}
	e.selA, e.selB = 11, 11
	e.toggleComment("//")
	if got := e.buf.Snapshot().Line(1); got != "\t// x()" {
		t.Fatalf("comment: %q", got)
	}
	e.selA, e.selB = 11, 11
	e.toggleComment("//")
	if got := e.buf.Snapshot().Line(1); got != "\tx()" {
		t.Fatalf("uncomment: %q", got)
	}
	hits := findHits("Foo foo é FOO", "foo")
	if len(hits) != 3 || hits[2] != [2]int{10, 13} {
		t.Fatalf("hits %v", hits)
	}
}

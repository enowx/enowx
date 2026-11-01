package canvas

import (
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

// A design that uses a prototype's state gets the runtime, one that doesn't
// stays as written, and on the shared page every design is a sandboxed frame
// with no way to its own origin.
func TestPrototypeDocuments(t *testing.T) {
	plain := proto.CanvasFrame{Name: "Plain", Width: 100, Height: 100, HTML: `<p>Hi</p>`}
	used := proto.CanvasFrame{Name: "Used", Width: 100, Height: 100, HTML: `<button data-set="a=b">Go</button>`}
	if strings.Contains(FrameDocument(plain, nil), "__enowxState") {
		t.Fatal("a plain design got the runtime")
	}
	if !strings.Contains(FrameDocument(used, nil), "__enowxState") {
		t.Fatal("a prototype's design has no runtime")
	}

	page := Standalone(proto.Canvas{Frames: []proto.CanvasFrame{plain, used}}, nil)
	if n := strings.Count(page, `<iframe `); n != 2 {
		t.Fatalf("%d frames", n)
	}
	if n := strings.Count(page, `sandbox="`+shareFrameSandbox+`"`); n != 2 {
		t.Fatalf("%d sandboxed frames of 2", n)
	}
	if strings.Contains(shareFrameSandbox, "allow-same-origin") || strings.Contains(shareFrameSandbox, "allow-top-navigation") {
		t.Fatalf("sandbox lets a design out: %s", shareFrameSandbox)
	}
	if strings.Contains(page, "parent.document") {
		t.Fatal("a design reaches into the page around it")
	}
}

// What a page left can't end the script it is put in.
func TestPrototypeSeedStaysInItsScript(t *testing.T) {
	got := PrototypeSeed(`{"a":"</script><script>alert(1)"}`)
	if strings.Count(got, "</script>") != 1 || !strings.Contains(got, `</script>`) {
		t.Fatalf("seed: %s", got)
	}
	if PrototypeSeed("") != "" {
		t.Fatal("no state gives no script")
	}
}

func TestWithHead(t *testing.T) {
	for doc, want := range map[string]string{
		`<html><head><title>x</title></head></html>`: `<html><head>S<title>x</title></head></html>`,
		`<html lang="en"><body></body></html>`:       `<html lang="en">S<body></body></html>`,
		`<p>x</p>`:                                   `S<p>x</p>`,
	} {
		if got := WithHead(doc, "S"); got != want {
			t.Errorf("%s: %s", doc, got)
		}
	}
}

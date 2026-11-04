package widget

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

const streamDoc = "# Title\n\nA paragraph\nthat goes on.\n\n- one\n- two\n  still two\n\n1. first\n\n2. second\n\n" +
	"```go\nfunc main() {\n\n\tprintln(\"blank line above stays in\")\n}\n```\n\n> quoted\n> more\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n---\n\nThe end, with no line after it"

func streamBlocks(s *MarkdownStream) []block { return append(slices.Clone(s.settled), s.tail...) }

// Fed in small pieces, a stream reads as the text parsed whole, at the end and at
// every step on the way.
func TestMarkdownStreamReadsAsTheWhole(t *testing.T) {
	var s MarkdownStream
	for n := 1; n <= len(streamDoc); n += 7 {
		s.update(streamDoc[:n])
	}
	s.update(streamDoc)
	if got, want := streamBlocks(&s), parseMarkdown(streamDoc); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if s.at == 0 || len(s.settled) == 0 {
		t.Fatal("nothing settled: every change parses the whole text")
	}
	// A blank line inside the fence isn't a place to settle.
	var f MarkdownStream
	f.update("```\none\n\ntwo")
	if len(f.settled) != 0 || f.fence != "```" {
		t.Fatalf("settled inside a fence: %+v, fence %q", f.settled, f.fence)
	}
	// Text that isn't the old text and more (the final answer's) starts over.
	var r MarkdownStream
	r.update("one\n\ntwo")
	r.update("other")
	if got := streamBlocks(&r); len(got) != 1 || got[0].text != "other" {
		t.Fatalf("replaced: %+v", got)
	}
}

// A long answer costs each change about what its last part does.
func TestMarkdownStreamStaysCheap(t *testing.T) {
	var b strings.Builder
	for i := range 4000 {
		fmt.Fprintf(&b, "Paragraph %d of a long answer that the agent is still writing, with `code` in it.\n\n", i)
	}
	doc := b.String()
	var s MarkdownStream
	s.update(doc)
	const changes = 200
	start := time.Now()
	for i := range changes {
		s.update(doc + strings.Repeat("x", i+1))
	}
	if per := time.Since(start) / changes; per > 2*time.Millisecond {
		t.Fatalf("a change of a %d KB answer takes %s", len(doc)>>10, per)
	}
	if len(parsedCache) != 0 && parsedCache[doc] != nil {
		t.Fatal("the stream kept a copy of the text in parsedCache")
	}
}

// A long code block still arriving shows its end in its frame, whatever its width.
func TestLiveCodeKeepsItsEnd(t *testing.T) {
	code := strings.Repeat("line of code that repeats\n", 2000) + "the last line"
	var s MarkdownStream
	src := "```a-language-with-a-very-long-name-that-doesnt-fit-in-this-narrow-frame-at-all\n" + code
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Size(240, 400).Clip().Children(func() { s.Draw(c, src, nil) })
	}, 240, 400)
	tt.Frame()
	tt.Frame()
	found := false
	for _, x := range tt.Texts() {
		if len(x) <= liveCodeKeep && strings.HasSuffix(x, "the last line") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no end of the code among %d texts", len(tt.Texts()))
	}
}

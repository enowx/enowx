package widget

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The worst of long content, drawn frame after frame: ten megabytes of lines, and one line
// of a megabyte. A frame stays well under what the eye notices, whatever the text's length.
func TestLongTextStaysQuick(t *testing.T) {
	for name, text := range map[string]string{
		"10 MB of lines": strings.Repeat(strings.Repeat("x", 79)+"\n", 128<<10),
		"a 1 MB line":    strings.Repeat("y", 1<<20),
	} {
		var l LongText
		began := time.Now()
		l.Load(LongSource{ID: name, Size: len(text), Make: func() (string, string) { return text, "" }}, nil)
		prepared := time.Since(began)
		tt := ui.NewTester(func(c *ui.Context) {
			c.After(time.Millisecond) // a new frame each time: nothing here may be skipped
			ui.Column(c).Width(800).Height(600).Children(func() { l.Draw(c, LongOptions{}) })
		}, 800, 600)
		tt.Advance(16 * time.Millisecond)
		l.list.ScrollTo(len(l.rows)/2, ui.Start)
		began = time.Now()
		const frames = 30
		for range frames {
			tt.Advance(16 * time.Millisecond)
			tt.Image()
		}
		frame := time.Since(began) / frames
		t.Logf("%s: prepared in %v, %d rows, %v a frame", name, prepared, len(l.rows), frame)
		if frame > 50*time.Millisecond {
			t.Errorf("%s: %v a frame", name, frame)
		}
	}
}

func TestLongTextRows(t *testing.T) {
	rows := cutRows([]longLine{{text: "abcdefghij"}, {text: "ab"}, {text: "世界世界"}}, 4)
	var got []string
	lines := []string{"abcdefghij", "ab", "世界世界"}
	for _, r := range rows {
		got = append(got, lines[r.line][r.from:r.to])
	}
	if want := "abcd efgh ij ab 世界 世界"; strings.Join(got, " ") != want {
		t.Fatalf("rows %q, want %q", got, want)
	}
	spans := spansBetween([]Span{{Text: "abc"}, {Text: "defg"}, {Text: "hi"}}, 2, 5)
	if len(spans) != 2 || spans[0].Text != "c" || spans[1].Text != "de" {
		t.Fatalf("spans %+v", spans)
	}
}

// JSON cut off partway, as the start of a long body is, still lays out.
func TestIndentJSON(t *testing.T) {
	got := IndentJSON([]byte(`{"a":[1,{"b":"x,{y"}],"c":{},"d":"cut`))
	want := "{\n  \"a\": [\n    1,\n    {\n      \"b\": \"x,{y\"\n    }\n  ],\n  \"c\": {},\n  \"d\": \"cut"
	if got != want {
		t.Fatalf("%s", got)
	}
}

// A long code block, in a chat or a note, draws only what's in its window.
func TestLongCodeBlockStaysQuick(t *testing.T) {
	code := strings.Repeat("func main() { fmt.Println(\"hello\") }\n", 2000)
	tt := ui.NewTester(func(c *ui.Context) {
		c.After(time.Millisecond)
		ui.Scroll(c).Width(800).Height(700).Children(func() { CodeBlock(c, "go", code) })
	}, 800, 700)
	tt.Image()
	began := time.Now()
	const builds = 16
	tt.Advance(builds * time.Millisecond)
	tt.Image()
	took := time.Since(began) / builds
	t.Logf("%v a frame", took)
	if took > 16*time.Millisecond {
		t.Errorf("%v a frame", took)
	}
}

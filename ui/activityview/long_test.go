package activityview

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/framecheck"
)

// A prompt, a tool's name and details and a file's path of a megabyte each, an answer of ten,
// and hundreds of turns, cost a frame little: a row shows only so much of them.
func TestLongActivityStaysQuick(t *testing.T) {
	now := time.Now()
	var entries []proto.TranscriptEntry
	add := func(e proto.TranscriptEntry) {
		e.Index, e.Time = len(entries), now
		entries = append(entries, e)
	}
	line := framecheck.Line()
	add(proto.TranscriptEntry{Kind: proto.TranscriptUser, Text: line})
	add(proto.TranscriptEntry{Kind: proto.TranscriptTool, Text: line, Detail: line, Done: true})
	add(proto.TranscriptEntry{Kind: proto.TranscriptTool, Text: "Edit", Detail: "/work/app/" + line, Added: 1, Done: true})
	add(proto.TranscriptEntry{Kind: proto.TranscriptAssistant, Text: framecheck.Lines()})
	for range 400 {
		add(proto.TranscriptEntry{Kind: proto.TranscriptUser, Text: "Another turn"})
		add(proto.TranscriptEntry{Kind: proto.TranscriptTool, Text: "Read", Detail: "/work/app/main.go", Done: true})
		add(proto.TranscriptEntry{Kind: proto.TranscriptAssistant, Text: "Done."})
	}
	v := New()
	v.apply(proto.Transcript{Supported: true, Rev: 1, Entries: entries}, nil, "/work/app")
	v.open[0] = true
	tt := framecheck.New(t, "activity", func(c *ui.Context) {
		c.After(time.Millisecond)
		ui.Column(c).Width(1000).Height(700).Children(func() { v.Draw(c, Options{}) })
	}, 1000, 700)
	framecheck.Quick(t, "activity", tt)
}

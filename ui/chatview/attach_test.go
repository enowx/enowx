package chatview

import (
	"errors"
	"github.com/enowx/enowx/proto"
	"slices"
	"testing"
)

// A message waits for its attachments to upload, then goes with their
// paths; one that failed is dropped.
func TestMessageGoesWithItsAttachments(t *testing.T) {
	var sent string
	var files []string
	o := Options{Send: func(text string, f []string) { sent, files = text, f }}
	ch := New()
	done := ch.Attach("shot.png", nil)
	failed := ch.Attach("big.mov", nil)
	if ch.send(o, "look at this") {
		t.Fatal("sent while an attachment uploads")
	}
	done("/srv/shot.png", nil)
	failed("", errors.New("too large"))
	if !ch.send(o, "look at this\n") || sent != "look at this" || !slices.Equal(files, []string{"/srv/shot.png"}) {
		t.Fatalf("sent %q with %q", sent, files)
	}
	if len(ch.attachments) != 0 {
		t.Fatal("attachments kept after sending")
	}
	// An image alone is a message too.
	ch.apply(proto.Transcript{Supported: true, Rev: 1, Entries: []proto.TranscriptEntry{userEntry(0, "look at this /srv/shot.png")}})
	ch.Attach("only.png", nil)("/srv/only.png", nil)
	if !ch.send(o, "") || !slices.Equal(files, []string{"/srv/only.png"}) {
		t.Fatalf("image alone: %q", files)
	}
}

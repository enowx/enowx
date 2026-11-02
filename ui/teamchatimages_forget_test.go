package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Past the limit the images least recently shown go, a message's on its way staying.
func TestForgetChatImages(t *testing.T) {
	now, bmp := time.Now(), &ui.Bitmap{}
	images := map[string]*chatImage{
		"old":     {bmp: bmp, used: now.Add(-time.Hour)},
		"new":     {bmp: bmp, used: now},
		"out:1:0": {bmp: bmp, used: now.Add(-2 * time.Hour)},
	}
	forgetChatImages(images, func(id string) bool { return strings.HasPrefix(id, "out:") }, 1)
	if images["old"] != nil || images["new"] == nil || images["out:1:0"] == nil {
		t.Fatalf("left: %v", images)
	}
}

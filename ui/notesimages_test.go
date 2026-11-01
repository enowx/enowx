package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	myui "github.com/egoist/mygo/ui"
	"github.com/enowx/enowx/proto"
)

// An image a note shows from its own files (as an import brings them)
// shows on its page, read from its host.
func TestNoteShowsItsImages(t *testing.T) {
	h := newNotesHarness(t)
	n, err := h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Trip"}, Body: "Before\n"})
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	png.Encode(&buf, img)
	dir := filepath.Join(h.dir, "notes", "assets", n.ID)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "view.png"), buf.Bytes(), 0o644)
	full, _ := h.cl.Note(n.ID)
	full.Body = "Before\n\n![the view](assets/" + n.ID + "/view.png)\n"
	h.cl.SaveNote(full)
	if _, err := h.cl.NoteAsset("assets/" + n.ID + "/view.png"); err != nil {
		t.Fatalf("asset: %v", err)
	}
	h.settle("the page listed", func() bool { return h.tt.HasText("Trip") })
	h.tt.Click("Trip")
	h.settle("the image shown", func() bool {
		return h.tt.HasText("Before") && !h.tt.HasText("Loading the image…") && !h.tt.HasText("The image didn't load")
	})
	if h.tt.HasText("![the view]") {
		t.Error("the image shows as its Markdown")
	}
}

func TestNoteAddImageButton(t *testing.T) {
	h := newNotesHarness(t)
	n, err := h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Image upload"}, Body: "Before\n"})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := range 180 {
		for x := range 320 {
			img.Set(x, y, color.RGBA{uint8(80 + x/3), uint8(90 + y/2), 180, 255})
		}
	}
	png.Encode(&buf, img)
	h.tt = myui.NewTester(func(c *myui.Context) {
		o := h.app.notesOptions()
		o.PickImage = func(id string, done func(string, error)) {
			ref, err := h.cl.AddNoteImage(id, "shot.png", buf.Bytes())
			done(ref, err)
		}
		myui.Column(c).Width(1100).Height(700).Children(func() { h.app.notes.Draw(c, o) })
	}, 1100, 700)
	h.settle("note listed", func() bool { return h.tt.HasText("Image upload") })
	h.tt.Click("Image upload")
	h.settle("note open", func() bool { return h.tt.HasText("Before") })
	if err := h.tt.Click("Add image…"); err != nil {
		t.Fatal(err)
	}
	h.settle("image saved", func() bool {
		full, _ := h.cl.Note(n.ID)
		return strings.Contains(full.Body, "![Image](assets/"+n.ID+"/shot.png)")
	})
	h.settle("image loaded", func() bool { return !h.tt.HasText("Loading the image…") })
	if h.tt.HasText("The image didn't load") {
		t.Fatal("uploaded image did not render")
	}
	got, err := h.cl.NoteAsset("assets/" + n.ID + "/shot.png")
	if err != nil || !bytes.Equal(got, buf.Bytes()) {
		t.Fatalf("uploaded image not stored: %v", err)
	}
	shot(t, h.tt, "notes-image.png")
}

func TestNoteImageWithFullWindowsPath(t *testing.T) {
	h := newNotesHarness(t)
	n, err := h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Full path image"}})
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 80, 50)))
	ref, err := h.cl.AddNoteImage(n.ID, "icon.png", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	full, _ := h.cl.Note(n.ID)
	full.Body = "![Uploaded photo](<C:\\Users\\Julian Fernando/.enowx-app/notes/" + ref + ">) Caption\n"
	if _, err := h.cl.SaveNote(full); err != nil {
		t.Fatal(err)
	}
	h.settle("note listed", func() bool { return h.tt.HasText("Full path image") })
	h.tt.Click("Full path image")
	h.settle("image loaded", func() bool { return h.tt.HasText("Uploaded photo") && !h.tt.HasText("Loading the image…") })
	if h.tt.HasText("The image didn't load") || !h.tt.HasText("Caption") {
		t.Fatalf("full path image did not render: %q", h.tt.Texts())
	}
	if err := h.tt.Click("Mention Uploaded photo"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Key(0, myui.KeyEscape)
	h.tt.Frame()
	if err := h.tt.Click("@Uploaded photo"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if !h.tt.HasText("Close image") {
		t.Fatal("full path image mention did not open its preview")
	}
}

func TestNoteImageDeletionSavesBody(t *testing.T) {
	h := newNotesHarness(t)
	n, err := h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Delete picture"}})
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 80, 50)))
	ref, err := h.cl.AddNoteImage(n.ID, "icon.png", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	full, _ := h.cl.Note(n.ID)
	full.Body = ":::columns\n::col\n![Image](" + ref + ")\n::col\n- [ ] Keep this task\n:::\n"
	if _, err := h.cl.SaveNote(full); err != nil {
		t.Fatal(err)
	}
	h.settle("note listed", func() bool { return h.tt.HasText("Delete picture") })
	h.tt.Click("Delete picture")
	h.settle("image open", func() bool { return h.tt.HasText("Delete Image 1") && !h.tt.HasText("Loading the image…") })
	if err := h.tt.Click("Delete Image 1"); err != nil {
		t.Fatal(err)
	}
	h.settle("image deletion saved", func() bool {
		note, _ := h.cl.Note(n.ID)
		return !strings.Contains(note.Body, "![Image]") && note.Body == "- [ ] Keep this task\n"
	})
	h.tt.Click("Read")
	h.tt.Frame()
	if h.tt.HasText("Delete Image 1") || !h.tt.HasText("Keep this task") {
		t.Fatalf("deleted image stayed visible: %q", h.tt.Texts())
	}
	h.tt.Click("Edit")
	h.tt.Frame()
	if err := h.tt.Click("Undo edit"); err != nil {
		t.Fatal(err)
	}
	h.settle("image undo saved", func() bool {
		note, _ := h.cl.Note(n.ID)
		return strings.Contains(note.Body, "![Image]("+ref+")") && strings.Contains(note.Body, "Keep this task")
	})
	if err := h.tt.Click("Redo edit"); err != nil {
		t.Fatal(err)
	}
	h.settle("image redo saved", func() bool {
		note, _ := h.cl.Note(n.ID)
		return note.Body == "- [ ] Keep this task\n"
	})
}

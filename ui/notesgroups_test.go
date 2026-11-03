package ui

import (
	"net"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// notesHarness is the Notes page on a real daemon, its events applied
// between frames as the app's main thread would.
type notesHarness struct {
	t     *testing.T
	app   *App
	cl    *client.Client
	tt    *ui.Tester
	later []func()
	dir   string // the daemon's folder
}

func newNotesHarness(t *testing.T) *notesHarness {
	dir := t.TempDir()
	srv, err := daemon.NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a, b := net.Pipe()
	go srv.ServeConn(a)
	h := &notesHarness{t: t, app: New(Options{}), dir: dir}
	h.app.onMain = func(fn func()) { h.later = append(h.later, fn) }
	h.cl, err = client.New(b, func(ev proto.Event) {
		var apply func()
		switch ev.Name {
		case proto.EventNotesChanged:
			apply = h.app.notesEvent(ev.Data)
		case proto.EventNoteGroupsChanged:
			apply = h.app.groupsEvent(ev.Data)
		case proto.EventNoteProposalsChanged:
			apply = h.app.proposalsEvent(ev.Data)
		case proto.EventFeaturesRequested, proto.EventFeaturesChanged:
			h.app.handleEvent(h.app.gen, ev)
		}
		if apply != nil {
			h.app.onMain(apply)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.cl.Close() })
	h.app.client, h.app.page, h.app.host.Name = h.cl, pageNotes, "this Mac"
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1100).Height(700).Background(theme.Background).Children(func() { h.app.notesPage(c) })
	}, 1100, 700)
	h.settle("the notes listed", func() bool { return h.app.noteList.loaded })
	return h
}

// settle runs frames, and what came from the host, until done.
func (h *notesHarness) settle(what string, done func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		h.tt.Frame()
		if done() {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("%s: never; shows %q", what, h.tt.Texts())
		}
		time.Sleep(10 * time.Millisecond)
		for _, fn := range h.later {
			fn()
		}
		h.later = nil
	}
}

// drag drags from the element showing from onto the one showing to.
func (h *notesHarness) drag(from, to string) {
	h.t.Helper()
	f, ok := h.tt.Find(from)
	g, ok2 := h.tt.Find(to)
	if !ok || !ok2 {
		h.t.Fatalf("no %q or %q in %q", from, to, h.tt.Texts())
	}
	x0, y0 := f.X+f.W/2, f.Y+f.H/2
	x1, y1 := g.X+g.W/2, g.Y+g.H/2
	h.tt.Press(x0, y0)
	for k := 1; k <= 8; k++ {
		h.tt.Move(x0+(x1-x0)*float32(k)/8, y0+(y1-y0)*float32(k)/8)
		h.tt.Frame()
	}
	h.tt.Release(x1, y1)
	h.tt.Frame()
}

// Groups in the Notes page: one made from its dialog, a page dragged into
// it, folded and unfolded; deleting it keeps its pages.
func TestNotesGroupsInTheTree(t *testing.T) {
	h := newNotesHarness(t)
	idea, _ := h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Debloater"}})
	h.cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Groceries"}})
	h.settle("the pages", func() bool { return h.tt.HasText("Groceries") })

	if err := h.tt.Click("Import and export"); err != nil {
		t.Fatal(err)
	}
	h.settle("the menu", func() bool { return h.tt.HasText("New group…") })
	if err := h.tt.Click("New group…"); err != nil {
		t.Fatal(err)
	}
	h.settle("the dialog", func() bool { return h.tt.HasText("New group") && h.tt.HasText("Create") })
	h.tt.Type("Ideas")
	if err := h.tt.Click("Group icon"); err != nil {
		t.Fatal(err)
	}
	h.settle("the icon picker", func() bool { return h.tt.HasText("Library") })
	shot(t, h.tt, "notes-group-picker.png")
	if err := h.tt.Click("Search icons: rocket, bug, calendar…"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("lightbulb")
	h.settle("the bulb", func() bool { _, ok := h.tt.Find("Icon lucide:lightbulb"); return ok })
	if err := h.tt.Click("Icon lucide:lightbulb"); err != nil {
		t.Fatal(err)
	}
	h.settle("the picker closed", func() bool { return !h.tt.HasText("Library") })
	if err := h.tt.Click("Create"); err != nil {
		t.Fatal(err)
	}
	h.settle("the group", func() bool { return len(h.app.noteList.groups) == 1 && h.tt.HasText("No group") })
	if g := h.app.noteList.groups[0]; g.Icon != "lucide:lightbulb" {
		t.Fatalf("group icon %q", g.Icon)
	}

	h.drag("Debloater", "Group Ideas")
	h.settle("the page in the group", func() bool {
		for _, n := range h.app.noteList.list {
			if n.ID == idea.ID {
				return n.Group == "Ideas"
			}
		}
		return false
	})
	shot(t, h.tt, "notes-groups.png")

	if err := h.tt.Click("Group Ideas"); err != nil {
		t.Fatal(err)
	}
	h.settle("the group folded", func() bool { return h.app.noteList.groups[0].Folded && !h.tt.HasText("Debloater") })

	if err := h.tt.RightClick("Group Ideas"); err != nil {
		t.Fatal(err)
	}
	h.settle("the menu", func() bool { return h.tt.HasText("Delete group…") })
	if err := h.tt.Click("Delete group…"); err != nil {
		t.Fatal(err)
	}
	h.settle("the confirmation", func() bool { return h.tt.HasText("pages stay, in no group") })
	if err := h.tt.Click("Delete group"); err != nil {
		t.Fatal(err)
	}
	h.settle("the group gone, its page kept", func() bool {
		n, err := h.cl.Note(idea.ID)
		return err == nil && n.Group == "" && len(h.app.noteList.groups) == 0 && h.tt.HasText("Debloater")
	})
}

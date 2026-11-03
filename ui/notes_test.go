package ui

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// A page made on the Notes page is written as the user types it, a block
// at a time: a title, then Markdown's shortcuts making a heading, a to-do
// and a list, saved on the host as Markdown.
func TestNotesPageWritesAPage(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a, b := net.Pipe()
	go srv.ServeConn(a)
	app := New(Options{})
	var later mainQueue
	app.onMain = later.post
	cl, err := client.New(b, func(ev proto.Event) {
		if ev.Name == proto.EventNotesChanged {
			if apply := app.notesEvent(ev.Data); apply != nil {
				app.onMain(apply)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	app.client = cl
	app.host.Name = "this Mac"
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1000).Height(700).Background(theme.Background).Children(func() { app.notesPage(c) })
	}, 1000, 700)
	// settle runs frames and what came back from the host until done.
	settle := func(what string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			tt.Frame()
			if done() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: never; shows %q", what, tt.Texts())
			}
			time.Sleep(10 * time.Millisecond)
			later.run()
		}
	}
	settle("the notes listed", func() bool { return app.noteList.loaded })
	if err := tt.Click("New note"); err != nil {
		t.Fatal(err)
	}
	settle("a page open", func() bool { return tt.HasText("Page icon") })
	if err := tt.Click("Title"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Release plan")
	tt.Frame()
	tt.Key(0, ui.KeyEnter) // into the body
	tt.Frame()
	for _, line := range []string{"# Steps", "[] tag the release", "- changelog"} {
		for _, part := range []string{line[:2], line[2:]} {
			tt.Type(part)
			tt.Frame()
		}
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if strings.HasPrefix(line, "[]") || strings.HasPrefix(line, "-") {
			// A list goes on with Enter; Enter on its empty item ends it.
			tt.Key(0, ui.KeyEnter)
			tt.Frame()
		}
	}
	tt.Type("/quo")
	tt.Frame()
	if !tt.HasText("Quote") {
		t.Fatalf("no slash menu: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Type("ship it")
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	want := "# Steps\n- [ ] tag the release\n- changelog\n> ship it\n"
	var got proto.Note
	settle("the page saved", func() bool {
		list, _ := cl.Notes()
		if len(list) != 1 {
			return false
		}
		got, _ = cl.Note(list[0].ID)
		return got.Title == "Release plan" && got.Body == want
	})
	if !strings.Contains(strings.Join(tt.Texts(), "\n"), "tag the release") {
		t.Fatalf("the page doesn't show its to-do: %q", tt.Texts())
	}
	shot(t, tt, "notes-page.png")

	// A page with every kind of block, under the first.
	rich, err := cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Architecture", Icon: "lucide:brain", Parent: got.ID},
		Body: "# Overview\nThe **daemon** owns sessions; the app is a `client`.\n## Pieces\n1. Daemon\n2. App\n   \n- [x] Protocol\n- [ ] Notes sync\n  - [ ] mobile\n> Keep local and remote one path.\n---\n```go\nfunc main() {}\n```\n| Part | Owner |\n|---|---|\n| core | daemon |\n"})
	if err != nil {
		t.Fatal(err)
	}
	settle("the page listed", func() bool { return len(app.noteList.list) == 2 })
	app.notes.Select(app.notesOptions(), rich.ID)
	settle("the page open", func() bool { return tt.HasText("Overview") })
	shot(t, tt, "notes-rich.png")
}

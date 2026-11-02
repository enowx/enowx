//go:build !windows

package ui

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The Notes page's agent: a suggestion asked starts the note's agent on
// notes only; what it proposes shows above the page as lines changed,
// accepting makes the change, and Undo puts the page back.
func TestNotesAgentProposesAndTheUserAccepts(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a, b := net.Pipe()
	go srv.ServeConn(a)
	app := New(Options{})
	var later []func()
	app.onMain = func(fn func()) { later = append(later, fn) }
	cl, err := client.New(b, func(ev proto.Event) {
		var apply func()
		switch ev.Name {
		case proto.EventNotesChanged:
			apply = app.notesEvent(ev.Data)
		case proto.EventNoteProposalsChanged:
			apply = app.proposalsEvent(ev.Data)
		}
		if apply != nil {
			app.onMain(apply)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	app.client = cl
	app.page = pageNotes
	app.agents = []proto.AgentInfo{{ID: "claude", Name: "Claude Code", Command: "claude", Installed: true}}
	n, err := cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Ideas"}, Body: "buy milk\ncall mom\n"})
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		// The page draws the agent's panel itself, in its frame.
		ui.Row(c).Width(1200).Height(800).Gap(theme.BentoGap).Background(theme.Ground).Children(func() {
			widget.Bento(ui.Column(c).Grow(1).FillHeight(), theme.Background).Children(func() { app.notesPage(c) })
		})
	}, 1200, 800)
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
			for _, fn := range later {
				fn()
			}
			later = nil
		}
	}
	settle("the notes listed", func() bool { return app.noteList.loaded })
	app.notes.Select(app.notesOptions(), n.ID)
	settle("the page open", func() bool { return tt.HasText("Note agent") })
	if err := tt.Click("Note agent"); err != nil {
		t.Fatal(err)
	}
	settle("the panel", func() bool { return tt.HasText(notesSuggestions[0].title) })
	tt.Advance(time.Second) // the panel slides in
	tt.Frame()
	shot(t, tt, "notes-agent-panel.png")
	// The chat's own composer, Enter sending.
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Make it a checklist")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	var agentSession proto.SessionInfo
	settle("the agent started (status "+app.status+")", func() bool {
		if app.status != "" {
			t.Fatalf("status %q", app.status)
		}
		list, _ := cl.Sessions()
		for _, s := range list {
			if s.NotesFor == n.ID {
				agentSession = s
				return true
			}
		}
		return false
	})
	if agentSession.Agent != "claude" || agentSession.Project != "" || agentSession.Prompt != "Make it a checklist" {
		t.Fatalf("agent session %+v", agentSession)
	}

	// Until the agent records the conversation, the question shows as sent.
	settle("the question shown", func() bool { return tt.HasText("Make it a checklist") && !tt.HasText("Nothing yet") })

	// What the agent writes, through the notes tools in its session.
	if _, err := cl.ProposeNote(proto.Note{NoteInfo: proto.NoteInfo{ID: n.ID}, Body: "## Errands\n- [ ] buy milk\n- [ ] call mom\n"}, agentSession.ID); err != nil {
		t.Fatal(err)
	}
	settle("the proposal shown", func() bool {
		return tt.HasText("Claude Code proposes changes to this page") && tt.HasText("## Errands")
	})
	shot(t, tt, "notes-proposal.png")
	if got, _ := cl.Note(n.ID); got.Body != "buy milk\ncall mom\n" {
		t.Fatalf("changed before accepting: %q", got.Body)
	}
	if err := tt.Click("Accept"); err != nil {
		t.Fatal(err)
	}
	settle("the change made", func() bool {
		got, _ := cl.Note(n.ID)
		return got.Body == "## Errands\n- [ ] buy milk\n- [ ] call mom\n" && tt.HasText("Change accepted.")
	})
	shot(t, tt, "notes-accepted.png")
	if err := tt.Click("Undo"); err != nil {
		t.Fatal(err)
	}
	settle("the change undone", func() bool {
		got, _ := cl.Note(n.ID)
		return got.Body == "buy milk\ncall mom\n"
	})
}

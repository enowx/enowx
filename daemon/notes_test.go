package daemon

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// A note saved by one app (or an agent's CLI) reaches the others' lists.
func TestNotesSavedReachEveryApp(t *testing.T) {
	srv := startServer(t)
	events := make(chan []proto.NoteInfo, 4)
	connect(t, srv, func(e proto.Event) {
		var list []proto.NoteInfo
		if e.Name == proto.EventNotesChanged && json.Unmarshal(e.Data, &list) == nil {
			events <- list
		}
	})
	c := connect(t, srv, nil)
	info, err := c.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Plan"}, Body: "- [ ] ship"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case list := <-events:
		if len(list) != 1 || list[0].ID != info.ID || list[0].Title != "Plan" {
			t.Fatalf("event %+v", list)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no notes event")
	}
	n, err := c.Note(info.ID)
	if err != nil || n.Body != "- [ ] ship" {
		t.Fatalf("note %+v, %v", n, err)
	}
}

// A Notes page agent starts in a folder of its own on notes only: Claude
// Code with the notes guide, only enowX's notes tools and its file and
// shell tools off, its notes proposed.
func TestNotesAgentStartsOnNotesOnly(t *testing.T) {
	srv := startServer(t)
	c := connect(t, srv, nil)
	n, err := c.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Ideas"}})
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", NotesFor: n.ID, Prompt: "tidy it", Project: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if info.NotesFor != n.ID || info.Project != "" || !strings.HasSuffix(info.Dir, filepath.Join("notes-chat", n.ID)) {
		t.Fatalf("session %+v", info)
	}
	cmd := strings.Join(info.Command, "\x00") + sessionGuide(t, srv, info.ID)
	for _, want := range []string{agent.NotesGuide, "--strict-mcp-config", "mcp__enowx__notes_write", "--disallowedTools"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command lacks %q", want)
		}
	}
	if strings.Contains(cmd, agent.Guide) {
		t.Error("the notes agent got the sessions' guide")
	}
	l, err := launchFor(proto.SessionCreateParams{Agent: "claude", NotesFor: n.ID}, chatFor{notes: n.ID})
	if err != nil || !slices.Contains(l.env, proto.EnvNotesPropose+"=1") {
		t.Fatalf("env %q, %v", l.env, err)
	}
	if _, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", NotesFor: "000000000000"}); err == nil {
		t.Fatal("an agent for a note that isn't")
	}
}

package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/core/transcript"
	"github.com/enowx/enowx/proto"
)

func TestTranscriptFollowsTheSessionsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv := startServer(t)
	c := connect(t, srv, nil)
	dir := t.TempDir()
	// A stand-in for Claude Code: what matters is the session's agent and
	// folder.
	sess, err := srv.sessions.Create(session.Spec{Command: []string{"sleep", "30"}, Dir: dir, Meta: session.Meta{Agent: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	shell, err := srv.sessions.Create(session.Spec{Command: []string{"sleep", "30"}, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}

	got, err := c.Transcript(sess.ID(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Supported || len(got.Entries) != 0 {
		t.Fatalf("before any record: %+v", got)
	}
	if got, _ := c.Transcript(shell.ID(), 0); got.Supported {
		t.Fatalf("a shell session has a transcript: %+v", got)
	}

	folder := filepath.Join(home, ".claude", "projects", claudeFolderOf(dir))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	// A transcript from before the session isn't its own.
	old := filepath.Join(folder, "old.jsonl")
	if err := os.WriteFile(old, []byte(`{"type":"user","message":{"content":"old"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "new.jsonl"), []byte(`{"type":"user","message":{"content":"hello"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(lookEvery + 100*time.Millisecond)
	got, err = c.Transcript(sess.ID(), got.Rev)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Reset || len(got.Entries) != 1 || got.Entries[0].Text != "hello" {
		t.Fatalf("after the agent recorded: %+v", got)
	}
	again, _ := c.Transcript(sess.ID(), got.Rev)
	if len(again.Entries) != 0 || again.Reset {
		t.Fatalf("nothing changed, yet: %+v", again)
	}
}

func TestUnshownTranscriptIsDroppedAndReadAnew(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv := startServer(t)
	c := connect(t, srv, nil)
	dir := t.TempDir()
	sess, err := srv.sessions.Create(session.Spec{Command: []string{"sleep", "30"}, Dir: dir, Meta: session.Meta{Agent: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(home, ".claude", "projects", claudeFolderOf(dir))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	var lines []byte
	for range 10 {
		lines = append(lines, `{"type":"user","message":{"content":"hi"}}`+"\n"...)
	}
	if err := os.WriteFile(filepath.Join(folder, "s.jsonl"), lines, 0o600); err != nil {
		t.Fatal(err)
	}
	ask := func(since int64) proto.Transcript {
		got, err := c.TranscriptPage(proto.TranscriptParams{ID: sess.ID(), Since: since, Tail: 3})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	got := ask(0)
	if len(got.Entries) != 3 {
		t.Fatalf("first page: %d entries", len(got.Entries))
	}

	// Nobody asked for a while: the next chat to ask drops it.
	srv.transcripts.mu.Lock()
	srv.transcripts.byID[sess.ID()].used = time.Now().Add(-dropAfter - time.Second)
	srv.transcripts.mu.Unlock()
	other, err := srv.sessions.Create(session.Spec{Command: []string{"sleep", "30"}, Dir: t.TempDir(), Meta: session.Meta{Agent: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Transcript(other.ID(), 0); err != nil {
		t.Fatal(err)
	}
	srv.transcripts.mu.Lock()
	_, kept := srv.transcripts.byID[sess.ID()]
	srv.transcripts.mu.Unlock()
	if kept {
		t.Fatal("an unshown transcript stays in memory")
	}

	// Shown again, it starts over with the last page, not every entry.
	again := ask(got.Rev)
	if !again.Reset || len(again.Entries) != 3 {
		t.Fatalf("shown again: reset %v, %d entries", again.Reset, len(again.Entries))
	}
}

func claudeFolderOf(dir string) string {
	out := []byte(dir)
	for i, b := range out {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9') {
			out[i] = '-'
		}
	}
	return string(out)
}

// A project's past conversations list with their first message, and one
// picked goes on in a new session.
func TestHistoryListsConversationsAndResumesOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := gitRepo(t)
	folder := filepath.Join(home, ".claude", "projects", transcript.ClaudeFolder(repo))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	conv := `{"type":"user","uuid":"u1","cwd":"` + repo + `","message":{"content":"Add a dark theme\nwith tokens"}}
{"type":"assistant","uuid":"a1","parentUuid":"u1","message":{"model":"claude-opus-5-5","content":"Done."}}
`
	if err := os.WriteFile(filepath.Join(folder, "3f2a.jsonl"), []byte(conv), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\necho \"args: $*\"\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	c := connect(t, srv, nil)
	list, err := c.History(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "3f2a" || list[0].Title != "Add a dark theme" || list[0].Messages != 2 || list[0].Branch != "main" || list[0].Model != "claude-opus-5-5" {
		t.Fatalf("history %+v", list)
	}
	info, err := c.CreateSession(proto.SessionCreateParams{Project: repo, Agent: "claude", Resume: list[0].ID, Cols: 120, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the agent resuming", func() bool {
		text, _ := c.Screen(info.ID)
		return strings.Contains(text, "--resume 3f2a")
	})
}

// A conversation lets go of memory with no chat left to ask for another.
func TestTranscriptDroppedWithNoChatAsking(t *testing.T) {
	var ts transcripts
	ts.follower("a").used = time.Now().Add(-dropAfter - time.Second)
	ts.follower("b")
	ts.mu.Lock()
	ts.dropLocked(time.Now())
	_, a := ts.byID["a"]
	_, b := ts.byID["b"]
	ts.mu.Unlock()
	if a || !b {
		t.Fatalf("a kept %v, b kept %v", a, b)
	}
}

// Asking for the file of an agent's conversation, as saving a session does every 20 s,
// reads none of it, and doesn't keep a chat's copy from being let go.
func TestTranscriptPathReadsNothing(t *testing.T) {
	srv := startServer(t)
	t.Setenv("HOME", t.TempDir())
	home, _ := os.UserHomeDir()
	dir := t.TempDir()
	sess, err := srv.sessions.Create(session.Spec{Command: []string{"sleep", "30"}, Dir: dir, Meta: session.Meta{Agent: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(home, ".claude", "projects", transcript.ClaudeFolder(dir))
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(folder, "11111111-1111-4111-8111-111111111111.jsonl")
	line := `{"type":"user","timestamp":"2026-10-22T06:00:00Z","message":{"role":"user","content":"hello"}}` + "\n"
	if err := os.WriteFile(file, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := srv.transcriptPath(sess.ID()); got != file {
		t.Fatalf("path %q, want %q", got, file)
	}
	srv.transcripts.mu.Lock()
	f := srv.transcripts.byID[sess.ID()]
	srv.transcripts.mu.Unlock()
	f.mu.Lock()
	entries := f.reader.Len()
	f.mu.Unlock()
	if entries != 0 {
		t.Fatalf("%d entries read in", entries)
	}
	// A chat that had read the conversation before it was let go is told to start over.
	back, err := srv.transcript(proto.TranscriptParams{ID: sess.ID(), Since: 5})
	if err != nil || !back.Reset || len(back.Entries) != 1 {
		t.Fatalf("%+v, %v", back, err)
	}
	srv.transcripts.mu.Lock()
	f.used = time.Now().Add(-dropAfter - time.Second)
	srv.transcripts.mu.Unlock()
	if got := srv.transcriptPath(sess.ID()); got != file {
		t.Fatalf("path %q the second time", got)
	}
	srv.transcripts.mu.Lock()
	srv.transcripts.dropLocked(time.Now())
	_, kept := srv.transcripts.byID[sess.ID()]
	srv.transcripts.mu.Unlock()
	if kept {
		t.Fatal("asking for the path kept it from being let go")
	}
}

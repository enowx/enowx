package ui

import (
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/chatview"
)

// chatAuditPages are a session's chat in the audit: a conversation, working,
// asking, and beside the side panel, where the composer goes compact. The
// chat is seeded: the audit's session runs no agent that records one.
func chatAuditPages() []auditPage {
	at := time.Now().Add(-5 * time.Minute)
	long := strings.Repeat("A line of words the bubble has to wrap, and no more than it can hold. ", 4)
	path := "/a/folder/with/a/rather/long/name/that/keeps/going/on/and/on/and/on/and/on/and/on/and/on/ui/chatview/composer.go"
	done := []proto.TranscriptEntry{
		{Index: 0, Kind: proto.TranscriptUser, Text: long, Time: at},
		{Index: 1, Kind: proto.TranscriptThinking, Text: "Where does it go?", DurationMs: 3000, Time: at.Add(time.Second)},
		{Index: 2, Kind: proto.TranscriptTool, Text: "Edit", Detail: path, Added: 12044, Removed: 3211, Done: true, DurationMs: 900, Result: "ok", Old: "a", New: "b"},
		{Index: 3, Kind: proto.TranscriptTool, Text: "Bash", Detail: "go test ./ui/chatview", Done: true, Failed: true, Result: "FAIL"},
		{Index: 4, Kind: proto.TranscriptAssistant, Text: long, Time: at.Add(time.Minute)},
	}
	chat := func(state string, entries []proto.TranscriptEntry, panel bool) func(a *App) {
		return func(a *App) {
			a.page, a.state.Panel.Open = "", false
			s := a.sessions[0]
			s.Agent, s.State, s.Exited = "claude", state, false
			a.upsert(s)
			a.selectSession(s)
			if a.state.Chats == nil {
				a.state.Chats = map[string]bool{}
			}
			a.state.Chats[s.ID] = true
			ch := chatview.New()
			ch.Seed(proto.Transcript{Supported: true, Entries: entries, Model: "opus", Mode: "default", Effort: "high", ContextTokens: 120_000, ContextWindow: 200_000})
			ch.InsertDraft("A draft to send")
			a.chats = map[string]*chatview.Chat{s.ID: ch}
			if panel {
				a.openPanel(panelChanges)
			}
		}
	}
	working := append(append([]proto.TranscriptEntry{}, done...), proto.TranscriptEntry{Index: 5, Kind: proto.TranscriptTool, Text: "Edit", Detail: path})
	return []auditPage{
		{name: "chat-done", open: chat("", done, false)},
		{name: "chat-working", open: chat(proto.StateWorking, working, false)},
		{name: "chat-asking", open: chat(proto.StateWaiting, working, false)},
		{name: "chat-panel", open: chat(proto.StateWorking, working, true)},
	}
}

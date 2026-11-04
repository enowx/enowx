package daemon

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// A note's agent asked from anywhere (the Notes page, another agent's
// tools): its conversation running here takes the prompt, or a new one
// starts; each turn it ends is kept with the note (notes/chats), synced
// with the notes.

// notesSessionOf is note id's newest conversation running here with
// agent ("" for any).
func (s *Server) notesSessionOf(id, agent string) (proto.SessionInfo, bool) {
	list := s.sessions.List()
	for i := len(list) - 1; i >= 0; i-- {
		info := list[i].Info()
		if info.NotesFor == id && !info.Exited && (agent == "" || info.Agent == agent) {
			return info, true
		}
	}
	return proto.SessionInfo{}, false
}

// noteChat asks note p.ID's agent p.Prompt.
func (s *Server) noteChat(p proto.NoteChatParams) (proto.NoteChatLog, error) {
	n, err := s.notesFor(p.ID).Get(p.ID)
	if err != nil {
		return proto.NoteChatLog{}, err
	}
	if strings.TrimSpace(p.Prompt) == "" {
		return proto.NoteChatLog{}, errors.New("notes: ask the note's agent something")
	}
	agent := p.Agent
	if agent == "" {
		agent = n.Agent
	}
	if agent == "" {
		agent = "claude"
	}
	if p.Agent != "" && p.Agent != n.Agent {
		if _, err := s.notesFor(n.ID).SetAgent(n.ID, p.Agent); err == nil {
			s.notesChanged()
		}
	}
	if p.New {
		s.forgetHelperConvo("", n.ID, agent)
	}
	session := ""
	if info, ok := s.notesSessionOf(n.ID, agent); ok && !p.New {
		session = info.ID
		if _, err := s.wakeThen(info.ID, func() error {
			return s.input(proto.SessionInputParams{ID: info.ID, Text: p.Prompt, Paste: true, Enter: true})
		}); err != nil {
			return proto.NoteChatLog{}, err
		}
	} else {
		sess, err := s.createSession(proto.SessionCreateParams{Agent: agent, NotesFor: n.ID, Prompt: p.Prompt,
			Cols: 100, Rows: 40, Model: p.Model, Effort: p.Effort})
		if err != nil {
			return proto.NoteChatLog{}, err
		}
		session = sess.ID()
	}
	// What was asked shows at once; the agent's transcript replaces it
	// once it has the turn.
	asked := proto.NoteChatMessage{Time: time.Now().UTC(), Role: proto.TranscriptUser, Text: p.Prompt, Agent: agent, Session: session, Host: s.userdata.Name}
	old, _ := s.notesFor(n.ID).ChatLog(n.ID)
	var mine []proto.NoteChatMessage
	for _, m := range old.Messages {
		if m.Session == session {
			mine = append(mine, m)
		}
	}
	log, err := s.notesFor(n.ID).KeepChat(n.ID, session, append(mine, asked))
	log.Session = session
	return log, err
}

// keepNoteChat keeps note id's conversation of session, as its agent
// recorded it.
func (s *Server) keepNoteChat(id, session string) {
	t, err := s.transcript(proto.TranscriptParams{ID: session})
	if err != nil || !t.Supported {
		return
	}
	sess, err := s.sessions.Get(session)
	if err != nil {
		return
	}
	agent := sess.Info().Agent
	var msgs []proto.NoteChatMessage
	for _, e := range t.Entries {
		if e.Kind != proto.TranscriptUser && e.Kind != proto.TranscriptAssistant || strings.TrimSpace(e.Text) == "" {
			continue
		}
		at := e.Time
		if at.IsZero() {
			at = time.Now().UTC()
		}
		msgs = append(msgs, proto.NoteChatMessage{Time: at.UTC(), Role: e.Kind, Text: e.Text, Agent: agent, Session: session, Host: s.userdata.Name})
	}
	if len(msgs) == 0 {
		return
	}
	if log, err := s.notesFor(id).KeepChat(id, session, msgs); err == nil {
		s.broadcast(proto.EventNoteChat, log)
	}
}

// noteChatLog is note id's conversations, those running here brought up
// to date first.
func (s *Server) noteChatLog(id string) (proto.NoteChatLog, error) {
	if _, err := s.notesFor(id).Get(id); err != nil {
		return proto.NoteChatLog{}, err
	}
	for _, sess := range s.sessions.List() {
		if info := sess.Info(); info.NotesFor == id {
			s.keepNoteChat(id, info.ID)
		}
	}
	log, err := s.notesFor(id).ChatLog(id)
	if info, ok := s.notesSessionOf(id, ""); ok {
		log.Session = info.ID
	}
	slices.SortStableFunc(log.Messages, func(a, b proto.NoteChatMessage) int { return a.Time.Compare(b.Time) })
	return log, err
}

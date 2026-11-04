package platformagent

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/core/platformagent/record"
)

// replayed is what a conversation's file says.
type replayed struct {
	found               bool // the file held a Meta
	model, effort, mode string
	budget              float64
	rules               []string
	history             []message
	total               float64
	used                int64
	lastPrompt          string
	dangling            []toolCall // calls of the last answer with no results
}

// openConversation opens the file Options say, a new one unless Resume or Continue find
// one, and reads it back.
func (a *Agent) openConversation() (replayed, error) {
	o := a.opts
	id := o.Resume
	if id != "" && (strings.ContainsAny(id, `/\`) || id != filepath.Base(id)) {
		return replayed{}, fmt.Errorf("platformagent: %q is no conversation ID", id)
	}
	if id == "" && o.Continue {
		newest, err := newestConversation(o.SessionDir)
		if err != nil {
			return replayed{}, err
		}
		id = newest
	}

	var saved replayed
	if id != "" {
		path := conversationPath(o.SessionDir, id)
		r, err := replayFile(path)
		if err != nil {
			return replayed{}, fmt.Errorf("platformagent: resume %s: %w", id, err)
		}
		saved = r
	} else {
		fresh, err := newConversationID()
		if err != nil {
			return replayed{}, err
		}
		id = fresh
	}

	file, err := openConvFile(conversationPath(o.SessionDir, id))
	if err != nil {
		return replayed{}, err
	}
	a.conv, a.file = id, file
	return saved, nil
}

// newestConversation is the ID of the file last written to, "" with none.
func newestConversation(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("platformagent: session folder: %w", err)
	}
	best, bestTime := "", int64(0)
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".jsonl")
		if !ok || e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if t := info.ModTime().UnixNano(); t > bestTime {
			best, bestTime = id, t
		}
	}
	return best, nil
}

// replayFile reads records back: from the last Compact on, the message-bearing ones not
// a subagent's are the history. A line cut by a crash is skipped.
func replayFile(path string) (replayed, error) {
	f, err := os.Open(path)
	if err != nil {
		return replayed{}, err
	}
	defer f.Close()

	var s replayed
	// Attachments make long lines, so no bufio.Scanner.
	in := bufio.NewReader(f)
	for {
		line, err := in.ReadBytes('\n')
		if len(line) > 0 {
			var r record.Record
			if json.Unmarshal(line, &r) == nil {
				s.apply(r)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return replayed{}, err
		}
	}
	s.dangling = danglingCalls(s.history)
	return s, nil
}

func (s *replayed) apply(r record.Record) {
	if r.TotalCost > 0 {
		s.total = r.TotalCost
	}
	if r.Type == record.Allow {
		s.rules = append(s.rules, r.Rule)
	}
	if r.Sidechain != "" {
		return
	}
	switch r.Type {
	case record.Meta:
		s.found = true
		s.model, s.effort, s.mode, s.budget = r.Model, r.Effort, r.Mode, r.Budget
	case record.Compact:
		s.history = s.history[:0]
		for _, raw := range r.Messages {
			s.history = append(s.history, message{role: "user", raw: raw})
		}
		s.used = 0
	case record.User:
		if r.Message != nil {
			s.history = append(s.history, message{role: "user", raw: r.Message})
			s.lastPrompt = r.Text
		}
	case record.Results:
		if r.Message != nil {
			s.history = append(s.history, message{role: "user", raw: r.Message})
		}
	case record.Assistant:
		if r.Message != nil {
			s.history = append(s.history, message{role: "assistant", raw: r.Message})
		}
		if r.Usage != nil {
			s.used = contextTokens(*r.Usage)
		}
	}
}

// danglingCalls are the tool calls of a final answer no results followed.
func danglingCalls(history []message) []toolCall {
	if len(history) == 0 || history[len(history)-1].role != "assistant" {
		return nil
	}
	var m struct {
		Content []struct {
			Type  string          `json:"type"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if json.Unmarshal(history[len(history)-1].raw, &m) != nil {
		return nil
	}
	var calls []toolCall
	for _, b := range m.Content {
		if b.Type == "tool_use" {
			calls = append(calls, toolCall{ID: b.ID, Name: b.Name, Input: b.Input})
		}
	}
	return calls
}

package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/platformagent/record"
)

// Found is a conversation an agent recorded, by the path a Reader opens it
// with, and when it last changed.
type Found struct {
	Path    string
	Changed time.Time
}

// readers are the agents whose conversations this package reads.
var readers = map[string]struct {
	find func(home, dir string) []Found
	open func(path string) *Reader
	id   func(path string) string // the id the agent resumes it by; nil: none
}{
	"claude":             {claudeFound, NewClaudeReader, claudeID},
	"claude-agent-teams": {claudeFound, NewClaudeReader, claudeID},
	"codex":              {CodexFiles, NewCodexReader, codexID},
	"omp":                {OMPFiles, NewOMPReader, OMPConversationID},
	record.AgentID:       {PlatformFiles, NewPlatformReader, claudeID},
}

func claudeID(path string) string { return strings.TrimSuffix(filepath.Base(path), ".jsonl") }

// ConversationID is the id agent resumes the conversation at path by, ""
// when it has none to give.
func ConversationID(agent, path string) string {
	if r, ok := readers[agent]; ok && r.id != nil && path != "" {
		return r.id(path)
	}
	return ""
}

// Supports reports whether the conversations of agent (a preset ID) can
// be read.
func Supports(agent string) bool {
	_, ok := readers[agent]
	return ok
}

// Stored lists the conversations an agent kept in store, a session's own folder
// (Preset.SessionDir), newest change first; nil when it keeps none there.
func Stored(store string) []Found {
	if store == "" {
		return nil
	}
	var out []Found
	entries, _ := os.ReadDir(store)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, Found{Path: filepath.Join(store, e.Name()), Changed: info.ModTime()})
		}
	}
	slices.SortFunc(out, func(a, b Found) int { return b.Changed.Compare(a.Changed) })
	return out
}

// Started is when the conversation at path began, as its first record's time says:
// Claude Code's and Codex's timestamp, OMP's and Claude Platform's. ok is false when no
// record says it (an older or a hand-written file), and the caller goes by the file's
// change instead.
func Started(agent, path string) (time.Time, bool) {
	if !Supports(agent) {
		return time.Time{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for i := 0; i < 40 && sc.Scan(); i++ {
		var rec struct {
			Timestamp time.Time `json:"timestamp"`
			Time      time.Time `json:"time"`
		}
		if json.Unmarshal(sc.Bytes(), &rec) != nil {
			continue
		}
		if !rec.Timestamp.IsZero() {
			return rec.Timestamp, true
		}
		if !rec.Time.IsZero() {
			return rec.Time, true
		}
	}
	return time.Time{}, false
}

// HomeOf is where agent keeps its files under home, "" for an agent with none known here.
func HomeOf(agent, home string) string {
	switch agent {
	case "claude", "claude-agent-teams":
		return ClaudeHome(home)
	case "codex":
		return codexHome(home)
	}
	return ""
}

// Find lists the conversations agent had in dir, newest change first.
func Find(agent, home, dir string) []Found {
	if r, ok := readers[agent]; ok {
		return r.find(home, dir)
	}
	return nil
}

// Open follows a conversation Find listed for agent; nil for an agent it
// doesn't read.
func Open(agent, path string) *Reader {
	if r, ok := readers[agent]; ok {
		return r.open(path)
	}
	return nil
}

func claudeFound(home, dir string) []Found {
	files := ClaudeFiles(home, dir)
	out := make([]Found, len(files))
	for i, f := range files {
		out[i] = Found{Path: f, Changed: ModTime(f)}
	}
	return out
}

// ForkID is the id agent forks the conversation at path by, "" when it has
// none to give: the one it resumes by.
func ForkID(agent, path string) string { return ConversationID(agent, path) }

// Compacted says the conversation at path holds a compaction, as Claude Code,
// Codex and Claude Platform record one.
func Compacted(agent, path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		if !bytes.Contains(line, []byte("ompact")) {
			continue
		}
		var rec struct {
			Type    string `json:"type"`
			Summary bool   `json:"isCompactSummary"`
		}
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		if rec.Summary || agent == "codex" && rec.Type == "compacted" || agent == record.AgentID && rec.Type == record.Compact {
			return true
		}
	}
	return false
}

// codexID is the session id in a rollout's first lines.
func codexID(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for i := 0; i < 5 && sc.Scan(); i++ {
		var rec struct {
			Type    string `json:"type"`
			Payload struct {
				ID string `json:"id"`
			} `json:"payload"`
		}
		if json.Unmarshal(sc.Bytes(), &rec) == nil && rec.Type == "session_meta" {
			return rec.Payload.ID
		}
	}
	return ""
}

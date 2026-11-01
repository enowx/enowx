package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// Claude Code records each session as JSON lines in
// ~/.claude/projects/<folder>/<session>.jsonl: user and assistant records
// whose message content is text or blocks (text, thinking, tool_use,
// tool_result). Records marked isMeta or isSidechain (subagents) aren't
// the conversation. See docs/chat-view.md.

// ClaudeFiles returns the transcripts Claude Code keeps for sessions run in
// dir, newest first.
func ClaudeFiles(home, dir string) []string {
	root := filepath.Join(home, ".claude", "projects")
	var folders []string
	if f := filepath.Join(root, ClaudeFolder(dir)); isDir(f) {
		folders = append(folders, f)
	} else {
		// The folder name is derived from the path; when it isn't where
		// expected, find the folder whose records ran in dir.
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			f := filepath.Join(root, e.Name())
			if e.IsDir() && claudeFolderRunsIn(f, dir) {
				folders = append(folders, f)
			}
		}
	}
	type file struct {
		path string
		mod  time.Time
	}
	var files []file
	for _, folder := range folders {
		entries, _ := os.ReadDir(folder)
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
				continue
			}
			if info, err := e.Info(); err == nil {
				files = append(files, file{filepath.Join(folder, e.Name()), info.ModTime()})
			}
		}
	}
	out := make([]string, 0, len(files))
	for len(files) > 0 {
		newest := 0
		for i, f := range files {
			if f.mod.After(files[newest].mod) {
				newest = i
			}
		}
		out = append(out, files[newest].path)
		files = append(files[:newest], files[newest+1:]...)
	}
	return out
}

var notNameChar = regexp.MustCompile(`[^A-Za-z0-9]`)

// claudeFolder is how Claude Code names the folder of a working directory.
// ClaudeFolder is the folder of ~/.claude/projects where Claude Code keeps
// the conversations it has in dir.
func ClaudeFolder(dir string) string { return notNameChar.ReplaceAllString(dir, "-") }

func claudeFolderRunsIn(folder, dir string) bool {
	entries, _ := os.ReadDir(folder)
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		f, err := os.Open(filepath.Join(folder, e.Name()))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 4<<20)
		for i := 0; i < 20 && sc.Scan(); i++ {
			var rec struct {
				Cwd string `json:"cwd"`
			}
			if json.Unmarshal(sc.Bytes(), &rec) == nil && rec.Cwd != "" {
				f.Close()
				return rec.Cwd == dir
			}
		}
		f.Close()
	}
	return false
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// ModTime is when the file last changed, zero when it is missing.
func ModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// NewClaudeReader follows a Claude Code transcript.
func NewClaudeReader(path string) *Reader { return newReader(path, newClaudeParser()) }

// claudeParser follows the branch of the conversation Claude Code shows.
// Records chain by parentUuid; when the user rewinds and sends a message
// again, the new message's parent is a record before the messages it
// replaces, which stay in the file.
// modelSet is /model's answer in a user record: "Set model to Haiku 5.5", its
// name maybe in backticks or a terminal's bold.
var modelSet = regexp.MustCompile(`local-command-stdout>Set model to (?:\\u001b\[\d+m|\x60)*([\w .()-]+?)(?:\\u001b|\x60| and | for |<|\\n)`)

type claudeParser struct {
	at   map[string]int // record → how many entries there were after it
	last string         // the record read last
}

func newClaudeParser() *claudeParser { return &claudeParser{at: map[string]int{}} }

func (*claudeParser) fresh() parser { return newClaudeParser() }

type claudeRecord struct {
	Type           string    `json:"type"`
	UUID           string    `json:"uuid"`
	ParentUUID     string    `json:"parentUuid"`
	IsMeta         bool      `json:"isMeta"`
	IsSidechain    bool      `json:"isSidechain"`
	Timestamp      time.Time `json:"timestamp"`
	PermissionMode string    `json:"permissionMode"`
	Effort         string    `json:"effort"`
	ThinkingMs     int64     `json:"thinkingDurationMs"`
	Message        struct {
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			Input       int64 `json:"input_tokens"`
			CacheRead   int64 `json:"cache_read_input_tokens"`
			CacheCreate int64 `json:"cache_creation_input_tokens"`
			Output      int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

func (p *claudeParser) parse(line []byte, r *Reader) {
	var rec claudeRecord
	if json.Unmarshal(line, &rec) != nil || rec.IsSidechain {
		return
	}
	if rec.UUID != "" {
		p.branch(r, rec)
		defer func() { p.at[rec.UUID], p.last = r.Len(), rec.UUID }()
	}
	// The settings come on records of their own and on messages.
	if rec.PermissionMode != "" {
		r.Mode = rec.PermissionMode
	}
	if rec.Type == "assistant" {
		if rec.Message.Model != "" && !strings.HasPrefix(rec.Message.Model, "<") {
			r.Model = rec.Message.Model
		}
		if rec.Effort != "" {
			r.Effort = rec.Effort
		}
		// What the model read for this answer, and what it wrote, is what
		// the next turn starts from.
		if u := rec.Message.Usage; u != nil {
			if n := u.Input + u.CacheRead + u.CacheCreate + u.Output; n > 0 {
				r.Context = n
			}
		}
	}
	// /model answers before the next message says it.
	if rec.Type == "user" {
		if m := modelSet.FindSubmatch(line); m != nil {
			r.Model = string(bytes.TrimSpace(m[1]))
		}
	}
	if rec.IsMeta {
		return
	}
	if rec.Type != "user" && rec.Type != "assistant" {
		return
	}
	var text string
	if json.Unmarshal(rec.Message.Content, &text) == nil {
		addClaudeText(r, rec, text)
		return
	}
	var blocks []claudeBlock
	if json.Unmarshal(rec.Message.Content, &blocks) != nil {
		return
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			addClaudeText(r, rec, b.Text)
		case "thinking":
			if strings.TrimSpace(b.Thinking) != "" {
				r.add(proto.TranscriptEntry{Kind: proto.TranscriptThinking, Time: rec.Timestamp, Text: b.Thinking, DurationMs: rec.ThinkingMs})
			}
		case "tool_use":
			e := proto.TranscriptEntry{Kind: proto.TranscriptTool, Time: rec.Timestamp, Text: b.Name, Detail: toolDetail(b.Input)}
			if b.Name == "AskUserQuestion" {
				e.Questions = toolQuestions(b.Input)
				if len(e.Questions) > 0 {
					e.Detail = clip(e.Questions[0].Question, 300)
				}
			}
			toolEdit(&e, b.Input)
			i := r.add(e)
			r.tools[b.ID] = i
		case "tool_result":
			i, ok := r.tools[b.ToolUseID]
			if !ok {
				continue
			}
			result := clip(strings.TrimSpace(blockText(b.Content)), 4000)
			r.change(i, func(e *proto.TranscriptEntry) {
				e.Done, e.Failed, e.Result = true, b.IsError, result
				e.DurationMs = tookMs(e.Time, rec.Timestamp)
			})
		}
	}
}

// branch drops what a message sent again after a rewind replaces: a
// message of the user's whose parent comes before the user's last message
// shown. Tool results also point back, to their call, but never past a
// message of the user's.
func (p *claudeParser) branch(r *Reader, rec claudeRecord) {
	if rec.Type != "user" || rec.IsMeta || rec.ParentUUID == "" || rec.ParentUUID == p.last {
		return
	}
	n, ok := p.at[rec.ParentUUID]
	if !ok || !isPrompt(rec.Message.Content) {
		return
	}
	for i := n; i < r.Len(); i++ {
		if r.entries[i].Kind == proto.TranscriptUser {
			r.truncate(n)
			return
		}
	}
}

// isPrompt reports whether a user record's content is a message typed,
// rather than tool results.
func isPrompt(content json.RawMessage) bool {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return true
	}
	var blocks []claudeBlock
	if json.Unmarshal(content, &blocks) != nil {
		return false
	}
	for _, b := range blocks {
		if b.Type == "text" {
			return true
		}
	}
	return false
}

var commandName = regexp.MustCompile(`<command-name>([^<]*)</command-name>`)

func addClaudeText(r *Reader, rec claudeRecord, text string) {
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		return
	case strings.HasPrefix(text, "<command-name>") || strings.HasPrefix(text, "<command-message>"):
		if m := commandName.FindStringSubmatch(text); m != nil {
			r.add(proto.TranscriptEntry{Kind: proto.TranscriptNotice, Time: rec.Timestamp, Text: "Ran " + m[1]})
		}
		return
	case strings.HasPrefix(text, "<local-command-stdout>") || strings.HasPrefix(text, "<local-command-stderr>"),
		strings.HasPrefix(text, "Caveat: The messages below were generated by the user while running local commands"):
		return
	case strings.HasPrefix(text, "[Request interrupted"):
		r.add(proto.TranscriptEntry{Kind: proto.TranscriptNotice, Time: rec.Timestamp, Text: "Interrupted"})
		return
	}
	kind := proto.TranscriptUser
	if rec.Type == "assistant" {
		kind = proto.TranscriptAssistant
	}
	r.add(proto.TranscriptEntry{Kind: kind, Time: rec.Timestamp, Text: text})
}

// toolDetail sums up what a tool call does from its input.
func toolDetail(input json.RawMessage) string {
	var in map[string]any
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	for _, key := range []string{"command", "file_path", "path", "pattern", "url", "query", "description", "prompt"} {
		if v, ok := in[key].(string); ok && strings.TrimSpace(v) != "" {
			return clip(strings.TrimSpace(v), 300)
		}
	}
	raw, _ := json.Marshal(in)
	return clip(string(raw), 300)
}

// toolQuestions are the questions an AskUserQuestion call asks.
func toolQuestions(input json.RawMessage) []proto.TranscriptQuestion {
	var in struct {
		Questions []struct {
			Question string `json:"question"`
			Header   string `json:"header"`
			Multi    bool   `json:"multiSelect"`
			Options  []struct {
				Label       string `json:"label"`
				Description string `json:"description"`
			} `json:"options"`
		} `json:"questions"`
	}
	if json.Unmarshal(input, &in) != nil {
		return nil
	}
	var out []proto.TranscriptQuestion
	for _, q := range in.Questions {
		tq := proto.TranscriptQuestion{Header: q.Header, Question: q.Question, Multi: q.Multi}
		for _, o := range q.Options {
			tq.Options = append(tq.Options, o.Label)
			tq.About = append(tq.About, o.Description)
		}
		out = append(out, tq)
	}
	return out
}

// toolEdit fills e with the change a file-editing call makes: Edit and
// MultiEdit replace text, Write writes a whole file.
func toolEdit(e *proto.TranscriptEntry, input json.RawMessage) {
	var in struct {
		Old     string  `json:"old_string"`
		New     string  `json:"new_string"`
		Content *string `json:"content"`
		Edits   []struct {
			Old string `json:"old_string"`
			New string `json:"new_string"`
		} `json:"edits"`
	}
	if json.Unmarshal(input, &in) != nil {
		return
	}
	old, repl := in.Old, in.New
	switch {
	case len(in.Edits) > 0:
		var olds, news []string
		for _, ed := range in.Edits {
			olds, news = append(olds, ed.Old), append(news, ed.New)
		}
		old, repl = strings.Join(olds, "\n"), strings.Join(news, "\n")
	case in.Content != nil && e.Text == "Write":
		old, repl = "", *in.Content
	case old == "" && repl == "":
		return
	}
	e.Added, e.Removed = lineChanges(old, repl)
	e.Old, e.New = clip(old, 3000), clip(repl, 3000)
}

// lineChanges counts the lines that new adds to old and those it drops,
// matching equal lines whatever their order.
func lineChanges(old, repl string) (added, removed int) {
	have := map[string]int{}
	if old != "" {
		for _, l := range strings.Split(old, "\n") {
			have[l]++
		}
	}
	if repl != "" {
		for _, l := range strings.Split(repl, "\n") {
			if have[l] > 0 {
				have[l]--
			} else {
				added++
			}
		}
	}
	for _, n := range have {
		removed += n
	}
	return added, removed
}

// blockText is the text of a tool result: a string or text blocks.
func blockText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []claudeBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// tookMs is how long a tool call took, from its call to its result, 0
// when either time is unknown.
func tookMs(call, result time.Time) int64 {
	if call.IsZero() || result.IsZero() || result.Before(call) {
		return 0
	}
	return result.Sub(call).Milliseconds()
}

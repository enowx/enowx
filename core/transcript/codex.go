package transcript

import (
	"bufio"
	"cmp"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// Codex records each session as JSON lines in
// ~/.codex/sessions/YYYY/MM/DD/rollout-<time>-<id>.jsonl, each line
// {"timestamp", "type", "payload"}: session_meta (with the cwd),
// turn_context (model, effort), response_item (what the model sees) and
// event_msg (token counts, and since about 0.160 each finished item of the
// conversation, as Codex shows it). Rollouts with those items are read
// from them: the model's tool calls there are scripts calling the real
// tools. Older ones are read from the response items. See
// docs/chat-view.md.

// codexHome is where Codex keeps its files under home.
func codexHome(home string) string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h
	}
	return filepath.Join(home, ".codex")
}

// codexLookAt is how many of the newest rollouts are opened to find a
// folder's: Codex keeps every session in one tree, by date.
const codexLookAt = 200

// CodexFiles returns the rollouts of the sessions Codex ran in dir, newest
// change first.
func CodexFiles(home, dir string) []Found {
	var all []Found
	filepath.WalkDir(filepath.Join(codexHome(home), "sessions"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		if info, err := d.Info(); err == nil {
			all = append(all, Found{Path: path, Changed: info.ModTime()})
		}
		return nil
	})
	slices.SortFunc(all, func(a, b Found) int { return b.Changed.Compare(a.Changed) })
	dirs := namesOfDir(dir)
	var out []Found
	for i, f := range all {
		if i == codexLookAt {
			break
		}
		if slices.Contains(dirs, codexCwd(f.Path)) {
			out = append(out, f)
		}
	}
	return out
}

// codexCwd is the folder a rollout's session ran in, from its first line.
func codexCwd(path string) string {
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
				Cwd string `json:"cwd"`
			} `json:"payload"`
		}
		if json.Unmarshal(sc.Bytes(), &rec) == nil && rec.Type == "session_meta" {
			return rec.Payload.Cwd
		}
	}
	return ""
}

// NewCodexReader follows a Codex rollout.
func NewCodexReader(path string) *Reader { return newReader(path, &codexParser{}) }

type codexParser struct {
	items bool // the rollout records items: response items only repeat them
}

func (*codexParser) fresh() parser { return &codexParser{} }

// before is the conversation a rollout continues. Rewinding forks the
// session into a new rollout, <base>_<id>.jsonl, whose session_meta says
// how much of its base it keeps (history_base.end_byte_offset); a base
// may continue another in turn.
func (*codexParser) before(path string) [][]byte {
	return codexHistory(path, 0)
}

func codexHistory(path string, depth int) [][]byte {
	f, err := os.Open(path)
	if err != nil || depth > 20 {
		return nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	if !sc.Scan() {
		return nil
	}
	var meta struct {
		Type    string `json:"type"`
		Payload struct {
			Base *struct {
				Thread string `json:"thread_id"`
				End    int64  `json:"end_byte_offset"`
			} `json:"history_base"`
		} `json:"payload"`
	}
	if json.Unmarshal(sc.Bytes(), &meta) != nil || meta.Type != "session_meta" || meta.Payload.Base == nil || meta.Payload.Base.End <= 0 {
		return nil
	}
	base := codexBase(path, meta.Payload.Base.Thread)
	if base == "" {
		return nil
	}
	lines := codexHistory(base, depth+1)
	bf, err := os.Open(base)
	if err != nil {
		return lines
	}
	defer bf.Close()
	bs := bufio.NewScanner(io.LimitReader(bf, meta.Payload.Base.End))
	bs.Buffer(make([]byte, 64<<10), 16<<20)
	for bs.Scan() {
		lines = append(lines, append([]byte(nil), bs.Bytes()...))
	}
	return lines
}

// codexBase is the rollout a fork continues: of the thread's rollouts,
// in any day's folder, the one started last before it. Their names start
// with the time they started.
func codexBase(fork, thread string) string {
	sessions := filepath.Join(filepath.Dir(fork), "..", "..", "..")
	matches, _ := filepath.Glob(filepath.Join(sessions, "*", "*", "*", "rollout-*-"+thread+"*.jsonl"))
	own := filepath.Base(fork)
	base := ""
	for _, m := range matches {
		if name := filepath.Base(m); name < own && (base == "" || name > filepath.Base(base)) {
			base = m
		}
	}
	return base
}

type codexRecord struct {
	Timestamp time.Time       `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexItem struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Summary []struct {
		Text string `json:"text"`
	} `json:"summary"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Input     string          `json:"input"`
	CallID    string          `json:"call_id"`
	Output    json.RawMessage `json:"output"`
	Action    struct {
		Command []string `json:"command"`
	} `json:"action"`

	// event_msg
	Message string          `json:"message"`
	Item    json.RawMessage `json:"item"`
	Info    *struct {
		Last struct {
			Total int64 `json:"total_tokens"`
		} `json:"last_token_usage"`
		Window int64 `json:"model_context_window"`
	} `json:"info"`
}

func (p *codexParser) parse(line []byte, r *Reader) {
	var rec codexRecord
	if json.Unmarshal(line, &rec) != nil {
		return
	}
	at := rec.Timestamp
	if rec.Type == "turn_context" {
		// Read on its own: its fields share names with other payloads'.
		var tc struct {
			Model  string `json:"model"`
			Effort string `json:"effort"`
		}
		if json.Unmarshal(rec.Payload, &tc) == nil {
			r.Model, r.Effort = cmp.Or(tc.Model, r.Model), cmp.Or(tc.Effort, r.Effort)
		}
		return
	}
	var it codexItem
	if json.Unmarshal(rec.Payload, &it) != nil {
		return
	}
	switch rec.Type {
	case "event_msg":
		switch it.Type {
		// The model's copy of the user's message comes with the
		// instructions and environment Codex adds; the event is what was
		// typed.
		case "user_message":
			if text := strings.TrimSpace(it.Message); text != "" {
				r.add(proto.TranscriptEntry{Kind: proto.TranscriptUser, Time: at, Text: text})
			}
		case "token_count":
			if it.Info != nil && it.Info.Last.Total > 0 {
				r.Context, r.Window = it.Info.Last.Total, it.Info.Window
			}
		case "turn_aborted":
			r.add(proto.TranscriptEntry{Kind: proto.TranscriptNotice, Time: at, Text: "Interrupted"})
		case "item_started", "item_completed":
			p.items = true
			codexThreadItem(r, it.Item, at)
		}
	case "response_item":
		if !p.items {
			codexResponse(r, it, at)
		}
	}
}

func codexResponse(r *Reader, it codexItem, at time.Time) {
	switch it.Type {
	case "message":
		if it.Role != "assistant" {
			return
		}
		var parts []string
		for _, c := range it.Content {
			if c.Type == "output_text" && strings.TrimSpace(c.Text) != "" {
				parts = append(parts, strings.TrimSpace(c.Text))
			}
		}
		if len(parts) > 0 {
			r.add(proto.TranscriptEntry{Kind: proto.TranscriptAssistant, Time: at, Text: strings.Join(parts, "\n\n")})
		}
	case "reasoning":
		var parts []string
		for _, s := range it.Summary {
			if strings.TrimSpace(s.Text) != "" {
				parts = append(parts, s.Text)
			}
		}
		if len(parts) > 0 {
			r.add(proto.TranscriptEntry{Kind: proto.TranscriptThinking, Time: at, Text: strings.Join(parts, "\n\n")})
		}
	case "function_call":
		r.tools[it.CallID] = r.add(codexCall(it.Name, it.Arguments, at))
	case "local_shell_call":
		args, _ := json.Marshal(map[string]any{"command": it.Action.Command})
		r.tools[it.CallID] = r.add(codexCall("shell", string(args), at))
	case "custom_tool_call":
		e := proto.TranscriptEntry{Kind: proto.TranscriptTool, Time: at, Text: it.Name, Detail: clip(it.Input, 300)}
		if it.Name == "apply_patch" {
			codexPatch(&e, it.Input)
		}
		r.tools[it.CallID] = r.add(e)
	case "function_call_output", "custom_tool_call_output":
		i, ok := r.tools[it.CallID]
		if !ok {
			return
		}
		out, failed := codexOutput(it.Output)
		r.change(i, func(e *proto.TranscriptEntry) {
			e.Done, e.Failed, e.Result = true, failed, clip(strings.TrimSpace(out), 4000)
			e.DurationMs = tookMs(e.Time, at)
		})
	}
}

// codexTools are Codex's tools by the names the chat knows them by.
var codexTools = map[string]string{
	"shell": "Bash", "container.exec": "Bash", "shell_command": "Bash", "exec_command": "Bash",
	"update_plan": "TodoWrite", "view_image": "Read", "web_search": "WebSearch",
}

func codexCall(name, args string, at time.Time) proto.TranscriptEntry {
	if n, ok := codexTools[name]; ok {
		name = n
	}
	e := proto.TranscriptEntry{Kind: proto.TranscriptTool, Time: at, Text: name}
	var in map[string]any
	if json.Unmarshal([]byte(args), &in) != nil {
		e.Detail = clip(args, 300)
		return e
	}
	// A shell call's command is a list, "bash -lc <script>" mostly, or a
	// string named cmd.
	if cmd, ok := in["command"].([]any); ok {
		var words []string
		for _, w := range cmd {
			s, _ := w.(string)
			words = append(words, s)
		}
		if len(words) == 3 && (words[1] == "-lc" || words[1] == "-c") {
			words = words[2:]
		}
		in["command"] = strings.Join(words, " ")
	}
	if cmd, ok := in["cmd"].(string); ok {
		in["command"] = cmd
	}
	raw, _ := json.Marshal(in)
	e.Detail = toolDetail(raw)
	return e
}

// codexPatch fills e with what an apply_patch call changes: the files it
// names and its - and + lines.
func codexPatch(e *proto.TranscriptEntry, patch string) {
	var files, olds, news []string
	for _, l := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(l, "*** Update File: "), strings.HasPrefix(l, "*** Add File: "), strings.HasPrefix(l, "*** Delete File: "):
			files = append(files, l[strings.Index(l, ": ")+2:])
		case strings.HasPrefix(l, "***"), strings.HasPrefix(l, "@@"), strings.HasPrefix(l, "--- "), strings.HasPrefix(l, "+++ "):
		case strings.HasPrefix(l, "-"):
			olds = append(olds, l[1:])
			e.Removed++
		case strings.HasPrefix(l, "+"):
			news = append(news, l[1:])
			e.Added++
		}
	}
	e.Text = "Edit"
	e.Detail = clip(strings.Join(files, ", "), 300)
	e.Old, e.New = clip(strings.Join(olds, "\n"), 3000), clip(strings.Join(news, "\n"), 3000)
}

var codexExit = regexp.MustCompile(`^Exit code: (\d+)`)

// codexOutput is a tool's output and whether it failed. The output is a
// string or a list of content items; a shell's says its exit code, as a
// first line or, from older Codex, as JSON metadata.
func codexOutput(raw json.RawMessage) (string, bool) {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		var items []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		json.Unmarshal(raw, &items)
		var parts []string
		for _, it := range items {
			if it.Text != "" {
				parts = append(parts, it.Text)
			}
		}
		return strings.Join(parts, "\n"), false
	}
	var old struct {
		Output   *string `json:"output"`
		Metadata struct {
			ExitCode int `json:"exit_code"`
		} `json:"metadata"`
	}
	if json.Unmarshal([]byte(s), &old) == nil && old.Output != nil {
		return *old.Output, old.Metadata.ExitCode != 0
	}
	if m := codexExit.FindStringSubmatch(s); m != nil {
		code, _ := strconv.Atoi(m[1])
		if _, out, ok := strings.Cut(s, "\nOutput:\n"); ok {
			return out, code != 0
		}
		return s, code != 0
	}
	return s, false
}

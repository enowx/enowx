package tokens

import (
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Gemini CLI keeps each session in ~/.gemini/tmp/<project>/chats/
// ($GEMINI_CLI_HOME over the home): session-*.jsonl, a metadata line then
// each message, appended again whenever it changes (older builds: one
// session-*.json with a messages list). A "gemini" message has its id,
// timestamp, model and tokens: input (the cached ones in), output,
// cached, thoughts, tool. The project folder's .project_root says where
// it ran.
//
// Qwen Code keeps each session as JSON lines in
// ~/.qwen/projects/<folder>/chats/<id>.jsonl ($QWEN_RUNTIME_DIR, else
// $QWEN_HOME, over ~/.qwen): each assistant record with its uuid, cwd,
// model and usageMetadata in Gemini's names (promptTokenCount, cached
// ones in; candidatesTokenCount; cachedContentTokenCount;
// thoughtsTokenCount).

func geminiFiles(env Env) []string {
	home := cmp.Or(os.Getenv("GEMINI_CLI_HOME"), env.Home)
	var out []string
	for _, f := range walk(filepath.Join(home, ".gemini", "tmp"), ".jsonl") {
		if strings.Contains(f, string(filepath.Separator)+"chats"+string(filepath.Separator)) {
			out = append(out, f)
		}
	}
	for _, f := range walk(filepath.Join(home, ".gemini", "tmp"), ".json") {
		if strings.HasPrefix(filepath.Base(f), "session-") {
			out = append(out, f)
		}
	}
	return out
}

type geminiMessage struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Model     string    `json:"model"`
	Tokens    *struct {
		Input    int64 `json:"input"`
		Output   int64 `json:"output"`
		Cached   int64 `json:"cached"`
		Thoughts int64 `json:"thoughts"`
		Tool     int64 `json:"tool"`
	} `json:"tokens"`
}

func (m geminiMessage) call(st *FileState) (Call, bool) {
	if m.Type != "gemini" || m.Tokens == nil || m.ID == "" {
		return Call{}, false
	}
	t := m.Tokens
	return Call{ID: "gemini:" + m.ID, Time: m.Timestamp, Model: m.Model, Provider: "google", Dir: st.Dir, Conversation: st.Conv,
		Input: max(t.Input-t.Cached, 0) + t.Tool, CacheRead: t.Cached, Output: t.Output + t.Thoughts, Reasoning: t.Thoughts}, true
}

// geminiRoot is where the sessions in chats ran, from its project
// folder's .project_root.
func geminiRoot(path string) string {
	dir := filepath.Dir(path)
	for range 3 {
		if data, err := os.ReadFile(filepath.Join(dir, ".project_root")); err == nil {
			return strings.TrimSpace(string(data))
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

func readGemini(path string, st *FileState) ([]Call, error) {
	if st.Dir == "" {
		st.Dir = geminiRoot(path)
	}
	var out []Call
	if strings.HasSuffix(path, ".json") {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var doc struct {
			SessionID string          `json:"sessionId"`
			Messages  []geminiMessage `json:"messages"`
		}
		if json.Unmarshal(data, &doc) != nil {
			return nil, nil
		}
		st.Conv = cmp.Or(doc.SessionID, st.Conv)
		for _, m := range doc.Messages {
			if c, ok := m.call(st); ok {
				out = append(out, c)
			}
		}
		return out, nil
	}
	err := readLines(path, st, func(line []byte) {
		var m geminiMessage
		if json.Unmarshal(line, &m) != nil {
			return
		}
		if m.Type == "" && st.Conv == "" {
			var meta struct {
				SessionID string `json:"sessionId"`
			}
			if json.Unmarshal(line, &meta) == nil {
				st.Conv = meta.SessionID
			}
		}
		if c, ok := m.call(st); ok {
			out = append(out, c)
		}
	})
	return out, err
}

func qwenFiles(env Env) []string {
	root := cmp.Or(os.Getenv("QWEN_RUNTIME_DIR"), os.Getenv("QWEN_HOME"), filepath.Join(env.Home, ".qwen"))
	return append(walk(filepath.Join(root, "projects"), ".jsonl"), walk(filepath.Join(root, "tmp"), ".jsonl")...)
}

type qwenLine struct {
	UUID      string    `json:"uuid"`
	SessionID string    `json:"sessionId"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Cwd       string    `json:"cwd"`
	Model     string    `json:"model"`
	Usage     *struct {
		Prompt   int64 `json:"promptTokenCount"`
		Output   int64 `json:"candidatesTokenCount"`
		Cached   int64 `json:"cachedContentTokenCount"`
		Thoughts int64 `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
}

func readQwen(path string, st *FileState) ([]Call, error) {
	var out []Call
	err := readLines(path, st, func(line []byte) {
		var l qwenLine
		if json.Unmarshal(line, &l) != nil {
			return
		}
		st.Dir, st.Conv = cmp.Or(l.Cwd, st.Dir), cmp.Or(l.SessionID, st.Conv)
		if l.Type != "assistant" || l.Usage == nil || l.UUID == "" {
			return
		}
		u := l.Usage
		out = append(out, Call{ID: "qwen:" + l.UUID, Time: l.Timestamp, Model: l.Model, Dir: st.Dir, Conversation: st.Conv,
			Input: max(u.Prompt-u.Cached, 0), CacheRead: u.Cached, Output: u.Output + u.Thoughts, Reasoning: u.Thoughts})
	})
	return out, err
}

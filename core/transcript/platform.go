package transcript

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/proto"
)

// Claude Platform, enowX's own agent, keeps a conversation in one file of JSON lines
// (package record): what the chat shows as it happens, the streamed text included, so a
// chat polling the file sees an answer grow. A session's file is in its --session-dir
// (FindIn); PlatformFiles finds those of the agent run by hand. See docs/chat-view.md.

// PlatformFiles returns the conversations Claude Platform kept in its default folder
// for sessions run in dir, newest first.
func PlatformFiles(home, dir string) []Found {
	root := record.SessionsDir(home)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	names := namesOfDir(filepath.Clean(dir))
	var out []Found
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(root, e.Name())
		if !slices.Contains(names, platformDir(path)) {
			continue
		}
		if info, err := e.Info(); err == nil {
			out = append(out, Found{Path: path, Changed: info.ModTime()})
		}
	}
	slices.SortFunc(out, func(a, b Found) int { return b.Changed.Compare(a.Changed) })
	return out
}

// platformDir is the folder the conversation at path ran in, as its first line says.
func platformDir(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	if !sc.Scan() {
		return ""
	}
	var rec record.Record
	if json.Unmarshal(sc.Bytes(), &rec) != nil || rec.Type != record.Meta || rec.Dir == "" {
		return ""
	}
	return filepath.Clean(rec.Dir)
}

// NewPlatformReader follows a Claude Platform conversation.
func NewPlatformReader(path string) *Reader { return newReader(path, newPlatformParser()) }

// platformBlock is a content block of an answer under way. Its entry comes with its
// first text: a thought with no text to show never shows.
type platformBlock struct {
	kind  string
	text  string // held until it has something on it
	entry int    // -1 until there is one
	used  bool
}

// platformParser turns records into entries. It keeps what an answer streamed by the
// answer's ID, until its Assistant record replaces that.
type platformParser struct {
	open     map[string]map[int]*platformBlock
	finished map[string]bool
}

func newPlatformParser() *platformParser {
	return &platformParser{open: map[string]map[int]*platformBlock{}, finished: map[string]bool{}}
}

func (*platformParser) fresh() parser { return newPlatformParser() }

func (p *platformParser) parse(line []byte, r *Reader) {
	var rec record.Record
	if json.Unmarshal(line, &rec) != nil {
		return
	}
	if rec.Sidechain != "" {
		p.sidechain(rec, r)
		return
	}
	switch rec.Type {
	case record.Meta:
		p.meta(rec, r)
	case record.User:
		p.closeTools(r, rec.Time)
		if text := platformUserText(rec); text != "" {
			r.add(proto.TranscriptEntry{Kind: proto.TranscriptUser, Time: rec.Time, Text: text})
		}
	case record.BlockStart:
		p.block(rec)
	case record.Delta:
		p.delta(rec, r)
	case record.Assistant:
		p.assistant(rec, r)
	case record.ToolStart:
		p.toolStart(rec, r)
	case record.ToolWait:
		p.toolWait(rec, r)
	case record.ToolEnd:
		p.toolEnd(rec, r)
	case record.Notice:
		if text := strings.TrimSpace(rec.Text); text != "" {
			r.add(proto.TranscriptEntry{Kind: proto.TranscriptNotice, Time: rec.Time, Text: text})
		}
	case record.Compact:
		r.Context = 0 // until the next answer says
		text := "Context compacted"
		if rec.Before > 0 {
			text = fmt.Sprintf("Context compacted, from %d tokens", rec.Before)
		}
		r.add(proto.TranscriptEntry{Kind: proto.TranscriptNotice, Time: rec.Time, Text: text})
	case record.TurnEnd:
		p.closeTools(r, rec.Time)
		if rec.Stop == "interrupted" {
			r.add(proto.TranscriptEntry{Kind: proto.TranscriptNotice, Time: rec.Time, Text: "Interrupted"})
		}
	}
}

func (*platformParser) meta(rec record.Record, r *Reader) {
	if rec.Model != "" && rec.Model != r.Model {
		r.Model, r.Window = rec.Model, 0 // another model's window isn't this one's
	}
	if rec.Mode != "" {
		r.Mode = rec.Mode
	}
	if rec.Effort != "" {
		r.Effort = rec.Effort
	}
	if rec.Window > 0 {
		r.Window = rec.Window
	}
	if rec.Budget > 0 {
		r.BudgetUSD = rec.Budget
	}
}

// platformUserText is what a user's message shows: its text, else what it sent.
func platformUserText(rec record.Record) string {
	text := strings.TrimSpace(rec.Text)
	if text == "" {
		text = platformContent(rec.Message)
	}
	if text == "" && len(rec.Attachments) > 0 {
		text = "Attached: " + strings.Join(rec.Attachments, ", ")
	}
	return text
}

// platformContent is the text blocks of a MessageParam.
func platformContent(message json.RawMessage) string {
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(message, &m) != nil {
		return ""
	}
	return strings.TrimSpace(blockText(m.Content))
}

// block is the block rec names, kept from then on.
func (p *platformParser) block(rec record.Record) *platformBlock {
	blocks := p.open[rec.Msg]
	if blocks == nil {
		blocks = map[int]*platformBlock{}
		p.open[rec.Msg] = blocks
	}
	b := blocks[rec.Block]
	if b == nil {
		b = &platformBlock{kind: rec.Kind, entry: -1}
		blocks[rec.Block] = b
	}
	return b
}

func (p *platformParser) delta(rec record.Record, r *Reader) {
	if rec.Msg == "" || rec.Text == "" || p.finished[rec.Msg] {
		return
	}
	b := p.block(rec)
	if b.kind == "" {
		b.kind = rec.Kind
	}
	kind := platformKind(b.kind)
	if kind == "" {
		return
	}
	if b.entry >= 0 {
		r.change(b.entry, func(e *proto.TranscriptEntry) { e.Text += rec.Text })
		return
	}
	b.text += rec.Text
	if strings.TrimSpace(b.text) != "" {
		b.entry = r.add(proto.TranscriptEntry{Kind: kind, Time: rec.Time, Text: strings.TrimLeft(b.text, " \t\r\n")})
	}
}

// platformKind is the entry a content block makes, "" for none (a tool call has records
// of its own).
func platformKind(block string) string {
	switch block {
	case "text", "":
		return proto.TranscriptAssistant
	case "thinking":
		return proto.TranscriptThinking
	}
	return ""
}

// assistant takes a finished answer: its settings and cost, and its text and thoughts
// in place of what streamed, each where it first showed.
func (p *platformParser) assistant(rec record.Record, r *Reader) {
	if rec.TotalCost > 0 {
		r.CostUSD = rec.TotalCost
	}
	if rec.Model != "" {
		r.Model = rec.Model
	}
	if u := rec.Usage; u != nil {
		if n := u.Input + u.CacheRead + u.CacheWrite + u.Output; n > 0 {
			r.Context = n
		}
	}
	if rec.Msg != "" && p.finished[rec.Msg] {
		return
	}
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	_ = json.Unmarshal(rec.Message, &msg)
	// A record's content is the API's blocks, whose index the deltas count by.
	var blocks []claudeBlock
	if json.Unmarshal(msg.Content, &blocks) != nil {
		var text string
		if json.Unmarshal(msg.Content, &text) == nil {
			blocks = []claudeBlock{{Type: "text", Text: text}}
		}
	}
	open := p.open[rec.Msg]
	for i, b := range blocks {
		text := b.Text
		if b.Type == "thinking" {
			text = b.Thinking
		}
		text = strings.TrimSpace(text)
		kind := platformKind(b.Type)
		if text == "" || kind == "" || b.Type == "" {
			continue
		}
		if s := streamedFor(open, i, b.Type); s != nil {
			s.used = true
			if r.entries[s.entry].Text != text {
				r.change(s.entry, func(e *proto.TranscriptEntry) { e.Text = text })
			}
			continue
		}
		r.add(proto.TranscriptEntry{Kind: kind, Time: rec.Time, Text: text})
	}
	if rec.Msg != "" {
		delete(p.open, rec.Msg)
		p.finished[rec.Msg] = true
	}
}

// streamedFor is what streamed of block index i of kind: at its index, else the first of
// its kind not taken.
func streamedFor(open map[int]*platformBlock, i int, kind string) *platformBlock {
	if b := open[i]; b != nil && b.kind == kind && b.entry >= 0 && !b.used {
		return b
	}
	var first *platformBlock
	best := -1
	for idx, b := range open {
		if b.kind == kind && b.entry >= 0 && !b.used && (first == nil || idx < best) {
			first, best = b, idx
		}
	}
	return first
}

func (*platformParser) toolStart(rec record.Record, r *Reader) {
	t := rec.Tool
	if t == nil || t.ID == "" {
		return
	}
	e := proto.TranscriptEntry{Kind: proto.TranscriptTool, Time: rec.Time, Text: t.Name, Detail: t.Detail}
	if e.Detail == "" {
		e.Detail = toolDetail(t.Input)
	}
	platformQuestions(&e, t)
	if t.Old == "" && t.New == "" {
		toolEdit(&e, t.Input)
	}
	platformEdit(&e, t)
	r.tools[t.ID] = r.add(e)
}

func (*platformParser) toolWait(rec record.Record, r *Reader) {
	t := rec.Tool
	if t == nil {
		return
	}
	if i, ok := r.tools[t.ID]; ok && len(t.Questions) > 0 {
		r.change(i, func(e *proto.TranscriptEntry) { platformQuestions(e, t) })
	}
}

func (p *platformParser) toolEnd(rec record.Record, r *Reader) {
	t := rec.Tool
	if t == nil || t.ID == "" {
		return
	}
	i, ok := r.tools[t.ID]
	if !ok {
		p.toolStart(rec, r) // its start wasn't read
		if i, ok = r.tools[t.ID]; !ok {
			return
		}
	}
	r.change(i, func(e *proto.TranscriptEntry) {
		e.Done, e.Failed, e.Result = true, t.Failed, clip(strings.TrimSpace(t.Result), 4000)
		e.DurationMs = t.DurationMs
		if e.DurationMs == 0 {
			e.DurationMs = tookMs(e.Time, rec.Time)
		}
		if t.Detail != "" {
			e.Detail = t.Detail
		}
		platformEdit(e, t)
	})
	delete(r.tools, t.ID)
}

// platformEdit fills e with the change t says it made.
func platformEdit(e *proto.TranscriptEntry, t *record.Tool) {
	if t.Old != "" || t.New != "" {
		e.Old, e.New = clip(t.Old, 3000), clip(t.New, 3000)
	}
	if t.Added != 0 || t.Removed != 0 {
		e.Added, e.Removed = t.Added, t.Removed
	}
}

// platformQuestions fills e with the questions t asks.
func platformQuestions(e *proto.TranscriptEntry, t *record.Tool) {
	if len(t.Questions) == 0 {
		return
	}
	e.Questions = e.Questions[:0]
	for _, q := range t.Questions {
		e.Questions = append(e.Questions, proto.TranscriptQuestion{Header: q.Header, Question: q.Question, Options: q.Options, About: q.About, Multi: q.Multi})
	}
	e.Detail = clip(t.Questions[0].Question, 300)
}

// closeTools ends the calls left running at a turn's end: their program was stopped.
func (*platformParser) closeTools(r *Reader, at time.Time) {
	for id, i := range r.tools {
		r.change(i, func(e *proto.TranscriptEntry) {
			e.Done, e.Failed = true, true
			if e.Result == "" {
				e.Result = "Interrupted"
			}
			e.DurationMs = tookMs(e.Time, at)
		})
		delete(r.tools, id)
	}
}

// sidechain folds what a subagent did into its Task call, as lines of its result until
// the call's own result replaces them; its answers count only in the cost.
func (*platformParser) sidechain(rec record.Record, r *Reader) {
	switch rec.Type {
	case record.Assistant:
		if rec.TotalCost > 0 {
			r.CostUSD = rec.TotalCost
		}
	case record.ToolStart:
		i, ok := r.tools[rec.Sidechain]
		if !ok || rec.Tool == nil {
			return
		}
		line := rec.Tool.Name
		if d := cmp.Or(rec.Tool.Detail, toolDetail(rec.Tool.Input)); d != "" {
			line += " " + oneLine(d)
		}
		r.change(i, func(e *proto.TranscriptEntry) {
			if e.Result != "" {
				line = e.Result + "\n" + line
			}
			e.Result = clip(line, 4000)
		})
	}
}

func oneLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

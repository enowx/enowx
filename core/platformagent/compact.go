package platformagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/enowx/enowx/core/platformagent/record"
)

// The context passing this share of the window starts a compaction.
const compactAt = 0.75

// compactRegrow is how much the context must have grown since a compaction failed before
// another is tried: with less, another would likely fail too, and each one costs.
const compactRegrow = 1.1

// Compact summarises the conversation now.
func (a *Agent) Compact(ctx context.Context) error {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	if a.client == nil {
		a.notice("error", errNoKey.Error())
		return errNoKey
	}
	if len(a.main.history) == 0 {
		a.notice("info", "There is nothing to compact yet.")
		return nil
	}
	if ok, err := a.mayGoOn(ctx); err != nil {
		return err
	} else if !ok {
		a.notice("warning", "The budget is spent; the conversation wasn't compacted.")
		return nil
	}
	return a.main.compact(ctx)
}

// compactIfFull compacts when the last answer passed the threshold. Its failure isn't
// the turn's: a Notice says so and the request goes on, the API refusing it only when
// the window is really full.
func (t *thread) compactIfFull(ctx context.Context) {
	a := t.a
	a.mu.Lock()
	window := specOf(a.model).window
	a.mu.Unlock()
	used := t.used
	if t.sidechain == "" {
		a.mu.Lock()
		used = a.used
		a.mu.Unlock()
	}
	if len(t.history) == 0 || float64(used) < compactAt*float64(window) {
		return
	}
	if t.compactFailed || float64(used) < float64(t.compactFailedAt)*compactRegrow {
		return
	}
	if err := t.compact(ctx); err != nil && ctx.Err() == nil {
		t.compactFailed, t.compactFailedAt = true, used
		a.notice("warning", "Compaction failed: "+err.Error())
	}
}

// compact asks for a summary and starts the history over from one user message holding
// it: the simple shape, which replays no earlier turn and no thinking block, so nothing
// carried over is tied to the old transcript. It only runs with every call answered.
// Nothing else ever edits the history.
func (t *thread) compact(ctx context.Context) error {
	a := t.a
	if open := danglingCalls(t.history); len(open) > 0 {
		return errors.New("a tool round is under way")
	}
	a.mu.Lock()
	before := a.used
	a.mu.Unlock()
	if t.sidechain != "" {
		before = t.used
	}

	// Offering the same tools keeps the cached prefix; only if the model calls one
	// anyway does the retry pay to forbid them.
	ask := callOptions{quiet: true, extra: []message{{role: "user", raw: textMessage(summaryRequest)}}}
	ans, err := t.call(ctx, ask)
	if err != nil {
		return err
	}
	if summaryOf(ans) == "" {
		t.recordSpend(ans)
		ask.noTools = true
		ans, err = t.call(ctx, ask)
		if err != nil {
			return err
		}
	}
	summary := summaryOf(ans)
	if summary == "" {
		t.recordSpend(ans)
		return errors.New("the model gave no summary")
	}

	cost, total := a.account(ans)
	msg := t.summaryMessage(summary)
	usage := ans.usage
	a.emit(record.Record{
		Type:      record.Compact,
		Messages:  []json.RawMessage{msg},
		Before:    before,
		Msg:       ans.id,
		Model:     ans.model,
		Usage:     &usage,
		Cost:      cost,
		TotalCost: total,
		Sidechain: t.sidechain,
	})
	t.history = []message{{role: "user", raw: msg}}
	t.stripBefore = 0
	t.compactFailed, t.compactFailedAt = false, 0
	t.setUsed(0)
	a.notice("info", fmt.Sprintf("The conversation was compacted: about %d tokens became a summary.", before))
	return nil
}

// summaryOf is the summary in an answer's text, "" when it holds none or was refused.
func summaryOf(ans *answer) string {
	if ans.stop == "refusal" {
		return ""
	}
	text := ans.text
	if start := strings.Index(text, "<summary>"); start >= 0 {
		text = text[start+len("<summary>"):]
		if end := strings.Index(text, "</summary>"); end >= 0 {
			text = text[:end]
		}
	}
	return strings.TrimSpace(text)
}

// summaryMessage holds what the new history starts from: the repository's instructions
// again, the summary, and the task as the user last put it.
func (t *thread) summaryMessage(summary string) json.RawMessage {
	var blocks []map[string]string
	if intro := startOfConversation(t.a.opts.Dir, time.Now()); intro != "" {
		blocks = append(blocks, map[string]string{"type": "text", "text": intro})
	}
	var b strings.Builder
	b.WriteString("This conversation was compacted. What came before it is this summary:\n\n<summary>\n")
	b.WriteString(summary)
	b.WriteString("\n</summary>\n")
	if t.lastPrompt != "" {
		b.WriteString("\nThe user's latest message was:\n\n<latest-message>\n")
		b.WriteString(t.lastPrompt)
		b.WriteString("\n</latest-message>\n")
	}
	b.WriteString("\nGo on from where the summary says things stand.")
	blocks = append(blocks, map[string]string{"type": "text", "text": b.String()})
	return marshalStable(map[string]any{"role": "user", "content": blocks})
}

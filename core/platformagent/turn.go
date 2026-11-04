package platformagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
	"github.com/enowx/enowx/proto"
)

// How far the loop goes on by itself in the two cases where it asks again unprompted.
const (
	maxCutoffs = 3 // answers in a row cut by the output limit
	maxPauses  = 5 // answers in a row the server paused
	maxFollows = 5 // messages in a row a turn's end asked for
)

// Stops of a TurnEnd.
const (
	stopDone        = "done"
	stopInterrupted = "interrupted"
	stopError       = "error"
	stopBudget      = "budget"
	stopRefusal     = "refusal"
)

// cutoffNudge continues an answer the output limit cut off.
const cutoffNudge = "Your last answer was cut off by the output limit. Continue from where it stopped, without repeating it."

// cutoffCallError answers calls the output limit cut off: their input may be partial.
const cutoffCallError = "This call was cut off by the output limit before it was complete, so it did not run. Send it again in smaller pieces."

// budgetSpentText tells that the user said no to spending more.
const budgetSpentText = "The budget is spent; the turn ends here."

// pausedCallError answers calls of an answer the loop won't go on from.
const pausedCallError = "The server kept pausing this turn, so this call did not run."

var errNoKey = errors.New("no API key: add one in enowX (Settings, vault, platform anthropic)")

// thread is a conversation with the model: the session's main one, or a subagent's. It
// belongs to the goroutine running it; side-by-side tools only read its tools.
type thread struct {
	a *Agent
	// sidechain is the parent call of a subagent's thread, "" for the main one.
	sidechain string

	toolsByName map[string]tools.Tool
	toolsJSON   json.RawMessage // frozen by setup
	systemJSON  json.RawMessage // frozen by setup

	history []message
	// stripBefore: answers before this index go without their thinking blocks, after
	// the API refused them once (api.go).
	stripBefore int
	used        int64 // tokens of the last answer, input and output
	lastPrompt  string
	// compactFailed says a compaction failed this turn, compactFailedAt the context then:
	// neither a tool round nor a new turn pays for another until it has grown.
	compactFailed   bool
	compactFailedAt int64
}

// Turn runs one user message to its end. It returns the error a Notice has already
// told, nil when it was interrupted.
func (a *Agent) Turn(ctx context.Context, text string, attachments []string) error {
	a.turnMu.Lock()
	defer a.turnMu.Unlock()
	defer a.state(proto.StateDone)

	own := text // what follows comes from hooks, not the user
	for follows := 0; ; follows++ {
		err := a.turnOnce(ctx, text, own, attachments)
		more := ""
		if a.hooks != nil {
			more = a.hooks.TurnDone(ctx)
		}
		if err != nil || more == "" || ctx.Err() != nil || follows >= maxFollows {
			return err
		}
		text, own, attachments = more, "", nil
	}
}

// turnOnce runs one message to its end.
func (a *Agent) turnOnce(ctx context.Context, text, own string, attachments []string) error {
	a.state(proto.StateWorking)
	end := func(stop string) { a.emit(record.Record{Type: record.TurnEnd, Stop: stop}) }

	var asked HookAnswer
	if a.hooks != nil {
		asked = a.hooks.Prompt(ctx, text)
	}
	if asked.Deny {
		a.notice("warning", first(asked.Why, "A hook refused this message."))
		end(stopDone)
		return nil
	}

	if a.client == nil {
		a.notice("error", errNoKey.Error())
		end(stopError)
		return errNoKey
	}
	t := a.main
	t.compactFailed = false
	// Before compacting, which costs: a refused budget must spend nothing more.
	mayGoOn, err := a.mayGoOn(ctx)
	if err != nil {
		end(stopInterrupted)
		return nil
	}
	if mayGoOn {
		t.compactIfFull(ctx)
	}

	msg, files := t.userMessage(withContext(text, asked.Context), own, attachments)
	if msg == nil {
		end(stopDone)
		return nil
	}
	t.appendUser(msg, text, files)
	if !mayGoOn {
		a.notice("warning", budgetSpentText)
		end(stopBudget)
		return nil
	}

	stop, _, err := t.run(ctx)
	if err != nil {
		a.notice("error", err.Error())
	}
	end(stop)
	if err == nil {
		err = a.fileFailure()
	}
	return err
}

func (a *Agent) state(s string) {
	if a.hooks != nil {
		a.hooks.State(s)
	}
}

// appendUser keeps a user message.
func (t *thread) appendUser(msg json.RawMessage, text string, attachments []string) {
	t.history = append(t.history, message{role: "user", raw: msg})
	t.lastPrompt = text
	t.a.emit(record.Record{
		Type: record.User, Message: msg, Text: text, Attachments: attachments, Sidechain: t.sidechain,
	})
}

// run asks the model and runs its tools until an answer ends the turn: how it stopped,
// the last answer's text, and an error when stop is "error". The history is valid for
// another request whatever comes back.
func (t *thread) run(ctx context.Context) (string, string, error) {
	a := t.a
	cutoffs, pauses := 0, 0
	for step := 0; ; step++ {
		if err := a.fileFailure(); err != nil {
			return stopError, "", err
		}
		if ctx.Err() != nil {
			return stopInterrupted, "", nil
		}
		if ok, err := a.mayGoOn(ctx); err != nil {
			return stopInterrupted, "", nil
		} else if !ok {
			a.notice("warning", budgetSpentText)
			return stopBudget, "", nil
		}
		// Between tool rounds is the only place inside a turn a summary can replace the
		// history: every call of the last answer has its result by then. Turn has seen
		// to the first request, before its user message went in.
		if step > 0 && t.endsWithUser() {
			t.compactIfFull(ctx)
		}

		ans, err := t.call(ctx, callOptions{})
		if err != nil {
			if ctx.Err() != nil {
				return stopInterrupted, "", nil
			}
			return stopError, "", err
		}
		t.keep(ans)

		switch ans.stop {
		case "refusal":
			a.notice("error", "The model declined this request (a safety classifier or its own refusal). Rephrase it, or clear the conversation if it keeps being refused.")
			return stopRefusal, "", nil
		case "max_tokens":
			if cutoffs++; cutoffs > maxCutoffs {
				a.notice("error", "The answer keeps hitting the output limit; stopping here.")
				if len(ans.calls) > 0 {
					t.answerWith(ans.calls, cutoffCallError)
				}
				return stopError, ans.text, errors.New("the output limit was hit too many times")
			}
			a.notice("warning", "The answer hit the output limit; asking the model to go on.")
			if len(ans.calls) > 0 {
				t.answerWith(ans.calls, cutoffCallError)
				continue
			}
			if ans.raw == nil {
				return stopError, "", errors.New("the output limit left no answer")
			}
			t.appendUser(textMessage(cutoffNudge), cutoffNudge, nil)
			continue
		case "pause_turn":
			if pauses++; pauses > maxPauses {
				if len(ans.calls) > 0 {
					t.answerWith(ans.calls, pausedCallError) // the next request would be refused
				}
				return stopError, ans.text, errors.New("the server kept pausing the turn")
			}
			if len(ans.calls) == 0 {
				// Sent back as it is, the server takes it up again.
				continue
			}
		}
		cutoffs, pauses = 0, 0

		if len(ans.calls) == 0 {
			return stopDone, ans.text, nil
		}
		results := t.runCalls(ctx, ans.calls)
		t.appendResults(results)
		if ctx.Err() != nil {
			return stopInterrupted, "", nil
		}
	}
}

func (t *thread) endsWithUser() bool {
	return len(t.history) > 0 && t.history[len(t.history)-1].role == "user"
}

// keep records an answer and adds it to the history, unless it was refused: a refusal's
// partial output is discarded, its cost stays.
func (t *thread) keep(ans *answer) {
	a := t.a
	if ans.stop == "refusal" {
		ans.raw, ans.calls, ans.text = nil, nil, ""
	}
	cost, total := a.account(ans)
	if ans.raw != nil {
		t.history = append(t.history, message{role: "assistant", raw: ans.raw})
	}
	t.setUsed(contextTokens(ans.usage))
	usage := ans.usage
	a.emit(record.Record{
		Type:      record.Assistant,
		Message:   ans.raw,
		Msg:       ans.id,
		Model:     ans.model,
		Usage:     &usage,
		Cost:      cost,
		TotalCost: total,
		Stop:      ans.stop,
		Sidechain: t.sidechain,
	})
}

func (t *thread) setUsed(tokens int64) {
	t.used = tokens
	if t.sidechain == "" {
		t.a.mu.Lock()
		t.a.used = tokens
		t.a.mu.Unlock()
	}
}

// contextTokens is what a request and its answer fill of the window.
func contextTokens(u record.Usage) int64 { return u.Input + u.CacheRead + u.CacheWrite + u.Output }

// appendResults keeps the user message of one answer's results.
func (t *thread) appendResults(results []json.RawMessage) {
	msg := marshalStable(struct {
		Role    string            `json:"role"`
		Content []json.RawMessage `json:"content"`
	}{"user", results})
	t.history = append(t.history, message{role: "user", raw: msg})
	t.a.emit(record.Record{Type: record.Results, Message: msg, Sidechain: t.sidechain})
}

// answerWith gives calls that won't run an error result each, so that the history holds
// no call without a result.
func (t *thread) answerWith(calls []toolCall, text string) {
	results := make([]json.RawMessage, 0, len(calls))
	for _, c := range calls {
		results = append(results, toolResultBlock(c.ID, tools.Result{Text: text, Failed: true}))
	}
	t.appendResults(results)
}

// textMessage is a user message of one text block.
func textMessage(text string) json.RawMessage {
	return marshalStable(map[string]any{
		"role":    "user",
		"content": []map[string]string{{"type": "text", "text": text}},
	})
}

// clip shortens text for the chat's copy of a result.
func clip(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && text[cut]&0xC0 == 0x80 { // not in the middle of a rune
		cut--
	}
	return fmt.Sprintf("%s\n[%d more bytes]", strings.TrimRight(text[:cut], "\n"), len(text)-cut)
}

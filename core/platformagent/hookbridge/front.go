package hookbridge

import (
	"context"

	"github.com/enowx/enowx/core/platformagent"
	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/proto"
)

var (
	_ platformagent.Hooks = (*Bridge)(nil)
	_ platformagent.Front = Front{}
)

// Front wraps what the user answers through so the session's daemon knows of it: a
// permission goes by the daemon first (a session in bypass, the judge), asking the
// user reads as waiting and an answer as working again, as an agent's own hooks say.
type Front struct {
	platformagent.Front
	Bridge *Bridge
	// Text is the text of an Assistant record, for the turn's last message.
	Text func(record.Record) string
}

// Event implements platformagent.Front.
func (f Front) Event(r record.Record) {
	text := ""
	if f.Text != nil && r.Type == record.Assistant {
		text = f.Text(r)
	}
	f.Bridge.Note(r, text)
	f.Front.Event(r)
}

// Permission implements platformagent.Front.
func (f Front) Permission(ctx context.Context, req platformagent.PermissionRequest) (platformagent.Decision, error) {
	if f.Bridge.inSession() {
		d, err := f.Bridge.event(ctx, decisionTimeout, "", proto.HookEvent{Event: proto.HookPermission, Tool: req.Tool, Command: req.Detail})
		if err == nil && d.Approve {
			return platformagent.AllowOnce, nil
		}
		if err == nil && d.Block {
			return platformagent.Deny, nil
		}
	}
	defer f.waiting(ctx)()
	return f.Front.Permission(ctx, req)
}

// Questions implements platformagent.Front.
func (f Front) Questions(ctx context.Context, qs []record.Question) ([]string, error) {
	defer f.waiting(ctx)()
	return f.Front.Questions(ctx, qs)
}

// OverBudget implements platformagent.Front.
func (f Front) OverBudget(ctx context.Context, spent, budget float64) (bool, error) {
	defer f.waiting(ctx)()
	return f.Front.OverBudget(ctx, spent, budget)
}

// waiting says the agent waits for the user, and returns what says it goes on.
func (f Front) waiting(ctx context.Context) func() {
	if !f.Bridge.inSession() {
		return func() {}
	}
	_, _ = f.Bridge.event(ctx, shortTimeout, proto.StateWaiting, proto.HookEvent{Event: proto.HookWaiting})
	return func() { f.Bridge.State(proto.StateWorking) }
}

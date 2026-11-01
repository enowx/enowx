package daemon

import "github.com/enowx/enowx/core/accounts"

// A session switched to another config starts again on its conversation, and
// its agent prints that conversation: the limit that had it switched among it,
// perhaps wrapped otherwise. Read as the new config's limit, every config would
// be ruled out in turn without being asked. So nothing is read while the agent
// starts, and the limit lines on its screen by then don't count.

// restarting says session id's agent is about to start on its conversation,
// the limit it acted on forgotten: the next config may say the same words.
func (w *accountWatch) restarting(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.settling[id] = true
	delete(w.handled, id)
}

// settled says session id's agent has started, screen what it shows by then.
func (w *accountWatch) settled(id, screen string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.settling, id)
	w.replayed[id] = accounts.LimitLines(screen)
}

// fresh is screen without what session id's agent replayed; ok false while it
// still starts.
func (w *accountWatch) fresh(id, screen string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.settling[id] {
		return "", false
	}
	return accounts.WithoutLimitLines(screen, w.replayed[id]), true
}

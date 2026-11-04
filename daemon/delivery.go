package daemon

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// The ledger lives for the program run. Entries are never evicted within a run:
// a delayed retry must not become a new paste. No text is kept in the ledger.
type deliveryState struct {
	mu   sync.Mutex
	runs map[*session.Session]*deliveryRun
}

type deliveryRun struct {
	sending bool
	entries map[string]*deliveryAttempt
}

type deliveryAttempt struct {
	text   [32]byte
	done   chan struct{} // publishes result to concurrent/reconnected retries
	result proto.DeliveryResult
}

const maxDeliveriesPerRun = 4096

func (s *Server) deliver(p proto.DeliveryParams) (proto.DeliveryResult, error) {
	sess, err := s.sessions.Get(p.ID)
	if err != nil {
		return proto.DeliveryResult{}, err
	}
	info := sess.Info()
	if p.Run == 0 || p.Run != info.Run || info.Exited || info.Queued {
		return proto.DeliveryResult{}, fmt.Errorf("the session's program changed or isn't running; check its terminal")
	}
	if p.Key == "" || len(p.Key) > 128 || p.Text == "" {
		return proto.DeliveryResult{}, fmt.Errorf("delivery needs a key and nonempty text")
	}
	seq := sess.InputSequence()
	screen := sess.Terminal().Text()
	ready := agent.PromptEmpty(screen) && agent.PromptFree(screen)
	d := &s.deliveries
	d.mu.Lock()
	if d.runs == nil {
		d.runs = make(map[*session.Session]*deliveryRun)
	}
	for old := range d.runs {
		if current, err := s.sessions.Get(old.ID()); err != nil || current != old {
			delete(d.runs, old)
		}
	}
	r := d.runs[sess]
	if r == nil {
		r = &deliveryRun{entries: make(map[string]*deliveryAttempt)}
		d.runs[sess] = r
	}
	hash := sha256.Sum256([]byte(p.Text))
	if a := r.entries[p.Key]; a != nil {
		d.mu.Unlock()
		if a.text != hash {
			return proto.DeliveryResult{}, fmt.Errorf("delivery key was already used for different text")
		}
		<-a.done
		return a.result, nil
	}
	if r.sending || len(r.entries) >= maxDeliveriesPerRun || working(info) || info.State == proto.StateWaiting || !ready {
		d.mu.Unlock()
		return proto.DeliveryResult{}, fmt.Errorf("the session isn't ready for a new chat delivery; check its terminal")
	}
	a := &deliveryAttempt{text: hash, done: make(chan struct{})}
	r.entries[p.Key], r.sending = a, true
	d.mu.Unlock()

	if !s.sessions.InputIfUnchanged(sess, []byte(pasteStart+p.Text+pasteEnd), seq) {
		a.result.Interrupted = true
	} else {
		timer := time.NewTimer(pasteSettle)
		defer timer.Stop()
		select {
		case <-sess.Done():
			a.result.Interrupted = true
		case <-timer.C:
			a.result.Interrupted = !s.sessions.InputIfUnchanged(sess, []byte("\r"), seq+1)
		}
	}
	d.mu.Lock()
	r.sending = false
	close(a.done)
	d.mu.Unlock()
	return a.result, nil
}

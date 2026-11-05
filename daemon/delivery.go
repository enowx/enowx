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
	// Only program replacement resolves an interrupted paste's draft uncertainty.
	uncertain bool
	entries   map[string]*deliveryAttempt
}

type deliveryAttempt struct {
	text   [32]byte
	done   chan struct{} // publishes result to concurrent/reconnected retries
	result proto.DeliveryResult
}

const maxDeliveriesPerRun = 4096

// A program that stops reading must not strand the ledger or a retry forever.
// This bounds only the new write-completion wait, not the paste settle delay.
const deliveryWriteLimit = 5 * time.Second

func (s *Server) deliver(p proto.DeliveryParams) (proto.DeliveryResult, error) {
	sess, err := s.sessions.Get(p.ID)
	if err != nil {
		return proto.DeliveryResult{}, err
	}
	guard := sess.DeliveryGuard()
	info := sess.Info()
	if p.Run == 0 || p.Run != info.Run || info.Exited || info.Queued {
		return proto.DeliveryResult{}, fmt.Errorf("the session's program changed or isn't running; check its terminal")
	}
	if p.Key == "" || len(p.Key) > 128 || p.Text == "" {
		return proto.DeliveryResult{}, fmt.Errorf("delivery needs a key and nonempty text")
	}
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
	if r.uncertain {
		d.mu.Unlock()
		return proto.DeliveryResult{}, fmt.Errorf("a previous chat delivery left an unconfirmed paste; restart the session's program before sending another chat message")
	}
	if r.sending || len(r.entries) >= maxDeliveriesPerRun || working(info) || info.State == proto.StateWaiting || !ready {
		d.mu.Unlock()
		return proto.DeliveryResult{}, fmt.Errorf("the session isn't ready for a new chat delivery; check its terminal")
	}
	a := &deliveryAttempt{text: hash, done: make(chan struct{})}
	r.entries[p.Key], r.sending = a, true
	d.mu.Unlock()

	next, written, pasted := s.sessions.DeliveryPaste(sess, []byte(pasteStart+p.Text+pasteEnd), guard)
	queued := pasted
	if pasted {
		limit := time.NewTimer(deliveryWriteLimit)
		pasted = deliveryWritten(written, sess.Done(), limit.C)
		limit.Stop()
	}
	if !pasted {
		a.result.Interrupted = true
	} else {
		timer := time.NewTimer(pasteSettle)
		defer timer.Stop()
		select {
		case <-sess.Done():
			a.result.Interrupted = true
		case <-timer.C:
			_, committed := s.sessions.DeliveryInput(sess, []byte("\r"), next, false)
			a.result.Interrupted = !committed
		}
	}
	d.mu.Lock()
	// A missing receipt or refused Enter can leave bytes queued or a late draft.
	// Neither a later receipt nor an empty screen proves that draft was resolved.
	if queued && a.result.Interrupted {
		r.uncertain = true
	}
	r.sending = false
	close(a.done)
	d.mu.Unlock()
	return a.result, nil
}

func deliveryWritten(written, stopped <-chan struct{}, expired <-chan time.Time) bool {
	select {
	case <-written:
		return true
	case <-stopped:
	case <-expired:
	}
	return false
}

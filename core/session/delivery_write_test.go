package session

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/enowx/enowx/proto"
)

type deliveryWriteConn struct {
	entered, release, closed          chan struct{}
	enterOnce, releaseOnce, closeOnce sync.Once
}

func (c *deliveryWriteConn) Read([]byte) (int, error) { <-c.closed; return 0, io.EOF }
func (c *deliveryWriteConn) Write(p []byte) (int, error) {
	c.enterOnce.Do(func() { close(c.entered) })
	select {
	case <-c.release:
		return len(p), nil
	case <-c.closed:
		return 0, io.ErrClosedPipe
	}
}
func (c *deliveryWriteConn) Close() error { c.closeOnce.Do(func() { close(c.closed) }); return nil }
func (c *deliveryWriteConn) resume()      { c.releaseOnce.Do(func() { close(c.release) }) }

func TestDeliveryPasteCompletionKeepsCancellationHistory(t *testing.T) {
	for _, mode := range []string{"current", "manual", "hook"} {
		t.Run(mode, func(t *testing.T) {
			conn := &deliveryWriteConn{entered: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{})}
			term, err := terminal.New(terminal.Options{Conn: conn})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { conn.resume(); term.Close() })
			term.Resize(80, 24)
			term.Feed([]byte("› Ask Codex to do anything"))
			s := &Session{id: "write", term: term, done: make(chan struct{}), state: proto.StateDone}
			m := NewManager()
			m.sessions[s.id] = s
			next, written, ok := m.DeliveryPaste(s, []byte("\x1b[200~DRAFT\x1b[201~"), s.DeliveryGuard())
			if !ok {
				t.Fatal("paste refused")
			}
			select {
			case <-conn.entered:
			case <-time.After(time.Second):
				t.Fatal("writer not entered")
			}
			select {
			case <-written:
				t.Fatal("acknowledged pending write")
			default:
			}
			s.mu.Lock()
			s.lastInput = time.Now().Add(-time.Hour)
			s.mu.Unlock()
			switch mode {
			case "manual":
				m.Input(s, []byte("USER"))
			case "hook":
				m.SetState(s, proto.StateWaiting)
				m.SetState(s, proto.StateDone)
			}
			s.mu.Lock()
			before := s.lastInput
			s.mu.Unlock()
			conn.resume()
			select {
			case <-written:
			case <-time.After(time.Second):
				t.Fatal("write completion stranded")
			}
			s.mu.Lock()
			after := s.lastInput
			s.mu.Unlock()
			if mode == "current" {
				if !after.After(before) {
					t.Fatal("echo window still anchored to enqueue")
				}
				s.noteScreen(time.Now(), true)
				s.mu.Lock()
				changed := !s.lastChange.IsZero()
				s.mu.Unlock()
				if changed {
					t.Fatal("delayed paste echo counted as agent activity")
				}
			} else if !after.Equal(before) {
				t.Fatal("write receipt overwrote cancellation history")
			}
			_, committed := m.DeliveryInput(s, []byte("\r"), next, false)
			if committed != (mode == "current") {
				t.Fatalf("Enter after %s: %v", mode, committed)
			}
		})
	}
}

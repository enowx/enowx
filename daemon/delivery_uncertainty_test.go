package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

func TestDeliveryInterruptedPasteRequiresProgramReplacement(t *testing.T) {
	for _, mode := range []string{"manual", "hook"} {
		t.Run(mode, func(t *testing.T) {
			srv, c, info, path := deliveryFixture(t)
			sess, _ := srv.sessions.Get(info.ID)
			p := proto.DeliveryParams{ID: info.ID, Run: info.Run, Key: "old", Text: "UNCERTAIN_DRAFT"}
			type answer struct {
				r   proto.DeliveryResult
				err error
			}
			done := make(chan answer, 1)
			go func() { r, e := c.Deliver(p); done <- answer{r, e} }()
			waitDelivery(t, func() bool { return strings.Contains(sess.Terminal().Text(), p.Text) })
			if mode == "manual" {
				srv.sessions.Input(sess, []byte("USER"))
			} else {
				srv.sessions.SetState(sess, proto.StateWaiting)
				srv.sessions.SetState(sess, proto.StateDone)
			}
			select {
			case a := <-done:
				if a.err != nil || !a.r.Interrupted {
					t.Fatalf("delivery not interrupted: %+v %v", a.r, a.err)
				}
			case <-time.After(deliveryWriteLimit + time.Second):
				t.Fatal("interrupted delivery did not finish")
			}
			for range 3 {
				if r, e := c.Deliver(p); e != nil || !r.Interrupted {
					t.Fatalf("original result lost: %+v %v", r, e)
				}
			}
			assertBlocked := func(key string) {
				t.Helper()
				// A stale empty screen must not resolve a draft in the actual program.
				sess.Terminal().Feed([]byte("\x1b[2J\x1b[H› Ask Codex to do anything"))
				next := p
				next.Key = key
				next.Text = "NEVER_PASTED"
				if _, e := c.Deliver(next); e == nil || !strings.Contains(e.Error(), "unconfirmed paste") {
					t.Fatalf("new key allowed: %v", e)
				}
			}
			assertBlocked("empty-screen")
			// Deliberate terminal input remains available, but cannot prove safe
			// automatic recovery of all older queued bytes for this program run.
			srv.sessions.Input(sess, []byte("\r"))
			waitDelivery(t, func() bool { return len(capturedLines(path)) > 0 })
			if got := strings.Join(capturedLines(path), ""); strings.Count(got, p.Text) != 1 || strings.Contains(got, "NEVER_PASTED") {
				t.Fatalf("draft changed: %q", got)
			}
			srv.sessions.SetState(sess, proto.StateDone)
			assertBlocked("manual-recovery-and-empty-screen")

			bin, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			newPath := filepath.Join(t.TempDir(), "replacement.jsonl")
			replacement, err := srv.sessions.Replace(session.Spec{ID: info.ID, Command: []string{bin, "-test.run=^TestDeliveryInputCapture$"}, Env: []string{"ENOWX_DELIVERY_CAPTURE=" + newPath}, Cols: 100, Rows: 24})
			if err != nil {
				t.Fatal(err)
			}
			waitDelivery(t, func() bool { b, _ := os.ReadFile(newPath); return strings.HasPrefix(string(b), "ready\n") })
			srv.sessions.SetState(replacement, proto.StateDone)
			waitDelivery(t, func() bool { return strings.Contains(replacement.Terminal().Text(), "›") })
			if _, e := c.Deliver(p); e == nil {
				t.Fatal("old run accepted after program replacement")
			}
			p.Run, p.Key, p.Text = replacement.Info().Run, "fresh", "FRESH_PROGRAM"
			if r, e := c.Deliver(p); e != nil || r.Interrupted {
				t.Fatalf("replacement still quarantined: %+v %v", r, e)
			}
			waitDelivery(t, func() bool { return strings.Contains(strings.Join(capturedLines(newPath), ""), p.Text) })
			t.Log("new keys refused after manual/hook cancellation and manual recovery; replacement accepts its own clean delivery")
		})
	}
}

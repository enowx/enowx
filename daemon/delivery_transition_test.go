package daemon

import (
	"strings"
	"testing"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

func TestDeliveryTransitionsDuringSettle(t *testing.T) {
	for _, mode := range []string{"waiting", "working", "hook-round-trip", "menu", "menu-round-trip", "working-screen", "permission-mode", "replace", "kill"} {
		t.Run(mode, func(t *testing.T) {
			srv, c, info, path := deliveryFixture(t)
			sess, _ := srv.sessions.Get(info.ID)
			p := proto.DeliveryParams{ID: info.ID, Run: info.Run, Key: "transition", Text: "UNSUBMITTED_DRAFT"}
			type answer struct {
				result proto.DeliveryResult
				err    error
			}
			done := make(chan answer, 1)
			go func() { r, e := c.Deliver(p); done <- answer{r, e} }()
			waitDelivery(t, func() bool { return strings.Contains(sess.Terminal().Text(), p.Text) })
			switch mode {
			case "waiting", "working":
				if err := srv.sessions.SetState(sess, mode); err != nil {
					t.Fatal(err)
				}
			case "hook-round-trip":
				srv.sessions.SetState(sess, proto.StateWaiting)
				srv.sessions.SetState(sess, proto.StateDone)
			case "menu", "menu-round-trip":
				sess.Terminal().Feed([]byte("\x1b[2J\x1b[HDo you want to proceed?\r\n > 1. Yes\r\n   2. No\r\n"))
				if mode == "menu-round-trip" {
					sess.NoteFrame(true)
					sess.Terminal().Feed([]byte("\x1b[2J\x1b[H› " + p.Text))
					sess.NoteFrame(true)
				}
			case "working-screen":
				sess.Terminal().Feed([]byte("\x1b[2J\x1b[H✶ Thinking… (esc to interrupt)\r\n"))
			case "permission-mode":
				srv.sessions.SetMode(sess, "review")
			case "replace":
				if _, err := srv.sessions.Replace(session.Spec{ID: info.ID, Exited: true}); err != nil {
					t.Fatal(err)
				}
			case "kill":
				if err := srv.sessions.Kill(info.ID); err != nil {
					t.Fatal(err)
				}
			}
			a := <-done
			if a.err != nil || !a.result.Interrupted {
				t.Fatalf("transition: %+v %v", a.result, a.err)
			}
			if mode == "replace" || mode == "kill" {
				return
			}
			for range 4 {
				r, err := c.Deliver(p)
				if err != nil || !r.Interrupted {
					t.Fatalf("retry: %+v %v", r, err)
				}
			}
			if got := capturedLines(path); len(got) != 0 {
				t.Fatalf("submitted draft: %q", got)
			}
			// Only an explicit manual Enter may submit the preserved single paste.
			srv.sessions.Input(sess, []byte("\r"))
			waitDelivery(t, func() bool { return len(capturedLines(path)) > 0 })
			if got := strings.Join(capturedLines(path), ""); strings.Count(got, p.Text) != 1 {
				t.Fatalf("repasted draft: %q", got)
			}
		})
	}
}

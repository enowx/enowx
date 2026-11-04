package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// This child process records lines read from its own PTY, rather than treating
// screen echo as evidence of receipt. Its parent closes only this fixture.
func TestDeliveryInputCapture(t *testing.T) {
	path := os.Getenv("ENOWX_DELIVERY_CAPTURE")
	if path == "" {
		t.Skip("PTY helper")
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("ready\n")
	fmt.Print("› ")
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		b, _ := json.Marshal(line)
		fmt.Fprintln(f, string(b))
	}
}

func deliveryFixture(t *testing.T) (*Server, *client.Client, proto.SessionInfo, string) {
	t.Helper()
	srv := startServer(t)
	c := connect(t, srv, nil)
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "received.jsonl")
	info, err := c.CreateSession(proto.SessionCreateParams{Command: []string{bin, "-test.run=^TestDeliveryInputCapture$"}, Env: []string{"ENOWX_DELIVERY_CAPTURE=" + path}, Cols: 100, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	waitDelivery(t, func() bool { b, _ := os.ReadFile(path); return strings.HasPrefix(string(b), "ready\n") })
	sess, _ := srv.sessions.Get(info.ID)
	srv.sessions.SetState(sess, proto.StateDone)
	waitDelivery(t, func() bool { return strings.Contains(sess.Terminal().Text(), "›") })
	return srv, c, info, path
}

func waitDelivery(t *testing.T, f func() bool) {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if f() {
			return
		}
	}
	t.Fatal("delivery fixture timed out")
}

func capturedLines(path string) []string {
	b, _ := os.ReadFile(path)
	var lines []string
	for _, row := range strings.Split(string(b), "\n") {
		var line string
		if json.Unmarshal([]byte(row), &line) == nil {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestDeliveryRetriesQueueInputOnce(t *testing.T) {
	srv, c, info, path := deliveryFixture(t)
	p := proto.DeliveryParams{ID: info.ID, Run: info.Run, Key: "once", Text: "a long\npasted message"}
	// Separate connections retry even while the first request is settling.
	var wg sync.WaitGroup
	for range 4 {
		other := connect(t, srv, nil)
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := other.Deliver(p)
			if err != nil || result.Interrupted {
				t.Errorf("delivery: %+v %v", result, err)
			}
		}()
	}
	wg.Wait()
	for range 4 {
		if _, err := c.Deliver(p); err != nil {
			t.Fatal(err)
		}
	}
	waitDelivery(t, func() bool { return strings.Contains(strings.Join(capturedLines(path), ""), "pasted message") })
	// The multiline paste may be read as several lines by this deliberately
	// simple helper; its text must occur once in the process's record.
	lines := strings.Join(capturedLines(path), "")
	if strings.Count(lines, "pasted message") != 1 {
		t.Fatalf("input was repeated: %q", lines)
	}
	p.Text = "different text"
	if _, err := c.Deliver(p); err == nil {
		t.Fatal("reused a key for different text")
	}
}

func TestDeliveryManualInputCancelsEnterAndRetry(t *testing.T) {
	srv, c, info, path := deliveryFixture(t)
	p := proto.DeliveryParams{ID: info.ID, Run: info.Run, Key: "manual", Text: "APP_DRAFT"}
	st, err := c.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	result := make(chan proto.DeliveryResult, 1)
	go func() {
		r, e := c.Deliver(p)
		if e != nil {
			t.Error(e)
		}
		result <- r
	}()
	sess, _ := srv.sessions.Get(info.ID)
	// Echo is used only to coordinate the intervention, never as an ack.
	waitDelivery(t, func() bool { return strings.Contains(sess.Terminal().Text(), "APP_DRAFT") })
	if _, err := st.Write([]byte("MANUAL")); err != nil {
		t.Fatal(err)
	}
	waitDelivery(t, func() bool { return strings.Contains(sess.Terminal().Text(), "MANUAL") })
	if r := <-result; !r.Interrupted {
		t.Fatal("pressed Enter after manual input")
	}
	for range 4 {
		r, e := c.Deliver(p)
		if e != nil || !r.Interrupted {
			t.Fatalf("retry: %+v %v", r, e)
		}
	}
	if got := capturedLines(path); len(got) != 0 {
		t.Fatalf("submitted a user's draft: %q", got)
	}
	// Only a deliberate manual Enter may submit that now-mixed draft.
	srv.sessions.Input(sess, []byte("\r"))
	waitDelivery(t, func() bool { return len(capturedLines(path)) > 0 })
	if got := strings.Join(capturedLines(path), ""); strings.Count(got, "APP_DRAFT") != 1 || !strings.Contains(got, "MANUAL") {
		t.Fatalf("draft changed: %q", got)
	}
}

func TestDeliveryLostResponseDoesNotReplay(t *testing.T) {
	srv, c, info, path := deliveryFixture(t)
	p := proto.DeliveryParams{ID: info.ID, Run: info.Run, Key: "lost-response", Text: "LOST_RESPONSE_ONCE"}
	done := make(chan error, 1)
	go func() { _, err := c.Deliver(p); done <- err }()
	sess, _ := srv.sessions.Get(info.ID)
	waitDelivery(t, func() bool {
		srv.deliveries.mu.Lock()
		defer srv.deliveries.mu.Unlock()
		r := srv.deliveries.runs[sess]
		return r != nil && r.entries[p.Key] != nil
	})
	c.Close() // the accepted request finishes independently of its response
	if err := <-done; err == nil {
		t.Fatal("fixture didn't lose the response")
	}
	other := connect(t, srv, nil)
	if r, err := other.Deliver(p); err != nil || r.Interrupted {
		t.Fatalf("reconnected retry: %+v %v", r, err)
	}
	waitDelivery(t, func() bool { return strings.Contains(strings.Join(capturedLines(path), ""), p.Text) })
	if got := strings.Join(capturedLines(path), ""); strings.Count(got, p.Text) != 1 {
		t.Fatalf("replayed after losing response: %q", got)
	}
}

func TestDeliveryRefusesBusyWaitingAndDifferentRun(t *testing.T) {
	srv, c, info, path := deliveryFixture(t)
	sess, _ := srv.sessions.Get(info.ID)
	p := proto.DeliveryParams{ID: info.ID, Run: info.Run, Key: "held", Text: "never pasted"}
	for _, state := range []string{proto.StateWorking, proto.StateWaiting} {
		srv.sessions.SetState(sess, state)
		if _, err := c.Deliver(p); err == nil {
			t.Fatal("pasted into busy/menu terminal", state)
		}
	}
	srv.sessions.SetState(sess, proto.StateDone)
	p.Run++
	if _, err := c.Deliver(p); err == nil {
		t.Fatal("accepted a different run")
	}
	if got := capturedLines(path); len(got) != 0 {
		t.Fatal(got)
	}
	// Replacing the program invalidates even the original token/run pair.
	_, err := srv.sessions.Replace(session.Spec{ID: info.ID, Exited: true})
	if err != nil {
		t.Fatal(err)
	}
	p.Run = info.Run
	if _, err := c.Deliver(p); err == nil {
		t.Fatal("replayed into a replacement program")
	}
}

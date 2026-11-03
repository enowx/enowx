package daemon

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

func startServer(t *testing.T) *Server {
	t.Helper()
	srv, err := NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	return srv
}

func connect(t *testing.T, srv *Server, onEvent func(proto.Event)) *client.Client {
	t.Helper()
	a, b := net.Pipe()
	go srv.ServeConn(a)
	c, err := client.New(b, onEvent)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// screen collects what a stream sends, as a terminal would read it.
type screen struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	size [2]int
}

func watch(st *client.Stream) *screen {
	s := &screen{}
	st.OnResize = func(cols, rows int) {
		s.mu.Lock()
		s.size = [2]int{cols, rows}
		s.mu.Unlock()
	}
	go func() {
		p := make([]byte, 4096)
		for {
			n, err := st.Read(p)
			s.mu.Lock()
			s.buf.Write(p[:n])
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return s
}

func (s *screen) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		ok := strings.Contains(s.buf.String(), want)
		s.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t.Fatalf("timed out waiting for %q; got %q", want, s.buf.String())
}

func (s *screen) waitSize(t *testing.T, cols, rows int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		got := s.size
		s.mu.Unlock()
		if got == [2]int{cols, rows} {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("size never became %dx%d", cols, rows)
}

func catCommand() []string {
	if runtime.GOOS == "windows" {
		return []string{"powershell", "-NoProfile", "-Command", "$host.UI.ReadLine() | Write-Output; Start-Sleep 30"}
	}
	return []string{"sh", "-c", "echo ready; cat"}
}

func TestSessionSurvivesReattach(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv := startServer(t)

	// First client: start a session, type into it, then disconnect.
	c1 := connect(t, srv, nil)
	info, err := c1.CreateSession(proto.SessionCreateParams{Command: catCommand(), Cols: 60, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	st, err := c1.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	s1 := watch(st)
	s1.waitSize(t, 60, 10)
	s1.waitFor(t, "ready")
	if _, err := st.Write([]byte("hello from client one\r")); err != nil {
		t.Fatal(err)
	}
	s1.waitFor(t, "hello from client one")
	c1.Close()

	// Second client: the session is still there and its screen comes back.
	c2 := connect(t, srv, nil)
	list, err := c2.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != info.ID || list[0].Exited {
		t.Fatalf("sessions = %+v", list)
	}
	st2, err := c2.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	s2 := watch(st2)
	s2.waitFor(t, "hello from client one")

	// Resizing from the client is announced back before the new frames.
	if err := st2.ResizePixels(100, 30, 8, 16); err != nil {
		t.Fatal(err)
	}
	s2.waitSize(t, 100, 30)
}

func TestEventsReportExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv := startServer(t)
	exited := make(chan proto.SessionInfo, 8)
	c := connect(t, srv, func(ev proto.Event) {
		if ev.Name != proto.EventSessionUpdated {
			return
		}
		var info proto.SessionInfo
		if err := json.Unmarshal(ev.Data, &info); err == nil && info.Exited {
			exited <- info
		}
	})
	if _, err := c.CreateSession(proto.SessionCreateParams{Command: []string{"sh", "-c", "exit 3"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case info := <-exited:
		if info.ExitCode != 3 {
			t.Fatalf("exit code = %d, want 3", info.ExitCode)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no exit event")
	}
}

func TestUnknownMethod(t *testing.T) {
	c := connect(t, startServer(t), nil)
	if err := c.Call("nope", nil, nil); err == nil || !strings.Contains(err.Error(), "unknown method") {
		t.Fatalf("err = %v", err)
	}
	if _, err := c.Info(); err != nil {
		t.Fatal(err)
	}
}

func TestBadProtocolVersionIsRejected(t *testing.T) {
	srv := startServer(t)
	a, b := net.Pipe()
	go srv.ServeConn(a)
	go func() {
		// Older than the oldest version the daemon speaks.
		_ = proto.WriteFrame(b, proto.Frame{Type: proto.TypeHello, Payload: []byte(`{"protocol":0}`)})
	}()
	if _, err := proto.ReadFrame(b); err != io.EOF && err != io.ErrClosedPipe {
		t.Fatalf("want connection closed, got %v", err)
	}
}

func TestKilledSessionIsNotAnnouncedAgain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv := startServer(t)
	var mu sync.Mutex
	var names []string
	c := connect(t, srv, func(ev proto.Event) {
		mu.Lock()
		names = append(names, ev.Name)
		mu.Unlock()
	})
	info, err := c.CreateSession(proto.SessionCreateParams{Command: []string{"sh", "-c", "sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.KillSession(info.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // let the exit callback run
	mu.Lock()
	defer mu.Unlock()
	if len(names) == 0 || names[len(names)-1] != proto.EventSessionRemoved {
		t.Fatalf("events = %v, want session.removed last", names)
	}
}

func TestActivityFollowsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	for _, attached := range []bool{false, true} {
		srv := startServer(t)
		c := connect(t, srv, nil)
		// Quiet, then output for a while, then quiet again.
		info, err := c.CreateSession(proto.SessionCreateParams{Cols: 80, Rows: 24,
			Command: []string{"sh", "-c", "sleep 5; for i in $(seq 30); do echo $i; sleep 0.05; done; sleep 30"}})
		if err != nil {
			t.Fatal(err)
		}
		if attached {
			st, err := c.Attach(info.ID)
			if err != nil {
				t.Fatal(err)
			}
			watch(st)
		}
		sess, err := srv.sessions.Get(info.ID)
		if err != nil {
			t.Fatal(err)
		}
		waitActive := func(want bool) {
			t.Helper()
			deadline := time.Now().Add(20 * time.Second)
			for sess.Info().Active != want {
				if time.Now().After(deadline) {
					t.Fatalf("attached %v: active never became %v", attached, want)
				}
				time.Sleep(20 * time.Millisecond)
			}
		}
		time.Sleep(2 * time.Second) // the program's start settles, before its output
		if sess.Info().Active {
			t.Fatalf("attached %v: active while the program is quiet", attached)
		}
		waitActive(true)
		waitActive(false)
	}
}

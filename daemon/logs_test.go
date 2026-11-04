package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/proto"
)

// loggedServer is a server writing its diagnostics to a log in its folder.
func loggedServer(t *testing.T) (*Server, *diag.Log, func() string) {
	t.Helper()
	srv := startServer(t)
	l := diag.Open(srv.dataDir, diag.DaemonLog)
	t.Cleanup(l.Close)
	srv.Diag = l
	return srv, l, func() string {
		t.Helper()
		l.Flush()
		data, err := os.ReadFile(filepath.Join(srv.dataDir, diag.DaemonLog))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
}

func waitForLog(t *testing.T, read func() string, want ...string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		text := read()
		missing := ""
		for _, w := range want {
			if !strings.Contains(text, w) {
				missing = w
			}
		}
		if missing == "" {
			return text
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %q in the log:\n%s", missing, text)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// An app's connection is logged at both ends under one ID, with who ended it; a
// command's ordinary one isn't.
func TestConnectionsAreLoggedByID(t *testing.T) {
	srv, _, read := loggedServer(t)
	old := client.Kind
	client.Kind = "app"
	t.Cleanup(func() { client.Kind = old })
	c := connect(t, srv, nil)
	if c.ID() == "" {
		t.Fatal("no ID")
	}
	waitForLog(t, read, "conn.open conn="+c.ID(), "kind=app")
	c.Close()
	<-c.Done()
	waitForLog(t, read, "conn.closed conn="+c.ID(), "by=client")
	if by := c.ClosedBy(); by != "client" {
		t.Fatalf("the client says it was closed by %q", by)
	}
	client.Kind = "cli"
	cmd := connect(t, srv, nil)
	if _, err := cmd.Info(); err != nil {
		t.Fatal(err)
	}
	cmd.Close()
	<-cmd.Done()
	time.Sleep(50 * time.Millisecond)
	if text := read(); strings.Contains(text, cmd.ID()) {
		t.Fatalf("a command's connection was logged:\n%s", text)
	}
}

// The daemon ending a connection says so, on its side and the client's.
func TestDaemonClosingIsSaidOnBothSides(t *testing.T) {
	srv, _, read := loggedServer(t)
	old := client.Kind
	client.Kind = "app"
	t.Cleanup(func() { client.Kind = old })
	c := connect(t, srv, nil)
	waitForLog(t, read, "conn.open conn="+c.ID())
	srv.mu.Lock()
	for sc := range srv.conns {
		sc.close()
	}
	srv.mu.Unlock()
	<-c.Done()
	waitForLog(t, read, "conn.closed conn="+c.ID(), "by=daemon")
	if by := c.ClosedBy(); by != "daemon" {
		t.Fatalf("the client says it was closed by %q", by)
	}
}

// A frame too large is logged with what it was and its size; slow or large calls too.
func TestProtocolFailuresAreLogged(t *testing.T) {
	srv, _, read := loggedServer(t)
	maker := connect(t, srv, nil)
	cv := bigDesign(t, srv, maker)
	nc, frames := oldApp(t, srv)
	if r := oldCall(t, nc, frames, 1, proto.MethodBrowserServe, struct{}{}); r.Error != "" {
		t.Fatal(r.Error)
	}
	if _, err := maker.RenderCanvas(proto.CanvasRenderParams{ID: cv.ID, Frame: cv.Frames[0].ID, Format: proto.RenderPNG}); err == nil {
		t.Fatal("rendered")
	}
	waitForLog(t, read, "proto.too_large", `"event canvas.render"`, "bytes=")
	if _, err := maker.RenderCanvas(proto.CanvasRenderParams{ID: cv.ID, Frame: cv.Frames[0].ID, Format: proto.RenderHTML}); err != nil {
		t.Fatal(err)
	}
	waitForLog(t, read, "rpc.slow", "method=canvas.render", "replyBytes=")
}

// Nothing a session or a canvas holds reaches the log: only names, IDs and sizes.
func TestLogHoldsNoContent(t *testing.T) {
	srv, _, read := loggedServer(t)
	const secret, words = "sk-ant-api03-abcdefghijklmnopqrstuvwx", "the-users-private-words"
	old := client.Kind
	client.Kind = "app"
	t.Cleanup(func() { client.Kind = old })
	c := connect(t, srv, nil)
	cv, err := c.CreateCanvas(proto.CanvasCreateParams{Title: words, Kind: proto.CanvasMockup})
	if err != nil {
		t.Fatal(err)
	}
	big := `<div id="a">` + words + secret + strings.Repeat("x", 18<<20) + `</div>`
	if _, err := c.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLPatch, HTML: big}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Canvas(cv.ID); err != nil {
		t.Fatal(err)
	}
	sess, err := c.CreateSession(proto.SessionCreateParams{Name: "s", Command: []string{"/bin/sh", "-c", "sleep 30"}, Prompt: words + " " + secret})
	if err == nil {
		_ = c.KillSession(sess.ID)
	}
	var bad struct{}
	_ = c.Call("no.such.method", map[string]string{"token": secret, "text": words}, &bad)
	// A log from an older version, which may hold anything, is masked in a bundle.
	text := waitForLog(t, read, "rpc.slow")
	for _, v := range []string{secret, words, "abcdefghijklmnopqrstuvwx"} {
		if strings.Contains(text, v) {
			t.Fatalf("%q reached the log:\n%s", v, text)
		}
	}
	for _, f := range srv.diagnostics().Files {
		if strings.Contains(f.Text, secret) || strings.Contains(f.Text, words) {
			t.Fatalf("%q in the bundle's %s", secret, f.Name)
		}
	}
}

// Another host's logs reach the app through the daemon, masked.
func TestDiagnosticsReadsTheHostsLogs(t *testing.T) {
	srv, l, _ := loggedServer(t)
	l.Event("conn.open", "conn", "abc", "err", "password=hunter2hunter2")
	l.Flush()
	c := connect(t, srv, nil)
	var out proto.Diagnostics
	if err := c.Call(proto.MethodDaemonDiagnostics, nil, &out); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), "conn.open") || strings.Contains(string(raw), "hunter2") {
		t.Fatalf("%s", raw)
	}
	if diag.Held(srv.dataDir) {
		t.Fatal("the hold stayed")
	}
}

package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hostcontrol"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// standInApp attaches to srv as the app does and answers its host.call
// requests the way the app's relay does (ui/hostcontrol.go): checked,
// asked of the user (yes says what they answer) for what an agent can't
// undo, then done on the host named, among hosts.
func standInApp(t *testing.T, srv *Server, hosts map[string]*client.Client, yes func(proto.HostCallParams) bool) {
	t.Helper()
	var app *client.Client
	app = connect(t, srv, func(ev proto.Event) {
		if ev.Name != proto.EventAppRequest {
			return
		}
		var req proto.AppRequest
		_ = json.Unmarshal(ev.Data, &req)
		go func() {
			rep := proto.AppReply{ID: req.ID}
			res, err := func() (json.RawMessage, error) {
				var p proto.HostCallParams
				if err := json.Unmarshal(req.Params, &p); err != nil {
					return nil, err
				}
				level, err := hostcontrol.Check(p.Method, "")
				if err != nil {
					return nil, err
				}
				cl := hosts[p.Host]
				if cl == nil {
					return nil, errors.New("no host " + p.Host)
				}
				if level == hostcontrol.Confirm && p.From != "" && !yes(p) {
					return nil, errors.New("the user said no")
				}
				var out json.RawMessage
				var params any
				if len(p.Params) > 0 {
					params = p.Params
				}
				err = cl.Call(p.Method, params, &out)
				return out, err
			}()
			if err != nil {
				rep.Error = err.Error()
			}
			rep.Result = res
			_ = app.ReplyToApp(rep)
		}()
	})
	if err := app.ServeApp(); err != nil {
		t.Fatal(err)
	}
}

// waitScreen waits for session id's screen on cl to show want.
func waitScreen(t *testing.T, cl *client.Client, id, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		text, err := cl.Screen(id)
		if err == nil && strings.Contains(text, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the screen never showed %q: %q %v", want, text, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A session on host A controls host B through the app: starts a session
// there, types into it and reads it, renames and interrupts it, closes it
// once the user says yes; a method out of control's reach is refused, and
// every request is in A's log. With no app, only A answers, and closing
// from an agent says it needs the app.
func TestSessionControlsAnotherHostThroughTheApp(t *testing.T) {
	a, b := startServer(t), startServer(t)
	ca := connect(t, a, nil)

	// No app: this host still answers, another host and a confirmed close
	// say why they can't.
	here, err := ca.OnHost("", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := here.Sessions(); err != nil {
		t.Fatalf("this host, no app: %v", err)
	}
	there, err := ca.OnHost(b.daemonID, "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := there.Sessions(); err == nil || !strings.Contains(err.Error(), "only the enowX app reaches other hosts") {
		t.Fatalf("another host, no app: %v", err)
	}
	local, err := a.sessions.Create(sessionSpec(t, "cat"))
	if err != nil {
		t.Fatal(err)
	}
	if err := here.KillSession(local.ID()); err == nil || !strings.Contains(err.Error(), "needs the user's yes") {
		t.Fatalf("a close from an agent, no app: %v", err)
	}
	if err := ca.HostCall("", "", proto.MethodSessionKill, proto.SessionRef{ID: local.ID()}, nil); err != nil {
		t.Fatalf("the user's own terminal closes without asking: %v", err)
	}

	// The app open, reaching B.
	var said atomic.Bool
	var asked atomic.Int32
	standInApp(t, a, map[string]*client.Client{b.daemonID: connect(t, b, nil), a.daemonID: connect(t, a, nil)}, func(proto.HostCallParams) bool {
		asked.Add(1)
		return said.Load()
	})
	info, err := there.CreateSession(proto.SessionCreateParams{Command: []string{"cat"}, Dir: t.TempDir(), Name: "worker", Cols: 80, Rows: 20})
	if err != nil {
		t.Fatalf("create on B: %v", err)
	}
	if _, err := b.sessions.Get(info.ID); err != nil {
		t.Fatalf("the session isn't on B: %v", err)
	}
	if _, err := a.sessions.Get(info.ID); err == nil {
		t.Fatal("the session is on A")
	}
	if err := there.Input(info.ID, "hello there", false, true); err != nil {
		t.Fatal(err)
	}
	waitScreen(t, there, info.ID, "hello there")
	if err := there.RenameSession(info.ID, "helper"); err != nil {
		t.Fatal(err)
	}
	list, err := there.Sessions()
	if err != nil || len(list) != 1 || list[0].Name != "helper" {
		t.Fatalf("B's sessions, renamed: %+v %v", list, err)
	}
	// Out of reach: the vault, whatever the host.
	if err := there.Call(proto.MethodVaultGet, nil, nil); err == nil || !strings.Contains(err.Error(), "can't be used") {
		t.Fatalf("the vault through control: %v", err)
	}
	// Closing asks the user: no, then yes.
	if err := there.KillSession(info.ID); err == nil || !strings.Contains(err.Error(), "said no") {
		t.Fatalf("a close the user refused: %v", err)
	}
	if _, err := b.sessions.Get(info.ID); err != nil {
		t.Fatal("closed though the user said no")
	}
	said.Store(true)
	if err := there.KillSession(info.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.sessions.Get(info.ID); err == nil {
		t.Fatal("still there after the user said yes")
	}
	if asked.Load() != 2 {
		t.Fatalf("the user was asked %d times", asked.Load())
	}
	// A close on A itself, from an agent, asks too.
	again, _ := a.sessions.Create(sessionSpec(t, "cat"))
	if err := here.KillSession(again.ID()); err != nil {
		t.Fatal(err)
	}
	if asked.Load() != 3 {
		t.Fatal("a close of this host's session from an agent wasn't asked")
	}

	log, err := ca.ControlLog()
	if err != nil {
		t.Fatal(err)
	}
	var onB, refused int
	for _, e := range log {
		if e.Host == b.daemonID {
			onB++
		}
		if e.Err != "" {
			refused++
		}
	}
	if onB < 6 || refused < 3 {
		t.Fatalf("the log: %d on B, %d refused: %+v", onB, refused, log)
	}
}

func sessionSpec(t *testing.T, cmd ...string) session.Spec {
	return session.Spec{Command: cmd, Dir: t.TempDir()}
}

// An agent's interrupt reaches its program as its user's key would; a
// session waiting on nothing has nothing to answer.
func TestInterruptAndAnswerFromOutside(t *testing.T) {
	srv := startServer(t)
	c := connect(t, srv, nil)
	info, err := c.CreateSession(proto.SessionCreateParams{Command: []string{"cat"}, Dir: t.TempDir(), Cols: 80, Rows: 20})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := c.SessionAsk(info.ID)
	if err != nil || ask.Waiting {
		t.Fatalf("ask: %+v %v", ask, err)
	}
	if _, err := c.AnswerSession(info.ID, "allow"); err == nil || !strings.Contains(err.Error(), "isn't waiting") {
		t.Fatalf("an answer to nothing: %v", err)
	}
	if err := c.InterruptSession(info.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the program ends", func() bool {
		s, err := srv.sessions.Get(info.ID)
		return err == nil && s.Info().Exited
	})
}

// A project's settings are read and written as .enowx.toml, its other
// tables kept; a folder that isn't a project is refused.
func TestProjectConfigThroughTheDaemon(t *testing.T) {
	srv := startServer(t)
	c := connect(t, srv, nil)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".enowx.toml"), []byte("[other]\nkeep = true\n"), 0o644)
	if _, err := c.ProjectConfig(dir); err == nil {
		t.Fatal("read the settings of a folder that isn't a project")
	}
	if _, err := c.AddProject(dir); err != nil {
		t.Fatal(err)
	}
	cfg, err := c.SaveProjectConfig(proto.ProjectConfig{Path: dir, Copy: []string{".env"}, Setup: "npm install"})
	if err != nil || len(cfg.Copy) != 1 || cfg.Setup != "npm install" {
		t.Fatalf("saved: %+v %v", cfg, err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".enowx.toml"))
	if !strings.Contains(string(data), "keep = true") || !strings.Contains(string(data), "npm install") {
		t.Fatalf("the file: %s", data)
	}
}

// Every method control reaches is one this daemon answers.
func TestControlMethodsHaveHandlers(t *testing.T) {
	for _, m := range hostcontrol.Methods() {
		if _, ok := handlers[m]; !ok {
			t.Errorf("control reaches %s, which has no handler", m)
		}
	}
}

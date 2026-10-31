package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hostcontrol"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// dialServer is a client of a daemon of its own, in dir.
func dialServer(t *testing.T, srv *daemon.Server, onEvent func(proto.Event)) *client.Client {
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

func newServer(t *testing.T) *daemon.Server {
	t.Helper()
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	return srv
}

// relayApp stands in for the app on srv: it does host.call requests on the
// hosts it reaches, asking yes() for what an agent can't undo.
func relayApp(t *testing.T, srv *daemon.Server, hosts map[string]*client.Client, yes func() bool) {
	t.Helper()
	var app *client.Client
	app = dialServer(t, srv, func(ev proto.Event) {
		if ev.Name != proto.EventAppRequest {
			return
		}
		var req proto.AppRequest
		_ = json.Unmarshal(ev.Data, &req)
		go func() {
			var p proto.HostCallParams
			_ = json.Unmarshal(req.Params, &p)
			rep := proto.AppReply{ID: req.ID}
			level, err := hostcontrol.Check(p.Method, "")
			cl := hosts[p.Host]
			switch {
			case err != nil:
			case cl == nil:
				err = errors.New("no host " + p.Host)
			case level == hostcontrol.Confirm && p.From != "" && !yes():
				err = errors.New("the user said no")
			default:
				var params any
				if len(p.Params) > 0 {
					params = p.Params
				}
				var res json.RawMessage
				err = cl.Call(p.Method, params, &res)
				rep.Result = res
			}
			if err != nil {
				rep.Error = err.Error()
			}
			_ = app.ReplyToApp(rep)
		}()
	})
	if err := app.ServeApp(); err != nil {
		t.Fatal(err)
	}
}

// From a session on host A, the same commands with --host reach host B
// through the app: a session started, listed, renamed, closed once the
// user says yes; B's settings and a project's changed there; what can't
// be undone refused without --yes, from the command line and MCP alike.
func TestCommandsReachAnotherHost(t *testing.T) {
	a, b := newServer(t), newServer(t)
	ca, cb := dialServer(t, a, nil), dialServer(t, b, nil)
	infoA, _ := ca.Info()
	infoB, _ := cb.Info()
	var yes atomic.Bool
	relayApp(t, a, map[string]*client.Client{infoB.ID: dialServer(t, b, nil), infoA.ID: dialServer(t, a, nil)}, yes.Load)
	var out bytes.Buffer
	r := runner{c: ca, out: &out}
	run := func(args ...string) (string, error) {
		out.Reset()
		err := r.command(args)
		return out.String(), err
	}
	t.Setenv(EnvSession, "")

	project := t.TempDir()
	if _, err := run("spawn", "--host", infoB.ID, "--agent", "shell", "--config", "official", "--model", "m", "--effort", "high", "--name", "worker", "--project", project); err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(out.String())
	if _, err := ca.Sessions(); err != nil {
		t.Fatal(err)
	}
	listB, _ := cb.Sessions()
	if len(listB) != 1 || listB[0].ID != id {
		t.Fatalf("B's sessions: %+v (spawned %q)", listB, id)
	}
	if listA, _ := ca.Sessions(); len(listA) != 0 {
		t.Fatalf("A has sessions: %+v", listA)
	}
	if got, err := run("ls", "--host", infoB.ID); err != nil || !strings.Contains(got, id) {
		t.Fatalf("ls --host: %q %v", got, err)
	}
	if _, err := run("rename", id, "helper", "--host", infoB.ID); err != nil {
		t.Fatal(err)
	}
	if listB, _ := cb.Sessions(); listB[0].Name != "helper" {
		t.Fatalf("renamed on B: %+v", listB[0])
	}
	if _, err := run("spawn", "--host", infoB.ID, "no project"); err == nil || !strings.Contains(err.Error(), "--project") {
		t.Fatalf("spawn on another host without its folder: %v", err)
	}

	// B's settings and a project's, from A.
	if _, err := run("settings", "set", "max-agents", "3", "--host", infoB.ID); err != nil {
		t.Fatal(err)
	}
	if st, _ := cb.Settings(); st.MaxAgents != 3 {
		t.Fatalf("B's max agents: %d", st.MaxAgents)
	}
	if st, _ := ca.Settings(); st.MaxAgents == 3 {
		t.Fatal("A's settings changed")
	}
	if got, err := run("project", "settings", project, "--copy", ".env", "--host", infoB.ID); err != nil || !strings.Contains(got, ".env") {
		t.Fatalf("project settings on B: %q %v", got, err)
	}

	// What can't be undone: --yes first, then the user, from an agent.
	if _, err := run("close", id, "--host", infoB.ID); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("close without --yes: %v", err)
	}
	if _, err := run("changes", id, "push", "--host", infoB.ID); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("push without --yes: %v", err)
	}
	t.Setenv(EnvSession, "agent-on-a")
	if _, err := run("close", id, "--yes", "--host", infoB.ID); err == nil || !strings.Contains(err.Error(), "said no") {
		t.Fatalf("a close the user refused: %v", err)
	}
	tool := toolNamed(t, "session_close")
	if res := r.callTool(tool, runner.command, map[string]any{"id": id, "host": infoB.ID, "yes": false}); res["isError"] != true {
		t.Fatalf("session_close without yes: %+v", res)
	}
	yes.Store(true)
	if res := r.callTool(tool, runner.command, map[string]any{"id": id, "host": infoB.ID, "yes": true}); res["isError"] != false {
		t.Fatalf("session_close: %+v", res)
	}
	if listB, _ := cb.Sessions(); len(listB) != 0 {
		t.Fatalf("still on B: %+v", listB)
	}
	if got, err := run("control", "log"); err != nil || !strings.Contains(got, "session.kill") || !strings.Contains(got, "said no") {
		t.Fatalf("the log: %q %v", got, err)
	}
}

// session_spawn on another host passes its config, model and effort as
// flags before the prompt, and the host for runner.command to take.
func TestSpawnToolOnAnotherHost(t *testing.T) {
	args := toolNamed(t, "session_spawn").args(map[string]any{"prompt": "fix it", "config": "work", "model": "opus", "effort": "high",
		"project": "/srv/app", "host": "vps"})
	host, rest, ok := hostFlag(args)
	got := strings.Join(rest, " ")
	if !ok || host != "vps" || !strings.HasSuffix(got, " fix it") || !strings.Contains(got, "--config work") ||
		!strings.Contains(got, "--model opus") || !strings.Contains(got, "--effort high") || !strings.Contains(got, "--project /srv/app") {
		t.Fatalf("%q on %q", got, host)
	}
}

func toolNamed(t *testing.T, name string) mcpTool {
	t.Helper()
	for _, g := range mcpGroups() {
		for _, tool := range g.tools {
			if tool.name == name {
				return tool
			}
		}
	}
	t.Fatalf("no tool %s", name)
	return mcpTool{}
}

// --host is taken from the commands that go to a host, and left to those
// with one of their own.
func TestHostFlag(t *testing.T) {
	cases := []struct {
		args []string
		host string
		rest string
	}{
		{[]string{"send", "--host", "vps", "abc", "hi"}, "vps", "send abc hi"},
		{[]string{"send", "abc", "hi", "--host=vps"}, "vps", "send abc hi"},
		{[]string{"project", "add", "/srv/app", "--host", "vps"}, "vps", "project add /srv/app"},
		{[]string{"peek", "abc", "--host", "vps"}, "", "peek abc --host vps"},
		{[]string{"project", "context", "P", "--host", "vps"}, "", "project context P --host vps"},
		{[]string{"features", "on", "x", "--host"}, "", "features on x --host"},
		{[]string{"send", "abc", "--", "--host", "vps"}, "", "send abc -- --host vps"},
	}
	for _, c := range cases {
		host, rest, _ := hostFlag(c.args)
		if host != c.host || strings.Join(rest, " ") != c.rest {
			t.Errorf("%v: %q, %v", c.args, host, rest)
		}
	}
}

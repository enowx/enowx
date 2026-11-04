package daemon

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

// The daemon measures itself and asks the attached app, whose answer comes back whole.
func TestProfileHoldsTheDaemonAndTheApp(t *testing.T) {
	srv := startServer(t)
	c := connect(t, srv, nil)
	var alone proto.Profile
	if err := c.Call(proto.MethodDaemonProfile, proto.ProfileParams{}, &alone); err != nil {
		t.Fatal(err)
	}
	if alone.Daemon.Kind != "daemon" || alone.Daemon.Go.Goroutines == 0 || alone.App != nil || alone.AppError == "" {
		t.Fatalf("%+v", alone)
	}
	var app *client.Client
	app = connect(t, srv, func(ev proto.Event) {
		if ev.Name != proto.EventAppRequest {
			return
		}
		var req proto.AppRequest
		_ = json.Unmarshal(ev.Data, &req)
		go func() {
			if req.Kind != "profile" {
				t.Errorf("kind %q", req.Kind)
			}
			res, _ := json.Marshal(proto.ProcessProfile{Kind: "app", PID: 42})
			_ = app.ReplyToApp(proto.AppReply{ID: req.ID, Result: res})
		}()
	})
	if err := app.ServeApp(); err != nil {
		t.Fatal(err)
	}
	var both proto.Profile
	if err := c.Call(proto.MethodDaemonProfile, proto.ProfileParams{}, &both); err != nil {
		t.Fatal(err)
	}
	if both.App == nil || both.App.PID != 42 || both.AppError != "" {
		t.Fatalf("%+v %q", both.App, both.AppError)
	}
}

// A command line's arguments may hold a secret: only a script's name is shown.
func TestProgramLabelShowsNoArguments(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"/usr/bin/node", "--flag", "/home/x/.bin/firebase", "mcp", "--token=abcdef0123456789abcdef"}, "node firebase"},
		{[]string{"/bin/sh", "-c", "curl -H 'Authorization: Bearer abcdef0123456789'"}, "sh"},
		{nil, "?"},
	} {
		got := programLabel(c.args)
		if got != c.want || strings.Contains(got, "abcdef") {
			t.Fatalf("%v: %q", c.args, got)
		}
	}
}

// A bundle from another host holds what its daemon measured.
func TestDiagnosticsHoldAProfile(t *testing.T) {
	srv, _, _ := loggedServer(t)
	var out proto.Diagnostics
	if err := connect(t, srv, nil).Call(proto.MethodDaemonDiagnostics, nil, &out); err != nil {
		t.Fatal(err)
	}
	for _, f := range out.Files {
		if f.Name == "profile.txt" && strings.Contains(f.Text, "heap live") {
			return
		}
	}
	t.Fatal("no profile.txt in the bundle")
}

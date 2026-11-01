//go:build !windows

package ui

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// The app skips the user's yes only for an asker its own daemon says runs
// without asking, on a host of the user's own: another host's daemon it
// doesn't know, a teammate's shared host and no mode all ask.
func TestHostCallFollowsTheAskersMode(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	cl, err := client.New(b1, func(proto.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	info, err := cl.Info()
	if err != nil {
		t.Fatal(err)
	}
	app := New(Options{})
	app.onMain = func(fn func()) { fn() }
	app.client, app.daemonInfo = cl, info
	app.host = hosts.Host{ID: hosts.LocalID, Name: "this Mac"}
	app.teamHostList = &hostTeamHosts{host: hosts.LocalID, hosts: []proto.TeamHost{{Device: "rinas-box", Name: "Rina's box", Owner: "Rina"}}}

	type answer struct {
		out any
		err error
	}
	ask := func(from string, p proto.HostCallParams) (done chan answer, bypassed *bool) {
		raw, _ := json.Marshal(p)
		done, bypassed = make(chan answer, 1), new(bool)
		app.hostAsks = nil
		app.hostCallAsked(from, proto.AppRequest{Params: raw}, func(out any, err error) { done <- answer{out, err} }, func() { *bypassed = true })
		return done, bypassed
	}
	session := func() string {
		s, err := cl.CreateSession(proto.SessionCreateParams{Command: []string{"cat"}, Dir: t.TempDir(), Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		return s.ID
	}
	kill := func(host, bypass string) proto.HostCallParams {
		params, _ := json.Marshal(proto.SessionRef{ID: session()})
		return proto.HostCallParams{Host: host, Method: proto.MethodSessionKill, Params: params, From: "asker", Bypass: bypass}
	}
	asked := func(what string, bypassed *bool) {
		t.Helper()
		if len(app.hostAsks) == 0 || *bypassed {
			t.Fatalf("%s: asks %v, bypassed %v", what, app.hostAsks, *bypassed)
		}
	}

	done, bypassed := ask(info.ID, kill(info.ID, "bypassPermissions"))
	select {
	case a := <-done:
		if a.err != nil || !*bypassed || len(app.hostAsks) > 0 {
			t.Fatalf("in bypass on the user's host: %+v, bypassed %v, asks %v", a, *bypassed, app.hostAsks)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("in bypass on the user's host: nothing done")
	}
	_, bypassed = ask(info.ID, kill(info.ID, ""))
	asked("no mode", bypassed)
	_, bypassed = ask("a daemon the app doesn't reach", kill(info.ID, "bypassPermissions"))
	asked("an unknown asker's daemon", bypassed)
	_, bypassed = ask(info.ID, kill("team:rinas-box", "bypassPermissions"))
	asked("a teammate's shared host", bypassed)

	// Hibernating on another host asks as what can't be undone does; a dry
	// run changes nothing, so it doesn't.
	hibernate := func(dry bool, bypass string) proto.HostCallParams {
		params, _ := json.Marshal(proto.SessionHibernateParams{IDs: []string{session()}, DryRun: dry})
		return proto.HostCallParams{Host: info.ID, Method: proto.MethodSessionHibernate, Params: params, From: "asker", Bypass: bypass}
	}
	_, bypassed = ask(info.ID, hibernate(false, ""))
	asked("hibernating", bypassed)
	done, bypassed = ask(info.ID, hibernate(true, ""))
	if len(app.hostAsks) > 0 || *bypassed {
		t.Fatal("a dry run asked")
	}
	<-done
	done, bypassed = ask(info.ID, hibernate(false, "bypassPermissions"))
	if len(app.hostAsks) > 0 || !*bypassed {
		t.Fatal("hibernating in bypass asked")
	}
	<-done
}

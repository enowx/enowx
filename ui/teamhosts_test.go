//go:build !windows

package ui

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/hostauth"
	"github.com/enowx/enowx/cloud/server"
	"github.com/enowx/enowx/cloud/store/storetest"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// A member opens a host a teammate shares from the rail: the app gets a
// ticket from the cloud through its own host, connects, and shows the
// shared project and its session.
func TestMemberOpensASharedHost(t *testing.T) {
	t.Setenv(proto.EnvTeamListen, "127.0.0.1:0")
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nsleep 30\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx := context.Background()
	db := storetest.Open(t)
	cloud := httptest.NewServer(server.New(server.Config{URL: "http://cloud.test"}, db, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(cloud.Close)
	host := func(gh int64, login string) (*client.Client, string) {
		dir := t.TempDir()
		auth := hostauth.Open(filepath.Join(dir, "cloud"))
		dk, _ := auth.DeviceKey()
		acc, _ := db.GitHubAccount(ctx, gh, login, login, "", "")
		dev, token, _ := db.AddDevice(ctx, acc.ID, login+"'s", api.DeviceHost, dk.Public())
		auth.Put(hostauth.State{URL: cloud.URL, Token: token, AccountID: acc.ID, Account: login, DeviceID: dev.ID})
		srv, err := daemon.NewServer(dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(srv.Close)
		a, b := net.Pipe()
		go srv.ServeConn(a)
		cl, err := client.New(b, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cl.Close() })
		return cl, acc.ID
	}
	ada, adaAcc := host(1, "ada")
	bob, bobAcc := host(2, "bob")
	team, _ := db.CreateTeam(ctx, adaAcc, "Design")
	inv, _ := db.CreateInvite(ctx, team.ID, adaAcc, api.InviteParams{Role: api.RoleMember})
	db.AcceptInvite(ctx, inv.Token, bobAcc)
	for _, cl := range []*client.Client{ada, bob} {
		var teams []api.Team
		if err := cl.CloudCall("GET", api.PathTeams, nil, &teams); err != nil { // the host learns its teams
			t.Fatal(err)
		}
	}
	project := t.TempDir()
	if _, err := ada.AddProject(project); err != nil {
		t.Fatal(err)
	}
	ada.CreateSession(proto.SessionCreateParams{Name: "Checkout page", Dir: project, Cols: 80, Rows: 10, Command: []string{"sh", "-c", "sleep 30"}})
	allow := true // bob starts a session of his own there
	if _, err := ada.SetHostShareTeam(proto.HostShareTeam{Team: team.ID, Projects: []string{project}, AllowSessions: &allow}); err != nil {
		t.Fatal(err)
	}
	for _, cl := range []*client.Client{ada, bob} {
		if _, err := cl.SyncSpace(team.ID); err != nil {
			t.Fatal(err)
		}
	}

	app := New(Options{})
	var mu sync.Mutex
	var later []func()
	app.onMain = func(fn func()) { mu.Lock(); later = append(later, fn); mu.Unlock() }
	app.gen, app.client = 1, bob
	app.host = hosts.Host{ID: hosts.LocalID, Name: "bob's Mac"}
	app.state.RailWide = true
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Row(c).Width(1100).Height(700).Background(theme.Background).TextColor(theme.Text).Children(func() {
			ui.Column(c).Width(navWidth).FillHeight().Children(func() { app.contextPane(c) })
			ui.Column(c).Grow(1).FillHeight().Children(func() {
				if app.page == pageTeamHost {
					app.teamHostPage(c)
				}
			})
		})
	}, 1100, 700)
	settle := func(what string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			tt.Frame()
			if done() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: never; shows %q", what, tt.Texts())
			}
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			fns := later
			later = nil
			mu.Unlock()
			for _, fn := range fns {
				fn()
			}
		}
	}
	settle("the shared host known", func() bool { return len(app.teamHostsOf().hosts) == 1 })
	// In the hosts menu, after the host's own devices.
	_, state := app.connectionState()
	if err := tt.Click("Host: bob's Mac, " + state); err != nil {
		t.Fatal(err)
	}
	h := app.teamHostsOf().hosts[0]
	item := ""
	settle("the shared host in the menu", func() bool {
		item = h.Name
		return tt.HasText(item)
	})
	if err := tt.Click(item); err != nil {
		t.Fatal(err)
	}
	settle("its shared project and session", func() bool { return tt.HasText("Checkout page") && tt.HasText("as member") })
	shot(t, tt, "team-host.png")

	// Bob watches it, and asks to take over; ada lets him.
	if err := tt.Click("Watch"); err != nil {
		t.Fatal(err)
	}
	settle("the session watched", func() bool {
		return app.visit.watch != nil && app.visit.watch.term != nil && tt.HasText("Ask to take over")
	})
	if err := tt.Click("Ask to take over"); err != nil {
		t.Fatal(err)
	}
	settle("asked", func() bool { return tt.HasText("asked ada") })
	var who []proto.Visitor
	settle("ada's host sees bob watching", func() bool {
		who, _ = ada.HostVisitors()
		return len(who) == 1 && len(who[0].Watching) == 1
	})
	// Ada's app asks her, and she lets him type.
	adaApp := New(Options{})
	adaApp.onMain = app.onMain
	adaApp.client = ada
	adaApp.host = hosts.Host{ID: hosts.LocalID, Name: "ada's Mac"}
	adaApp.presence = &hostPresence{host: hosts.LocalID, visitors: who,
		asks: []proto.TakeoverAsk{{Session: who[0].Watching[0], Name: "Checkout page", Visitor: who[0]}}}
	owner := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(900).Height(600).Background(theme.Background).TextColor(theme.Text).Children(func() { adaApp.asksView(c) })
	}, 900, 600)
	owner.Frame()
	if !owner.HasText("bob asks to take over") {
		t.Fatalf("ada isn't asked: %q", owner.Texts())
	}
	if err := owner.Click("Let them type"); err != nil {
		t.Fatal(err)
	}
	settle("in control", func() bool { return tt.HasText("you have control") })
	shot(t, tt, "team-host-watch.png")

	// Bob starts a session of his own there.
	if err := tt.Click("Stop watching"); err != nil {
		t.Fatal(err)
	}
	settle("the host's projects again", func() bool { return tt.HasText("Claude Code session") })
	if err := tt.Click("Claude Code session"); err != nil {
		t.Fatal(err)
	}
	settle("his own session, watched", func() bool { return tt.HasText("your session") })
}

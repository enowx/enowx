package ui

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
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

// The owner deletes a team from its Options: the dialog says what goes
// and deletes nothing until the team's name is typed exactly. A member's
// host drops the space at its next sync, and their app says the team was
// deleted until they saw it.
func TestOwnerDeletesTeamByTypingItsName(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	cloud := httptest.NewServer(server.New(server.Config{URL: "http://cloud.test"}, db, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(cloud.Close)
	var mu sync.Mutex
	var later []func()
	onMain := func(fn func()) { mu.Lock(); later = append(later, fn); mu.Unlock() }
	host := func(gh int64, login string, app *App) (*client.Client, string) {
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
		cl, err := client.New(b, func(ev proto.Event) {
			var apply func()
			switch ev.Name {
			case proto.EventSpacesChanged:
				apply = app.spacesEvent(ev.Data)
			case proto.EventSpacesGone:
				apply = app.spacesGoneEvent(ev.Data)
			}
			if apply != nil {
				onMain(apply)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cl.Close() })
		app.onMain, app.client, app.host = onMain, cl, hosts.Host{ID: hosts.LocalID, Name: login + "'s Mac"}
		return cl, acc.ID
	}
	adaApp, bobApp := New(Options{}), New(Options{})
	ada, adaAcc := host(1, "ada", adaApp)
	bob, bobAcc := host(2, "bob", bobApp)
	team, _ := db.CreateTeam(ctx, adaAcc, "Design")
	inv, _ := db.CreateInvite(ctx, team.ID, adaAcc, api.InviteParams{Role: api.RoleMember})
	db.AcceptInvite(ctx, inv.Token, bobAcc)
	for _, cl := range []*client.Client{ada, bob} {
		var teams []api.Team
		if err := cl.CloudCall("GET", api.PathTeams, nil, &teams); err != nil { // the host learns its teams
			t.Fatal(err)
		}
	}
	for _, cl := range []*client.Client{ada, bob, ada} {
		if _, err := cl.SyncSpace(team.ID); err != nil {
			t.Fatal(err)
		}
	}

	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(900).Height(900).Padding(24, 32).Gap(16).Background(theme.Background).TextColor(theme.Text).Children(func() {
			adaApp.cloudSection(c)
		})
	}, 900, 900)
	member := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(900).Height(600).Background(theme.Background).TextColor(theme.Text).Children(func() {
			ui.Text(c, "Spaces: "+spaceNames(bobApp.spacesOf().list))
		})
		bobApp.cornerCards(c)
	}, 900, 600)
	settle := func(what string, done func() bool) {
		t.Helper()
		for deadline := time.Now().Add(15 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			tt.Frame()
			member.Frame()
			mu.Lock()
			fns := later
			later = nil
			mu.Unlock()
			for _, fn := range fns {
				fn()
			}
			if done() {
				tt.Frame()
				member.Frame()
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: never; shows %q and %q", what, tt.Texts(), member.Texts())
			}
		}
	}
	click := func(tt *ui.Tester, label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("%v; shows %q", err, tt.Texts())
		}
	}
	settle("the team", func() bool { return tt.HasText("Design") && member.HasText("Spaces: Design") })
	click(tt, "Team Design")
	settle("the team's dialog", func() bool { return tt.HasText("You're its owner · 2 members") })
	click(tt, "Options")
	settle("the options", func() bool { return tt.HasText("Delete Design…") })
	click(tt, "Delete Design…")
	settle("the delete dialog", func() bool { return tt.HasText("Type Design to confirm") && tt.HasText("Delete the team") })
	tt.Advance(time.Second) // the dialog in, for its picture
	shot(t, tt, "team-delete.png")
	for _, typed := range []string{"", "design", "Desig"} {
		adaApp.cloudSet.teams.deleteName = typed
		tt.Frame()
		tt.Click("Delete the team")
		settle("nothing deleted", func() bool { return adaApp.cloudSet.busy == "" })
		if _, err := db.Team(ctx, team.ID, adaAcc); err != nil {
			t.Fatalf("deleted with %q typed: %v", typed, err)
		}
	}
	adaApp.cloudSet.teams.deleteName = "Design"
	tt.Frame()
	click(tt, "Delete the team")
	settle("deleted", func() bool { return adaApp.cloudSet.teams.open == "" && !tt.HasText("Team Design") })
	if _, err := db.Team(ctx, team.ID, adaAcc); err == nil {
		t.Fatal("the team is still in the cloud")
	}
	if sp, _ := ada.Spaces(); len(sp) != 0 {
		t.Fatalf("the owner's host kept the space: %+v", sp)
	}

	// Bob's host syncs: the space leaves his rail, a card says why.
	if _, err := bob.SyncSpace(team.ID); err == nil {
		t.Fatal("a deleted team synced")
	}
	settle("the member told", func() bool {
		return member.HasText("Design was deleted") && member.HasText("Spaces: ")
	})
	member.Advance(time.Second)
	shot(t, member, "team-deleted-card.png")
	click(member, "OK")
	settle("seen", func() bool { return !member.HasText("Design was deleted") })
	if gone, _ := bob.SpacesGone(); len(gone) != 0 {
		t.Fatalf("seen, the host still says it: %+v", gone)
	}
}

func spaceNames(list []proto.Space) string {
	out := ""
	for _, sp := range list {
		out += sp.Name + " "
	}
	return out
}

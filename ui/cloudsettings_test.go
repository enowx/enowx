package ui

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	cloudclient "github.com/enowx/enowx/cloud/client"
	"github.com/enowx/enowx/cloud/seal"
	"github.com/enowx/enowx/cloud/server"
	"github.com/enowx/enowx/cloud/store/storetest"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// From signed out to a team's invite: the rail's Sign in, a code approved
// elsewhere, the account at the rail's foot and in Settings › Cloud with
// its devices, a team made and opened, and an invite link made for it.
func TestCloudSettingsSignInAndInvite(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	cloud := httptest.NewServer(server.New(server.Config{URL: "https://cloud.example.com"}, db, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(cloud.Close)
	acc, _ := db.GitHubAccount(ctx, 1, "ada", "Ada", "", "")
	k, _ := seal.NewDeviceKey()
	_, phoneToken, _ := db.AddDevice(ctx, acc.ID, "Ada's other laptop", api.DeviceApp, k.Public())
	other, _ := cloudclient.New(cloud.URL, phoneToken)

	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	pa, pb := net.Pipe()
	go srv.ServeConn(pa)
	app := New(Options{})
	var later []func()
	app.onMain = func(fn func()) { later = append(later, fn) }
	cl, err := client.New(pb, func(ev proto.Event) { app.cloudEvent(ev) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	app.client, app.host = cl, hosts.Host{ID: hosts.LocalID, Name: "This Mac"}
	app.state.RailWide = true
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Row(c).Width(980).Height(900).Background(theme.Background).Children(func() {
			ui.Column(c).Width(220).FillHeight().Padding(8).Children(func() { app.accountAction(c) })
			ui.Column(c).Grow(1).FillHeight().Padding(24, 32).Gap(16).Children(func() { app.cloudSection(c) })
		})
		app.cloudSignInDialog(c)
		app.confirmDialog(c)
	}, 980, 900)
	settle := func(what string, done func() bool) {
		t.Helper()
		for deadline := time.Now().Add(15 * time.Second); ; time.Sleep(20 * time.Millisecond) {
			tt.Frame()
			fns := later
			later = nil
			for _, fn := range fns {
				fn()
			}
			if done() {
				tt.Frame()
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: never; shows %q", what, tt.Texts())
			}
		}
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("%v; shows %q", err, tt.Texts())
		}
	}

	// This cloud, as a self-hosted one is set (enowx-app cloud url): the app
	// signs in to enowX's otherwise.
	if _, err := cl.SetCloudURL(cloud.URL); err != nil {
		t.Fatal(err)
	}
	settle("signed out", func() bool {
		return tt.HasText("Not signed in. Sign in with GitHub: the browser opens on a code to approve.")
	})
	shot(t, tt, "cloud-off.png")
	click("Sign in") // the rail's: the dialog, asking for a code at once
	settle("a code", func() bool {
		return app.cloudSigningIn && app.cloudSet.st.Pending != nil && tt.HasText(app.cloudSet.st.Pending.UserCode)
	})
	shot(t, tt, "cloud-code.png")
	if err := other.ApproveDevice(ctx, app.cloudSet.st.Pending.UserCode); err != nil {
		t.Fatal(err)
	}
	settle("signed in, said in the dialog", func() bool { return tt.HasText("You're signed in") })
	shot(t, tt, "cloud-signedin.png")
	click("Done")
	settle("signed in, on the Teams tab", func() bool { return !app.cloudSigningIn && tt.HasText("No teams yet") })
	click("Devices (2)")
	settle("the devices", func() bool { return tt.HasText("Ada's other laptop") && tt.HasText("This device") })
	click("Make a recovery key")
	settle("the recovery key, once", func() bool {
		return strings.HasPrefix(app.cloudSet.recovery.shown, "enxr-") && tt.HasText("Your recovery key")
	})
	click("I kept it")
	settle("set", func() bool { return tt.HasText("Make a new one…") })
	if _, ok := tt.Find("Account: Ada"); !ok {
		t.Fatalf("the rail's account isn't there: %q", tt.Texts())
	}

	click("Teams (0)")
	settle("the account's line, on the Teams tab", func() bool { return tt.HasText("This Mac · a computer · " + cloud.URL) })
	click("New team")
	settle("the new team's dialog", func() bool { return tt.HasText("Make team") })
	app.cloudSet.teams.newName = "enowX Labs"
	tt.Frame()
	click("Make team")
	settle("the team, its invite open", func() bool { return tt.HasText("Invite as") && tt.HasText("You're its owner · 1 member") })
	shot(t, tt, "cloud-team-dialog.png")
	click("Make a link")
	settle("an invite", func() bool {
		return strings.HasPrefix(app.cloudSet.teams.invite.URL, "https://cloud.example.com/invite#enxi_")
	})
	shot(t, tt, "cloud-team.png")
	if !tt.HasText(app.cloudSet.teams.invite.URL) {
		t.Fatalf("the link isn't shown: %q", tt.Texts())
	}

	// The team's projects: added by a Git link, not here yet, so cloned.
	click("Projects")
	settle("the projects tab", func() bool { return tt.HasText("Add a project") })
	app.teamProjectsOf().adding = "https://github.com/enowx/app"
	tt.Frame()
	click("Add")
	settle("the project", func() bool { return tt.HasText("github.com/enowx/app") && tt.HasText("Clone") })
	shot(t, tt, "cloud-team-projects.png")
	click("Kit")
	settle("the kit tab", func() bool { return tt.HasText("Nothing shared yet.") })
	click("Options")
	settle("the team's options", func() bool { return tt.HasText("Copying out") })
	shot(t, tt, "cloud-team-settings.png")
	click("Activity")
	settle("the team's log", func() bool { return tt.HasText("@ada made the team") })

	// A viewer made a member from their role's menu.
	bob, _ := db.GitHubAccount(ctx, 2, "bob", "Bob", "", "")
	teamID := app.cloudSet.teams.team.ID
	inv, _ := db.CreateInvite(ctx, teamID, acc.ID, api.InviteParams{Role: api.RoleViewer})
	if _, err := db.AcceptInvite(ctx, inv.Token, bob.ID); err != nil {
		t.Fatal(err)
	}
	app.openCloudTeam(app.cloudSet, teamID)
	settle("bob, a viewer", func() bool { return tt.HasText("@bob") })
	click("Role of Bob")
	settle("the roles", func() bool { return tt.HasText("Member") && tt.HasText("Viewer (now)") })
	click("Member")
	settle("bob, a member", func() bool {
		full, _ := db.Team(ctx, teamID, acc.ID)
		for _, m := range full.Members {
			if m.Account.ID == bob.ID {
				return m.Role == api.RoleMember
			}
		}
		return false
	})

	click("Done")
	settle("the team's card", func() bool { return app.cloudSet.teams.open == "" && tt.HasText("enowX Labs") })
	shot(t, tt, "cloud-teams.png")
	click("Sign out")
	settle("asked first", func() bool { return app.confirming != nil && tt.HasText("Sign This Mac out?") })
	if !app.cloudSet.st.SignedIn {
		t.Fatal("signed out before confirming")
	}
	app.confirming.do() // the dialog's Sign out, under the page's own
	app.confirming = nil
	settle("signed out", func() bool { return !app.cloudSet.st.SignedIn && tt.HasText("Sign in") })
	if tt.HasText("enowX Labs") {
		t.Fatal("the teams still show, signed out")
	}
}

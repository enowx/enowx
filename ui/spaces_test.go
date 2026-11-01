//go:build !windows

package ui

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/hostauth"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// The account's menu picks the space: on a host signed in as a member of a team, it
// switches Notes between the user's own pages and the team's, and a page
// made there is the team's.
func TestAccountMenuPicksTheTeamsSpace(t *testing.T) {
	dir := t.TempDir()
	const team = "0123456789abcdef"
	if err := hostauth.Open(filepath.Join(dir, "cloud")).Put(hostauth.State{URL: "https://cloud.test", Token: "enxc_test", AccountID: "acc1", Account: "Ada"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "teams", team), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "teams", team, "team.json"), []byte(`{"team":"`+team+`","name":"Design","account":"acc1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, err := daemon.NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	app := New(Options{})
	var mu sync.Mutex
	var later []func()
	app.onMain = func(fn func()) { mu.Lock(); later = append(later, fn); mu.Unlock() }
	a, b := net.Pipe()
	go srv.ServeConn(a)
	cl, err := client.New(b, func(ev proto.Event) { app.handleEvent(1, ev) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	app.gen, app.client, app.page = 1, cl, pageNotes
	app.host = hosts.Host{ID: hosts.LocalID, Name: "this Mac"}
	mine, err := cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Mine"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Roadmap", Team: team}}); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Row(c).Width(1100).Height(720).Background(theme.Background).TextColor(theme.Text).Children(func() {
			ui.Column(c).Width(navWidth).FillHeight().Children(func() { app.accountAction(c) })
			ui.Column(c).Grow(1).FillHeight().Children(func() { app.notesPage(c) })
			app.spaceMoveDialog(c)
		})
	}, 1100, 720)
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

	settle("the user's own notes, the account on Personal", func() bool {
		return tt.HasText("Mine") && tt.HasText("Personal") && app.cloudOf().st.SignedIn
	})
	if tt.HasText("Roadmap") {
		t.Fatal("the team's note shows among the user's own")
	}
	if err := tt.Click("Account: Ada"); err != nil {
		t.Fatal(err)
	}
	settle("the menu's spaces", func() bool { return tt.HasText("Design") })
	shot(t, tt, "account-spaces.png")
	if err := tt.Click("Design"); err != nil {
		t.Fatal(err)
	}
	settle("the team's notes", func() bool { return tt.HasText("Roadmap") && tt.HasText("Design · on this Mac") })
	if tt.HasText("Mine") {
		t.Fatal("the user's own note shows in the team's space")
	}

	done := make(chan proto.NoteInfo, 1)
	app.notesOptions().Save(proto.Note{NoteInfo: proto.NoteInfo{Title: "Specs"}}, func(info proto.NoteInfo, err error) {
		if err != nil {
			t.Error(err)
		}
		done <- info
	})
	var made proto.NoteInfo
	settle("the new page saved", func() bool {
		select {
		case made = <-done:
			return true
		default:
			return false
		}
	})
	if made.Team != team {
		t.Fatalf("a page made in the team's space: %+v", made)
	}
	if app.state.Space != team {
		t.Fatalf("the space picked isn't kept: %q", app.state.Space)
	}

	// The user's own note, moved to the team from its dialog.
	app.openNoteSpaceMove(mine)
	settle("the move dialog", func() bool { return tt.HasText("Move or copy “Mine”") })
	shot(t, tt, "space-move.png")
	if err := tt.Click("Move to Design"); err != nil {
		t.Fatal(err)
	}
	settle("moved to the team", func() bool {
		for _, n := range app.notesOf().list {
			if n.ID == mine.ID && n.Team == team {
				return true
			}
		}
		return false
	})
}

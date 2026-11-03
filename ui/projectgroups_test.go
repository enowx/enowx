package ui

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/ui/theme"
)

// Dragging a project's header before another's moves it on the host, and
// the group dialog puts projects in a group the list shows them under.
func TestProjectsMoveByDragAndJoinAGroup(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a, b := net.Pipe()
	go srv.ServeConn(a)
	cl, err := client.New(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	root := t.TempDir()
	paths := map[string]string{}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
		p, err := cl.AddProject(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		paths[name] = p.Path
	}
	app := New(Options{})
	app.state.SessionsByProject = true
	app.onMain = func(fn func()) {}
	app.client = cl
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(700).Height(600).Background(theme.Background).Children(func() {
			app.sessionsBody(c)
			app.groupDialog(c)
		})
	}, 700, 600)
	// until waits for the host's list to be want, as names in order with
	// their groups.
	until := func(want []string, groups map[string]string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			list, err := cl.Projects()
			if err != nil {
				t.Fatal(err)
			}
			app.projects = list
			var got []string
			same := true
			for _, p := range list {
				got = append(got, p.Name)
				same = same && p.Group == groups[p.Name]
			}
			if same && slices.Equal(got, want) {
				tt.Frame()
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("projects %+v, want %v in %v (status %q)", list, want, groups, app.status)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	until([]string{"alpha", "beta", "gamma"}, nil)

	from, ok := tt.Find("gamma")
	to, ok2 := tt.Find("alpha")
	if !ok || !ok2 {
		t.Fatalf("no headers in %q", tt.Texts())
	}
	tt.Press(from.X+from.W/2, from.Y+from.H/2)
	for i := 1; i <= 8; i++ {
		tt.Move(from.X+from.W/2, from.Y+from.H/2+(to.Y-from.Y)*float32(i)/8)
		tt.Frame()
	}
	tt.Release(to.X+to.W/2, to.Y+to.H/2)
	tt.Frame()
	until([]string{"gamma", "alpha", "beta"}, nil)

	app.groupForm = &groupForm{open: true, project: paths["beta"]}
	tt.Frame()
	if err := tt.Click("Clients, Side projects…"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Clients")
	if err := tt.Click("Add"); err != nil {
		t.Fatal(err)
	}
	until([]string{"gamma", "alpha", "beta"}, map[string]string{"beta": "Clients"})
	// Another project joins it from the dialog's list of groups, after it.
	app.groupForm = &groupForm{open: true, project: paths["gamma"]}
	tt.Frame()
	if err := tt.Click("Group “Clients”"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Add"); err != nil {
		t.Fatal(err)
	}
	until([]string{"alpha", "beta", "gamma"}, map[string]string{"beta": "Clients", "gamma": "Clients"})
	if !tt.HasText("Clients") {
		t.Fatalf("no group in %q", tt.Texts())
	}
	shot(t, tt, "project-groups.png")
}

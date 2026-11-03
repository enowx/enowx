package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// A session whose folder is a real repository, with changes, a remote and
// checks, gives the tools what their lists and center tabs show: each state
// has to hold in the window's sizes, its picture kept with SHOTS set.
func TestSessionToolsHoldTheirFrames(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	dir := t.TempDir()
	run := func(where string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = where
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@a", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@a")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(rel, text string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	code := strings.Repeat("func a() {\n\treturn\n}\n\n", 20)
	long := "docs/session-tab-strip-and-center/viewers-behaviour-notes.md"
	for _, f := range []string{"ui/connection.go", "ui/goa.go", "ui/workspace.go", "client/attach.go", "docs/goa.md", long} {
		write(f, "package a\n\n"+code)
	}
	run(dir, "init", "-q", "-b", "main")
	run(dir, "add", ".")
	run(dir, "commit", "-qm", "first")
	remote := t.TempDir()
	run(remote, "init", "-q", "--bare")
	run(dir, "remote", "add", "origin", remote)
	run(dir, "push", "-q", "-u", "origin", "main")
	for _, f := range []string{"ui/connection.go", "ui/goa.go", "docs/goa.md", long} {
		write(f, "package b\n\n// Changed.\n"+code)
	}
	write("ui/goa_tabs.go", "package a\n\n"+code)

	if _, err := h.cl.AddProject(dir); err != nil {
		t.Fatal(err)
	}
	h.app.setProjects([]proto.ProjectInfo{{Path: dir, Name: "enowx", IsGit: true}})
	s, err := h.cl.CreateSession(proto.SessionCreateParams{Agent: "shell", Project: dir, Name: "GOA session tabs", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	h.app.upsert(s)
	checks := []proto.Check{
		{Name: "lint", Command: "echo linted"},
		{Name: "build", Command: "echo built"},
		{Name: "vet", Command: "echo 'ui/goa_tabs.go:88: unreachable code'; exit 1"},
		{Name: "test", Command: "sleep 600"},
	}
	if _, err := h.cl.SetChecks(s.ID, checks); err != nil {
		t.Fatal(err)
	}
	if _, err := h.cl.RunChecks(s.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.cl.StopChecks(s.ID) })

	states := []struct {
		name, want string
		open       func(a *App)
	}{
		{"git", "5 files changed", func(a *App) { a.openPanel(panelChanges) }},
		{"diff", "Side by side", func(a *App) {
			a.openPanel(panelChanges)
			a.openCenter(s, centerDiff+"ui/goa_tabs.go")
		}},
		{"diff-long", "Unified", func(a *App) {
			a.openPanel(panelChanges)
			a.openCenter(s, centerDiff+long)
		}},
		{"files", "ui", func(a *App) { a.openPanel(panelFiles) }},
		{"file", "File text", func(a *App) {
			a.openPanel(panelFiles)
			a.openCenter(s, fileTabPrefix+"ui/goa_tabs.go")
		}},
		{"checks", "Stop", func(a *App) { a.openPanel(panelChecks) }},
		{"check", "exit status 1", func(a *App) {
			a.openPanel(panelChecks)
			a.openCenter(s, centerCheck+"vet")
		}},
		{"activity", "", func(a *App) { a.openPanel(panelActivity) }},
		{"ports", "", func(a *App) { a.openPanel(panelPorts) }},
		{"browser", "", func(a *App) { a.openPanel(panelBrowser) }},
		{"no-session", "Select a session to see its tools.", func(a *App) {
			a.openPanel(panelFiles)
			a.sessions, a.selected = nil, ""
		}},
		{"port-offer", "node is listening on :5173", func(a *App) {
			a.portOffers = []portOffer{{session: s.ID, port: proto.Port{Port: 5173, Addr: "127.0.0.1", PID: 8123, Process: "node"}}}
		}},
		{"browser-as-view", "Switch it to Terminal or Chat to have it here.", func(a *App) {
			a.openPanel(panelBrowser)
			a.state.BrowserViews = map[string]bool{s.ID: true}
		}},
	}
	var problems []string
	sessions := h.app.sessions
	for _, st := range states {
		for _, size := range [][2]int{{1200, 720}, {1440, 900}, {1920, 1080}} {
			closeAudit(h.app)
			h.app.page, h.app.state.Panel.Open, h.app.sessions = "", false, sessions
			h.app.portOffers, h.app.state.BrowserViews = nil, nil
			delete(h.app.center, s.ID)
			h.app.selectSession(s)
			st.open(h.app)
			tt := auditTester(h.app, size[0], size[1])
			h.tt = tt
			if st.want != "" {
				h.settle(st.name, func() bool { return tt.HasText(st.want) })
			}
			if st.name == "files" && !tt.HasText("connection.go") {
				if err := tt.Click("ui"); err != nil {
					t.Fatal(err)
				}
				h.settle("the folder unfolded", func() bool { return tt.HasText("connection.go") })
			}
			for range 6 {
				h.settle("", func() bool { return true })
			}
			name := fmt.Sprintf("%s-%dx%d", st.name, size[0], size[1])
			shot(t, tt, filepath.Join("tools", name+".png"))
			for _, pr := range layoutProblems(tt, float32(size[0]), float32(size[1]), false) {
				problems = append(problems, name+": "+pr)
			}
		}
	}
	if len(problems) > 0 {
		t.Errorf("%d layout problems:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

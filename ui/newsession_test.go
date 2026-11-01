package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// A git project's optional fields can exceed the window's height. Its
// actions must stay visible while the form scrolls to the prompt.
func TestNewSessionFitsSmallWindow(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {1024, 768}, {800, 600}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			a := &App{
				host: hosts.Host{ID: "remote", Name: "devbox"},
				agents: []proto.AgentInfo{
					{ID: "kiro", Name: "Kiro CLI", Command: "kiro-cli", Installed: true,
						CanSkipPermissions: true, SignIn: "kiro-cli login --use-device-flow"},
					{ID: "codex", Name: "Codex", Command: "codex", Installed: true},
				},
				daemonInfo: proto.DaemonInfo{DisplayNote: "needs Xpra", DisplayInstall: strings.Repeat("apt-get install xpra && ", 20)},
				newSession: newSessionForm{open: true, loaded: true, agent: "kiro", branchMode: proto.BranchNew, base: "main",
					project: proto.ProjectDetail{ProjectInfo: proto.ProjectInfo{Name: "project", Path: "/srv/project", IsGit: true, Branch: "main"},
						Branches: []string{"main"}}},
			}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(theme.Window())
				ui.Column(c).FillWidth().FillHeight().Background(theme.Background).Children(func() { a.newSessionDialog(c) })
			}, size[0], size[1])
			tt.Frame()
			tt.Advance(300 * time.Millisecond)
			shot(t, tt, fmt.Sprintf("new-session-%dx%d.png", size[0], size[1]))
			for _, label := range []string{"Close", "Cancel", "Start session"} {
				r, ok := tt.Find(label)
				if !ok || r.Y < 0 || r.Y+r.H > float32(size[1]) {
					t.Fatalf("%q outside the window: %+v", label, r)
				}
			}
			footer, _ := tt.Find("Start session")
			tt.Scroll(float32(size[0])/2, float32(size[1])/2, 0, 10000)
			if got, _ := tt.Find("Start session"); got != footer {
				t.Fatalf("scroll moved the footer: %+v to %+v", footer, got)
			}
			tt.Scroll(float32(size[0])/2, float32(size[1])/2, 0, -10000)
			prompt, ok := tt.Find("Prompt")
			if !ok || prompt.Y < 0 || prompt.Y+prompt.H > footer.Y {
				t.Fatalf("prompt not reachable by scrolling: %+v", prompt)
			}
			tt.ClickAt(prompt.X+10, prompt.Y+35)
			tt.Type("Inspect the project")
			if a.newSession.prompt != "Inspect the project" {
				t.Fatalf("prompt is not editable: %q", a.newSession.prompt)
			}
			if err := tt.Click("Start session"); err != nil {
				t.Fatal(err)
			}
			if a.newSession.err != "Name the new branch." {
				t.Fatalf("start action did not validate the form: %q", a.newSession.err)
			}
			tt.SetSize(size[0], 480)
			if r, ok := tt.Find("Start session"); !ok || r.Y < 0 || r.Y+r.H > 480 {
				t.Fatalf("start action outside the resized window: %+v", r)
			}
			if err := tt.Click("Cancel"); err != nil {
				t.Fatal(err)
			}
			if a.newSession.open {
				t.Fatal("cancel did not close the form")
			}
		})
	}
}

func TestNewSessionShortFormStaysCompact(t *testing.T) {
	a := &App{host: hosts.Host{ID: hosts.LocalID, Name: "This PC"},
		agents: []proto.AgentInfo{{ID: "shell", Name: "Shell", Installed: true}},
		newSession: newSessionForm{open: true, loaded: true, agent: "shell",
			project: proto.ProjectDetail{ProjectInfo: proto.ProjectInfo{Name: "project", Path: "/srv/project"}}}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		a.newSessionDialog(c)
	}, 1000, 800)
	tt.Frame()
	tt.Advance(300 * time.Millisecond)
	header, foundHeader := tt.Find("New session")
	footer, foundFooter := tt.Find("Start session")
	if !foundHeader || !foundFooter || header.Y < 80 || footer.Y+footer.H > 720 {
		t.Fatalf("short form unnecessarily fills the window: header %+v, footer %+v", header, footer)
	}
}

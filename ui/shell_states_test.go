package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// aPicture is a real picture: what a chat holds once decoded.
func aPicture(t *testing.T, w, h int) ([]byte, *ui.Bitmap) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{uint8(40 + 200*x/w), uint8(90 + 120*y/h), 180, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	bmp, err := ui.DecodeBitmap(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes(), bmp
}

// The title bar's update chip, the At work strip, a session's banner, a team's
// row in the pipeline dialog and a chat's real pictures, in the whole window.
func TestShellAndTeamStates(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	base := h.app.sessions[0]
	session := func(a *App, change func(s *proto.SessionInfo)) {
		s := base
		change(&s)
		a.sessions[0] = s
		a.page = ""
		a.selectSession(s)
	}
	updating := func(a *App, state int, total int64) {
		a.page = ""
		ovUpdate(a, state, "")
		a.updates.total = total
	}
	data, bmp := aPicture(t, 800, 400)
	states := []struct {
		name  string
		open  func(a *App)
		wants []string
	}{
		{"chip-available", func(a *App) { updating(a, updateAvailable, 100) }, []string{"Update to 0.6.0"}},
		{"chip-percent", func(a *App) { updating(a, updateDownloading, 100) }, []string{"Updating · 40%"}},
		{"chip-unknown", func(a *App) { updating(a, updateDownloading, 0) }, []string{"Updating…"}},
		{"chip-ready", func(a *App) { updating(a, updateReady, 100) }, []string{"Restart to update"}},
		{"at-work", func(a *App) {
			a.page = pageNotes
			a.state.RailWide = true
			for i := range 6 {
				a.sessions = append(a.sessions, proto.SessionInfo{ID: "w" + strconv.Itoa(i), Name: "Working on part " + strconv.Itoa(i), Agent: "claude", State: proto.StateWorking, Created: time.Now()})
			}
		}, []string{"At work", "and more in Sessions"}},
		{"budget", func(a *App) {
			session(a, func(s *proto.SessionInfo) { s.Budget, s.Worked = 90*60, 32*60 })
		}, []string{"worked 32m of 1h 30m"}},
		{"budget-reached", func(a *App) {
			session(a, func(s *proto.SessionInfo) { s.Budget, s.Worked, s.OverBudget = 30*60, 30*60, true })
		}, []string{"worked 30m of 30m · budget reached"}},
		{"continue", func(a *App) {
			session(a, func(s *proto.SessionInfo) {
				s.Exited, s.Agent, s.ExitWhy = true, "claude", "It reached its time budget and was interrupted"
			})
		}, []string{"It reached its time budget and was interrupted", "Continue"}},
		{"pipeline-team", func(a *App) {
			auditTeamData(a)
			a.page = ""
			a.agents = ovAgents()
			a.openPipeline(a.projects[0])
		}, []string{"enowX's pipelines", "Share with enowX"}},
		{"chat-images", func(a *App) {
			auditTeamData(a)
			a.page, a.state.Panel.Team = "", "t1"
			a.selectSession(a.sessions[0])
			a.openPanel(panelChat)
			hc := a.chatsOf()
			hc.pending = []*pendingImage{{key: 1, name: "picked.png", preview: bmp}, {key: 2, name: "sent.png", preview: bmp, att: &proto.ChatAttachment{ID: "up"}}}
			hc.images["i1"] = &chatImage{bmp: bmp, data: data, asked: true}
			hc.images["i2"] = &chatImage{bmp: bmp, data: data, asked: true}
			hc.byTeam["t1"] = append(hc.byTeam["t1"], proto.ChatMessage{ID: "5", Team: "t1", Account: "rina", Name: "Rina", At: time.Now(),
				Attachments: []proto.ChatAttachment{{ID: "i1", Width: 800, Height: 400}}},
				proto.ChatMessage{ID: "6", Team: "t1", Account: "rina", Name: "Rina", At: time.Now(),
					Attachments: []proto.ChatAttachment{{ID: "i1", Width: 800, Height: 400}, {ID: "i2", Width: 800, Height: 400}}})
		}, nil},
		{"chat-viewer", func(a *App) {
			auditTeamData(a)
			a.page, a.state.Panel.Team = "", "t1"
			a.selectSession(a.sessions[0])
			a.openPanel(panelChat)
			hc := a.chatsOf()
			hc.images["i1"] = &chatImage{bmp: bmp, data: data, asked: true}
			att := proto.ChatAttachment{ID: "i1", Name: "launch.png", Width: 800, Height: 400, Size: len(data)}
			hc.viewing, hc.viewingOf = &att, proto.ChatMessage{ID: "5", Team: "t1", Name: "Rina", At: time.Now()}
		}, []string{"launch.png", "Save to Downloads"}},
	}
	for _, st := range states {
		t.Run(st.name, func(t *testing.T) {
			closeAudit(h.app)
			h.app.updates.found, h.app.updates.state = nil, updateIdle
			h.app.sessions = h.app.sessions[:1]
			h.app.sessions[0] = base
			st.open(h.app)
			tt := ui.NewTester(func(c *ui.Context) { h.app.view(c) }, 1280, 800)
			h.tt = tt
			for range 6 {
				h.settle("", func() bool { return true })
			}
			tt.Advance(time.Second)
			tt.Frame()
			shot(t, tt, filepath.Join("shell-states", st.name+".png"))
			for _, want := range st.wants {
				if !tt.HasText(want) {
					t.Errorf("no %q in %q", want, tt.Texts())
				}
			}
			for _, pr := range layoutProblems(tt, 1280, 800, false) {
				t.Error(pr)
			}
		})
	}
}

// The floating control's steps to look at: a project picker with sessions of
// no project, and the ring of sessions.
func TestFloatingStepsPictures(t *testing.T) {
	app := New(Options{})
	app.host = LocalHost()
	app.setProjects([]proto.ProjectInfo{{Path: "/api", Name: "API"}})
	app.sessions = []proto.SessionInfo{
		{ID: "api", Project: "/api", Name: "API task", State: proto.StateWorking},
		{ID: "loose", Name: "Loose task", State: proto.StateWaiting},
	}
	tt := floatingTester(t, app)
	floatingNavigate(t, tt, "Active sessions", "This PC")
	if !tt.HasText("API") || !tt.HasText("Other sessions") {
		t.Fatalf("the project step: %q", tt.Texts())
	}
	tt.Advance(time.Second)
	shot(t, tt, filepath.Join("shell-states", "floating-picker.png"))
	floatingNavigate(t, tt, "Other sessions")
	if !tt.HasText("Loose task") {
		t.Fatalf("the ring: %q", tt.Texts())
	}
	tt.Advance(time.Second)
	shot(t, tt, filepath.Join("shell-states", "floating-sessions.png"))
}

// A chat's card waits while the pointer rests on it, and goes when its time is
// up.
func TestChatToastWaitsWhileHovered(t *testing.T) {
	app := New(Options{})
	toast := &chatToast{msg: proto.ChatMessage{ID: "a", Team: "t1", Account: "rina", Name: "Rina", Text: "Plain message"}, shown: time.Now().Add(-chatToastLife + time.Second)}
	app.chatToasts = []*chatToast{toast}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(800).Height(400).Children(func() { app.chatToastsView(c) })
	}, 800, 400)
	tt.Frame()
	r, ok := tt.Find("Plain message")
	if !ok {
		t.Fatalf("no card: %q", tt.Texts())
	}
	tt.Move(r.X+4, r.Y+4)
	tt.Frame()
	tt.Frame()
	if time.Since(toast.shown) > time.Second {
		t.Fatalf("hovered, it still counts down from %s ago", time.Since(toast.shown))
	}
	tt.Move(4, 390)
	tt.Frame()
	toast.shown = time.Now().Add(-chatToastLife - time.Second)
	tt.Frame()
	if len(app.chatToasts) != 0 || tt.HasText("Plain message") {
		t.Fatalf("its time is up and it stays: %q", tt.Texts())
	}
}

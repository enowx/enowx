package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/framecheck"
	"github.com/enowx/enowx/ui/theme"
)

// longContentTester draws body, a new frame each time, in a window as the app's.
func longContentTester(t *testing.T, name string, body func(c *ui.Context)) *ui.Tester {
	t.Helper()
	return framecheck.New(t, name, func(c *ui.Context) {
		c.After(time.Millisecond)
		c.SetTheme(theme.Window())
		ui.Column(c).Width(1000).Height(700).Background(theme.Ground).TextColor(theme.Text).Children(func() { body(c) })
	}, 1000, 700)
}

// longSessionsApp is an app with n sessions, the first with a name, a note, a title and a
// prompt of a megabyte, and selected.
func longSessionsApp(n int) *App {
	app := New(Options{})
	app.projects = []proto.ProjectInfo{{Name: "app", Path: "/p/app"}}
	app.project = "/p/app"
	line := framecheck.Line()
	for i := range n {
		app.sessions = append(app.sessions, proto.SessionInfo{ID: fmt.Sprint("s", i), Name: fmt.Sprint("Session ", i), Agent: "claude", Project: "/p/app"})
	}
	first := &app.sessions[0]
	first.Name, first.Note, first.Title, first.Prompt = line, line, line, line
	app.selected = first.ID
	return app
}

func TestLongSessionListsStayQuick(t *testing.T) {
	app := longSessionsApp(1000)
	for name, draw := range map[string]func(c *ui.Context){"by state": app.sessionsByState, "by project": app.sessionsByProject} {
		tt := longContentTester(t, name, func(c *ui.Context) {
			ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Children(func() { draw(c) })
			})
		})
		framecheck.Quick(t, name, tt)
	}
}

func TestLongPaletteStaysQuick(t *testing.T) {
	app := longSessionsApp(1000)
	app.palette.open = true
	for _, query := range []string{"", "session"} {
		app.palette.query = query
		tt := longContentTester(t, "palette "+query, func(c *ui.Context) { app.commandPalette(c) })
		framecheck.Quick(t, "palette "+query, tt)
	}
}

func TestLongGOAPanelStaysQuick(t *testing.T) {
	app := longSessionsApp(500)
	line, lines := framecheck.Line(), framecheck.Lines()
	for i := range 500 {
		app.goa.board.Tasks = append(app.goa.board.Tasks, proto.GOATask{ID: fmt.Sprint("t", i), Session: fmt.Sprint("s", i), Title: fmt.Sprint("Task ", i),
			Status: proto.GOATaskWorking, Created: time.Now()})
	}
	long := proto.GOATask{ID: "long", Session: "s0", Title: line, SessionName: line, Why: line, Asks: line, Urgent: "payment", Result: lines,
		Status: proto.GOATaskWaiting, Created: time.Now()}
	app.goa.board.Tasks = append(app.goa.board.Tasks, long)
	app.goa.board.Waiting = []string{line}
	app.goa.flags = []proto.GOAFlag{{ID: "f", Command: line, Time: time.Now()}}
	for name, draw := range map[string]func(c *ui.Context){
		"tasks": app.tasksTab,
		"sessions": func(c *ui.Context) {
			goaScroll(c, func() {
				for _, s := range app.sessions {
					app.goaSessionRow(c.Key("goa-s:"+s.ID), "this Mac", s)
				}
			})
		},
		"a task in full": func(c *ui.Context) { app.goaTaskBody(c, long) },
	} {
		framecheck.Quick(t, name, longContentTester(t, name, draw))
	}
}

func TestLongTeamChatStaysQuick(t *testing.T) {
	app := New(Options{})
	at := time.Now().Add(-time.Hour)
	var msgs []proto.ChatMessage
	for i := range 2000 {
		msgs = append(msgs, proto.ChatMessage{ID: fmt.Sprint("m", i), Account: "a", Name: "Ada", Text: fmt.Sprint("Message ", i), At: at})
	}
	// The list starts at its end: these are the ones in view.
	msgs[1998].Text, msgs[1999].Text = framecheck.Line(), framecheck.Lines()
	tt := longContentTester(t, "team chat", func(c *ui.Context) { app.chatMessages(c, "team", msgs) })
	framecheck.Quick(t, "team chat", tt)
}

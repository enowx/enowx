package ui

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// A development build (make dev, ENOWX_DEV) can be asked to draw itself, for a
// picture of a surface where the system refuses to capture its window: its
// own view, with its own state, drawn off screen. Nothing of this runs
// outside a development build.

// devRequest is what a request file holds: the steps to get to the surface,
// and where its picture goes.
type devRequest struct {
	Out   string   `json:"out"`
	Size  [2]int   `json:"size"`
	Steps []string `json:"steps"`
}

// startDevDriver answers the request files of a development build's home.
func (a *App) startDevDriver() {
	home := os.Getenv("ENOWX_HOME")
	if os.Getenv("ENOWX_DEV") == "" || home == "" {
		return
	}
	file := filepath.Join(home, "devshot.json")
	go func() {
		for {
			time.Sleep(300 * time.Millisecond)
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			_ = os.Remove(file)
			var req devRequest
			if err := json.Unmarshal(data, &req); err != nil {
				continue
			}
			a.onMain(func() { a.runDevRequest(req) })
		}
	}()
}

// runDevRequest takes the steps, one at a time so that what they load arrives
// between them, then writes the picture and what it shows.
func (a *App) runDevRequest(req devRequest) {
	w, h := req.Size[0], req.Size[1]
	if w == 0 {
		w, h = 1440, 900
	}
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, w, h)
	var log []string
	frames := func() {
		for range 8 {
			tt.Advance(100 * time.Millisecond)
			tt.Frame()
		}
	}
	var next func(i int)
	next = func(i int) {
		frames()
		if i == len(req.Steps) {
			a.finishDevRequest(req, tt, log)
			return
		}
		if err := a.devStep(tt, req.Steps[i]); err != nil {
			log = append(log, req.Steps[i]+": "+err.Error())
		}
		time.AfterFunc(900*time.Millisecond, func() { a.onMain(func() { next(i + 1) }) })
	}
	frames()
	time.AfterFunc(900*time.Millisecond, func() { a.onMain(func() { next(0) }) })
}

// devStep takes one step.
func (a *App) devStep(tt *ui.Tester, step string) error {
	verb, arg, _ := strings.Cut(step, ":")
	switch verb {
	case "click":
		return tt.Click(arg)
	case "right":
		return tt.RightClick(arg)
	case "type":
		tt.Type(arg)
	case "page":
		a.page = arg
	case "mode":
		a.state.SessionsByProject = arg == "project"
	case "fold":
		a.state.RailWide = arg != "on"
	case "panel":
		if arg == "" {
			a.state.Panel.Open = false
		} else {
			a.openPanel(arg)
		}
	case "section":
		a.page, a.settingsSection = pageSettings, arg
	case "session":
		for _, s := range a.sessions {
			if a.sessionName(s) == arg {
				a.selectSession(s)
			}
		}
	case "view":
		if s, ok := a.selectedSession(); ok {
			a.centerOf(s.ID).front = ""
			a.showView(s, arg)
		}
	case "theme":
		a.state.Theme = arg
		a.applyTheme()
	case "move":
		if r, ok := tt.Find(arg); ok {
			tt.Move(r.X+r.W/2, r.Y+r.H/2)
		}
	}
	return nil
}

// finishDevRequest writes the picture and what it shows.
func (a *App) finishDevRequest(req devRequest, tt *ui.Tester, log []string) {
	result := strings.Join(append(log, tt.Texts()...), "\n")
	f, err := os.Create(req.Out)
	if err == nil {
		err = png.Encode(f, tt.Image())
		f.Close()
	}
	if err != nil {
		result = fmt.Sprintf("picture: %v\n%s", err, result)
	}
	_ = os.WriteFile(req.Out+".txt", []byte(result), 0o644)
}

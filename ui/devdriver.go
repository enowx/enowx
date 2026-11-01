package ui

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
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

// runDevRequest takes the steps, then writes the picture and what it shows.
func (a *App) runDevRequest(req devRequest) {
	w, h := req.Size[0], req.Size[1]
	if w == 0 {
		w, h = 1440, 900
	}
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, w, h)
	settle := func(d time.Duration) {
		for t := time.Duration(0); t < d; t += 100 * time.Millisecond {
			tt.Advance(100 * time.Millisecond)
			tt.Frame()
		}
	}
	settle(500 * time.Millisecond)
	var log []string
	for _, step := range req.Steps {
		verb, arg, _ := strings.Cut(step, ":")
		var err error
		switch verb {
		case "click":
			err = tt.Click(arg)
		case "right":
			err = tt.RightClick(arg)
		case "type":
			tt.Type(arg)
		case "wait":
			ms, _ := strconv.Atoi(arg)
			time.Sleep(time.Duration(ms) * time.Millisecond)
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
		case "move":
			if r, ok := tt.Find(arg); ok {
				tt.Move(r.X+r.W/2, r.Y+r.H/2)
			}
		}
		if err != nil {
			log = append(log, step+": "+err.Error())
		}
		settle(600 * time.Millisecond)
	}
	settle(600 * time.Millisecond)
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

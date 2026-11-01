// Package appwindow shows a desktop app that runs on a session's virtual
// display (core/display) in a window of its own on this device, as if the
// app ran here: the window holds nothing but the app, follows its title,
// and resizing it resizes the app. It loads the display's HTML5 client
// (Xpra's) in a native web view, so it works wherever enowX does.
package appwindow

import (
	_ "embed"
	"math"
	"net/url"
	"strings"
	"sync"

	"github.com/egoist/mygo"
)

//go:embed shell.js
var shell string

// Titles the shell script gives the page (shell.js).
const (
	titleApp  = "enowx-app:"
	titleWait = "enowx-wait"
)

// Window is an app's window. It belongs to the main thread.
type Window struct {
	w       *mygo.Window
	closed  bool
	closing bool // Close asked: not the user
}

// quitting is set as enowX quits, which closes the windows too. Main
// thread only.
var (
	quitting  bool
	watchQuit sync.Once
)

// Options open a Window.
type Options struct {
	// Addr is where the display's client is served here (host:port): its
	// port, forwarded for a server.
	Addr string
	// Name titles the window until the app gives its own.
	Name string
	// Show shows the window at once, before the app opens one: when the
	// user asked for it. Otherwise it shows with the app's first window.
	Show bool
	// Scale is what the display's apps draw at (display.Display.Scale), 1
	// when 0: the window zooms so each of their pixels is one of the
	// screen's, and they show sharp.
	Scale int
	// OnClosed runs on the main thread once the window closed; byUser
	// tells the user closed it, as one quits an app, rather than enowX
	// (Close, or quitting).
	OnClosed func(byUser bool)
}

// ScreenScale is the scale of this device's main screen, rounded: what
// apps on a display should draw at to show sharp here.
func ScreenScale() int {
	return max(int(math.Round(mygo.Screen.PrimaryDisplay().ScaleFactor)), 1)
}

// Available reports whether app windows can show here: they need a web
// view.
func Available() bool { return mygo.WebViewAvailable() }

// Open opens a window for the display at o.Addr.
func Open(o Options) *Window {
	q := url.Values{}
	for k, v := range map[string]string{
		"floating_menu": "false", "clock": "false", "sound": "false", "printing": "false",
		"file_transfer": "false", "remote_logging": "false", "reconnect": "true", "steal": "true",
		// Reading the clipboard on each click makes WebKit offer a Paste
		// button that takes the click; pastes still go through.
		"clipboard_poll": "false",
		// Video encodings blur the app while it changes.
		"video": "false",
	} {
		q.Set(k, v)
	}
	zoom := 1.0
	if o.Scale > 1 {
		zoom = mygo.Screen.PrimaryDisplay().ScaleFactor / float64(o.Scale)
	}
	a := &Window{}
	a.w = mygo.NewWindow(mygo.WindowOptions{
		Title:           o.Name,
		URL:             "http://" + o.Addr + "/index.html?" + q.Encode(),
		Width:           1100,
		Height:          760,
		Hidden:          !o.Show,
		BackgroundColor: "#000000",
		Page:            mygo.PageOptions{PreloadScript: shell, ZoomFactor: zoom},
	})
	a.w.Page().OnPageTitleUpdated(func(e *mygo.TitleEvent) {
		e.PreventDefault()
		switch {
		case strings.HasPrefix(e.Title, titleApp):
			if t := strings.TrimPrefix(e.Title, titleApp); t != "" {
				a.w.SetTitle(t)
			}
			if !a.w.IsVisible() {
				a.w.ShowInactive() // an app starting doesn't take the keyboard
			}
		case e.Title == titleWait && !o.Show:
			// No window: the app quit or restarts, as on a desktop.
			a.w.Hide()
		}
	})
	watchQuit.Do(func() { mygo.App.OnWillQuit(func(*mygo.QuitEvent) { quitting = true }) })
	a.w.OnClosed(func() {
		a.closed = true
		if o.OnClosed != nil {
			o.OnClosed(!a.closing && !quitting)
		}
	})
	return a
}

// Focus shows the window in front.
func (a *Window) Focus() {
	if a.closed {
		return
	}
	a.w.Show()
	a.w.Focus()
}

// Close closes the window; the app goes on running on its display.
func (a *Window) Close() {
	if !a.closed {
		a.closing = true
		a.w.Close()
	}
}

package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
	"github.com/enowx/enowx/version"
)

// The app updates itself: it looks for a new version a little after it
// starts and every few hours, shows it in a card in the window's corner
// (until put off to the next launch) and in the title bar, and installs it
// when the user says so. Restarting runs the new version, which replaces
// the local daemon (hostconn), and the daemon brings the sessions back.

const (
	firstUpdateCheck  = 15 * time.Second
	checkUpdatesEvery = 6 * time.Hour
	checkTimeout      = 30 * time.Second
)

// updateSource finds and installs new versions: mygo.Updater, which
// downloads signed releases, or a stand-in in tests.
type updateSource interface {
	// Enabled reports whether this build can update itself: it was built
	// with a feed to update from, and can write where it is installed.
	Enabled() bool
	// Check returns a newer version, or nil when the app is up to date.
	Check(ctx context.Context) (*newVersion, error)
	// Relaunch quits and starts the version installed.
	Relaunch()
}

// newVersion is a version newer than the running one.
type newVersion struct {
	Version string
	Notes   string // Markdown
	Date    time.Time
	// Install downloads and installs it; the running app goes on until it
	// relaunches.
	Install func(ctx context.Context, progress func(done, total int64)) error
}

// mygoUpdates updates from the feed `mygo build` publishes.
type mygoUpdates struct{}

func (mygoUpdates) Enabled() bool { return mygo.Updater.Enabled() }

func (mygoUpdates) Check(ctx context.Context) (*newVersion, error) {
	up, err := mygo.Updater.Check(ctx)
	if err != nil || up == nil {
		return nil, err
	}
	return &newVersion{Version: up.Version, Notes: up.Notes, Date: up.Date, Install: up.Install}, nil
}

func (mygoUpdates) Relaunch() { mygo.App.Relaunch() }

// Update states.
const (
	updateIdle        = iota
	updateChecking    // asking the feed
	updateAvailable   // a new version, not downloaded
	updateDownloading // installing it
	updateReady       // installed: restart to run it
)

// updater is the app's update state. It belongs to the main thread.
type updater struct {
	source  updateSource
	post    func(func()) // runs a function on the main thread: the window's Update
	state   int
	found   *newVersion
	done    int64 // bytes downloaded, of total
	total   int64
	err     string // why the last check or install failed
	checked time.Time
	dialog  bool
	later   bool               // the corner card put off until the next launch
	cancel  context.CancelFunc // stops the download
}

// updatePrefs are the user's update choices, remembered between launches.
type updatePrefs struct {
	Manual  bool   `json:"manual,omitempty"`  // don't look for updates on its own
	Skipped string `json:"skipped,omitempty"` // a version the user skipped
}

// startUpdates looks for updates in the background while the app runs.
func (a *App) startUpdates() {
	if a.updates.source == nil {
		a.updates.source = mygoUpdates{}
	}
	if a.updates.post == nil {
		a.updates.post = a.win.Update
	}
	go func() {
		wait := firstUpdateCheck
		for {
			time.Sleep(wait)
			wait = checkUpdatesEvery
			a.updates.post(func() {
				if !a.state.Updates.Manual {
					a.checkForUpdates(false)
				}
			})
		}
	}()
}

// checkForUpdates asks for a new version. A check the user asked for says
// how it went; one of the app's own only shows a version found that the
// user didn't skip.
func (a *App) checkForUpdates(asked bool) {
	u := &a.updates
	if u.state == updateChecking || u.state == updateDownloading || u.state == updateReady {
		if asked {
			u.dialog = u.state != updateChecking
		}
		return
	}
	if !u.source.Enabled() {
		if asked {
			a.notify("This build of enowX doesn't update itself: it has no update feed, or can't write where it is installed.")
		}
		return
	}
	u.state, u.err = updateChecking, ""
	src := u.source
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		defer cancel()
		found, err := src.Check(ctx)
		u.post(func() {
			u.state, u.checked = updateIdle, time.Now()
			switch {
			case err != nil:
				if asked {
					a.status = "Couldn't check for updates: " + err.Error()
				}
			case found == nil:
				u.found = nil
				if asked {
					a.notify("enowX " + version.String + " is the latest version.")
				}
			case !asked && found.Version == a.state.Updates.Skipped:
				u.found = nil
			default:
				u.state, u.found = updateAvailable, found
				u.dialog = asked
			}
		})
	}()
}

// installUpdate downloads and installs the version found.
func (a *App) installUpdate() {
	u := &a.updates
	if u.state != updateAvailable || u.found == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.state, u.err, u.done, u.total, u.cancel = updateDownloading, "", 0, 0, cancel
	install := u.found.Install
	go func() {
		err := install(ctx, func(done, total int64) {
			u.post(func() { u.done, u.total = done, total })
		})
		u.post(func() {
			u.cancel = nil
			switch {
			case errors.Is(err, context.Canceled):
				u.state = updateAvailable
			case err != nil:
				u.state, u.err = updateAvailable, err.Error()
			default:
				u.state = updateReady
			}
		})
	}()
}

// updateLabel is what the title bar says of the update, "" for nothing.
func (a *App) updateLabel() string {
	u := &a.updates
	switch u.state {
	case updateAvailable:
		return "Update to " + u.found.Version
	case updateDownloading:
		if u.total > 0 {
			return fmt.Sprintf("Updating · %d%%", u.done*100/u.total)
		}
		return "Updating…"
	case updateReady:
		return "Restart to update"
	}
	return ""
}

// updateChip is the title bar's update button, while there is one.
func (a *App) updateChip(c *ui.Context) {
	label := a.updateLabel()
	if label == "" {
		return
	}
	b := titleChip(c, "update", label)
	b.TextColor(theme.Accent).Children(func() {
		ui.Icon(c, icon.ArrowUp).FontSize(11)
		ui.Text(c, label).FontSize(theme.TextLabel).SingleLine()
	})
	if b.Clicked() {
		a.updates.dialog = true
	}
}

// updateDialog shows the new version, its notes, and what to do with it.
func (a *App) updateDialog(c *ui.Context) {
	u := &a.updates
	if !u.dialog || u.found == nil {
		u.dialog = false
		return
	}
	v := u.found
	desc := "You have " + version.String
	if !v.Date.IsZero() {
		desc += " · released " + textfmt.Age(v.Date, time.Now()) + " ago"
	}
	widget.Sheet(c, &u.dialog, 520, func() {
		widget.SheetHead(c, &u.dialog, "enowX "+v.Version+" is available", desc)
	}, func() {
		if v.Notes != "" {
			ui.Scroll(c).FillWidth().MaxHeight(280).Children(func() {
				ui.Column(c).FillWidth().Gap(theme.S2).Children(func() { widget.Markdown(c, v.Notes) })
			})
		}
		switch u.state {
		case updateDownloading:
			frac := 0.0
			if u.total > 0 {
				frac = float64(u.done) / float64(u.total)
			}
			widget.Meter(c, widget.Segment{Fraction: frac, Color: theme.Accent})
			ui.Text(c, "Downloading and checking its signature…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		case updateReady:
			ui.Text(c, "Installed. Restarting runs it: the daemon updates with it and brings your sessions back, agents continuing their conversations.").
				FontSize(theme.TextUI).TextColor(theme.TextSoft)
		}
		if u.err != "" {
			ui.Text(c, "The update failed: "+u.err).FontSize(theme.TextUI).TextColor(theme.Error)
		}
		auto := !a.state.Updates.Manual
		if widget.Checkbox(c, &auto, "Check for updates automatically") {
			a.state.Updates.Manual = !auto
		}
	}, func() {
		switch u.state {
		case updateAvailable:
			if widget.GhostButton(c, "Skip this version").Clicked() {
				a.state.Updates.Skipped = v.Version
				u.state, u.found, u.dialog = updateIdle, nil, false
			}
			if widget.GhostButton(c, "Later").Clicked() {
				u.dialog = false
			}
			label := "Install update"
			if u.err != "" {
				label = "Try again"
			}
			if widget.PrimaryButton(c, icon.ArrowUp, label, "").Clicked() {
				a.installUpdate()
			}
		case updateDownloading:
			if widget.GhostButton(c, "Cancel").Clicked() && u.cancel != nil {
				u.cancel()
			}
		case updateReady:
			if widget.GhostButton(c, "Later").Clicked() {
				u.dialog = false
			}
			if widget.PrimaryButton(c, icon.Restore, "Restart now", "").Clicked() {
				u.source.Relaunch()
			}
		}
	})
}

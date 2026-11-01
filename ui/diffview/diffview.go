// Package diffview shows what a session's branch changed against its base,
// committed or not, and the actions that take the work on from there:
// commit, push, open a pull request, merge into the base, discard the
// worktree. The daemon runs git; the view asks it again every few seconds
// while it shows, since the agent keeps changing files.
package diffview

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/widget"
)

// refreshEvery is how often a shown view asks for the diff again.
const refreshEvery = 4 * time.Second

// Options connect a view to the app.
type Options struct {
	Client  *client.Client
	Session proto.SessionInfo
	// Update runs a function on the main thread; Wanted, called there,
	// reports whether the view still shows, else its poller stops.
	Update func(func())
	Wanted func() bool
	// OpenURL opens a pull request in the browser; Notify tells news, as
	// a status bar does.
	OpenURL func(url string)
	Notify  func(msg string)
	// Send types a prompt into the session's agent.
	Send func(text string)
	// Conflicts are the files another running session changes too;
	// ShowSession selects a session.
	Conflicts   []ConflictNote
	ShowSession func(id string)
	// OpenDiff shows a changed file in a center tab, OpenFile the file itself;
	// Current is the changed file in front, to mark it in the list.
	OpenDiff func(path string)
	OpenFile func(path string)
	Current  func() string
}

// ConflictNote is a file the session and another one both change.
type ConflictNote struct {
	File    string
	Session string // the other session's ID
	With    string // its name
	Lines   bool   // they change lines in common
}

// View is one session's diff and its actions. It belongs to the main
// thread; its poller reports through Options.Update.
type View struct {
	base    string // the base the user picked; "" for the session's default
	status  proto.ReviewStatus
	diff    proto.Diff
	loaded  bool
	err     string
	polling bool
	gen     int           // counts bases picked, so a poller of an old one stops
	refresh chan struct{} // asks the poller to load now
	busy    string        // the action under way, as a verb: "Committing"
	asked   string        // an action asked for before the view loaded (Ask)

	rows        []row        // the lines of the file open
	fileList    ui.ListState // the lines of the file open
	file        string       // the path of the file open
	split       bool         // rows lay it out side by side
	splitWanted bool         // the user reads changes side by side
	comments []Comment    // review comments not sent yet
	draft    *draft       // the comment being written

	baseMenuState widget.ContextMenu
	moreMenu      widget.ContextMenu
	commit        *commitForm
	pr            *prForm
	merging       bool // asking to confirm a merge
	discard       *discardForm
	lastPRURL     string
}

type commitForm struct {
	open    bool
	message string
}

type prForm struct {
	open        bool
	title, body string
}

type discardForm struct {
	open         bool
	deleteBranch bool
}

// New returns a view with nothing loaded.
func New() *View {
	return &View{refresh: make(chan struct{}, 1)}
}

// follow polls the diff while the view is wanted, unless it already does.
func (v *View) follow(o Options) {
	if v.polling || o.Client == nil {
		return
	}
	v.polling = true
	cl, id, base, gen := o.Client, o.Session.ID, v.base, v.gen
	go func() {
		for {
			st, err := cl.ReviewStatus(id, base)
			var d proto.Diff
			if err == nil {
				d, err = cl.Diff(id, base)
			}
			keep := make(chan bool, 1)
			o.Update(func() {
				if gen != v.gen {
					keep <- false // the poller of a newer base took over
					return
				}
				if !o.Wanted() {
					v.polling = false
					keep <- false
					return
				}
				keep <- true
				v.loaded = true
				if err != nil {
					v.err = client.Explain(err)
					return
				}
				v.err, v.status = "", st
				v.setDiff(d)
			})
			if !<-keep {
				return
			}
			select {
			case <-time.After(refreshEvery):
			case <-v.refresh:
			}
		}
	}()
}

// reload asks the poller for the diff now.
func (v *View) reload() {
	select {
	case v.refresh <- struct{}{}:
	default:
	}
}

// setBase compares with base from now on; its poller starts over.
func (v *View) setBase(base string) {
	if base == v.base {
		return
	}
	v.base, v.loaded = base, false
	v.gen++
	v.polling = false // the next frame starts a poller for base
	v.reload()
}

// run runs an action off the main thread, then reloads; done, if set,
// runs on the main thread when it succeeded.
func (v *View) run(o Options, verb string, action func(cl *client.Client) error, done func()) {
	if v.busy != "" || o.Client == nil {
		return
	}
	v.busy = verb
	cl := o.Client
	go func() {
		err := action(cl)
		o.Update(func() {
			v.busy = ""
			if err != nil {
				v.err = client.Explain(err)
				return
			}
			v.err = ""
			if done != nil {
				done()
			}
			v.reload()
		})
	}()
}

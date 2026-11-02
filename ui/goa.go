package ui

import (
	"encoding/json"
	"slices"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/proto"
)

// GOD of Agents mode (docs/goa.md): the connected host's GOA session is
// the one the user drives, the session list folded away, the side panel
// kept with the Sessions and Tasks tools, Notes, Canvas and the other
// pages reached from the title bar's pane. The app also tells GOA's host
// of its workers on other hosts, which only it sees.

// goaApp is the app's side of GOA, on the main thread.
type goaApp struct {
	host     string // the host the board is of (its hosts.Host ID)
	loading  bool
	board    proto.GOABoard
	flags    []proto.GOAFlag
	agents   []proto.GOAAgent
	start    *goaStartForm
	override *goaOverrideForm
	detail   *goaDetail    // a session's details, from the Sessions tool
	rules    *goaRulesForm // its rules being edited (goa_rules.go)
	stall    *goaStallForm // when a task counts as stalled, being edited (goa_stall.go)
	// tabs are the sessions open beside GOA, as center tabs of its session
	// (goa_tabs.go).
	tabs []*goaTab
	err  string
	// chatSet is the GOA session last opened on its chat, once: the user's
	// own choice of view is kept after.
	chatSet string
}

// goaOn says the app shows GOA mode: asked for, and GOA known on the host.
func (a *App) goaOn() bool {
	return a.state.GOA && a.client != nil && a.goa.host == a.host.ID && a.goa.board.Session != ""
}

// goaLoad reads the connected host's board and flags, once per host.
func (a *App) goaLoad() {
	if a.client == nil || a.goa.loading || a.goa.host == a.host.ID {
		return
	}
	a.goa.loading = true
	cl, host := a.client, a.host.ID
	go func() {
		var b proto.GOABoard
		err := cl.Call(proto.MethodGOABoard, nil, &b)
		var flags []proto.GOAFlag
		if err == nil {
			_ = cl.Call(proto.MethodGOAFlags, nil, &flags)
		}
		a.onMain(func() {
			a.goa.loading = false
			if a.host.ID != host {
				return
			}
			a.goa.host, a.goa.board, a.goa.flags = host, b, flags // an older daemon: none
		})
	}()
}

// goaEvent takes the connected host's GOA events; it runs on the client's
// goroutine.
func (a *App) goaEvent(ev proto.Event) {
	switch ev.Name {
	case proto.EventGOABoard:
		var b proto.GOABoard
		if json.Unmarshal(ev.Data, &b) == nil {
			a.onMain(func() {
				if a.goa.host == a.host.ID {
					a.goa.board = b
				}
			})
		}
	case proto.EventGOAFlag:
		var f proto.GOAFlag
		if json.Unmarshal(ev.Data, &f) == nil {
			a.onMain(func() {
				i := slices.IndexFunc(a.goa.flags, func(x proto.GOAFlag) bool { return x.ID == f.ID })
				if i >= 0 {
					a.goa.flags[i] = f
				} else {
					a.goa.flags = append(a.goa.flags, f)
				}
			})
		}
	}
}

// goaRelay tells GOA's host that its worker info, on host (as the app
// names it), changed: only the app sees other hosts.
func (a *App) goaRelay(host string, info proto.SessionInfo) {
	if a.client == nil || a.goa.host != a.host.ID || a.goaTaskOf(info.ID) == nil {
		return
	}
	cl := a.client
	go func() { _ = cl.Call(proto.MethodGOAWorker, proto.GOAWorkerParams{Host: host, Session: info}, nil) }()
}

// goaCatchUp relays, of host's sessions as a poll read them, the workers
// whose open task the board shows otherwise: changes the app missed while
// closed or not connected there, or before it had the board. A settled
// task is left alone, GOA's word on it standing.
func (a *App) goaCatchUp(host string, sessions []proto.SessionInfo) {
	for _, s := range sessions {
		t := a.goaTaskOf(s.ID)
		if t == nil || goaSettled(*t) {
			continue
		}
		if st := goa.WorkerStatus(s); st != "" && st != t.Status {
			a.goaRelay(host, s)
		}
	}
}

// goaTaskOf is session id's latest task on the board, nil for none.
func (a *App) goaTaskOf(id string) *proto.GOATask {
	tasks := a.goa.board.Tasks
	for i := len(tasks) - 1; i >= 0; i-- {
		if tasks[i].Session == id {
			return &tasks[i]
		}
	}
	return nil
}

// enterGOA shows GOA mode, or asks which agent to start GOA on.
func (a *App) enterGOA() {
	if a.client == nil {
		return
	}
	if a.goa.host == a.host.ID && a.goa.board.Session != "" {
		a.state.GOA = true
		a.page, a.maximized = "", ""
		a.selected = a.goa.board.Session
		a.openPanel(panelSessions)
		return
	}
	a.openGOAStart()
}

// leaveGOA goes back to the projects and their sessions; GOA keeps
// running.
func (a *App) leaveGOA() {
	a.closeGOATabs()
	a.state.GOA = false
	a.page = ""
	if a.selected == a.goa.board.Session {
		a.selected = ""
	}
	if t := a.panelTab(); t == panelSessions || t == panelTasks {
		a.state.Panel.Tab = panelChanges
	}
}

// toggleGOA enters GOA mode, or leaves it once the user confirms.
func (a *App) toggleGOA() {
	if a.goaOn() {
		a.confirmLeaveGOA(nil)
		return
	}
	a.enterGOA()
}

// confirmLeaveGOA leaves GOA mode once the user confirms, then runs then
// (nil for nothing): leaving takes the user away from the one session
// they drive, so it asks first, as every action out of a mode does.
func (a *App) confirmLeaveGOA(then func()) {
	a.confirm("Leave GOD of Agents?", "GOA keeps running, and you can come back to it.", "Leave", func() {
		a.leaveGOA()
		if then != nil {
			then()
		}
	})
}

// goaSession is GOA's session in the list, if there.
func (a *App) goaSession() (proto.SessionInfo, bool) {
	i := slices.IndexFunc(a.sessions, func(s proto.SessionInfo) bool { return s.ID == a.goa.board.Session })
	if i < 0 {
		return proto.SessionInfo{}, false
	}
	return a.sessions[i], true
}

// goaCall runs method on the connected host, then fn on the main thread
// with its error.
func (a *App) goaCall(method string, params, out any, fn func(error)) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		err := cl.Call(method, params, out)
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
			}
			if fn != nil {
				fn(err)
			}
		})
	}()
}

// goaSettled says whether task t is over.
func goaSettled(t proto.GOATask) bool { return goa.Settled(t.Status) }

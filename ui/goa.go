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
// pages reached from a strip on top of the main view. The app also tells
// GOA's host of its workers on other hosts, which only it sees.

// goaApp is the app's side of GOA, on the main thread.
type goaApp struct {
	host     string // the host the board is of (its hosts.Host ID)
	loading  bool
	board    proto.GOABoard
	flags    []proto.GOAFlag
	agents   []proto.GOAAgent
	start    *goaStartForm
	override *goaOverrideForm
	detail   *goaDetail // a session's details, from the Sessions tool
	err      string
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
	a.state.GOA = false
	a.page = ""
	if a.selected == a.goa.board.Session {
		a.selected = ""
	}
	if t := a.panelTab(); t == panelSessions || t == panelTasks {
		a.state.Panel.Tab = panelChanges
	}
}

// toggleGOA enters GOA mode, or leaves it.
func (a *App) toggleGOA() {
	if a.goaOn() {
		a.leaveGOA()
		return
	}
	a.enterGOA()
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

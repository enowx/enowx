package ui

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hostcontrol"
	"github.com/enowx/enowx/proto"
)

// A session controlling another host's sessions and settings (host.call,
// daemon/hostcontrol.go): the app, the only one reaching other hosts,
// checks what is asked against what the user may do there (all on their
// own hosts; on a teammate's shared host, what their role allows, which
// that host checks again), asks the user first for what can't be undone,
// and does it with its connection to the host.

// hostCallAsked does what an agent on the host of daemon from asked.
func (a *App) hostCallAsked(from string, req proto.AppRequest, reply func(any, error)) {
	var p proto.HostCallParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		reply(nil, err)
		return
	}
	level, err := hostcontrol.Check(p.Method, "")
	if err != nil {
		reply(nil, err)
		return
	}
	var params any
	if len(p.Params) > 0 {
		params = p.Params
	}
	call := func(cl *client.Client, done func()) {
		go func() {
			if done != nil {
				defer done()
			}
			var out json.RawMessage
			err := cl.Call(p.Method, params, &out)
			if err != nil {
				err = errors.New(strings.TrimPrefix(err.Error(), p.Method+": "))
			}
			reply(out, err)
		}()
	}
	run, hostName, ok := a.controlTarget(p, call, reply)
	if !ok {
		return
	}
	if level != hostcontrol.Confirm || p.From == "" {
		run()
		return
	}
	asker, askerHost := p.From, ""
	if h, ok := a.reach(from); ok {
		askerHost = " on " + h.host.Name
	}
	if s, ok := a.sessionByID(p.From); ok {
		asker = a.sessionName(s)
	}
	a.confirmOr("Let a session do this?",
		"The agent of “"+asker+"”"+askerHost+" asks to "+controlWhat(p)+" on "+hostName+". It can't be undone.",
		"Allow", run, func() { reply(nil, errors.New("the user said no")) })
}

// controlTarget finds the host p names: one of the user's the app is
// connected to, or a teammate's shared host, visited with the user's
// role. run does p there; ok is false when it answered why it can't.
func (a *App) controlTarget(p proto.HostCallParams, call func(*client.Client, func()), reply func(any, error)) (run func(), name string, ok bool) {
	for _, h := range a.reachable() {
		if h.info.ID == p.Host || h.host.ID == p.Host || strings.EqualFold(h.host.Name, p.Host) {
			return func() { call(h.cl, nil) }, h.host.Name, true
		}
	}
	if th := a.teamHostList; th != nil && a.client != nil {
		cl := a.client
		for _, h := range th.hosts {
			if "team:"+h.Device != p.Host && h.Device != p.Host && !strings.EqualFold(h.Name, p.Host) {
				continue
			}
			return func() {
				go func() {
					vc, v, err := visitHost(cl, h, nil)
					if err != nil {
						reply(nil, err)
						return
					}
					if _, err := hostcontrol.Check(p.Method, v.Role); err != nil {
						vc.Close()
						reply(nil, err)
						return
					}
					call(vc, func() { vc.Close() })
				}()
			}, h.Name + " (" + h.Owner + "'s, shared)", true
		}
	}
	reply(nil, errors.New("no host “"+p.Host+"” in the app's reach now: "+strings.Join(a.reachableNames(), ", ")))
	return nil, "", false
}

func (a *App) reachableNames() []string {
	var out []string
	for _, h := range a.reachable() {
		out = append(out, h.host.Name)
	}
	return out
}

// controlWhat says what p does, for the user to judge.
func controlWhat(p proto.HostCallParams) string {
	var args struct {
		ID, Session, Path, Name, Base string
		DeleteBranch                  bool
	}
	_ = json.Unmarshal(p.Params, &args)
	sess := args.ID
	if sess == "" {
		sess = args.Session
	}
	switch p.Method {
	case proto.MethodSessionKill:
		return "close session " + sess + " (its terminal and what runs in it end)"
	case proto.MethodReviewPush:
		return "push session " + sess + "'s branch"
	case proto.MethodReviewPR:
		return "push session " + sess + "'s branch and open a pull request"
	case proto.MethodReviewMerge:
		return "merge session " + sess + "'s branch into " + nonEmpty(args.Base, "its base")
	case proto.MethodReviewDiscard:
		if args.DeleteBranch {
			return "discard session " + sess + "'s worktree and delete its branch"
		}
		return "discard session " + sess + "'s worktree"
	case proto.MethodProjectRemove:
		return "remove the project " + args.Path + " from the list"
	case proto.MethodSessionEnvSet:
		return "set the variable " + args.Name + " for every session"
	}
	return p.Method
}

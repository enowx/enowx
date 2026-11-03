package ui

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hostcontrol"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/widget"
)

// A session controlling another host's sessions and settings (host.call,
// daemon/hostcontrol.go): the app, the only one reaching other hosts,
// checks what is asked against what the user may do there (all on their
// own hosts; on a teammate's shared host, what their role allows, which
// that host checks again), asks the user first for what can't be undone
// unless the asking agent runs without asking on the user's own hosts, and
// does it with its connection to the host.

// hostCallAsked does what an agent on the host of daemon from asked;
// bypassed marks it done without asking the user.
func (a *App) hostCallAsked(from string, req proto.AppRequest, reply func(any, error), bypassed func()) {
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
			if err := urgentForUser(cl, p); err != nil {
				reply(nil, err)
				return
			}
			var out json.RawMessage
			err := cl.Call(p.Method, params, &out)
			if err != nil {
				err = errors.New(strings.TrimPrefix(err.Error(), p.Method+": "))
			}
			reply(out, err)
		}()
	}
	run, hostName, shared, ok := a.controlTarget(p, call, reply)
	if !ok {
		return
	}
	hibernate := agentHibernates(p)
	if (level != hostcontrol.Confirm && !hibernate) || p.From == "" {
		run()
		return
	}
	asker, askerHost := p.From, ""
	h, ownAsker := a.reach(from)
	if ownAsker {
		askerHost = " on " + h.host.Name
	}
	var teammate bool
	if s, ok := a.sessionByID(p.From); ok {
		asker = a.sessionName(s)
		teammate = s.Member != nil
	}
	askUser := func() {
		after := " It can't be undone."
		if hibernate {
			after = " It can be continued later."
		}
		q := &hostAsk{key: a.hostAskKey + 1, until: askUntil(hostAskLife),
			about: "The agent of “" + asker + "”" + askerHost + " asks to " + controlWhat(p) + " on " + hostName + "." + after,
			run:   run, refuse: func() { reply(nil, errors.New("the user said no")) }}
		if (p.Method == proto.MethodSessionKill || hibernate) && ownAsker && !shared && !teammate {
			q.about += alwaysAllowNote(asker)
			q.always = func() { a.allowAlways(h.cl, p.From, run) }
		}
		a.hostAskKey++
		a.hostAsks = append(a.hostAsks, q)
	}
	// A teammate's shared host always asks: the asker's mode is about the
	// user's own machines, not their teammate's.
	switch {
	case p.Bypass == proto.ControlAlways && ownAsker && !shared:
		a.whenNothingLost(p, func() { bypassed(); run() }, askUser)
	case p.Bypass != "" && ownAsker && !shared:
		bypassed()
		run()
	default:
		askUser()
	}
}

// hostAsk is a session waiting for the user's yes.
type hostAsk struct {
	key    int
	about  string
	run    func()
	refuse func()
	always func() // Always allow, when its session can have that
	until  time.Time
}

// hostControlAsk is the card of the oldest session waiting.
func (a *App) hostControlAsk(*ui.Context) *widget.Ask {
	if len(a.hostAsks) == 0 {
		return nil
	}
	q := a.hostAsks[0]
	answer := func(do func()) func() {
		return func() {
			a.hostAsks = slices.DeleteFunc(a.hostAsks, func(x *hostAsk) bool { return x == q })
			do()
		}
	}
	buttons := []widget.AskButton{{Kind: widget.Secondary, Label: "Cancel", Do: answer(q.refuse)}}
	if q.always != nil {
		buttons = append(buttons, widget.AskButton{Kind: widget.Secondary, Label: "Always allow", Do: answer(q.always)})
	}
	buttons = append(buttons, widget.AskButton{Kind: widget.Primary, Label: "Allow", Do: answer(q.run)})
	return &widget.Ask{Key: "host:" + strconv.Itoa(q.key), Title: "Let a session do this?", About: q.about, Buttons: buttons}
}

// agentHibernates says p has an agent stop sessions' agents on another
// host, which that host can't ask about: the session is none of its own.
func agentHibernates(p proto.HostCallParams) bool {
	if p.Method != proto.MethodSessionHibernate || p.From == "" {
		return false
	}
	var args proto.SessionHibernateParams
	return json.Unmarshal(p.Params, &args) == nil && !args.DryRun
}

// controlTarget finds the host p names: one of the user's the app is
// connected to, or a teammate's shared host (shared), visited with the
// user's role. run does p there; ok is false when it answered why it can't.
func (a *App) controlTarget(p proto.HostCallParams, call func(*client.Client, func()), reply func(any, error)) (run func(), name string, shared, ok bool) {
	for _, h := range a.reachable() {
		if h.info.ID == p.Host || h.host.ID == p.Host || strings.EqualFold(h.host.Name, p.Host) {
			return func() { call(h.cl, nil) }, h.host.Name, false, true
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
			}, h.Name + " (" + h.Owner + "'s, shared)", true, true
		}
	}
	reply(nil, errors.New("no host “"+p.Host+"” in the app's reach now: "+strings.Join(a.reachableNames(), ", ")))
	return nil, "", false, false
}

// urgentForUser refuses a session's answer to what a GOA worker asks that
// is the user's (SessionAsk.Urgent), as the worker's host would for a
// session of its own (docs/goa.md).
func urgentForUser(cl *client.Client, p proto.HostCallParams) error {
	if p.Method != proto.MethodSessionAnswer || p.From == "" {
		return nil
	}
	var ref proto.SessionAnswerParams
	if json.Unmarshal(p.Params, &ref) != nil {
		return nil
	}
	if ask, err := cl.SessionAsk(ref.ID); err == nil && ask.Urgent != "" {
		return errors.New("goa: this one is the user's to answer (" + ask.Urgent + "): tell them, don't answer it")
	}
	return nil
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
		IDs                           []string
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
	case proto.MethodSessionHibernate:
		if len(args.IDs) == 0 {
			return "stop the idle sessions' agents"
		}
		return "stop the agent of session " + strings.Join(args.IDs, ", ")
	case proto.MethodSessionEnvSet:
		return "set the variable " + args.Name + " for every session"
	}
	return p.Method
}

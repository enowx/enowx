package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/browserview"
)

// enowx browser's actions, which the daemon hands this app
// (proto.EventBrowserRequest): done in the session's Browser, the same page
// the user sees in its side panel, opened for the agent when needed.

// driveBrowser does req in its session's Browser and replies. It runs on
// the main thread; the action itself, which waits on the page, doesn't.
func (a *App) driveBrowser(req proto.BrowserRequest) {
	cl := a.client
	if cl == nil {
		return
	}
	reply := func(r proto.BrowserResult, err error) {
		out := proto.BrowserReply{ID: req.ID, Result: r}
		if err != nil {
			out.Error = err.Error()
		}
		go func() { _ = cl.BrowserReply(out) }()
	}
	i := slices.IndexFunc(a.sessions, func(s proto.SessionInfo) bool { return s.ID == req.Session })
	if i < 0 {
		reply(proto.BrowserResult{}, errors.New("no session "+req.Session))
		return
	}
	s := a.sessions[i]
	if req.Action == proto.BrowserCapture {
		on := req.Text == "on"
		a.setWebCapture(a.topOf(s.ID), on)
		reply(proto.BrowserResult{Text: req.Text}, nil)
		return
	}
	v := a.browserOf(s.ID)
	if req.Action == proto.BrowserOpen {
		if err := v.Open(a.browserOptions(s), req.URL); err != nil {
			reply(proto.BrowserResult{}, err)
			return
		}
		if a.selected == s.ID && !a.state.BrowserViews[s.ID] {
			a.browserTabBehind(s)
			a.showBrowserSoon() // the user sees what the agent opened, unless at something else
		}
	} else if v.Page() == nil {
		reply(proto.BrowserResult{}, errors.New("its Browser has no page open: enowx-app browser open URL first"))
		return
	}
	go func() {
		r, err := a.browserAction(cl, v, req.BrowserAction)
		reply(r, err)
	}()
}

// browserAction does a in v's page, waiting for it to open, and says
// where the page is after.
func (a *App) browserAction(cl *client.Client, v *browserview.View, act proto.BrowserAction) (proto.BrowserResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	page := a.pageOf(ctx, v)
	if page == nil {
		return proto.BrowserResult{}, errors.New("the Browser didn't open a page (does this device have a web view?)")
	}
	ev, ok := page.(browserview.Evaluator)
	if !ok {
		return proto.BrowserResult{}, errors.New("this Browser can't be driven")
	}
	var r proto.BrowserResult
	var err error
	switch act.Action {
	case proto.BrowserOpen:
		browserview.Settle(ctx, page)
	case proto.BrowserSnapshot:
		r.Text, err = browserview.Snapshot(ctx, ev)
	case proto.BrowserClick:
		if err = browserview.Click(ctx, ev, act.Ref); err == nil {
			browserview.Settle(ctx, page)
		}
	case proto.BrowserFill:
		err = browserview.Fill(ctx, ev, act.Ref, act.Text)
	case proto.BrowserScreenshot:
		r.Image, err = page.Capture()
	case proto.BrowserBack, proto.BrowserForward, proto.BrowserReload:
		a.onMain(func() {
			switch act.Action {
			case proto.BrowserBack:
				page.Back()
			case proto.BrowserForward:
				page.Forward()
			default:
				page.Reload()
			}
		})
		time.Sleep(200 * time.Millisecond) // the page starts going
		browserview.Settle(ctx, page)
	case proto.BrowserEval:
		var v any
		if v, err = ev.Eval(ctx, act.Text); err == nil {
			r.Text = evalText(v)
		}
	case proto.BrowserWidth:
		n, _ := strconv.Atoi(act.Text)
		was := make(chan int, 1)
		a.onMain(func() { was <- v.Width(); v.SetWidth(n) })
		r.Text = strconv.Itoa(<-was)
		time.Sleep(500 * time.Millisecond) // laid out again at its width
		browserview.Settle(ctx, page)
	default:
		err = errors.New("unknown action " + act.Action)
	}
	r.URL, r.Title = page.URL(), browserview.Title(ctx, ev)
	return r, err
}

// pageOf waits for v's page, which opening it makes on the main thread.
func (a *App) pageOf(ctx context.Context, v *browserview.View) browserview.Page {
	for ctx.Err() == nil {
		got := make(chan browserview.Page, 1)
		a.onMain(func() { got <- v.Page() })
		select {
		case p := <-got:
			if p != nil {
				return p
			}
		case <-ctx.Done():
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

// evalText is a value the page's JavaScript gave, as an agent reads it.
func evalText(v any) string {
	switch v := v.(type) {
	case nil:
		return "undefined"
	case string:
		return v
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

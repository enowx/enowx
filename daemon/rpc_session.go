package daemon

import (
	"fmt"
	"time"

	"github.com/enowx/enowx/proto"
)

var sessionHandlers = map[string]handler{
	proto.MethodSessionCreate: with(func(c *conn, p proto.SessionCreateParams) (any, error) {
		sess, err := c.srv.createSession(p)
		if err != nil {
			return nil, err
		}
		return sess.Info(), nil
	}),
	proto.MethodSessionFanOut: with(func(c *conn, p proto.FanOutParams) (any, error) {
		return c.srv.fanOut(p)
	}),
	proto.MethodSessionList: bare(func(c *conn) (any, error) {
		list := c.srv.sessions.List()
		infos := make([]proto.SessionInfo, 0, len(list))
		for _, sess := range list {
			infos = append(infos, sess.Info())
		}
		return infos, nil
	}),
	proto.MethodSessionAttach: with(func(c *conn, p proto.SessionRef) (any, error) {
		id, err := c.attach(p.ID)
		if err != nil {
			return nil, err
		}
		return proto.AttachResult{Stream: id}, nil
	}),
	proto.MethodSessionDetach: with(func(c *conn, p proto.StreamRef) (any, error) {
		c.detach(p.Stream)
		return ok, nil
	}),
	proto.MethodSessionResize: with(func(c *conn, p proto.ResizeParams) (any, error) {
		st := c.stream(p.Stream)
		if st == nil {
			return nil, fmt.Errorf("unknown stream %d", p.Stream)
		}
		st.resize(p)
		return ok, nil
	}),
	proto.MethodSessionTranscript: with(func(c *conn, p proto.TranscriptParams) (any, error) {
		t, err := c.srv.transcript(p)
		return maskLentIn(c.srv, t), err
	}),
	proto.MethodSessionMode: with(func(c *conn, p proto.SessionModeParams) (any, error) {
		return ok, c.srv.switchMode(p)
	}),
	proto.MethodAgentHistory: with(func(c *conn, p proto.HistoryParams) (any, error) {
		return c.srv.history(p)
	}),
	proto.MethodSessionNote: with(func(c *conn, p proto.SessionNoteParams) (any, error) {
		sess, err := c.srv.sessions.Get(p.ID)
		if err != nil {
			return nil, err
		}
		c.srv.sessions.SetNote(sess, p.Note)
		return ok, nil
	}),
	proto.MethodSessionScreen: with(func(c *conn, p proto.SessionRef) (any, error) {
		sess, err := c.srv.sessions.Get(p.ID)
		if err != nil {
			return nil, err
		}
		return proto.SessionScreen{Text: c.srv.maskLent(sess.Terminal().Text())}, nil
	}),
	proto.MethodSessionInput: with(func(c *conn, p proto.SessionInputParams) (any, error) {
		if p.From != "" {
			c.srv.goaMarkTold(p.From, p.ID)
		}
		return ok, c.srv.input(p)
	}),
	proto.MethodSessionRewind: with(func(c *conn, p proto.SessionRewindParams) (any, error) {
		return ok, c.srv.rewind(p)
	}),
	proto.MethodSessionCommands: with(func(c *conn, p proto.SessionRef) (any, error) {
		return c.srv.commands(p)
	}),
	proto.MethodSessionState: with(func(c *conn, p proto.SessionStateParams) (any, error) {
		sess, err := c.srv.sessions.Get(p.ID)
		if err != nil {
			return nil, err
		}
		return ok, c.srv.sessions.SetState(sess, p.State)
	}),
	proto.MethodDisplayScale: with(func(c *conn, p proto.DisplayScaleParams) (any, error) {
		c.srv.displays.scale.Store(int32(min(max(p.Scale, 1), 4)))
		return ok, nil
	}),
	proto.MethodSessionFork: with(func(c *conn, p proto.SessionForkParams) (any, error) { return c.srv.forkSession(p) }),
	proto.MethodSessionContinue: with(func(c *conn, p proto.SessionRef) (any, error) {
		sess, err := c.srv.continueSession(p.ID)
		if err != nil {
			return nil, err
		}
		return sess.Info(), nil
	}),
	proto.MethodSessionDisplayStop: with(func(c *conn, p proto.DisplayStopParams) (any, error) {
		return ok, c.srv.stopDisplay(p)
	}),
	proto.MethodSessionDisplay: with(func(c *conn, p proto.SessionRef) (any, error) {
		return c.srv.sessionDisplay(p.ID)
	}),
	proto.MethodSessionBudget: with(func(c *conn, p proto.SessionBudgetParams) (any, error) {
		sess, err := c.srv.sessions.Get(p.ID)
		if err != nil {
			return nil, err
		}
		c.srv.sessions.SetBudget(sess, time.Duration(p.Seconds)*time.Second)
		c.srv.persist.update(sess, nil)
		return ok, nil
	}),
	proto.MethodTunnelOpen: with(func(c *conn, p proto.TunnelOpenParams) (any, error) {
		id, err := c.openTunnel(p)
		if err != nil {
			return nil, err
		}
		return proto.TunnelInfo{Stream: id}, nil
	}),
	proto.MethodSessionPorts: with(func(c *conn, p proto.SessionRef) (any, error) {
		return c.srv.ports(p.ID)
	}),
	proto.MethodSessionClear: with(func(c *conn, p proto.SessionRef) (any, error) {
		sess, err := c.srv.sessions.Get(p.ID)
		if err != nil {
			return nil, err
		}
		sess.Terminal().ClearScreen()
		return ok, nil
	}),
	proto.MethodSessionKill: with(func(c *conn, p proto.SessionRef) (any, error) {
		if err := c.srv.deleteSession(p.ID); err != nil {
			return nil, err
		}
		c.srv.broadcast(proto.EventSessionRemoved, p)
		return ok, nil
	}),
}

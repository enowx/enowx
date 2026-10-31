package daemon

import (
	"path/filepath"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/tokens"
	"github.com/enowx/enowx/proto"
)

// tokenHandlers are token usage's methods (daemon/tokens.go).
var tokenHandlers = map[string]handler{
	proto.MethodTokensSettings:    bare(func(c *conn) (any, error) { return c.srv.tok.settings.Get(), nil }),
	proto.MethodTokensSetSettings: with(func(c *conn, p proto.TokensSettings) (any, error) { return c.srv.setTokenSettings(p) }),
	proto.MethodTokensSummary:     with(func(c *conn, p proto.TokensQuery) (any, error) { return c.srv.tokenSummary(p), nil }),
	proto.MethodTokensSession: with(func(c *conn, p proto.SessionRef) (any, error) {
		return c.srv.tokenSummary(proto.TokensQuery{Session: p.ID, By: proto.TokensByModel, Since: time.Now().AddDate(0, 0, -tokens.KeepDays)}), nil
	}),
	proto.MethodTokensSessions: bare(func(c *conn) (any, error) {
		return c.srv.tok.store.SessionTotals(time.Now().AddDate(0, 0, -60), c.srv.prices()), nil
	}),
	proto.MethodTokensCoverage: bare(func(c *conn) (any, error) { return tokenCoverage(), nil }),
	proto.MethodTokensBackfill: with(func(c *conn, p proto.TokensBackfill) (any, error) { return c.srv.backfillTokens(p.Since), nil }),
	proto.MethodTokensPrices:   bare(func(c *conn) (any, error) { return c.srv.prices().All(), nil }),
	proto.MethodTokensHosts:    bare(func(c *conn) (any, error) { return c.srv.hostDays(), nil }),
	proto.MethodTokensBudgets: bare(func(c *conn) (any, error) {
		st := c.srv.tok.settings.Get()
		return c.srv.tok.store.Budgets(st.Budgets, tokens.NewPrices(st.Prices), time.Now()), nil
	}),
}

// setTokenSettings keeps the settings; turning tracking on reads back the
// records already on disk.
func (s *Server) setTokenSettings(p proto.TokensSettings) (proto.TokensSettings, error) {
	was := s.tok.settings.Get()
	st, err := s.tok.settings.Set(p)
	if err != nil {
		return st, err
	}
	if st.On && !was.On {
		go s.backfillTokens(time.Time{})
	}
	return s.tok.settings.Get(), nil
}

// backfillTokens reads every record back to since (zero: the settings'
// backfill days); the store counts each call once.
func (s *Server) backfillTokens(since time.Time) proto.TokensBackfillResult {
	if !s.tokensOn() {
		return proto.TokensBackfillResult{}
	}
	if since.IsZero() {
		since = time.Now().AddDate(0, 0, -s.tok.settings.Get().BackfillDays)
	}
	s.tok.scanner.Reset()
	added, files := s.scanTokens(since)
	return proto.TokensBackfillResult{Calls: len(added), Files: files}
}

// tokenSummary is the host's summary as q asks, its groups labelled.
func (s *Server) tokenSummary(q proto.TokensQuery) proto.TokensSummary {
	st := s.tok.settings.Get()
	prices := tokens.NewPrices(st.Prices)
	out := s.tok.store.Summarize(q, prices, time.Now())
	out.On, out.Host = st.On, s.userdata.Name
	out.HostID, _ = s.userdata.ID()
	for _, b := range s.tok.store.Budgets(st.Budgets, prices, time.Now()) {
		if b.Level >= 80 {
			out.Budgets = append(out.Budgets, b)
		}
	}
	for i, g := range out.Groups {
		out.Groups[i].Label = s.tokenLabel(q.By, g.Key)
	}
	return out
}

// tokenLabel is how a group's key reads: an agent's name, a session's,
// a project's folder name.
func (s *Server) tokenLabel(by, key string) string {
	switch {
	case key == "":
		switch by {
		case proto.TokensBySession:
			return "Outside enowX"
		case proto.TokensByProject:
			return "No project"
		case proto.TokensByConfig:
			return "No config"
		}
		return "Unknown"
	case by == proto.TokensByAgent:
		if p, ok := agent.Find(key); ok {
			return p.Name
		}
	case by == proto.TokensBySession:
		if sess, err := s.sessions.Get(key); err == nil {
			info := sess.Info()
			if info.Name != "" {
				return info.Name
			}
			if info.Title != "" {
				return info.Title
			}
		}
	case by == proto.TokensByProject:
		return filepath.Base(key)
	}
	return key
}

// tokenCoverage is every agent's layer, with its name.
func tokenCoverage() []proto.TokenCoverage {
	var ids []string
	for _, p := range agent.Presets {
		ids = append(ids, p.ID)
	}
	list := tokens.Coverage(ids)
	for i, c := range list {
		if p, ok := agent.Find(c.Agent); ok {
			list[i].Name = p.Name
		}
	}
	return list
}

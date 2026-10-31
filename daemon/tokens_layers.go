package daemon

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/tokens"
	"github.com/enowx/enowx/proto"
)

// The layers past the agents' own records, enowX's own use, and what a
// budget at its limit does to new sessions.

// tokenTrafficEnv is what a new session of agentID gets when its tokens
// are metered from its traffic (tracking and metering on, an agent that
// keeps no record): the interceptor as its proxy, its source kept with
// the targets so it comes back with the daemon.
func (s *Server) tokenTrafficEnv(id, agentID, project, label string) []string {
	if _, isAgent := agent.Find(agentID); !isAgent || tokens.LayerOf(agentID) != proto.TokenLayerTraffic {
		return nil
	}
	if st := s.tok.settings.Get(); !st.On || !st.Traffic {
		return nil
	}
	t, err := s.trafficOn()
	if err != nil {
		s.logger().Warn("tokens: the interceptor didn't start", "err", err)
		return nil
	}
	src := t.proxy.AddSource(proto.TargetSession, cmp.Or(label, "Tokens"), id, project)
	port, err := t.proxy.SourcePort(src, 0)
	if err != nil {
		t.proxy.RemoveSource(src.ID)
		return nil
	}
	tg := &trafficTarget{Src: src, Port: port, Agent: true, TrafficTarget: proto.TrafficTarget{
		Source: src.ID, Kind: proto.TargetSession, Label: src.Label, Session: id,
		Proxy: "http://127.0.0.1:" + strconv.Itoa(port), Attached: time.Now(),
		Note: "Its agent's model API answers are metered for its tokens (Resource usage → Tokens).",
	}}
	t.mu.Lock()
	t.targets[src.ID] = tg
	t.mu.Unlock()
	s.targetsChanged()
	return s.trafficEnvOf(tg)
}

// meterExchange reads a finished exchange of a session's agent with a
// model API for its usage (only the counts).
func (s *Server) meterExchange(sum proto.TrafficSummary) {
	if sum.State != proto.TrafficDone || sum.Session == "" || sum.Status >= 400 {
		return
	}
	provider, ok := tokens.ModelAPI(sum.Host, sum.Path)
	if !ok || !s.tokensOn() {
		return
	}
	go func() {
		ex, err := s.trafficExchange(sum.ID)
		if err != nil {
			return
		}
		body := ex.Body("response", true, true, 0, 64<<20)
		c, ok := tokens.Meter(provider, body.Data)
		if !ok {
			return
		}
		if c.ID == "" {
			c.ID = "traffic:" + sum.Session + ":" + strconv.FormatInt(sum.Started.UnixNano(), 36)
		}
		c.Time, c.Session = sum.Started, sum.Session
		if sess, err := s.sessions.Get(sum.Session); err == nil {
			c.Agent = sess.Info().Agent
		}
		s.keepCalls([]tokens.Call{c})
		s.afterScan()
	}()
}

// screenCalls reads the screens of running sessions whose agents only
// show their tokens: those without records whose traffic isn't metered
// (the screen is read for the lines agents print their counts in).
func (s *Server) screenCalls() []tokens.Call {
	var out []tokens.Call
	now := time.Now()
	metered := s.meteredSessions()
	for _, sess := range s.sessions.List() {
		info := sess.Info()
		if _, isAgent := agent.Find(info.Agent); info.Exited || !isAgent || tokens.LayerOf(info.Agent) == proto.TokenLayerRecords || metered[info.ID] {
			continue
		}
		for _, c := range s.tok.screen.Read(info.ID, sess.Terminal().Text(), now) {
			c.Agent = info.Agent
			out = append(out, c)
		}
	}
	return out
}

// judgeTokens keeps a decision's tokens as the judge's, enowX's own.
func (s *Server) judgeTokens(d proto.Decision) {
	if d.Tokens <= 0 || d.Judge == "" {
		return
	}
	// A judge's ID is kind:agent-or-credential:model; its tokens come
	// as one count, kept as input.
	parts := strings.SplitN(d.Judge, ":", 3)
	c := tokens.Call{ID: "judge:" + d.ID, Time: d.Time, Session: d.Session, Project: d.Project, Own: proto.TokensOwnJudge,
		Input: int64(d.Tokens), Cost: d.Cost}
	if len(parts) == 3 {
		c.Model = parts[2]
		if parts[0] == proto.JudgeLocal {
			c.Provider = "local"
		}
	}
	s.recordOwn(c)
}

// ragTokens keeps an index's embedding tokens as RAG's, enowX's own.
func (s *Server) ragTokens(dir string, n int64) {
	if n <= 0 {
		return
	}
	st := s.ragSettings.Get()
	now := time.Now()
	s.recordOwn(tokens.Call{ID: "rag:" + dir + ":" + strconv.FormatInt(now.UnixNano(), 36), Time: now, Project: dir,
		Own: proto.TokensOwnRAG, Provider: st.Embedder, Model: st.Model, Input: n})
}

// tokenGate refuses a new agent session while a budget it falls under is
// at its limit and stops new ones, or asks first (the app asks the user
// and sends TokenBudgetOK).
func (s *Server) tokenGate(p proto.SessionCreateParams, project string) error {
	if _, isAgent := agent.Find(p.Agent); !isAgent || p.SignIn || p.Parent != "" {
		return nil
	}
	st := s.tok.settings.Get()
	if !st.On || len(st.Budgets) == 0 {
		return nil
	}
	action, b := tokens.AtLimit(s.tok.store.Budgets(st.Budgets, tokens.NewPrices(st.Prices), time.Now()), p.Agent, project, "")
	switch {
	case action == proto.BudgetStop:
		return fmt.Errorf("%s %s; new agent sessions are stopped until it resets, or raise it in Resource usage → Tokens", proto.TokenBudgetStops, budgetName(b))
	case action == proto.BudgetAsk && !p.TokenBudgetOK:
		return fmt.Errorf("%s %s", proto.TokenBudgetAsks, budgetName(b))
	}
	return nil
}

// budgetName says a budget in words: "the day's budget for claude (100%)".
func budgetName(b proto.TokenBudgetState) string {
	what := "everything"
	if b.Scope != proto.BudgetAll {
		what = b.Scope + " " + b.Target
	}
	return fmt.Sprintf("the %s budget for %s is at %d%%", b.Period, what, int(b.Fraction*100))
}

// meteredSessions are the sessions whose agent's traffic goes through
// the interceptor.
func (s *Server) meteredSessions() map[string]bool {
	t := &s.traf
	t.mu.Lock()
	defer t.mu.Unlock()
	out := map[string]bool{}
	for _, tg := range t.targets {
		if tg.Agent && !tg.Src.Quiet {
			out[tg.Session] = true
		}
	}
	return out
}

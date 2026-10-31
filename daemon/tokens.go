package daemon

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/tokens"
	"github.com/enowx/enowx/proto"
)

// Token usage on this host (core/tokens): while tracking is on, the
// agents' records are read every scanEvery and when a session's turn
// ends, each call tied to its session, project and account, kept a file
// a day; budgets are checked after each scan, and the host's daily
// totals written to the synced settings for the hosts the app doesn't
// reach. Nothing leaves the host but what the apps ask for. See
// docs/tokens.md.

// scanEvery is how often the agents' records are read while tracking.
const scanEvery = 20 * time.Second

// hostDaysEvery is how often the host's synced daily totals are written.
const hostDaysEvery = 30 * time.Minute

// hostDaysKept is how many days of totals a host syncs.
const hostDaysKept = 90

// tokenState is the daemon's tracking. The store, scanner, settings,
// screen and alerts are safe for use by several goroutines; scan runs
// on one at a time (scanMu); the rest belongs to mu.
type tokenState struct {
	store    *tokens.Store
	scanner  *tokens.Scanner
	settings *tokens.Settings
	screen   tokens.Screen
	alerts   tokens.Alerts
	scanMu   sync.Mutex
	poke     chan struct{}

	mu       sync.Mutex
	hostDays time.Time // when the synced totals were last written
	rolledUp string    // the day old days were last rolled up
}

func (s *Server) newTokens(dir string) {
	t := &s.tok
	t.store = tokens.Open(filepath.Join(dir, "tokens"))
	t.scanner = tokens.NewScanner(filepath.Join(dir, "tokens", "scan.json"))
	t.settings = tokens.OpenSettings(dir)
	t.poke = make(chan struct{}, 1)
	// The judge's tokens are enowX's own; near a budget it asks the
	// cheapest judges first.
	s.judge.OnRecord = s.judgeTokens
	s.judge.Cheaper = func() bool {
		st := t.settings.Get()
		return st.On && len(st.Budgets) > 0 && tokens.Near(t.store.Budgets(st.Budgets, tokens.NewPrices(st.Prices), time.Now()))
	}
}

// tokensOn reports whether tracking is on.
func (s *Server) tokensOn() bool { return s.tok.settings.Get().On }

// prices is the price table the user's settings make.
func (s *Server) prices() *tokens.Prices { return tokens.NewPrices(s.tok.settings.Get().Prices) }

// pokeTokens asks for a scan soon: a turn ended.
func (s *Server) pokeTokens() {
	select {
	case s.tok.poke <- struct{}{}:
	default:
	}
}

// runTokens scans while the daemon runs.
func (s *Server) runTokens(done <-chan struct{}) {
	tick := time.NewTicker(scanEvery)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
		case <-s.tok.poke:
			time.Sleep(2 * time.Second) // the agent finishes writing
		}
		if s.tokensOn() {
			s.scanTokens(time.Time{})
		}
	}
}

// scanTokens reads what changed in the agents' records (back to since
// for a backfill, else the settings' backfill days) and the screens of
// agents that only show their tokens, keeps the new calls, and tells.
func (s *Server) scanTokens(since time.Time) (added []tokens.Call, files int) {
	t := &s.tok
	t.scanMu.Lock()
	defer t.scanMu.Unlock()
	st := t.settings.Get()
	if since.IsZero() {
		since = time.Now().AddDate(0, 0, -st.BackfillDays)
	}
	home, _ := os.UserHomeDir()
	calls, files := t.scanner.Scan(tokens.Env{Home: home, Dirs: s.tokenDirs()}, since, nil)
	if st.Screen {
		calls = append(calls, s.screenCalls()...)
	}
	added = s.keepCalls(calls)
	s.afterScan()
	return added, files
}

// tokenDirs are the folders of the host's projects and sessions.
func (s *Server) tokenDirs() []string {
	var dirs []string
	for _, p := range s.projects.List() {
		dirs = append(dirs, p.Path)
	}
	for _, sess := range s.sessions.List() {
		if d := sess.Info().Dir; d != "" && !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// keepCalls ties calls to their sessions and keeps those new, telling the
// apps of the sessions whose use grew.
func (s *Server) keepCalls(calls []tokens.Call) []tokens.Call {
	if len(calls) == 0 {
		return nil
	}
	sessions := s.sessions.List()
	infos := make([]proto.SessionInfo, 0, len(sessions))
	for _, sess := range sessions {
		infos = append(infos, sess.Info())
	}
	projects := s.projects.List()
	for i := range calls {
		s.attribute(&calls[i], infos, projects)
	}
	added, err := s.tok.store.Add(calls)
	if err != nil {
		s.logger().Warn("tokens", "err", err)
	}
	changed := map[string]bool{}
	for _, c := range added {
		if c.Session != "" && c.Own == "" {
			changed[c.Session] = true
		}
	}
	if len(changed) > 0 {
		var list []proto.TokenSessionTotal
		for _, t := range s.tok.store.SessionTotals(time.Now().AddDate(0, 0, -60), s.prices()) {
			if changed[t.Session] {
				list = append(list, t)
			}
		}
		s.broadcast(proto.EventTokensChanged, proto.TokensChanged{Sessions: list})
	}
	return added
}

// attribute ties call c to the session that made it (the same agent's,
// run in its folder, started before it; the latest such), its project
// and account; a call made outside enowX gets the project its folder is
// in.
func (s *Server) attribute(c *tokens.Call, sessions []proto.SessionInfo, projects []proto.ProjectInfo) {
	if c.Session == "" && c.Dir != "" {
		dir := samePath(c.Dir)
		var best *proto.SessionInfo
		for i, info := range sessions {
			if agent.Base(info.Agent) != agent.Base(c.Agent) || samePath(info.Dir) != dir || info.Created.After(c.Time.Add(time.Minute)) {
				continue
			}
			if best == nil || info.Created.After(best.Created) {
				best = &sessions[i]
			}
		}
		if best != nil {
			c.Session = best.ID
		}
	}
	if c.Session != "" {
		if i := slices.IndexFunc(sessions, func(info proto.SessionInfo) bool { return info.ID == c.Session }); i >= 0 {
			info := sessions[i]
			c.Agent = info.Agent // Claude Agent Teams' calls are Claude Code's records
			if c.Project == "" {
				c.Project = sessionProject(info)
			}
			if c.Account == "" {
				c.Account = s.accountName(info.Account)
			}
		}
	}
	if c.Project == "" && c.Dir != "" {
		c.Project = projectOf(samePath(c.Dir), projects)
	}
	if !c.Plan && c.Own == "" && planAgent(c.Agent) && agentAPIOf(agent.Base(c.Agent)) == nil {
		c.Plan = true
	}
}

// planAgents are agents usually signed in to a subscription, whose cost
// is then API-equivalent (unless pointed at a model API of the user's).
var planAgents = []string{"claude", "codex", "copilot", "cursor", "kiro", "qoder", "qoder-cn", "trae", "codebuddy", "aug", "antigravity", "droid", "devin", "rovo"}

func planAgent(id string) bool { return slices.Contains(planAgents, agent.Base(id)) }

// projectOf is the project folder dir is in, "" for none.
func projectOf(dir string, projects []proto.ProjectInfo) string {
	best := ""
	for _, p := range projects {
		pp := samePath(p.Path)
		if (dir == pp || strings.HasPrefix(dir, pp+string(filepath.Separator))) && len(pp) > len(best) {
			best = p.Path
		}
	}
	return best
}

// samePath is p as a comparable path: cleaned, links resolved (macOS's
// /tmp is /private/tmp).
func samePath(p string) string {
	if p == "" {
		return ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// recordOwn keeps a call of enowX's own (the judge's, RAG's), apart from
// the agents'.
func (s *Server) recordOwn(c tokens.Call) {
	if !s.tokensOn() || c.Empty() {
		return
	}
	if c.Layer == "" {
		c.Layer = proto.TokenLayerRecords
	}
	if c.Provider == "" {
		c.Provider = tokens.ProviderOf(c.Model)
	}
	if _, err := s.tok.store.Add([]tokens.Call{c}); err != nil {
		s.logger().Warn("tokens", "err", err)
	}
}

// afterScan checks the budgets, rolls up old days once a day, and writes
// the host's synced totals now and then.
func (s *Server) afterScan() {
	now := time.Now()
	st := s.tok.settings.Get()
	prices := tokens.NewPrices(st.Prices)
	if len(st.Budgets) > 0 {
		for _, b := range s.tok.alerts.Crossed(s.tok.store.Budgets(st.Budgets, prices, now)) {
			s.logger().Info("token budget", "id", b.ID, "level", b.Level)
			s.broadcast(proto.EventTokensBudget, b)
		}
	}
	t := &s.tok
	t.mu.Lock()
	today := now.Format(time.DateOnly)
	rollup := t.rolledUp != today
	t.rolledUp = today
	writeDays := now.Sub(t.hostDays) > hostDaysEvery
	if writeDays {
		t.hostDays = now
	}
	t.mu.Unlock()
	if rollup {
		if err := t.store.Rollup(now, prices); err != nil {
			s.logger().Warn("tokens: roll up", "err", err)
		}
	}
	if writeDays {
		s.writeHostDays(now, prices)
	}
}

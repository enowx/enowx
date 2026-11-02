package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/accounts"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
)

// An agent's account hitting its usage limit, running out of credits or
// signed out, as its session's screen says: noted, logged, and by the
// account preferences the session switched to the next account (resuming
// its conversation, told to go on) or another agent, or the user asked
// first; and once the first account's limit reset, back to it. See
// docs/accounts.md.

// limitLook is how often a session's screen is read for a limit, at most.
const limitLook = 3 * time.Second

// spentFor is how long a limit that doesn't say when it resets rules its
// account out; credits and sign-ins until the user switches back to it.
const (
	spentLimit = time.Hour
	spentOther = 24 * time.Hour
)

// accountWatch is what the daemon keeps of accounts' limits. mu guards it.
type accountWatch struct {
	mu      sync.Mutex
	file    string
	limits  map[string]limitRecord // by agent + "\x00" + account name
	looked  map[string]time.Time   // sessions, when last read
	handled map[string]string      // sessions, the line last acted on
	// settling and replayed: see accounts_replay.go.
	settling map[string]bool
	replayed map[string]int
	log      []proto.AccountLogEntry
}

func newAccountWatch(dir string) *accountWatch {
	w := &accountWatch{file: filepath.Join(dir, "account-limits.json"), limits: map[string]limitRecord{},
		looked: map[string]time.Time{}, handled: map[string]string{}, settling: map[string]bool{}, replayed: map[string]int{}}
	if data, err := os.ReadFile(w.file); err == nil {
		_ = json.Unmarshal(data, &w.limits)
	}
	if data, err := os.ReadFile(w.logFile()); err == nil {
		for line := range strings.SplitSeq(string(data), "\n") {
			var e proto.AccountLogEntry
			if json.Unmarshal([]byte(line), &e) == nil {
				w.log = append(w.log, e)
			}
		}
	}
	return w
}

func (w *accountWatch) logFile() string {
	return strings.TrimSuffix(w.file, "account-limits.json") + "account-log.jsonl"
}

// limitRecord is a limit an account hit, and when.
type limitRecord struct {
	Reason string    `json:"reason"`
	Line   string    `json:"line,omitempty"`
	Until  time.Time `json:"until,omitzero"`
	At     time.Time `json:"at"`
}

func limitKey(agentID, name string) string { return agentID + "\x00" + strings.ToLower(name) }

// note keeps the limit an account hit. w.mu is held.
func (w *accountWatch) note(agentID, name string, l accounts.Limit) {
	w.limits[limitKey(agentID, name)] = limitRecord{Reason: l.Reason, Line: l.Line, Until: l.Until, At: time.Now()}
	if data, err := json.Marshal(w.limits); err == nil {
		_ = envsync.WriteFileAtomic(w.file, data, 0o600)
	}
}

// spent says account name of agentID can't be used now, and the limit.
func (w *accountWatch) spent(agentID, name string, now time.Time) (limitRecord, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	r, ok := w.limits[limitKey(agentID, name)]
	if !ok {
		return r, false
	}
	switch {
	case !r.Until.IsZero():
		return r, r.Until.After(now)
	case r.Reason == proto.AccountLimit:
		return r, now.Sub(r.At) < spentLimit
	}
	return r, now.Sub(r.At) < spentOther
}

// clear forgets account name's limit: it works again.
func (w *accountWatch) clear(agentID, name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.limits, limitKey(agentID, name))
	if data, err := json.Marshal(w.limits); err == nil {
		_ = envsync.WriteFileAtomic(w.file, data, 0o600)
	}
}

// record logs an account used.
func (s *Server) recordAccount(e proto.AccountLogEntry) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	w := s.acctWatch
	w.mu.Lock()
	w.log = append(w.log, e)
	if len(w.log) > 500 {
		w.log = slices.Clone(w.log[len(w.log)-500:])
	}
	w.mu.Unlock()
	if data, err := json.Marshal(e); err == nil {
		if f, err := os.OpenFile(w.logFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			_, _ = f.Write(append(data, '\n'))
			f.Close()
		}
	}
}

// accountLog is the accounts used, the latest first.
func (s *Server) accountLog() []proto.AccountLogEntry {
	s.acctWatch.mu.Lock()
	defer s.acctWatch.mu.Unlock()
	out := slices.Clone(s.acctWatch.log)
	slices.Reverse(out)
	return out
}

// accountName is the name of agentID's account id, "" its own.
func (s *Server) accountName(id string) string {
	if a, ok := s.accounts.Get(id); ok {
		return a.Name
	}
	return ""
}

// accountNames are agentID's accounts' names, its own left out.
func (s *Server) accountNames(agentID string) []string {
	var out []string
	for _, a := range s.accounts.List() {
		if a.Agent == agentID && a.Member == "" {
			out = append(out, a.Name)
		}
	}
	return out
}

// fallsBack says session info's agent has somewhere to fall back to.
func (s *Server) fallsBack(agentID string) bool {
	pr := s.accountPrefs.Of(agentID)
	return len(s.accountNames(agentID)) > 0 || pr.ToAgent != "" || pr.Fallback != proto.FallbackOff
}

// watchLimitsLoop reads the sessions' screens for limits now and then,
// until done closes: a screen going quiet tells nothing of itself.
func (s *Server) watchLimitsLoop(done <-chan struct{}) {
	t := time.NewTicker(limitLook)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			for _, sess := range s.sessions.List() {
				s.watchLimits(sess.Info())
			}
		}
	}
}

// watchLimits reads session info's screen for a limit its agent says it
// hit, when it stopped working.
func (s *Server) watchLimits(info proto.SessionInfo) {
	base := agent.Base(info.Agent)
	if !userSession(info) || info.Exited || info.Agent == "" || info.State == proto.StateWorking || info.State == "" && info.Active || !s.fallsBack(base) {
		return
	}
	w := s.acctWatch
	now := time.Now()
	w.mu.Lock()
	if now.Sub(w.looked[info.ID]) < limitLook-500*time.Millisecond {
		w.mu.Unlock()
		return
	}
	w.looked[info.ID] = now
	w.mu.Unlock()
	sess, err := s.sessions.Get(info.ID)
	if err != nil {
		return
	}
	text, ok := w.fresh(info.ID, sess.Terminal().Text())
	if !ok {
		return
	}
	l, ok := accounts.Classify(text, now)
	if !ok {
		return
	}
	l.Line = s.maskLent(l.Line)
	w.mu.Lock()
	if w.handled[info.ID] == l.Line {
		w.mu.Unlock()
		return
	}
	w.handled[info.ID] = l.Line
	w.note(base, s.accountName(info.Account), l)
	w.mu.Unlock()
	go s.limitReached(info, l)
	go s.goaLimitHit(info, l.Line)
}

// limitReached acts on session info's account hitting limit l, by its
// agent's preferences.
func (s *Server) limitReached(info proto.SessionInfo, l accounts.Limit) {
	base := agent.Base(info.Agent)
	from := s.accountName(info.Account)
	pr := s.accountPrefs.Of(base)
	s.recordAccount(proto.AccountLogEntry{Session: info.ID, Agent: base, Account: from, Event: "limit", Reason: l.Reason + ": " + l.Line})
	fb := proto.AccountFallback{Session: info.ID, Agent: base, From: from, Reason: l.Reason, Line: l.Line, Until: l.Until}
	now := time.Now()
	next, ok := accounts.Next(pr, s.accountNames(base), from, info.Project, func(n string) bool { _, spent := s.acctWatch.spent(base, n, now); return spent })
	switch {
	case ok:
		fb.To = next
		if a, found := s.accounts.ByName(base, next); found {
			fb.ToID = a.ID
		}
	case pr.ToAgent != "":
		fb.ToAgent = pr.ToAgent
	}
	switch {
	case pr.Fallback == proto.FallbackOff || fb.To == "" && fb.ToID == "" && fb.ToAgent == "" && !ok:
		s.broadcast(proto.EventAccountFallback, fb)
		s.broadcast(proto.EventNotifySound, proto.NotifySoundParams{Kind: proto.NotifyError, Session: info.ID, Text: accountLabel(base, from) + " " + limitWords(l.Reason)})
	case pr.Fallback == proto.FallbackAsk:
		fb.Ask = true
		s.broadcast(proto.EventAccountFallback, fb)
		s.broadcast(proto.EventNotifySound, proto.NotifySoundParams{Kind: proto.NotifyInput, Session: info.ID, Text: accountLabel(base, from) + " " + limitWords(l.Reason) + ": switch?"})
	default:
		_, err := s.switchAccount(proto.AccountSwitchParams{Session: info.ID, Account: fb.ToID, Agent: fb.ToAgent, Continue: true, Reason: "fallback: " + l.Reason})
		if err != nil {
			s.logger().Warn("could not fall back to another account", "session", info.ID, "err", err)
			return
		}
		fb.Switched = true
		s.broadcast(proto.EventAccountFallback, fb)
		to := accountLabel(base, fb.To)
		if fb.ToAgent != "" {
			to = agentName(fb.ToAgent)
		}
		s.broadcast(proto.EventNotifySound, proto.NotifySoundParams{Kind: proto.NotifyInput, Session: info.ID, Text: accountLabel(base, from) + " " + limitWords(l.Reason) + ": went on with " + to})
		s.fireHooks(info, proto.HookEvent{Event: proto.HookFallback, Text: from + " → " + to, Kind: l.Reason, Via: proto.HookOutside})
	}
}

func limitWords(reason string) string {
	switch reason {
	case proto.AccountCredits:
		return "ran out of credits"
	case proto.AccountAuth:
		return "was signed out"
	}
	return "hit its usage limit"
}

func agentName(id string) string {
	if p, ok := agent.Find(id); ok {
		return p.Name
	}
	return id
}

// accountLabel is "Claude Code (Work)", its own account bare.
func accountLabel(agentID, name string) string {
	if name == "" {
		return agentName(agentID)
	}
	return agentName(agentID) + " (" + name + ")"
}

// switchAccount has session p.Session go on as account p.Account (its
// agent's own for ""), or as agent p.Agent: its program stopped, started
// again on its conversation (a new agent on a summary of it), told to go
// on when p.Continue.
func (s *Server) switchAccount(p proto.AccountSwitchParams) (proto.SessionInfo, error) {
	sess, err := s.sessions.Get(p.Session)
	if err != nil {
		return proto.SessionInfo{}, err
	}
	info, meta := sess.Info(), sess.Meta()
	if p.Agent != "" && agent.Base(p.Agent) != agent.Base(meta.Agent) {
		return s.switchAgent(p)
	}
	base := agent.Base(meta.Agent)
	var acc proto.Account
	if p.Account != "" {
		a, ok := s.accounts.Get(p.Account)
		if !ok {
			return proto.SessionInfo{}, fmt.Errorf("accounts: no account %s", p.Account)
		}
		if a.Agent != base {
			return proto.SessionInfo{}, fmt.Errorf("accounts: %s is an account of %s", a.Name, a.Agent)
		}
		acc = a
	}
	if !accounts.Allowed(s.accountPrefs.Of(base), acc.Name, meta.Project) {
		return proto.SessionInfo{}, fmt.Errorf("accounts: %s is kept to other projects", acc.Name)
	}
	if !meta.Relaunch {
		return proto.SessionInfo{}, errors.New("this session's command can't be started again")
	}
	if meta.AccountFirst == "" && meta.Account != p.Account {
		meta.AccountFirst = "=" + s.accountName(meta.Account) // "=": its own is a first too
	}
	// A key's credential goes with its account.
	if old, ok := s.accounts.Get(meta.Account); ok && old.Cred != "" {
		meta.Creds = slices.DeleteFunc(meta.Creds, func(c string) bool { return c == old.Cred })
	}
	if acc.Cred != "" && !slices.Contains(meta.Creds, acc.Cred) {
		meta.Creds = append(meta.Creds, acc.Cred)
	}
	meta.Account = p.Account
	if strings.TrimPrefix(meta.AccountFirst, "=") == acc.Name && strings.HasPrefix(meta.AccountFirst, "=") {
		meta.AccountFirst = "" // back on the first
	}
	s.acctWatch.restarting(p.Session)
	if !info.Exited {
		if _, err := s.sessions.Replace(stoppedSpec(sess, meta)); err != nil {
			s.acctWatch.settled(p.Session, sess.Terminal().Text())
			return proto.SessionInfo{}, err
		}
		reapSession(info.ID)
	} else if _, err := s.sessions.Replace(stoppedSpec(sess, meta)); err != nil {
		s.acctWatch.settled(p.Session, sess.Terminal().Text())
		return proto.SessionInfo{}, err
	}
	next, err := s.resumeSession(p.Session, "")
	if err != nil {
		s.acctWatch.settled(p.Session, sess.Terminal().Text())
		return proto.SessionInfo{}, err
	}
	event := "switch"
	if strings.HasPrefix(p.Reason, "fallback") {
		event = "fallback"
	} else if strings.HasPrefix(p.Reason, "back") {
		event = "back"
	}
	s.recordAccount(proto.AccountLogEntry{Session: p.Session, Agent: base, Account: acc.Name, Event: event, Reason: p.Reason})
	prompt := ""
	if p.Continue {
		prompt = "Go on with what you were doing: the account you ran on hit its limit, and you are now on another."
	}
	go s.tellToGoOn(next.ID(), prompt)
	return next.Info(), nil
}

// tellToGoOn waits for session id's agent to show its input, then has its
// screen counted (accounts_replay.go) and prompt, if any, typed.
func (s *Server) tellToGoOn(id, prompt string) {
	deadline := time.Now().Add(promptTimeout)
	quietSince := time.Time{}
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		sess, err := s.sessions.Get(id)
		if err != nil {
			s.acctWatch.settled(id, "")
			return
		}
		info := sess.Info()
		if info.Exited {
			s.acctWatch.settled(id, sess.Terminal().Text())
			return
		}
		if info.Active {
			quietSince = time.Time{}
			continue
		}
		if quietSince.IsZero() {
			quietSince = time.Now()
		}
		if time.Since(quietSince) >= promptQuiet {
			break
		}
	}
	sess, err := s.sessions.Get(id)
	if err != nil {
		s.acctWatch.settled(id, "")
		return
	}
	s.acctWatch.settled(id, sess.Terminal().Text())
	if prompt != "" {
		_ = s.input(proto.SessionInputParams{ID: id, Text: prompt, Paste: true, Enter: true})
	}
}

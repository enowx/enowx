package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/accounts"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/core/transcript"
	"github.com/enowx/enowx/proto"
)

// Changing what GOD of Agents runs on (docs/goa.md). Its session stays, ID and all, so its
// tabs, its board, its collection and its waiting lines stay with it, and its workers,
// sessions of their own, never notice. Another config restarts it on the same conversation;
// another agent on a new one, which it can't carry. All that can be checked is checked
// before it stops, and what fails after that puts it back as it was.

// goaSwitchSettle is how long a restarted agent is watched: one that can't take the
// conversation or can't start at all says so and ends within this.
var goaSwitchSettle = 5 * time.Second

// goaSwitchPlan is a change that can be made.
type goaSwitchPlan struct {
	preset       agent.Preset
	account      proto.Account // ID "": the official config
	next         session.Meta  // what it runs as after
	toAgent      bool
	same         bool
	conversation string // proto.GOAConversation*
	convo        string // the id to go on with, "" none or the agent's own last
	note         string
}

func (s *Server) goaSwitch(p proto.GOASwitchParams) (proto.GOASwitchResult, error) {
	g, ok := s.goaSession()
	if !ok {
		return proto.GOASwitchResult{}, errors.New("goa: no GOD of Agents on this host: start it first (goa start)")
	}
	if p.From == g.ID && !p.Check {
		return proto.GOASwitchResult{}, errors.New("goa: GOD of Agents can't change its own agent or config, as that ends the turn it asks in: tell the user to change it in Settings in GOA's header, or with `goa switch` in a terminal")
	}
	sess, err := s.sessions.Get(g.ID)
	if err != nil {
		return proto.GOASwitchResult{}, err
	}
	plan, err := s.goaPlanSwitch(sess, p)
	if err != nil {
		return proto.GOASwitchResult{}, err
	}
	res := goaSwitchResultOf(g, plan)
	if p.Check || plan.same {
		return res, nil
	}
	if res.AtWork && !p.Force || plan.toAgent && !p.NewConversation {
		return res, nil // not a word of it done: the yes it needs first
	}
	if err := s.goaKeyReadable(g.ID, plan.account); err != nil {
		return proto.GOASwitchResult{}, fmt.Errorf("goa: GOD of Agents keeps running as it was: %w", err)
	}
	if !s.goaHoldLines() {
		return proto.GOASwitchResult{}, errors.New("goa: GOD of Agents is being changed already")
	}
	defer s.goaReleaseLines()
	started, err := s.goaRunSwitch(sess, plan)
	if err != nil {
		return proto.GOASwitchResult{}, err
	}
	res.Session, res.Restarted, res.Mode = started.Info(), true, started.Meta().Mode
	return res, nil
}

func goaSwitchResultOf(g proto.SessionInfo, plan goaSwitchPlan) proto.GOASwitchResult {
	level, _ := goa.LevelOf(plan.preset.ID)
	return proto.GOASwitchResult{Session: g, Same: plan.same, Agent: plan.preset.ID, Level: level, Config: plan.account.Name,
		AtWork:       !g.Exited && (g.State == proto.StateWorking || g.State == proto.StateWaiting),
		Conversation: plan.conversation, Mode: plan.next.Mode, Note: plan.note}
}

// goaHoldLines keeps what waits for GOA waiting while its agent is replaced: the hook
// waiting to wake it ends with nothing, rather than take lines to a process about to go.
func (s *Server) goaHoldLines() bool {
	s.goa.mu.Lock()
	if s.goa.switching {
		s.goa.mu.Unlock()
		return false
	}
	s.goa.switching = true
	w := s.goa.waker
	s.goa.waker = nil
	s.goa.mu.Unlock()
	if w != nil {
		w.text <- ""
	}
	return true
}

func (s *Server) goaReleaseLines() {
	s.goa.mu.Lock()
	s.goa.switching = false
	s.goa.mu.Unlock()
	go s.goaFlushIdle()
}

// goaPlanSwitch settles what p changes and says if it can't be done. It changes nothing,
// and asks the vault for nothing, as it is also what the app asks at each pick.
func (s *Server) goaPlanSwitch(sess *session.Session, p proto.GOASwitchParams) (goaSwitchPlan, error) {
	meta := sess.Meta()
	id := nonEmpty(p.Agent, meta.Agent)
	preset, err := s.goaAgentReady(id)
	if err != nil {
		return goaSwitchPlan{}, err
	}
	if !preset.Installed() {
		return goaSwitchPlan{}, fmt.Errorf("goa: %s isn't installed on this host", preset.Name)
	}
	if other := preset.NamesakeHere(); other != "" {
		return goaSwitchPlan{}, fmt.Errorf("goa: %s at %s isn't the agent, but another program with its name", preset.Name, other)
	}
	plan := goaSwitchPlan{preset: preset, toAgent: id != meta.Agent, conversation: proto.GOAConversationNew}
	if plan.account, err = s.goaConfigOf(id, p.Config, meta, plan.toAgent); err != nil {
		return goaSwitchPlan{}, err
	}
	plan.same = !plan.toAgent && plan.account.ID == meta.Account
	if err := s.goaConfigReady(plan.account); err != nil {
		return goaSwitchPlan{}, err
	}
	plan.next = goaNextMeta(meta, preset.ID, plan.account, s.accounts)
	plan.note = goaConfigNote(plan.account)
	if sess.QuickExits() >= quickExitsMax {
		return goaSwitchPlan{}, fmt.Errorf("goa: it ended right after starting %d times in a row: start it again first", quickExitsMax)
	}
	if plan.toAgent {
		return plan, nil
	}
	if !meta.Relaunch {
		return goaSwitchPlan{}, errors.New("goa: this GOD of Agents' command can't be started again")
	}
	plan.conversation, plan.convo, err = s.goaConversationFor(sess, plan.account)
	return plan, err
}

// goaConfigOf is the config asked for of agent id: by ID or name, the official one by that
// word; none asked, the one it runs on (the same agent) or the preferred (another).
func (s *Server) goaConfigOf(id, asked string, meta session.Meta, toAgent bool) (proto.Account, error) {
	base := agent.Base(id)
	switch {
	case asked == proto.GOAConfigOfficial || asked == "default":
		return proto.Account{}, nil
	case asked == "" && !toAgent:
		asked = meta.Account
	case asked == "":
		asked = s.defaultAccount(id, "")
	}
	if asked == "" {
		return proto.Account{}, nil
	}
	c, ok := s.accounts.Get(asked)
	if !ok {
		c, ok = s.accounts.ByName(base, asked)
	}
	if !ok || c.Agent != base || c.Member != "" {
		return proto.Account{}, fmt.Errorf("goa: %s has no config %s: `configs` lists them", agentName(id), asked)
	}
	if !accounts.Allowed(s.accountPrefs.Of(base), c.Name, "") {
		return proto.Account{}, fmt.Errorf("goa: %s is kept to some projects, and GOD of Agents is in none", c.Name)
	}
	return c, nil
}

// goaConfigReady says what keeps config c from starting, as far as can be known without
// asking the vault.
func (s *Server) goaConfigReady(c proto.Account) error {
	switch {
	case c.ID == "":
		return nil
	case c.Kind == proto.ConfigKindAPI:
		if c.API == nil {
			return fmt.Errorf("goa: %s has no model API set", c.Name)
		}
	case c.Cred == "" && !s.accounts.HasHome(c.ID):
		return fmt.Errorf("goa: %s isn't signed in on this host: sign it in first (Settings → Agents)", c.Name)
	}
	return nil
}

// goaKeyReadable reads config c's key once before GOA stops, so a vault that won't lend
// it (no app to answer) leaves it running. Asked as the session it is for.
func (s *Server) goaKeyReadable(id string, c proto.Account) error {
	if c.Kind != proto.ConfigKindAPI || c.API == nil || c.API.KeyCred == "" {
		return nil
	}
	_, err := s.configKey(id, c)
	return err
}

// goaNextMeta is meta as GOA runs on agent id with config c. Its credential goes with
// its config; the user chose this, so nothing later takes it back to a "first" config.
func goaNextMeta(meta session.Meta, id string, c proto.Account, store *accounts.Store) session.Meta {
	if old, ok := store.Get(meta.Account); ok && old.Cred != "" {
		meta.Creds = slices.DeleteFunc(slices.Clone(meta.Creds), func(v string) bool { return v == old.Cred })
	}
	if c.Cred != "" && !slices.Contains(meta.Creds, c.Cred) {
		meta.Creds = append(slices.Clone(meta.Creds), c.Cred)
	}
	meta.Agent, meta.Account, meta.AccountFirst = id, c.ID, ""
	return meta
}

// goaConfigNote says what GOA leaves of config c.
func goaConfigNote(c proto.Account) string {
	if len(goa.ConfigArgs(c.Args)) == len(c.Args) && len(goa.ConfigEnv(c.Env)) == len(c.Env) {
		return ""
	}
	return "GOD of Agents runs on this config without its flags other than model and effort, and without the variables that would loosen what holds it."
}

// goaConversationFor says how config c goes on with what sess is on: the same conversation,
// or none yet; an error if c can't reach it.
func (s *Server) goaConversationFor(sess *session.Session, c proto.Account) (how, convo string, err error) {
	meta := sess.Meta()
	if !transcript.Supports(meta.Agent) {
		return proto.GOAConversationSame, "", nil // it goes on with its folder's last by itself
	}
	path := s.transcriptPath(sess.ID())
	if path == "" {
		if meta.Conversation != "" {
			return "", "", fmt.Errorf("goa: its conversation %s can't be found, so no config can go on with it: GOD of Agents keeps running as it was", meta.Conversation)
		}
		return proto.GOAConversationNone, "", nil
	}
	if convo = transcript.ForkID(meta.Agent, path); convo == "" {
		return "", "", fmt.Errorf("goa: %s doesn't say which conversation it is on, so another config can't go on with it: GOD of Agents keeps running as it was", agentName(meta.Agent))
	}
	if err := s.goaConfigReaches(meta.Agent, c, path); err != nil {
		return "", "", err
	}
	return proto.GOAConversationSame, convo, nil
}

// goaConfigReaches says whether config c, in the home it runs with, sees the conversation
// at path: an account's home links the agent's, unless it has its own folder there.
func (s *Server) goaConfigReaches(id string, c proto.Account, path string) error {
	if c.ID == "" {
		return nil
	}
	base := agent.Base(id)
	env, err := s.accounts.Env(base, c.ID) // makes its links
	if err != nil {
		return fmt.Errorf("goa: %w", err)
	}
	name := accounts.HomeVar(base)
	home := ""
	for _, v := range append(env, c.Env...) {
		if value, ok := strings.CutPrefix(v, name+"="); ok && name != "" {
			home = value
		}
	}
	userHome, err := os.UserHomeDir()
	if home == "" || err != nil {
		return nil // the agent's own home, where it is
	}
	rel, err := filepath.Rel(transcript.HomeOf(id, userHome), path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil
	}
	if _, err := os.Stat(filepath.Join(home, rel)); err != nil {
		return fmt.Errorf("goa: %s keeps conversations apart and doesn't have this one, so it can't go on with it: GOD of Agents keeps running as it was", c.Name)
	}
	return nil
}

// goaRunSwitch stops GOA and starts it as plan says; failing, it starts it as it was.
func (s *Server) goaRunSwitch(sess *session.Session, plan goaSwitchPlan) (*session.Session, error) {
	id := sess.ID()
	before, wasStopped := sess.Meta(), sess.Info().Exited
	beforeConvo := ""
	if !wasStopped {
		_, beforeConvo, _ = s.goaConversationFor(sess, proto.Account{})
	}
	s.acctWatch.restarting(id)
	if err := s.goaStop(sess, plan.next); err != nil {
		s.acctWatch.settled(id, sess.Terminal().Text())
		return nil, fmt.Errorf("goa: GOD of Agents keeps running as it was: %w", err)
	}
	if plan.toAgent {
		s.transcripts.forget(id) // what it followed is the other agent's
	}
	next, err := s.goaStart(id, plan)
	if err == nil {
		err = s.goaStaysUp(id)
	}
	if err != nil {
		return nil, s.goaPutBack(id, before, beforeConvo, wasStopped, plan, err)
	}
	s.acctWatch.clear(agent.Base(plan.preset.ID), plan.account.Name) // picked: taken as working
	s.recordAccount(proto.AccountLogEntry{Session: id, Agent: agent.Base(plan.preset.ID), Account: plan.account.Name, Event: "switch", Reason: "GOD of Agents' settings (from " + accountLabel(before.Agent, s.accountName(before.Account)) + ")"})
	go s.tellToGoOn(id, "")
	s.goaChanged()
	return next, nil
}

// goaStop ends sess's program, its session stopped as meta. Its workers, sessions of their
// own, go on.
func (s *Server) goaStop(sess *session.Session, meta session.Meta) error {
	info := sess.Info()
	if !info.Exited {
		s.procsLook()
	}
	if _, err := s.sessions.Replace(stoppedSpec(sess, meta)); err != nil {
		return err
	}
	if !info.Exited {
		s.reapSession(info.ID)
	}
	return nil
}

// goaStart starts GOA, stopped and of plan's meta, on plan's agent and config.
func (s *Server) goaStart(id string, plan goaSwitchPlan) (*session.Session, error) {
	if plan.toAgent {
		return s.goaStartOnAgent(id, plan)
	}
	return s.resumeSession(id, plan.convo)
}

// goaStartOnAgent starts GOA on a new conversation of another agent, as `goa start` would.
func (s *Server) goaStartOnAgent(id string, plan goaSwitchPlan) (*session.Session, error) {
	sess, err := s.sessions.Get(id)
	if err != nil {
		return nil, err
	}
	info, meta := sess.Info(), plan.next
	p := proto.SessionCreateParams{Agent: plan.preset.ID, GOA: true, Account: plan.account.ID, SkipUnheard: meta.SkipUnheard}
	goaParams(&p, &meta)
	if p.SkipUnheard && len(agent.HookArgs(p.Agent)) == 0 {
		p.SkipPermissions = true
	}
	chat := s.goaChat(s.chatOf("", "", s.guideIn(meta, info.Dir)), meta)
	chat.store, chat.files = s.agentStore(p.Agent, id, true), s.agentFilesDir(id)
	run, err := s.configRunFor(id, meta)
	if err != nil {
		return nil, err
	}
	chat.config, meta.Config = run, run.sessionInfo()
	if meta.Config == nil {
		p = s.withAgentStart(p)
	}
	meta.Model, meta.Effort, meta.Mode = strings.TrimPrefix(p.Model, "default"), strings.TrimPrefix(p.Effort, "default"), p.Mode
	meta.ModelStarted, meta.ModelMoved = meta.Model, false
	l, err := launchFor(p, chat)
	if err != nil {
		return nil, err
	}
	meta.Conversation, meta.ConvoSince = "", time.Now()
	meta.Restart, meta.Relaunch, meta.SkipPermissions = l.restart, l.relaunch, l.skip
	meta.Restored, meta.Hibernated, meta.HibernatedByHand, meta.HibernatedAt = false, false, false, time.Time{}
	meta.Rules = s.rulesAtStart(meta, info.Dir, true)
	env, err := s.agentEnv(meta)
	if err != nil {
		return nil, err
	}
	spec := session.Spec{ID: id, Dir: info.Dir, Created: info.Created, History: sess.History(), Command: l.command, Meta: meta,
		Env: append(append(append(append(append(s.hostEnvFor(nil), env...), l.env...), s.vaultEnvFor(id, "", meta.Creds, meta.Agent)...), run.environ()...), s.tokenTrafficEnv(id, p.Agent, "", meta.Name)...)}
	next, err := s.sessions.Replace(spec)
	if err != nil {
		return nil, err
	}
	s.began(next, l.command, l.typed)
	return next, nil
}

// goaStaysUp watches a restarted agent long enough to see it take: one that can't resume
// or start ends, leaving the shell after it. What it can't tell yet (a key an API refuses
// at its first request) is its screen's to say.
func (s *Server) goaStaysUp(id string) error {
	deadline := time.Now().Add(goaSwitchSettle)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		sess, err := s.sessions.Get(id)
		if err != nil {
			return err
		}
		if info := sess.Info(); info.Exited || s.agentGone(id) {
			return fmt.Errorf("its agent ended right after starting: %s", lastScreenLine(sess.Terminal().Text()))
		}
	}
	return nil
}

// lastScreenLine is what a screen ends on, cut, and what reads as a key masked.
func lastScreenLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return clip(accounts.MaskSecrets(strings.TrimSpace(lines[len(lines)-1])), 200)
}

// goaPutBack starts GOA again as it was, after why kept the change from taking.
func (s *Server) goaPutBack(id string, before session.Meta, convo string, wasStopped bool, plan goaSwitchPlan, why error) error {
	defer s.acctWatch.settled(id, "")
	tried := accountLabel(plan.preset.ID, plan.account.Name)
	s.logger().Warn("goa: a change didn't take, going back", "session", id, "to", tried, "err", why)
	sess, err := s.sessions.Get(id)
	if err == nil {
		err = s.goaStop(sess, before)
	}
	if err == nil && plan.toAgent {
		s.transcripts.forget(id)
	}
	if err == nil && !wasStopped {
		_, err = s.resumeSession(id, convo)
	}
	if err != nil {
		return fmt.Errorf("goa: %s didn't take (%w), and GOD of Agents couldn't come back as it was: %v. Continue its session", tried, why, err)
	}
	s.goaChanged()
	return fmt.Errorf("goa: %s didn't take (%w): GOD of Agents is back as it was", tried, why)
}

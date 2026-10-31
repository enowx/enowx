package daemon

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/accounts"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// accountsBackLook is how often sessions that fell back are looked at, to
// go back to their first account once its limit reset.
const accountsBackLook = time.Minute

// stoppedSpec is sess as stopped, its screen kept, with meta, to start
// again.
func stoppedSpec(sess *session.Session, meta session.Meta) session.Spec {
	info := sess.Info()
	return session.Spec{ID: info.ID, Dir: info.Dir, Created: info.Created, History: sess.History(),
		Command: info.Command, Exited: true, Meta: meta}
}

// reapSession ends what session id's program started.
func reapSession(id string) { reap(tag{key: session.EnvSession, value: id}) }

// switchAgent has session p.Session go on as agent p.Agent, which can't
// take the other's conversation: started on a summary of it, the task as
// the user gave it and where the other agent left it.
func (s *Server) switchAgent(p proto.AccountSwitchParams) (proto.SessionInfo, error) {
	sess, err := s.sessions.Get(p.Session)
	if err != nil {
		return proto.SessionInfo{}, err
	}
	info, meta := sess.Info(), sess.Meta()
	preset, ok := agent.Find(p.Agent)
	if !ok {
		return proto.SessionInfo{}, fmt.Errorf("unknown agent %q", p.Agent)
	}
	from := agentName(meta.Agent)
	prompt := s.handoffPrompt(sess, from)
	pr := s.accountPrefs.Of(agent.Base(preset.ID))
	acc := ""
	if name := accounts.Default(pr, meta.Project); name != "" {
		if a, ok := s.accounts.ByName(agent.Base(preset.ID), name); ok {
			acc = a.ID
		}
	}
	chat := s.chatOf("", "", s.guideFor(meta))
	chat.store = s.agentStore(preset.ID, info.ID, true)
	next := meta
	next.Agent, next.Account = preset.ID, acc
	run, err := s.configRunFor(info.ID, next) // the other agent's default config
	if err != nil {
		return proto.SessionInfo{}, err
	}
	chat.config = run
	l, err := launchFor(proto.SessionCreateParams{Agent: preset.ID, Prompt: prompt, SkipPermissions: meta.SkipPermissions}, chat)
	if err != nil {
		return proto.SessionInfo{}, err
	}
	if !info.Exited {
		if _, err := s.sessions.Replace(stoppedSpec(sess, meta)); err != nil {
			return proto.SessionInfo{}, err
		}
		reapSession(info.ID)
	}
	meta.Agent, meta.Account, meta.AccountFirst, meta.Conversation, meta.Config = preset.ID, acc, "", "", run.sessionInfo()
	meta.Restart, meta.Relaunch, meta.SkipPermissions = l.restart, l.relaunch, l.skip
	env, err := s.agentEnv(meta)
	if err != nil {
		return proto.SessionInfo{}, err
	}
	spec := session.Spec{ID: info.ID, Dir: info.Dir, Created: info.Created, History: sess.History(),
		Env: append(append(append(append(s.hostEnvFor(meta.Member), env...), l.env...), s.vaultEnvFor(info.ID, meta.Project, meta.Creds)...), run.environ()...), Command: l.command, Meta: meta}
	started, err := s.sessions.Replace(spec)
	if err != nil {
		return proto.SessionInfo{}, err
	}
	s.began(started, l.command, l.typed)
	s.recordAccount(proto.AccountLogEntry{Session: info.ID, Agent: agent.Base(preset.ID), Account: s.accountName(acc), Event: "switch", Reason: p.Reason + " (from " + from + ")"})
	return started.Info(), nil
}

// handoffPrompt is what an agent taking over sess from agent from is told: the
// user's prompts, and the last of what from said and did.
func (s *Server) handoffPrompt(sess *session.Session, from string) string {
	var b strings.Builder
	b.WriteString("You take over this task from " + from + ", which stopped at its account's limit. Go on from where it left off; check the folder's changes (git status, git diff) before redoing anything.\n")
	tr, err := s.transcript(proto.TranscriptParams{ID: sess.ID(), Tail: 80})
	entries := tr.Entries
	if err != nil || len(entries) == 0 {
		if p := strings.TrimSpace(sess.Meta().Prompt); p != "" {
			b.WriteString("\nThe task: " + p + "\n")
		}
		return b.String()
	}
	var prompts, said []string
	for _, e := range entries {
		switch e.Kind {
		case proto.TranscriptUser:
			prompts = append(prompts, strings.TrimSpace(e.Text))
		case proto.TranscriptAssistant:
			said = append(said, strings.TrimSpace(e.Text))
		}
	}
	if len(prompts) > 0 {
		b.WriteString("\nWhat the user asked:\n")
		for _, p := range prompts[max(0, len(prompts)-5):] {
			b.WriteString("- " + clip(p, 600) + "\n")
		}
	}
	if len(said) > 0 {
		b.WriteString("\nWhat " + from + " said last:\n")
		for _, t := range said[max(0, len(said)-3):] {
			b.WriteString(clip(t, 1200) + "\n")
		}
	}
	return b.String()
}

// goBackLoop has sessions that fell back go back to their first account
// once its limit reset, between turns, when their agent's preferences ask,
// until done closes.
func (s *Server) goBackLoop(done <-chan struct{}) {
	t := time.NewTicker(accountsBackLook)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case now := <-t.C:
			s.goBack(now)
		}
	}
}

// goBack has the sessions that fell back go back to their first account,
// its limit reset by now, when their preferences ask.
func (s *Server) goBack(now time.Time) {
	for _, sess := range s.sessions.List() {
		meta, info := sess.Meta(), sess.Info()
		if !strings.HasPrefix(meta.AccountFirst, "=") || info.Exited || info.State == proto.StateWorking || info.Active {
			continue
		}
		base := agent.Base(meta.Agent)
		if !s.accountPrefs.Of(base).Return {
			continue
		}
		first := strings.TrimPrefix(meta.AccountFirst, "=")
		if _, spent := s.acctWatch.spent(base, first, now); spent {
			continue
		}
		id := ""
		if a, ok := s.accounts.ByName(base, first); ok {
			id = a.ID
		} else if first != "" {
			continue // gone
		}
		if _, err := s.switchAccount(proto.AccountSwitchParams{Session: info.ID, Account: id, Reason: "back: its limit reset"}); err == nil {
			s.broadcast(proto.EventAccountFallback, proto.AccountFallback{Session: info.ID, Agent: base, From: s.accountName(meta.Account), To: first, ToID: id, Back: true, Switched: true})
		}
	}
}

// accountsInfo is the agents' accounts with their state and what they
// have left, and the preferences.
func (s *Server) accountsInfo() proto.AccountsInfo {
	out := proto.AccountsInfo{Prefs: s.accountPrefs.All()}
	report := s.credits.Report(false)
	credit := func(agentID, id string) *proto.CreditAccount {
		for _, c := range report.Accounts {
			if c.Agent == agentID && c.AccountID == id {
				return &c
			}
		}
		return nil
	}
	now := time.Now()
	add := func(a proto.Account, own bool) {
		st := proto.AccountStatus{Account: a, Own: own, State: proto.AccountOK, Credits: credit(a.Agent, a.ID)}
		if r, spent := s.acctWatch.spent(a.Agent, a.Name, now); spent {
			st.State, st.Until, st.Line = r.Reason, r.Until, r.Line
		}
		out.Accounts = append(out.Accounts, st)
	}
	// The official config of each agent with others, and of those that
	// can have accounts.
	list := s.accounts.List()
	officials := []string{}
	for _, id := range []string{"claude", "codex"} {
		if p, ok := agent.Find(id); ok && p.Installed() {
			officials = append(officials, id)
		}
	}
	for _, a := range list {
		if a.Member == "" && !slices.Contains(officials, a.Agent) {
			officials = append(officials, a.Agent)
		}
	}
	for _, id := range officials {
		add(proto.Account{Agent: id, Name: ""}, true)
	}
	for _, a := range list {
		if a.Member == "" {
			add(a, false)
		}
	}
	return out
}

// setAccountPrefs keeps agent's preferences; an account put first again
// is taken as working.
func (s *Server) setAccountPrefs(p proto.AccountPrefsParams) (proto.AccountsInfo, error) {
	if err := s.accountPrefs.Set(p.Agent, p.Prefs); err != nil {
		return proto.AccountsInfo{}, err
	}
	return s.accountsInfo(), nil
}

// defaultAccount is the account a new session of agentID in project
// starts on, by the preferences: its ID, "" its own.
func (s *Server) defaultAccount(agentID, project string) string {
	name := accounts.Default(s.accountPrefs.Of(agent.Base(agentID)), project)
	if a, ok := s.accounts.ByName(agent.Base(agentID), name); ok {
		return a.ID
	}
	return ""
}

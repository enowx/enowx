package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/agentapi"
	"github.com/enowx/enowx/core/decide"
	"github.com/enowx/enowx/core/rag"
	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
)

// Decisions on this host (core/decide): the judge asked whether a quiet
// session needs the user, whether an agent's command may run (hook action
// judge), what of a long output the agent needs (enowx decide trim), and
// whether an agent loops. With the use off or no answer, enowX does what
// it did without. See docs/decide.md.

// loopSteps are the tool runs kept of a session, to see a loop in.
const loopSteps = 12

// loopAgain is how long after asking about a session's loop it is asked
// again.
const loopAgain = 3 * time.Minute

// decideState is what the daemon follows of sessions for the judge. mu
// guards it.
type decideState struct {
	mu    sync.Mutex
	steps map[string][]step // by session
	asked map[string]time.Time
}

type step struct {
	tool, command string
	failed        bool
}

// newDecide makes the host's judge: keys and the credentials' list from
// the vault, agents found on the PATH, the host's secrets masked in its
// log.
func (s *Server) newDecide(dir string) {
	s.judge = decide.NewEngine(dir, decide.OpenSettings(dir))
	fromVault := s.vaultKey("enowX decisions: a model API's key")
	s.judge.KeyOf = func(cred string) (string, error) {
		if agentID, ok := strings.CutPrefix(cred, "agentapi:"); ok {
			if api := agentAPIOf(agentID); api != nil {
				return api.APIKey, nil
			}
			return "", fmt.Errorf("decide: %s is no longer set to a model API", agentID)
		}
		return fromVault(cred)
	}
	s.judge.Found = func() decide.Found {
		return decide.Found{LookPath: exec.LookPath, Creds: s.vaultIndex().Credentials, Local: decide.LocalServers,
			AgentAPI: func(agentID string) *decide.AgentAPI {
				api := agentAPIOf(agentID)
				if api == nil {
					return nil
				}
				return &decide.AgentAPI{BaseURL: api.BaseURL, Model: orStr(api.SmallModel, api.Model), Protocol: api.Protocol}
			}}
	}
	s.judge.Mask = func(text string) string { return vault.Redact(text, s.lentSecrets()) }
	s.decideRun = decideState{steps: map[string][]step{}, asked: map[string]time.Time{}}
}

// agentAPIOf is the model API agent id is set to on this host, nil for
// its own.
func agentAPIOf(id string) *agentapi.API {
	home, err := os.UserHomeDir()
	if err != nil || !slices.Contains(agentapi.Supported(), id) {
		return nil
	}
	a, err := agentapi.Of(home, id)
	if err != nil {
		return nil
	}
	return a.Current
}

func orStr(s, or string) string {
	if s == "" {
		return or
	}
	return s
}

// vaultKey reads a credential's value from the vault, for what.
func (s *Server) vaultKey(what string) func(cred string) (string, error) {
	return func(cred string) (string, error) {
		lend, err := s.vaultAsk(proto.VaultAsk{Action: proto.VaultEnv, Name: cred, Command: what})
		if err != nil {
			return "", err
		}
		for _, v := range lend.Env {
			if v != "" {
				return v, nil
			}
		}
		if lend.Secret != "" {
			return lend.Secret, nil
		}
		return "", fmt.Errorf("decide: the credential %s has no value", cred)
	}
}

// decideFor asks the judge about session info (or none), a.Use's
// question; ErrOff when the use is off.
func (s *Server) decideFor(info proto.SessionInfo, a proto.DecideAsk) (proto.Decision, error) {
	dir := sessionProject(info)
	if a.Project == "" {
		a.Project = dir
	}
	r := decide.Request{DecideAsk: a, Agent: agent.Base(info.Agent), AgentName: s.agentLabel(info.Agent)}
	if a.Project != "" {
		r.ProjectID = rag.ProjectID(a.Project)
	}
	r.Session = info.ID
	if info.Config != nil {
		r.Provider = info.Config.Provider // the session's config's, not the agent's
	}
	return s.judge.Decide(context.Background(), r)
}

func (s *Server) agentLabel(id string) string {
	if p, ok := agent.Find(agent.Base(id)); ok {
		return p.Name
	}
	return ""
}

// sure says whether decision d is one to act on: answered, as sure as its
// use asks.
func (s *Server) sure(d proto.Decision) bool {
	return d.Judge != "" && d.Confidence >= decide.Use(s.judge.Settings().Get(), d.Use).MinConfidence
}

// acted records what came of decision d, tells the apps, and says it as
// recorded.
func (s *Server) acted(d proto.Decision, what string, saved int) proto.Decision {
	if out, ok := s.judge.Acted(d.ID, what, saved); ok {
		d = out
	}
	s.broadcast(proto.EventDecision, d)
	return d
}

// judgeHook is the hook action judge: session info's agent is about to
// run ev's command; the judge allows it, leaves it to the user, or
// refuses it.
func (s *Server) judgeHook(info proto.SessionInfo, ev proto.HookEvent) (proto.HookDecision, string) {
	if ev.Command == "" {
		return proto.HookDecision{}, "no command to judge"
	}
	d, err := s.decideFor(info, proto.DecideAsk{Use: proto.DecideApprove, Command: ev.Command})
	switch {
	case err != nil:
		return proto.HookDecision{}, err.Error()
	case d.Judge == "":
		return proto.HookDecision{}, "no answer: " + d.Error
	case !s.sure(d):
		s.acted(d, proto.ActedNothing, 0)
		return proto.HookDecision{}, fmt.Sprintf("%s, not sure enough (%.2f): %s", d.Verdict, d.Confidence, d.Why)
	}
	switch d.Verdict {
	case proto.VerdictAllow:
		s.acted(d, proto.ActedAllowed, 0)
		return proto.HookDecision{Approve: true}, "allowed: " + d.Why
	case proto.VerdictRefuse:
		s.acted(d, proto.ActedRefused, 0)
		return proto.HookDecision{Block: true, Reason: "enowX's judge refused this command: " + d.Why + " If it is needed, ask the user to run it."}, "refused: " + d.Why
	}
	s.acted(d, proto.ActedNothing, 0)
	return proto.HookDecision{}, "left to the user: " + d.Why
}

// judgeState asks whether session info's agent, its screen quiet, is done,
// waits for the user, is stuck or failed; it marks the session waiting,
// or alerts the user.
func (s *Server) judgeState(info proto.SessionInfo) {
	if !decide.Use(s.judge.Settings().Get(), proto.DecideState).On {
		return
	}
	sess, err := s.sessions.Get(info.ID)
	if err != nil {
		return
	}
	screen := strings.TrimRight(s.maskLent(sess.Terminal().Text()), " \n")
	if lines := strings.Split(screen, "\n"); len(lines) > 60 {
		screen = strings.Join(lines[len(lines)-60:], "\n")
	}
	d, err := s.decideFor(info, proto.DecideAsk{Use: proto.DecideState, Screen: screen})
	if err != nil || !s.sure(d) || sess.Info().State == proto.StateWorking {
		if err == nil {
			s.acted(d, proto.ActedNothing, 0)
		}
		return
	}
	name := info.Name
	if name == "" {
		name = s.agentLabel(info.Agent)
	}
	switch d.Verdict {
	case proto.VerdictWaiting:
		_ = s.sessions.SetState(sess, proto.StateWaiting)
		s.broadcast(proto.EventNotifySound, proto.NotifySoundParams{Kind: "input", Session: info.ID, Text: name + " needs you: " + d.Why})
		s.acted(d, proto.ActedWaiting, 0)
	case proto.VerdictStuck, proto.VerdictFailed:
		s.broadcast(proto.EventNotifySound, proto.NotifySoundParams{Kind: "error", Session: info.ID, Text: name + " seems " + d.Verdict + ": " + d.Why})
		s.acted(d, proto.ActedAlerted, 0)
	default:
		s.acted(d, proto.ActedNothing, 0)
	}
}

// toolStep notes a tool session info's agent ran (ev, tool.after), and
// asks whether it loops when its last steps keep failing alike.
func (s *Server) toolStep(info proto.SessionInfo, ev proto.HookEvent) {
	if !decide.Use(s.judge.Settings().Get(), proto.DecideLoop).On {
		return
	}
	st := &s.decideRun
	st.mu.Lock()
	steps := append(st.steps[info.ID], step{tool: ev.Tool, command: ev.Command, failed: ev.Failed})
	if len(steps) > loopSteps {
		steps = steps[len(steps)-loopSteps:]
	}
	st.steps[info.ID] = steps
	ask := loopLike(steps) && time.Since(st.asked[info.ID]) > loopAgain
	if ask {
		st.asked[info.ID] = time.Now()
	}
	steps = append([]step(nil), steps...)
	st.mu.Unlock()
	if ask {
		go s.judgeLoop(info, steps)
	}
}

// promptStep forgets session id's steps: a new prompt, a new task.
func (s *Server) promptStep(id string) {
	s.decideRun.mu.Lock()
	delete(s.decideRun.steps, id)
	s.decideRun.mu.Unlock()
}

// loopLike says whether steps look like a loop worth asking about: of the
// last eight, three failed, two of them the same command.
func loopLike(steps []step) bool {
	if len(steps) > 8 {
		steps = steps[len(steps)-8:]
	}
	failed, seen, same := 0, map[string]int{}, false
	for _, st := range steps {
		if !st.failed {
			continue
		}
		failed++
		key := st.tool + " " + strings.Join(strings.Fields(st.command), " ")
		if len(key) > 60 {
			key = key[:60]
		}
		seen[key]++
		same = same || seen[key] >= 2
	}
	return failed >= 3 && same
}

// judgeLoop asks whether session info's agent loops; if so, it stops the
// agent (Escape) and tells the user.
func (s *Server) judgeLoop(info proto.SessionInfo, steps []step) {
	lines := make([]string, len(steps))
	for i, st := range steps {
		mark := "ok"
		if st.failed {
			mark = "FAILED"
		}
		lines[i] = fmt.Sprintf("%d. %s: %s → %s", i+1, st.tool, st.command, mark)
	}
	d, err := s.decideFor(info, proto.DecideAsk{Use: proto.DecideLoop, Steps: lines})
	if err != nil {
		return
	}
	if !s.sure(d) || d.Verdict != proto.VerdictLooping {
		s.acted(d, proto.ActedNothing, 0)
		return
	}
	sess, err := s.sessions.Get(info.ID)
	if err != nil || sess.Info().Exited {
		return
	}
	s.sessions.Input(sess, []byte("\x1b")) // Escape stops Claude Code's and Codex's turn
	s.acted(d, proto.ActedPaused, 0)
}

// trimOutput is enowx decide trim's: command's long output, what the
// agent needs of it.
func (s *Server) trimOutput(info proto.SessionInfo, a proto.DecideAsk) (proto.Decision, error) {
	a.Output = s.maskLent(a.Output)
	d, err := s.decideFor(info, a)
	if err != nil {
		return d, err
	}
	if d.Verdict == proto.VerdictTrimmed && s.sure(d) && len(d.Text) < len(a.Output) {
		return s.acted(d, proto.ActedTrimmed, len(a.Output)-len(d.Text)), nil
	}
	d = s.acted(d, proto.ActedNothing, 0)
	d.Text = ""
	return d, nil
}

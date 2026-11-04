package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// What each session's agent has of its rules. Each rules have a version, the
// session keeps the one its agent last got (SessionInfo.Rules, saved with it),
// and a hook compares at each prompt: a change, a compaction or a resume gives
// them once, and nothing else does. An agent without hooks reads what it reads:
// docs/agents.md says what each gets.

const (
	// rulesLook is how often rules that came by sync or by hand are noticed.
	rulesLook = 30 * time.Second
	// rulesInHookMax is the most rules a hook's answer holds: agents cut what
	// their hooks say past about 10,000 characters, with other context besides.
	rulesInHookMax = 8000
)

// rulesVersion is rules' version, "" for none.
func rulesVersion(rules string) string {
	rules = strings.TrimSpace(rules)
	if rules == "" {
		return ""
	}
	h := sha256.Sum256([]byte(rules))
	return hex.EncodeToString(h[:4])
}

// rulesVersionLine is what a guide says beside rules, for an agent that may
// outlive them to know.
func rulesVersionLine(rules string) string {
	return "Version of these rules: " + rulesVersion(rules) + ". They can change while you work: `\"$ENOWX_BIN\" guide` prints the current ones."
}

// rulesFolder is the folder whose rules hold for a session, "" for none.
func rulesFolder(meta session.Meta, dir string) string {
	if meta.GOA || meta.NotesFor != "" || meta.CanvasFor != "" {
		return ""
	}
	if meta.Project != "" {
		return meta.Project
	}
	return dir
}

// rulesHolding is which rules hold for a session of meta in folder dir, and
// them; forWhat is "" for a session they don't concern.
func (s *Server) rulesHolding(meta session.Meta, dir string) (forWhat, rules string) {
	if _, ok := agent.Find(meta.Agent); !ok || meta.NotesFor != "" || meta.CanvasFor != "" {
		return "", ""
	}
	if meta.GOA {
		return proto.RulesForGOA, s.goaRules().Rules
	}
	folder := rulesFolder(meta, dir)
	if folder == "" {
		return "", ""
	}
	r, err := s.projectRules(folder)
	if err != nil {
		return "", ""
	}
	return proto.RulesForProject, strings.TrimSpace(r.Rules)
}

// hookGivesRules says agent id's hooks can add to its context at a prompt.
func hookGivesRules(id string) bool {
	return agent.HookSupport(id)[proto.HookPromptStart] == proto.HookInAgent
}

// rulesAtStart is what the agent of a session of meta has as it starts, its guide just
// written with the rules as they are. An agent told to read a file with hooks to give
// them gets them from its hooks too, since nothing says it read it.
func (s *Server) rulesAtStart(meta session.Meta, dir string, resumed bool) *proto.SessionRules {
	forWhat, rules := s.rulesHolding(meta, dir)
	if forWhat == "" {
		return nil
	}
	ver := rulesVersion(rules)
	r := proto.SessionRules{For: forWhat, Current: ver, Hook: hookGivesRules(meta.Agent)}
	if meta.Rules != nil {
		r.Version, r.Via, r.At = meta.Rules.Version, meta.Rules.Via, meta.Rules.At
	}
	switch delivery := agent.RulesDelivery(meta.Agent); {
	case delivery == proto.RulesViaStart, delivery == proto.RulesViaFile && !r.Hook:
		noteRulesGiven(&r, ver, delivery)
	}
	r.Again = resumed && r.Hook && ver != ""
	return &r
}

// noteRulesGiven says r's agent now has version ver, by via.
func noteRulesGiven(r *proto.SessionRules, ver, via string) {
	r.Version, r.At = ver, time.Now()
	r.Via = via
	if ver == "" {
		r.Via, r.At = "", time.Time{}
	}
}

// giveRules takes event ev of session sess's agent, and is what to add to its context:
// the rules, when it hasn't them as they are, "" otherwise.
func (s *Server) giveRules(sess *session.Session, info proto.SessionInfo, ev proto.HookEvent) string {
	switch ev.Event {
	case proto.HookCompact, proto.HookSessionStart, proto.HookPromptStart:
	default:
		return ""
	}
	forWhat, rules := s.rulesHolding(sess.Meta(), info.Dir)
	if forWhat == "" {
		return ""
	}
	ver := rulesVersion(rules)
	dropped := ev.Event == proto.HookCompact
	if ev.Event == proto.HookSessionStart {
		switch ev.Source {
		case "compact", "resume", "clear":
			dropped = true
		}
	}
	var was string
	var again, send bool
	s.sessions.UpdateRules(sess, func(r *proto.SessionRules) {
		r.For, r.Current, r.Hook = forWhat, ver, hookGivesRules(info.Agent)
		r.Again = r.Again || dropped
		if ev.Event == proto.HookCompact {
			return // gone as it compacts: given with what follows it
		}
		was, again = r.Version, r.Again
		r.Again = false
		if was == ver && !again || ver == "" && was == "" {
			return
		}
		send = true
		noteRulesGiven(r, ver, proto.RulesViaHook)
	})
	if !send {
		return ""
	}
	return s.rulesForHook(info.ID, forWhat, rules, ver, was)
}

// rulesForHook is what a hook tells an agent that has version was of rules, now
// version ver, any secret in them masked as in a guide.
func (s *Server) rulesForHook(id, forWhat, rules, ver, was string) string {
	whose, yours := "this project's rules", "follow them in all you do here"
	if forWhat == proto.RulesForGOA {
		whose, yours = "their rules for you as GOD of Agents", "follow them in all you do as GOD of Agents"
	}
	switch {
	case ver == "":
		return "The user removed " + whose + ": those you were given no longer hold."
	case was != "" && was != ver:
		whose = "The user changed " + whose + "; these replace those you were given (version " + ver + "):"
	default:
		whose = "The user's rules (version " + ver + "), " + yours + "; they stand even if your context was compacted or you resumed:"
	}
	body := guideWithoutSecrets(rules)
	if len(body) > rulesInHookMax {
		path, err := agent.PlaceRulesFile(s.agentFilesDir(id), body)
		if err != nil {
			s.logger().Warn("rules: could not write them for a session", "id", id, "err", err)
			body = "They are too long to give here: `\"$ENOWX_BIN\" guide` prints them. Run it now, before anything else."
		} else {
			body = "They are too long to give here: read " + path + " now, before anything else."
		}
	}
	return whose + "\n\n" + body
}

// guideReadRules is what the guide printed for p adds of the rules of p's session, for
// the agents it is the way to them, or who ran it to read them; one with no hooks then
// has them, as far as can be known.
func (s *Server) guideReadRules(p proto.FeaturesParams) string {
	if p.Session == "" {
		return ""
	}
	sess, err := s.sessions.Get(p.Session)
	if err != nil {
		return ""
	}
	meta, info := sess.Meta(), sess.Info()
	if !p.Reading && agent.RulesDelivery(meta.Agent) != proto.RulesViaAsked {
		return ""
	}
	forWhat, rules := s.rulesHolding(meta, info.Dir)
	if forWhat == "" {
		return ""
	}
	if p.Reading && !hookGivesRules(meta.Agent) {
		ver := rulesVersion(rules)
		s.sessions.UpdateRules(sess, func(r *proto.SessionRules) {
			r.For, r.Current = forWhat, ver
			noteRulesGiven(r, ver, proto.RulesViaAsked)
		})
	}
	if forWhat == proto.RulesForGOA {
		return guideWithoutSecrets(goaRulesGuide(rules))
	}
	return guideWithoutSecrets(s.projectGuide(rulesFolder(meta, info.Dir)))
}

// refreshRules brings each running session up to the rules as they are: the app's
// record of them, and the guide's file its agent may read.
func (s *Server) refreshRules() {
	for _, sess := range s.sessions.List() {
		info := sess.Info()
		if info.Exited {
			continue // its guide is written as it comes back
		}
		meta := sess.Meta()
		forWhat, rules := s.rulesHolding(meta, info.Dir)
		if forWhat == "" {
			continue
		}
		ver := rulesVersion(rules)
		if meta.Rules != nil && meta.Rules.Current == ver {
			continue
		}
		start := s.rulesAtStart(meta, info.Dir, false) // a session from before rules were kept
		s.writeSessionGuide(meta, info)
		hook := hookGivesRules(meta.Agent)
		toFile := !hook && agent.RulesDelivery(meta.Agent) == proto.RulesViaFile
		s.sessions.UpdateRules(sess, func(r *proto.SessionRules) {
			if r.For == "" {
				*r = *start
			}
			r.Current = ver
			if toFile {
				noteRulesGiven(r, ver, proto.RulesViaFile)
			}
		})
	}
}

// writeSessionGuide rewrites the guide's file of a session, leaving what its agent
// loaded as it started.
func (s *Server) writeSessionGuide(meta session.Meta, info proto.SessionInfo) {
	chat := s.goaChat(s.chatOf(meta.NotesFor, meta.CanvasFor, s.guideIn(meta, info.Dir)), meta)
	chat.files = s.agentFilesDir(info.ID)
	chat.agentGuide(meta.Agent, guideWithoutSecrets(chat.guide))
}

// watchRules notices rules that changed without a save here: by a sync, or by hand.
func (s *Server) watchRules(done <-chan struct{}) {
	t := time.NewTicker(rulesLook)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			s.refreshRules()
		}
	}
}

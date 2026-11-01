package daemon

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// The user's rules for GOD of Agents: Markdown in the synced settings, so
// every host's GOA follows them. GOA reads them in its guide when it
// starts, and, its agent having a start-of-prompt hook, again with the
// next prompt after they change.

func (s *Server) goaRulesFile() string { return filepath.Join(s.dataDir, "settings", "goa-rules.md") }

// goaRules are the user's rules for GOA.
func (s *Server) goaRules() proto.GOARules {
	data, _ := os.ReadFile(s.goaRulesFile())
	return proto.GOARules{Rules: strings.TrimSpace(string(data))}
}

// saveGOARules keeps r, refused when too long or holding a secret.
func (s *Server) saveGOARules(r proto.GOARules) (proto.GOARules, error) {
	rules := strings.TrimSpace(r.Rules)
	if len(rules) > maxRules {
		return proto.GOARules{}, fmt.Errorf("rules: %d KB at most, they go in GOA's guide", maxRules>>10)
	}
	if err := secretsIn(map[string][]byte{"the rules": []byte(rules)}); err != nil {
		return proto.GOARules{}, errors.New("rules: they look like they hold a secret: GOA reads them, keep credentials in the vault")
	}
	file := s.goaRulesFile()
	if rules == "" {
		os.Remove(file)
	} else if err := writeTeamFile(file, []byte(rules+"\n")); err != nil {
		return proto.GOARules{}, err
	}
	return s.goaRules(), nil
}

// goaRulesGuide is the rules as GOA's guide gives them, "" for none.
func goaRulesGuide(rules string) string {
	if rules == "" {
		return ""
	}
	return "\n\n## The user's rules for you\n\nFollow these in all you do as GOD of Agents. Pass on to a worker, in its prompt, those that bear on its task.\n\n" + rules + "\n"
}

// goaRulesChanged is GOA session id's rules when they changed since it
// was last given them ("" otherwise), and marks them given.
func (s *Server) goaRulesChanged(id string) string {
	rules := s.goaRules().Rules
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(rules)))
	s.goa.mu.Lock()
	defer s.goa.mu.Unlock()
	if s.goa.rulesGiven == nil {
		s.goa.rulesGiven = map[string]string{}
	}
	prev, seen := s.goa.rulesGiven[id]
	s.goa.rulesGiven[id] = sum
	if !seen || prev == sum {
		return "" // its first prompt: its guide had them; or nothing changed
	}
	if rules == "" {
		return "The user removed their rules for you as GOD of Agents: those you were given no longer hold."
	}
	return "The user changed their rules for you as GOD of Agents; these replace those you were given:\n\n" + rules
}

// rulesSystem is what the AI drafting rules is told, by what they're for.
func rulesSystem(forWhat string) string {
	about := "a software project's rules: Markdown every coding agent working in the project (Claude Code, Codex and others) reads first and follows"
	if forWhat == proto.RulesForGOA {
		about = "the rules of GOD of Agents, an orchestrating agent that hands coding tasks to other agents' sessions on several hosts, routes their models, follows their work and answers their questions: Markdown it reads first and follows"
	}
	return "You write " + about + ".\n\n" +
		"Write them from what the user says they want, in English, as a short Markdown list grouped under a few ### headings when there are several topics. " +
		"Each rule one line, imperative, concrete and checkable (name the command, the branch, the folder, the agent), with the reason after a colon when it isn't obvious. " +
		"Keep every rule the user's current rules hold unless the user asks otherwise; merge duplicates; drop vague wishes that can't be acted on. " +
		"Never invent facts about the project, and never write credentials. " +
		"Answer with the rules alone: no preamble, no code fence, no closing remark."
}

// draftRules has an AI write rules from what the user asked (p).
func (s *Server) draftRules(p proto.RulesDraftParams) (proto.RulesDraft, error) {
	ask := strings.TrimSpace(p.Ask)
	if ask == "" {
		return proto.RulesDraft{}, errors.New("rules: say what the rules should hold")
	}
	// It goes to an AI, perhaps off this host.
	if err := secretsIn(map[string][]byte{"the rules": []byte(ask + "\n" + p.Current)}); err != nil {
		return proto.RulesDraft{}, errors.New("rules: that looks like it holds a secret: nothing was sent to the AI")
	}
	var user strings.Builder
	if p.For == proto.RulesForProject && p.Path != "" {
		fmt.Fprintf(&user, "The project: %s\n\n", filepath.Base(p.Path))
	}
	if cur := strings.TrimSpace(p.Current); cur != "" {
		fmt.Fprintf(&user, "The current rules:\n\n%s\n\n", cur)
	}
	fmt.Fprintf(&user, "What the user wants:\n\n%s\n", ask)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	text, by, err := s.judge.Write(ctx, rulesSystem(p.For), user.String())
	if err != nil {
		return proto.RulesDraft{}, err
	}
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(text, "```markdown"), "```"), "```"))
	if len(text) > maxRules {
		text = text[:maxRules]
	}
	return proto.RulesDraft{Rules: text, By: by}, nil
}

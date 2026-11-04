package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
	before := s.goaRules().Rules
	if rules == "" && before != "" && !r.Clear {
		return proto.GOARules{}, errors.New("rules: saving them empty would delete every rule: say so to clear them")
	}
	if len(rules) > maxRules {
		return proto.GOARules{}, fmt.Errorf("rules: %d KB at most, they go in GOA's guide", maxRules>>10)
	}
	if err := secretsIn(map[string][]byte{"the rules": []byte(rules)}); err != nil {
		return proto.GOARules{}, errors.New("rules: they look like they hold a secret: GOA reads them, keep credentials in the vault")
	}
	if rules != before {
		s.keepRulesVersion("goa", before)
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
	return "You write new rules to add to " + about + ".\n\n" +
		"Write only the new rules the user asks for, as a short Markdown list, in English. The current rules, if shown, stay exactly as they are and aren't yours to repeat, reword, reorder or remove: never include them. " +
		"Each rule one line, imperative, concrete and checkable (name the command, the branch, the folder, the agent), with the reason after a colon when it isn't obvious. Leave out a rule the current ones already say; drop vague wishes that can't be acted on. " +
		"When a new rule goes against a current one, still write it, and add a line starting with \"" + rulesConflict + "\" saying which. " +
		"Never invent facts about the project, and never write credentials. " +
		"Answer with the new rules alone: no preamble, no code fence, no closing remark."
}

// rulesConflict starts a line of the AI's saying a rule goes against another.
const rulesConflict = "CONFLICT: "

// draftRules has an AI write new rules from what the user asked (p). The
// rules there are only shown to it, and what comes back is only ever added to
// them, in the app.
func (s *Server) draftRules(p proto.RulesDraftParams) (proto.RulesDraft, error) {
	ask := strings.TrimSpace(p.Ask)
	if ask == "" {
		return proto.RulesDraft{}, errors.New("rules: say what the rules should hold")
	}
	// The file's too: an editor can be behind, or empty for a reason.
	current := strings.TrimSpace(p.Current)
	if stored := s.storedRules(p); stored != "" && !strings.Contains(current, stored) {
		current = strings.TrimSpace(stored + "\n" + current)
	}
	// It goes to an AI, perhaps off this host.
	if err := secretsIn(map[string][]byte{"the rules": []byte(ask + "\n" + current)}); err != nil {
		return proto.RulesDraft{}, errors.New("rules: that looks like it holds a secret: nothing was sent to the AI")
	}
	var user strings.Builder
	if p.For == proto.RulesForProject && p.Path != "" {
		fmt.Fprintf(&user, "The project: %s\n\n", filepath.Base(p.Path))
	}
	if current != "" {
		fmt.Fprintf(&user, "The current rules, which stay as they are:\n\n%s\n\n", current)
	}
	fmt.Fprintf(&user, "What the user wants:\n\n%s\n", ask)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	text, by, err := s.judge.Write(ctx, rulesSystem(p.For), user.String())
	if err != nil {
		return proto.RulesDraft{}, err
	}
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(text, "```markdown"), "```"), "```"))
	out := proto.RulesDraft{By: by}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if said, ok := strings.CutPrefix(strings.TrimLeft(line, "-* "), rulesConflict); ok {
			out.Conflicts = append(out.Conflicts, strings.TrimSpace(said))
			continue
		}
		lines = append(lines, line)
	}
	out.Rules = strings.TrimSpace(strings.Join(lines, "\n"))
	if out.Rules == "" {
		return proto.RulesDraft{}, errors.New("rules: the AI wrote nothing to add")
	}
	if len(out.Rules) > maxRules {
		return proto.RulesDraft{}, fmt.Errorf("rules: the AI wrote over %d KB: nothing was added", maxRules>>10)
	}
	return out, nil
}

// storedRules are the rules p's are, as saved.
func (s *Server) storedRules(p proto.RulesDraftParams) string {
	if p.For == proto.RulesForGOA {
		return s.goaRules().Rules
	}
	if p.Path == "" {
		return ""
	}
	r, err := s.projectRules(p.Path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(r.Rules)
}

// rulesVersionsKept is how many earlier versions of each rules a host keeps.
const rulesVersionsKept = 20

func (s *Server) rulesHistoryDir(key string) string {
	h := sha256.Sum256([]byte(key))
	return filepath.Join(s.dataDir, "rules-history", hex.EncodeToString(h[:8]))
}

// keepRulesVersion keeps rules as they were before a save, on this host, the
// oldest going past rulesVersionsKept. A failure costs only the history.
func (s *Server) keepRulesVersion(key, rules string) {
	if strings.TrimSpace(rules) == "" {
		return
	}
	dir := s.rulesHistoryDir(key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.logger().Warn("rules: keeping a version", "err", err)
		return
	}
	name := fmt.Sprintf("%020d.md", time.Now().UnixNano())
	if err := os.WriteFile(filepath.Join(dir, name), []byte(rules), 0o600); err != nil {
		s.logger().Warn("rules: keeping a version", "err", err)
		return
	}
	entries, _ := os.ReadDir(dir)
	for i := 0; i < len(entries)-rulesVersionsKept; i++ {
		os.Remove(filepath.Join(dir, entries[i].Name()))
	}
}

// rulesHistory lists p's rules as they were, newest first.
func (s *Server) rulesHistory(p proto.RulesDraftParams) []proto.RulesVersion {
	keys := []string{"goa"}
	if p.For != proto.RulesForGOA {
		keys = rulesHistoryKeys(p.Path)
	}
	out := []proto.RulesVersion{}
	for _, key := range keys {
		dir := s.rulesHistoryDir(key)
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			n, err := strconv.ParseInt(strings.TrimSuffix(e.Name(), ".md"), 10, 64)
			data, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
			if err == nil && rerr == nil {
				out = append(out, proto.RulesVersion{Time: time.Unix(0, n), Rules: string(data)})
			}
		}
	}
	slices.SortFunc(out, func(a, b proto.RulesVersion) int { return b.Time.Compare(a.Time) })
	return out
}

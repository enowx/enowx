package decide

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/enowx/enowx/proto"
)

// The uses added after the first four: a prompt's difficulty (route),
// which search results bear on a prompt (context), whether an agent's last
// answer holds a decision to keep (memory), how a chat message notifies
// (chat), and whether a check failed by chance (flaky).

// More limits of what a judge is given, in characters.
const (
	maxPrompt  = 4000
	maxPart    = 1500
	maxAnswer  = 6000
	maxMessage = 2000
	maxCheck   = 12000
)

// laterQuestion is the question of a.Use, when it is one of these.
func laterQuestion(a proto.DecideAsk, agentName string) (question, bool) {
	switch a.Use {
	case proto.DecideRoute:
		return question{verdicts: []string{proto.VerdictEasy, proto.VerdictMedium, proto.VerdictHard},
			prompt: fmt.Sprintf(`The user is about to give %s this prompt in the project %q. How hard is it for a coding agent? enowX picks a cheaper model and less reasoning for what doesn't need more.
- "easy": small and clear, done from a few files: a rename, a typo, a log line, a one-line fix, explaining a function, running a command.
- "medium": ordinary feature or bug work over a few files, with some reasoning.
- "hard": design across many files, a subtle or unknown bug, concurrency, security, performance, a migration, or an open-ended task.

<prompt>
%s
</prompt>`, agentName, a.Project, head(a.Prompt, maxPrompt))}, true
	case proto.DecideContext:
		var parts strings.Builder
		for i, p := range a.Parts {
			fmt.Fprintf(&parts, "<part n=\"%d\">\n%s\n</part>\n", i+1, head(p, maxPart))
		}
		n := len(a.Parts)
		return question{verdicts: []string{proto.VerdictRelevant, proto.VerdictNone},
			extra: `, "keep": [<the numbers of the parts kept>]`,
			check: func(v verdict) error {
				if v.Verdict != proto.VerdictRelevant {
					return nil
				}
				if len(v.Keep) == 0 {
					return errors.New("decide: relevant, with no part kept")
				}
				for _, k := range v.Keep {
					if k < 1 || k > n {
						return fmt.Errorf("decide: no part %d", k)
					}
				}
				return nil
			},
			prompt: fmt.Sprintf(`The user gives %s the prompt below. enowX's search of the project found the parts below it, to add to the prompt. Which of them would help the agent with this prompt? Leave out those about something else: they only fill its context.
- "relevant": "keep" lists the numbers of the parts that bear on the prompt.
- "none": none of them does ("keep" empty).

<prompt>
%s
</prompt>
%s`, agentName, head(a.Prompt, maxPrompt), parts.String())}, true
	case proto.DecideMemory:
		return question{verdicts: []string{proto.VerdictDecision, proto.VerdictNothing},
			extra: `, "text": "<the decision, or empty>"`,
			check: func(v verdict) error {
				if v.Verdict == proto.VerdictDecision && strings.TrimSpace(v.Text) == "" {
					return errors.New("decide: a decision with no text")
				}
				return nil
			},
			prompt: fmt.Sprintf(`This is what %s said last, at the end of its work in a project. Does it hold a decision worth keeping for later work on the project: a choice made and why (a library, a design, a convention), a constraint or a gotcha found, something a future session should know? A list of changes, a question, a plan not carried out or a status report are not.
- "decision": "text" is the decision in one or two plain sentences that stand alone: what, and why.
- "nothing": nothing worth keeping ("text" empty).

<answer>
%s
</answer>`, agentName, tail(a.Text, maxAnswer))}, true
	case proto.DecideChat:
		forMe := "It doesn't name the user or answer them."
		if a.Mention {
			forMe = "It names the user or answers them."
		}
		return question{verdicts: []string{proto.VerdictUrgent, proto.VerdictNormal, proto.VerdictQuiet},
			prompt: fmt.Sprintf(`A team member's message just came in the user's team chat %q. How should enowX tell the user?
- "urgent": it needs the user now: something broken for users, a blocker only they can lift, a deadline within hours, a request waiting on them.
- "normal": an ordinary message worth a notification.
- "quiet": chatter, thanks, a reaction in words, news that can wait until they read the chat.
%s

From: %s
<message>
%s
</message>`, a.Place, forMe, a.From, head(a.Text, maxMessage))}, true
	case proto.DecideFlaky:
		return question{verdicts: []string{proto.VerdictFlaky, proto.VerdictReal},
			prompt: fmt.Sprintf(`A check of a project failed (it exited with %d). From its output, did it fail by chance, or is something wrong?
- "flaky": the failure comes from timing, a timeout, the network, a port in use, the order tests run in, state shared between tests, the machine's resources or a race, not from the code under test: run again, it may pass.
- "real": the code or a test is wrong (an assertion that fails on values, a compile or type error, a missing file or symbol): run again, it fails the same way.

<command>
%s
</command>
<output>
%s
</output>`, a.Exit, head(a.Command, maxCommand), tail(a.Output, maxCheck))}, true
	}
	return question{}, false
}

// Keep returns the parts d keeps, in their order: those it numbered.
func Keep[T any](parts []T, d proto.Decision) []T {
	var out []T
	for i, p := range parts {
		if slices.Contains(d.Keep, i+1) {
			out = append(out, p)
		}
	}
	return out
}

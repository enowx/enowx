package agent

import (
	"regexp"
	"slices"
	"strings"
)

// What a screen shows an agent is stopped on, for any agent and wherever it
// runs, a shell tab included: no hook tells of every prompt (one in bypass
// mode, a startup dialog, a sign-in), and none runs in a shell started by hand.

// ScreenPrompt is what a screen asks.
type ScreenPrompt struct {
	Text     string // the line that asks
	Deadline string // the time it gives, as shown ("1:51"), "" for none
	Known    bool   // a kind listed in knownPrompts; else only the fallback's reading
}

// knownPrompts are lines that stop an agent, by kind, lower case.
var knownPrompts = regexp.MustCompile(`do you want to (proceed|continue|make this edit|allow|create|run|use)|` +
	`permission required|allow (once|always)|requires? (your )?approval|` +
	`approve (this|the) plan|would you like to proceed|` +
	`do you trust|trust (the|this) folder|new mcp server|mcp servers? .*(approve|enable)|` +
	`accept the (new )?terms|terms of service.*accept|` +
	`blocked by (auto|the classifier)|classifier blocked|` +
	`usage limit|rate limit reached|limit reached|out of credits|insufficient credits|upgrade your plan|` +
	`signed out|not logged in|please (log|sign) in|session expired|login expired|open (this|the) (url|link) .*(browser|sign)|waiting for (the )?browser|` +
	`update available|restart to update|press enter to (continue|update|restart)|` +
	`api error.*(press|retry\?)|failed.*press enter`)

// permissionPrompts are the known prompts that ask to allow something, which an
// answer settles: not a sign-in, a limit or an update.
var permissionPrompts = regexp.MustCompile(`(?i)do you want to (proceed|continue|make this edit|allow|create|run|use)|permission required|allow (once|always)|requires? (your )?approval`)

// PromptAsksPermission says the prompt text asks to allow something.
func PromptAsksPermission(text string) bool { return permissionPrompts.MatchString(text) }

// ScreenEnd is a screen's last n lines with something on them.
func ScreenEnd(screen string, n int) string { return strings.Join(tailLines(screen, n), "\n") }

// PromptKeys are the keys answering a prompt in agent id. A shell tab's agent is
// known by its screen: Claude Code's numbered menu is the one enowX knows.
func PromptKeys(id, screen string) ApproveKeys {
	if p, ok := Find(id); ok && p.Approve.Allow != "" {
		return p.Approve
	}
	if strings.Contains(screen, "1. Yes") {
		if p, ok := Find("claude"); ok {
			return p.Approve
		}
	}
	return ApproveKeys{}
}

// deadlineShown is a time a prompt counts down, as "automatically in 1:51".
var deadlineShown = regexp.MustCompile(`(?i)\b(?:in|within)\s+(\d{1,2}:\d{2}|\d+\s?s(?:ec(?:onds?)?)?)\b`)

// A choice under a question, as the selection marker or a numbered menu shows.
var (
	choiceLine = regexp.MustCompile(`^\s*(?:[│|]\s*)?(?:[❯›>▶●]\s*)?(?:\d+[.)]\s+\S|\[\s?[yn]\s?\])|\(y/n\)|\[y/n\]`)
	selectLine = regexp.MustCompile(`^\s*(?:[│|]\s*)?[❯›▶]\s+\S`)
)

// interruptedShown is what Claude Code leaves on the screen after an interrupt.
// It reads as a question, but what it waits for is the next prompt, and it
// stays there through what follows.
var interruptedShown = regexp.MustCompile(`(?i)interrupted\s*·\s*what should .+ do instead\?`)

// workingShown are what agents show while they work: no fallback then.
var workingShown = regexp.MustCompile(`(?i)esc (to )?interrupt|ctrl\+c to (interrupt|stop)|[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]`)

// PromptShown reads what screen asks, from its bottom, where an agent asks;
// ok false when it asks nothing listed.
func PromptShown(screen string) (ScreenPrompt, bool) {
	lines := tailLines(screen, 10)
	// Words like these are only talk in what an agent is busy writing.
	if workingShown.MatchString(strings.Join(lines, "\n")) {
		return ScreenPrompt{}, false
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if knownPrompts.MatchString(strings.ToLower(lines[i])) {
			return ScreenPrompt{Text: askedAbove(lines, i), Deadline: deadlineIn(lines), Known: true}, true
		}
	}
	return ScreenPrompt{}, false
}

// promptEdges are what boxes draw around a line.
const promptEdges = " │|┃"

// askedAbove is the question a matched line belongs to: itself, or, for a choice
// under a question, that question.
func askedAbove(lines []string, i int) string {
	for j := i; j >= 0 && j >= i-4; j-- {
		if strings.HasSuffix(strings.Trim(lines[j], promptEdges), "?") {
			return strings.Trim(lines[j], promptEdges)
		}
	}
	line := strings.Trim(lines[i], promptEdges)
	// Choices alone say nothing: what they're about is above them.
	if i > 0 && strings.Contains(strings.ToLower(line), "allow once") {
		return strings.Trim(lines[i-1], promptEdges) + ": " + line
	}
	return line
}

// What shows at a turn's end when no hook says so: the time it took ("Baked for 27s",
// "─ Worked for 1m 03s ─"), or the composer's prompt mark with nothing typed.
var (
	timeTaken   = regexp.MustCompile(`^[─━\-\s]*[✻✽✶✢*·•]?\s*[A-Z][a-z]+(ed|ing)? for \d+\s*[smh]`)
	promptAlone = regexp.MustCompile(`^[│|┃]?\s*[❯›>$%#]\s*[│|┃]?$`)
)

// turnOverLines is how far up a turn's end is looked for: an agent's footer alone
// (a status line, its running agents) can take ten lines.
const turnOverLines = 20

// InterruptShown says a screen's end shows a turn was interrupted.
func InterruptShown(screen string) bool {
	return strings.Contains(strings.Join(tailLines(screen, 12), "\n"), "Interrupted")
}

// TurnOverShown says a screen, not at work and not asking, shows an agent whose
// turn has ended. Only what follows its end can say otherwise: above it is what the
// turn said, in any words ("the usage limit is...", "shall we?"), and under it a
// footer's words are only that ("Update available").
func TurnOverShown(screen string) bool {
	lines := tailLines(screen, turnOverLines)
	if i := lastMatch(lines, timeTaken); i >= 0 {
		return !asksOrWorks(lines[i+1:])
	}
	i := lastMatch(lines, promptAlone)
	if i < 0 || i > 0 && strings.HasSuffix(strings.Trim(lines[i-1], promptEdges), "?") {
		return false // a question right above a bare mark is a program reading its answer
	}
	return PromptFree(strings.Join(lines[max(0, i-3):i+1], "\n")) && !asksOrWorks(lines[i+1:])
}

// lastMatch is the last of lines re matches, -1 for none.
func lastMatch(lines []string, re *regexp.Regexp) int {
	for i := len(lines) - 1; i >= 0; i-- {
		if re.MatchString(lines[i]) {
			return i
		}
	}
	return -1
}

// asksOrWorks says lines under a turn's end show it has moved on: an agent at work,
// or asking to allow something, or picking from a menu.
func asksOrWorks(lines []string) bool {
	text := strings.Join(lines, "\n")
	if workingShown.MatchString(text) || permissionPrompts.MatchString(text) {
		return true
	}
	_, asks := StalledPrompt(text)
	return asks
}

// PromptFree says a screen shows an agent neither at work nor asking anything:
// what it is given next, it takes. The caller says it has been still for long
// enough, and that nothing runs.
func PromptFree(screen string) bool {
	if strings.TrimSpace(screen) == "" || workingShown.MatchString(strings.Join(tailLines(screen, 12), "\n")) {
		return false
	}
	if _, ok := PromptShown(screen); ok {
		return false
	}
	_, ok := StalledPrompt(screen)
	return !ok
}

// StalledPrompt is the fallback for prompts nobody listed: a screen that hasn't
// changed, not saying it works, whose end is a question or a menu to pick from.
// The caller says how long it has been still: a long command prints nothing
// either, but ends on no question.
func StalledPrompt(screen string) (ScreenPrompt, bool) {
	lines := slices.DeleteFunc(tailLines(screen, 12), interruptedShown.MatchString)
	if len(lines) == 0 || workingShown.MatchString(strings.Join(lines, "\n")) {
		return ScreenPrompt{}, false
	}
	choices, question := 0, ""
	for i, line := range lines {
		// A selection is a menu's with more lines under it, as a composer's
		// prompt mark on its own isn't.
		if choiceLine.MatchString(line) || selectLine.MatchString(line) && i < len(lines)-1 {
			choices++
		}
		if t := strings.Trim(line, " │|"); strings.HasSuffix(t, "?") {
			question = t
		}
	}
	last := strings.Trim(lines[len(lines)-1], " │|")
	switch {
	case question != "" && choices >= 1:
	case strings.HasSuffix(last, "?") || strings.Contains(strings.ToLower(last), "(y/n)") || strings.Contains(strings.ToLower(last), "[y/n]"):
		question = last
	case choices >= 1 && menuBelow(lines):
		question = "A menu waits for a choice: " + last
	default:
		return ScreenPrompt{}, false
	}
	return ScreenPrompt{Text: question, Deadline: deadlineIn(lines)}, true
}

// menuBelow says a selection has at least two other short lines under it.
func menuBelow(lines []string) bool {
	for i, l := range lines {
		if selectLine.MatchString(l) && len(lines)-1-i >= 2 {
			return len(lines[i+1]) < 40 && len(lines[i+2]) < 40 && !strings.Contains(lines[i+1], "shortcuts")
		}
	}
	return false
}

func deadlineIn(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		low := strings.ToLower(lines[i])
		if !strings.Contains(low, "automatic") && !strings.Contains(low, "timeout") && !strings.Contains(low, "expire") && !strings.Contains(low, "block") {
			continue
		}
		if m := deadlineShown.FindStringSubmatch(lines[i]); m != nil {
			return m[1]
		}
	}
	return ""
}

// tailLines are screen's last n lines with text, trimmed, oldest first.
func tailLines(screen string, n int) []string {
	var out []string
	all := strings.Split(screen, "\n")
	for i := len(all) - 1; i >= 0 && len(out) < n; i-- {
		if t := strings.TrimSpace(all[i]); t != "" {
			out = append([]string{t}, out...)
		}
	}
	return out
}

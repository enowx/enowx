package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/enowx/enowx/core/platformagent/record"
)

// Modes in Shift+Tab's order; the last only when bypass is allowed. Shown is what
// enowX's Settings say the screen shows while the mode is on (agent.Choice.Shown).
var modeCycle = []struct{ id, shown string }{
	{record.ModeDefault, "ask first mode on"},
	{record.ModeAcceptEdits, "accept edits on"},
	{record.ModePlan, "plan mode on"},
	{record.ModeBypass, "bypass permissions on"},
}

// modeShown is what the status says of mode.
func modeShown(mode string) string {
	for _, m := range modeCycle {
		if m.id == mode {
			return m.shown
		}
	}
	return modeCycle[0].shown
}

// cycleMode goes to the next mode.
func (t *Terminal) cycleMode() {
	at := 0
	for i, m := range modeCycle {
		if m.id == t.view.mode {
			at = i
		}
	}
	next := modeCycle[(at+1)%len(modeCycle)]
	if next.id == record.ModeBypass && !t.cfg.AllowBypass {
		next = modeCycle[0]
	}
	t.setMode(next.id)
}

// setMode changes the mode and says so. The footer says it in a terminal; piped output
// has none.
func (t *Terminal) setMode(id string) {
	if err := t.r.SetMode(id); err != nil {
		t.sc.Line(t.sc.paint(styleRed, "! "+oneLine(err.Error())))
		return
	}
	t.refresh()
	if !t.cfg.Interactive {
		t.sc.Line(modeShown(t.view.mode))
	}
}

// parseSlash reads a line as a command: a slash, a name, the rest. A line starting with
// a path isn't one: the app's composer sends attached files' paths as messages.
func parseSlash(text string) (name, arg string, ok bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	first, rest, _ := strings.Cut(text, " ")
	if nl := strings.IndexByte(first, '\n'); nl >= 0 {
		first, rest = first[:nl], first[nl+1:]+" "+rest
	}
	if strings.Contains(first[1:], "/") || strings.Contains(first[1:], ".") {
		return "", "", false
	}
	return first[1:], strings.TrimSpace(rest), true
}

const slashHelp = `/help               this list
/model NAME         the model, from the next request
/effort LEVEL       the effort, from the next request
/mode NAME          default, acceptEdits, plan or bypassPermissions (Shift+Tab cycles)
/compact            summarise the conversation now
/clear              start a new conversation
/cost               dollars spent so far
/budget USD         how much the session may spend before asking; 0 for no limit
/context            how much of the model's window the conversation fills
/system-prompt PATH the system prompt from a file, for this session
/quit               leave
Esc interrupts, Ctrl+C twice leaves, Ctrl+D leaves on an empty line.`

// systemPromptSetter is a Runner that can change its system prompt as it runs.
type systemPromptSetter interface {
	SetSystemPrompt(string) error
}

// slash runs a command of the terminal's: the model never sees these.
func (t *Terminal) slash(name, arg string) {
	fail := func(format string, args ...any) {
		t.sc.Line(t.sc.paint(styleRed, "! "+fmt.Sprintf(format, args...)))
	}
	say := func(text string) { t.sc.Line(text) }
	switch name {
	case "help":
		say(slashHelp)
	case "model":
		if arg == "" {
			say("Model: " + t.view.model)
			return
		}
		if err := t.r.SetModel(arg); err != nil {
			fail("/model: %s", oneLine(err.Error()))
			return
		}
		t.refresh()
		say("Model: " + t.view.model)
	case "effort":
		if arg == "" {
			say("Effort: " + t.view.effort)
			return
		}
		if err := t.r.SetEffort(arg); err != nil {
			fail("/effort: %s", oneLine(err.Error()))
			return
		}
		t.refresh()
		say("Effort: " + t.view.effort)
	case "mode":
		t.modeCommand(arg, fail, say)
	case "compact":
		t.enqueue(job{compact: true})
	case "clear":
		if t.job != nil {
			fail("/clear: wait for the turn, or interrupt it (Esc)")
			return
		}
		t.pushAsk(&yesNoAsk{
			text:   "This forgets the conversation here (its file stays). Do you want to continue? (y/n)",
			answer: func(yes bool) { t.clearAnswered(yes) },
		})
	case "cost":
		t.refresh()
		line := fmt.Sprintf("Spent $%.2f", t.view.cost)
		if t.view.budget > 0 {
			line += fmt.Sprintf(" of a $%.2f budget", t.view.budget)
		}
		say(line)
	case "budget":
		t.budgetCommand(arg, fail, say)
	case "context":
		t.refresh()
		if t.view.window == 0 {
			say(fmt.Sprintf("Context: %d tokens", t.view.tokens))
			return
		}
		say(fmt.Sprintf("Context: %d of %d tokens (%d%%)", t.view.tokens, t.view.window, t.view.tokens*100/t.view.window))
	case "system-prompt":
		t.systemPromptCommand(arg, fail, say)
	case "quit", "exit":
		t.quit()
	default:
		fail("Unknown command /%s: /help lists them", name)
	}
}

func (t *Terminal) modeCommand(arg string, fail func(string, ...any), say func(string)) {
	if arg == "" {
		say("Mode: " + t.view.mode)
		return
	}
	for _, m := range modeCycle {
		if m.id != arg {
			continue
		}
		if m.id == record.ModeBypass && !t.cfg.AllowBypass {
			fail("/mode: bypassPermissions needs --allow-dangerously-skip-permissions")
			return
		}
		t.setMode(m.id)
		return
	}
	fail("/mode: unknown mode %q: default, acceptEdits, plan or bypassPermissions", arg)
}

func (t *Terminal) budgetCommand(arg string, fail func(string, ...any), say func(string)) {
	if arg == "" {
		say(fmt.Sprintf("Budget: $%.2f", t.view.budget))
		return
	}
	usd, err := strconv.ParseFloat(strings.TrimPrefix(arg, "$"), 64)
	if err != nil || usd < 0 {
		fail("/budget: %q isn't a number of dollars", arg)
		return
	}
	t.r.SetBudget(usd)
	t.refresh()
	say(fmt.Sprintf("Budget: $%.2f", usd))
}

func (t *Terminal) systemPromptCommand(arg string, fail func(string, ...any), say func(string)) {
	if arg == "" {
		fail("/system-prompt: which file?")
		return
	}
	setter, ok := t.r.(systemPromptSetter)
	if !ok {
		fail("/system-prompt: this agent can't change it while it runs: start it with --system-prompt-file")
		return
	}
	path := arg
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, rest)
		}
	}
	text, err := os.ReadFile(path)
	if err != nil {
		fail("/system-prompt: %s", oneLine(err.Error()))
		return
	}
	if err := setter.SetSystemPrompt(string(text)); err != nil {
		fail("/system-prompt: %s", oneLine(err.Error()))
		return
	}
	say("System prompt: " + path)
}

// clearAnswered does /clear once it was confirmed.
func (t *Terminal) clearAnswered(yes bool) {
	if !yes {
		t.sc.Line(t.sc.paint(styleDim, "Kept the conversation"))
		return
	}
	if err := t.r.Clear(); err != nil {
		t.sc.Line(t.sc.paint(styleRed, "! /clear: "+oneLine(err.Error())))
		return
	}
	t.refresh()
	t.sc.Line(t.sc.paint(styleDim, "New conversation"))
}

package transcript

import (
	"os"
	"strings"
)

// settingsWindows are how much of a transcript's end LastSettings reads, in
// turn: an agent says its model with each answer, so the end nearly always
// holds it, and a long conversation isn't read whole every time it grows.
var settingsWindows = []int64{256 << 10, 4 << 20, 32 << 20}

// Settings are what an agent last said it runs on in its transcript; "" for what it doesn't say.
type Settings struct {
	Model, Effort, Mode string
	ModelChosen         string // Reader.ModelChosen
}

// LastSettings is what agent last said in its transcript at path, read from its end.
func LastSettings(agent, path string) Settings { return LastSettingsSince(agent, path, 0) }

// LastSettingsSince is LastSettings of what the transcript says past its first floor bytes: a
// session switched to another model leaves what came before behind.
func LastSettingsSince(agent, path string, floor int64) Settings {
	info, err := os.Stat(path)
	if err != nil {
		return Settings{}
	}
	if floor > info.Size() {
		floor = 0 // it started over
	}
	var out Settings
	for _, window := range settingsWindows {
		r := Open(agent, path)
		if r == nil {
			return Settings{}
		}
		// A line cut by the window's start doesn't parse, and is skipped.
		r.offset = max(info.Size()-window, floor)
		if err := r.Update(); err != nil {
			return Settings{}
		}
		out = Settings{Model: r.Model, Effort: r.Effort, Mode: r.Mode, ModelChosen: r.ModelChosen}
		if out.Model != "" || window >= info.Size()-floor {
			return out
		}
	}
	return out
}

// TellsModelChoice says agent's transcript tells a model the user chose in it from one it
// moved to by itself. Claude Code records /model's answer; on the others, Codex's turn_context
// and OMP's model_change among them, both read the same.
func TellsModelChoice(agent string) bool {
	return Supports(agent) && strings.Contains(agent, "claude")
}

// SameModel says two names are one model as far as can be told: an agent records
// "claude-opus-5-5-20261101" for "claude-opus-5-5", "Opus 5.5" and "anthropic/claude-opus-5-5[1m]".
func SameModel(a, b string) bool {
	x, y := modelKey(a), modelKey(b)
	return x != "" && y != "" && (strings.Contains(x, y) || strings.Contains(y, x))
}

func modelKey(name string) string {
	name, _, _ = strings.Cut(strings.ToLower(strings.TrimSpace(name)), "[")
	name = name[strings.LastIndex(name, "/")+1:]
	return strings.NewReplacer(" ", "-", ".", "-", "_", "-").Replace(name)
}

// ModelStart is what a session counts as started on, after a look at its transcript: started
// as it was, until the user chooses another in the agent. An alias, as "opus" or "default",
// names no version to compare, so it becomes whatever the agent first says it means.
func ModelStart(started string, s Settings) string {
	if s.ModelChosen != "" && !SameModel(started, s.ModelChosen) {
		started = s.ModelChosen
	}
	if started == "" || !strings.ContainsAny(started, "0123456789") || SameModel(started, s.Model) {
		return s.Model
	}
	return started
}

package transcript

import "os"

// settingsWindows are how much of a transcript's end LastSettings reads, in
// turn: an agent says its model with each answer, so the end nearly always
// holds it, and a long conversation isn't read whole every time it grows.
var settingsWindows = []int64{256 << 10, 4 << 20, 32 << 20}

// LastSettings is the model and effort agent last said in its transcript at
// path, read from its end; "" for what it doesn't say.
func LastSettings(agent, path string) (model, effort, mode string) {
	info, err := os.Stat(path)
	if err != nil {
		return "", "", ""
	}
	for _, window := range settingsWindows {
		r := Open(agent, path)
		if r == nil {
			return "", "", ""
		}
		// A line cut by the window's start doesn't parse, and is skipped.
		r.offset = max(info.Size()-window, 0)
		if err := r.Update(); err != nil {
			return "", "", ""
		}
		model, effort, mode = r.Model, r.Effort, r.Mode
		if model != "" || r.offset-window <= 0 && window >= info.Size() {
			return model, effort, mode
		}
	}
	return model, effort, mode
}

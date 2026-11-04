package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ResolvePath is the absolute path a tool acts on for p, relative to dir. The
// permission engine judges the same path the tool then uses, so both come through here.
func ResolvePath(dir, p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

// shownPath is abs as the model and the user read it: from dir when inside it.
func shownPath(dir, abs string) string {
	if dir == "" {
		return abs
	}
	rel, err := filepath.Rel(dir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return abs
	}
	return filepath.ToSlash(rel)
}

// decode reads a tool's input.
func decode[T any](input json.RawMessage) (T, error) {
	var in T
	if err := json.Unmarshal(input, &in); err != nil {
		return in, fmt.Errorf("invalid input: %w", err)
	}
	return in, nil
}

// failf is a call that failed, its text telling the model why.
func failf(format string, args ...any) Result {
	return Result{Text: fmt.Sprintf(format, args...), Failed: true}
}

// errorResult is a walk that ended on err: the user's interrupting it, or a failure.
func errorResult(ctx context.Context, err error) Result {
	if ctx.Err() != nil {
		return failf("interrupted")
	}
	return failf("%v", err)
}

// oneLine fits s on a line of at most most bytes, showing where its lines broke: a
// permission prompt built from it must not hide that a command goes on.
func oneLine(s string, most int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", " ↵ ")
	return clip(s, most, "…")
}

// clip cuts s to at most most bytes on a character's boundary, marking the cut.
func clip(s string, most int, mark string) string {
	if len(s) <= most {
		return s
	}
	i := most
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i] + mark
}

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// An edit reads the whole file, and writes it whole.
const editMost = 32 << 20

const editSchema = `{"type":"object","properties":{"file_path":{"type":"string","description":"The file to edit, absolute or relative to the working folder"},"old_string":{"type":"string","description":"The exact text to replace, whitespace included"},"new_string":{"type":"string","description":"The text to put in its place"},"replace_all":{"type":"boolean","description":"Replace every occurrence (default false: old_string must occur once)"}},"required":["file_path","old_string","new_string"],"additionalProperties":false}`

type editTool struct{}

func (editTool) Name() string { return "Edit" }
func (editTool) Description() string {
	return "Replace exact text in a file. old_string must occur exactly once unless replace_all is set: give enough surrounding lines to make it unique. The file's line endings are kept."
}
func (editTool) Schema() json.RawMessage { return json.RawMessage(editSchema) }
func (editTool) ReadOnly() bool          { return false }
func (editTool) Edits() bool             { return true }

type editInput struct {
	FilePath   string  `json:"file_path"`
	OldString  *string `json:"old_string"`
	NewString  *string `json:"new_string"`
	ReplaceAll bool    `json:"replace_all"`
}

func (editTool) Detail(input json.RawMessage) string {
	in, err := decode[editInput](input)
	if err != nil {
		return "Edit"
	}
	return oneLine(in.FilePath, 200)
}

func (editTool) Run(_ context.Context, input json.RawMessage, env Env) Result {
	in, err := decode[editInput](input)
	if err != nil {
		return failf("%v", err)
	}
	if in.FilePath == "" || in.OldString == nil || in.NewString == nil {
		return failf("file_path, old_string and new_string are required")
	}
	oldText, newText := *in.OldString, *in.NewString
	if oldText == "" {
		return failf("old_string is empty: to write a whole file, use Write")
	}
	if oldText == newText {
		return failf("old_string and new_string are the same: nothing to change")
	}
	abs := ResolvePath(env.Dir, in.FilePath)
	shown := shownPath(env.Dir, abs)
	target, info, err := writeTarget(abs)
	switch {
	case err != nil:
		return failf("edit %s: %v", shown, err)
	case info == nil:
		return failf("%s doesn't exist: to create it, use Write", shown)
	case info.Size() > editMost:
		return failf("%s is %d bytes, too large to edit", shown, info.Size())
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return failf("edit %s: %v", shown, err)
	}
	content := string(data)
	if looksBinary(data[:min(len(data), sniffSize)]) {
		return failf("%s is a binary file", shown)
	}
	from, to, count := matchEnding(content, oldText, newText)
	switch {
	case count == 0:
		return failf("old_string isn't in %s. %s", shown, whyNoMatch(content, oldText))
	case count > 1 && !in.ReplaceAll:
		return failf("old_string occurs %d times in %s, at lines %s: add surrounding lines to make it unique, or set replace_all", count, shown, matchLines(content, from))
	}
	if err := writeAtomic(target, []byte(strings.ReplaceAll(content, from, to)), info); err != nil {
		return failf("edit %s: %v", shown, err)
	}
	added, removed := lineChanges(toLF(oldText), toLF(newText))
	noun := "occurrence"
	if count != 1 {
		noun += "s"
	}
	return Result{
		Text: fmt.Sprintf("Replaced %d %s in %s", count, noun, shown),
		Old:  clip(toLF(oldText), diffClip, "\n…"), New: clip(toLF(newText), diffClip, "\n…"),
		Added: added * count, Removed: removed * count,
	}
}

// matchEnding is the text to replace and its replacement in the file's line endings,
// and how often it occurs: models write \n whatever a file has.
func matchEnding(content, oldText, newText string) (from, to string, count int) {
	if crlf := strings.Count(content, "\r\n"); crlf*2 > strings.Count(content, "\n") {
		from, to = toCRLF(oldText), toCRLF(newText)
		if count = strings.Count(content, from); count > 0 {
			return from, to, count
		}
		// A file of mixed endings may hold the text with bare \n.
		return oldText, newText, strings.Count(content, oldText)
	}
	if count = strings.Count(content, oldText); count > 0 {
		return oldText, newText, count
	}
	from, to = toLF(oldText), toLF(newText)
	return from, to, strings.Count(content, from)
}

func toLF(s string) string   { return strings.ReplaceAll(s, "\r\n", "\n") }
func toCRLF(s string) string { return strings.ReplaceAll(toLF(s), "\n", "\r\n") }

// matchLines says where the first few occurrences of from start.
func matchLines(content, from string) string {
	var lines []string
	at, line := 0, 1
	for len(lines) < 5 {
		i := strings.Index(content[at:], from)
		if i < 0 {
			return strings.Join(lines, ", ")
		}
		line += strings.Count(content[at:at+i], "\n")
		lines = append(lines, strconv.Itoa(line))
		line += strings.Count(from, "\n")
		at += i + len(from)
	}
	if strings.Contains(content[at:], from) {
		lines = append(lines, "…")
	}
	return strings.Join(lines, ", ")
}

// whyNoMatch says how near the file comes to the text, ignoring lines' indentation, so
// the model needn't read the file again to see what differs.
func whyNoMatch(content, oldText string) string {
	have := strings.Split(toLF(content), "\n")
	want := strings.Split(strings.TrimLeft(toLF(oldText), "\n"), "\n")
	best, bestLen := -1, 0
	for i := range have {
		n := 0
		for n < len(want) && i+n < len(have) && strings.TrimSpace(have[i+n]) == strings.TrimSpace(want[n]) {
			n++
		}
		if n > bestLen {
			best, bestLen = i, n
		}
	}
	if best < 0 {
		return fmt.Sprintf("No line of the file reads like its first line, %q: read the file again.", oneLine(want[0], 120))
	}
	if bestLen < len(want) {
		return fmt.Sprintf("Line %d starts like it, but line %d differs: the file has %q where you gave %q.",
			best+1, best+bestLen+1, fileLineAt(have, best+bestLen), oneLine(want[bestLen], 120))
	}
	for n := range want {
		if have[best+n] != want[n] {
			return fmt.Sprintf("It's at line %d but for whitespace: the file has %q on line %d where you gave %q.",
				best+1, oneLine(have[best+n], 120), best+n+1, oneLine(want[n], 120))
		}
	}
	return "It differs only in its line endings."
}

func fileLineAt(lines []string, i int) string {
	if i >= len(lines) {
		return "(the end of the file)"
	}
	return oneLine(lines[i], 120)
}

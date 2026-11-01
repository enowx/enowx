package chatview

import (
	"fmt"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// groupFrom is how many tool calls in a row fold into one line: fewer
// read fine one by one.
const groupFrom = 3

// step is a line of a turn's work: a remark or thought, or tool calls made
// one after another.
type step struct {
	entry *proto.TranscriptEntry // a remark or a thought
	tools []proto.TranscriptEntry
}

func workSteps(work []proto.TranscriptEntry) []step {
	var out []step
	for i := range work {
		e := &work[i]
		if e.Kind != proto.TranscriptTool {
			out = append(out, step{entry: e})
			continue
		}
		if n := len(out); n > 0 && out[n-1].entry == nil {
			out[n-1].tools = append(out[n-1].tools, *e)
			continue
		}
		out = append(out, step{tools: []proto.TranscriptEntry{*e}})
	}
	return out
}

// toolKind sorts a tool call by what it did, so a run of them sums up as
// "Read 3 files, searched 2 times". Shell commands count by their program.
type toolKind int

const (
	kindRun toolKind = iota
	kindRead
	kindSearch
	kindList
	kindEdit
	kindOther
)

func kindOf(e proto.TranscriptEntry) toolKind {
	switch e.Text {
	case "Read", "NotebookRead":
		return kindRead
	case "Grep", "Glob", "WebSearch", "WebFetch":
		return kindSearch
	case "LS":
		return kindList
	case "Edit", "MultiEdit", "NotebookEdit", "Write":
		return kindEdit
	case "Bash":
		switch program(e.Detail) {
		case "cat", "sed", "head", "tail", "nl", "less", "bat", "wc":
			return kindRead
		case "rg", "grep", "find", "fd", "ag", "git grep":
			return kindSearch
		case "ls", "tree", "pwd":
			return kindList
		}
		return kindRun
	}
	return kindOther
}

// program is the command a shell line is about: the first of its steps
// that isn't a cd, pwd or echo, past environment assignments; "git grep"
// counts as its own.
func program(line string) string {
	first := ""
	for _, part := range strings.FieldsFunc(line, func(r rune) bool { return r == '&' || r == ';' || r == '|' }) {
		words := strings.Fields(part)
		for len(words) > 0 && strings.Contains(words[0], "=") {
			words = words[1:]
		}
		if len(words) == 0 {
			continue
		}
		name := path.Base(words[0])
		if name == "git" && len(words) > 1 && words[1] == "grep" {
			name = "git grep"
		}
		if first == "" {
			first = name
		}
		switch name {
		case "cd", "pwd", "echo":
			continue
		}
		return name
	}
	return first
}

// shellLook is the icon and verb of a shell command, by what it does.
func shellLook(e proto.TranscriptEntry) (*ui.SVG, string) {
	switch kindOf(e) {
	case kindRead:
		return icon.File, "Read"
	case kindSearch:
		return icon.Search, "Search"
	case kindList:
		return icon.Folder, "List"
	}
	if program(e.Detail) == "git" {
		return icon.Git, "Git"
	}
	return icon.Terminal, "Run"
}

// summary sums up tool calls: "Read 3 files, searched 2 times, ran 1
// command".
func summary(tools []proto.TranscriptEntry) string {
	var n [kindOther + 1]int
	for _, e := range tools {
		n[kindOf(e)]++
	}
	var parts []string
	add := func(count int, one, many string) {
		if count == 0 {
			return
		}
		if len(parts) > 0 {
			one, many = strings.ToLower(one[:1])+one[1:], strings.ToLower(many[:1])+many[1:]
		}
		text := plural(count, one, many)
		if strings.Contains(text, "%d") {
			text = fmt.Sprintf(text, count)
		}
		parts = append(parts, text)
	}
	add(n[kindRead], "Read %d file", "Read %d files")
	add(n[kindSearch], "Searched once", "Searched %d times")
	add(n[kindList], "Listed %d folder", "Listed %d folders")
	add(n[kindEdit], "Edited %d file", "Edited %d files")
	add(n[kindRun], "Ran %d command", "Ran %d commands")
	add(n[kindOther], "Used %d tool", "Used %d tools")
	return strings.Join(parts, ", ")
}

// toolRow is one tool call: what it did, opening to its output or change.
// Calls that went well say nothing more; a failed one says so.
func (ch *Chat) toolRow(c *ui.Context, e proto.TranscriptEntry, latest bool) {
	svg, verb := toolLook(e.Text)
	if e.Text == "Bash" {
		svg, verb = shellLook(e)
	}
	status := func() {
		switch {
		case e.Added+e.Removed > 0:
			ui.Text(c, fmt.Sprintf("+%d", e.Added)).FontSize(theme.TextLabel).TextColor(theme.Done).SingleLine()
			ui.Text(c, fmt.Sprintf("−%d", e.Removed)).FontSize(theme.TextLabel).TextColor(theme.Error).SingleLine()
		case e.Failed:
			ui.Text(c, "failed").FontSize(theme.TextLabel).TextColor(theme.Error).SingleLine()
		case latest && !e.Done:
			widget.Spinner(c, 11)
		}
	}
	ch.row(c, e.Index, svg, verb, toolDetailShown(e), status, func() { ch.toolBody(c, e) })
}

// toolDetailShown is a call's detail as the row shows it: a file by its
// name, then its folder.
func toolDetailShown(e proto.TranscriptEntry) string {
	switch e.Text {
	case "Read", "Edit", "MultiEdit", "Write", "NotebookRead", "NotebookEdit":
		if strings.HasPrefix(e.Detail, "/") && !strings.Contains(e.Detail, ", ") {
			return path.Base(e.Detail) + "  " + path.Dir(e.Detail)
		}
	}
	return e.Detail
}

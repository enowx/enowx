package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	grepDefaultLimit = 250
	grepLimitMost    = 5000
	grepTextMost     = 64 << 10
	grepLineMost     = 500
	grepFileMost     = 16 << 20
	grepContextMost  = 50
)

const grepSchema = `{"type":"object","properties":{"pattern":{"type":"string","description":"The regular expression (RE2 syntax), matched line by line"},"path":{"type":"string","description":"The file or folder to search (default the working folder)"},"glob":{"type":"string","description":"Only files whose path matches this, such as *.go or src/**/*.ts"},"output_mode":{"type":"string","enum":["files_with_matches","content","count"],"description":"files_with_matches lists files (default), content shows matching lines, count says how many in each file"},"-i":{"type":"boolean","description":"Ignore case"},"-n":{"type":"boolean","description":"Show line numbers in content mode (default true)"},"-A":{"type":"integer","minimum":0,"description":"Lines to show after each match, in content mode"},"-B":{"type":"integer","minimum":0,"description":"Lines to show before each match, in content mode"},"-C":{"type":"integer","minimum":0,"description":"Lines to show on both sides of each match, in content mode"},"head_limit":{"type":"integer","minimum":1,"description":"The most lines of output (default 250)"}},"required":["pattern"],"additionalProperties":false}`

type grepTool struct{}

func (grepTool) Name() string { return "Grep" }
func (grepTool) Description() string {
	return "Search files' contents with a regular expression. Follows .gitignore, skips binary files and .git. Output is capped: narrow with path or glob, or raise head_limit."
}
func (grepTool) Schema() json.RawMessage { return json.RawMessage(grepSchema) }
func (grepTool) ReadOnly() bool          { return true }
func (grepTool) Edits() bool             { return false }

type grepInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	Glob       string `json:"glob"`
	OutputMode string `json:"output_mode"`
	IgnoreCase bool   `json:"-i"`
	Numbers    *bool  `json:"-n"`
	After      int    `json:"-A"`
	Before     int    `json:"-B"`
	Around     int    `json:"-C"`
	HeadLimit  int    `json:"head_limit"`
}

func (grepTool) Detail(input json.RawMessage) string {
	in, err := decode[grepInput](input)
	if err != nil {
		return "Grep"
	}
	if in.Path != "" {
		return oneLine(in.Pattern+" in "+in.Path, 200)
	}
	return oneLine(in.Pattern, 200)
}

func (grepTool) ReadPaths(input json.RawMessage, dir string) []string {
	in, err := decode[grepInput](input)
	if err != nil {
		return nil
	}
	return []string{ResolvePath(dir, cmpOr(in.Path, "."))}
}

func (grepTool) Run(ctx context.Context, input json.RawMessage, env Env) Result {
	in, err := decode[grepInput](input)
	if err != nil {
		return failf("%v", err)
	}
	if in.Pattern == "" {
		return failf("pattern is required")
	}
	expr := in.Pattern
	if in.IgnoreCase {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return failf("the pattern isn't a valid regular expression: %v", err)
	}
	s := &grepSearch{ctx: ctx, re: re, env: env, numbers: in.Numbers == nil || *in.Numbers}
	switch in.OutputMode {
	case "", "files_with_matches", "files":
		s.mode = "files"
	case "content", "count":
		s.mode = in.OutputMode
	default:
		return failf("output_mode %q isn't files_with_matches, content or count", in.OutputMode)
	}
	s.limit = grepDefaultLimit
	if in.HeadLimit > 0 {
		s.limit = min(in.HeadLimit, grepLimitMost)
	}
	s.before, s.after = in.Before, in.After
	if in.Around > 0 {
		s.before, s.after = in.Around, in.Around
	}
	s.before, s.after = min(max(s.before, 0), grepContextMost), min(max(s.after, 0), grepContextMost)

	root := ResolvePath(env.Dir, cmpOr(in.Path, "."))
	info, err := os.Stat(root)
	if err != nil {
		return failf("%s doesn't exist", shownPath(env.Dir, root))
	}
	if !info.IsDir() {
		// A file named outright is searched, ignored or filtered or not.
		s.file(root, info)
	} else {
		var filter *globPattern
		if in.Glob != "" {
			if filter, err = newGlob(in.Glob); err != nil {
				return failf("%v", err)
			}
		}
		err = walkFiles(ctx, root, true, func(abs, rel string, d fs.DirEntry) bool {
			if !d.Type().IsRegular() || filter != nil && !filter.match(rel) {
				return true
			}
			if info, err := d.Info(); err == nil {
				s.file(abs, info)
			}
			return !s.full
		})
		if err != nil {
			return errorResult(ctx, err)
		}
	}
	if ctx.Err() != nil {
		return failf("interrupted")
	}
	return Result{Text: s.text()}
}

// grepSearch is one call's search, run by its one goroutine.
type grepSearch struct {
	ctx           context.Context
	re            *regexp.Regexp
	env           Env
	mode          string
	numbers       bool
	before, after int
	limit         int

	out     strings.Builder
	lines   int
	full    bool
	tooBig  int
	matched bool
}

func (s *grepSearch) text() string {
	text := s.out.String()
	if !s.matched {
		text = "No matches found\n"
	}
	if s.full {
		text += fmt.Sprintf("[output capped at %d lines: narrow the search, or raise head_limit]\n", s.lines)
	}
	if s.tooBig > 0 {
		text += fmt.Sprintf("[%d files over %d MB not searched]\n", s.tooBig, grepFileMost>>20)
	}
	return text
}

// emit adds a line of output, false once the output is at its cap.
func (s *grepSearch) emit(line string) bool {
	if s.lines >= s.limit || s.out.Len() >= grepTextMost {
		s.full = true
		return false
	}
	s.out.WriteString(line)
	s.out.WriteByte('\n')
	s.lines++
	return true
}

// file searches one file, whole in memory: the cap keeps that small.
func (s *grepSearch) file(abs string, info os.FileInfo) {
	if s.ctx.Err() != nil || !info.Mode().IsRegular() {
		return
	}
	if info.Size() > grepFileMost {
		s.tooBig++
		return
	}
	data, err := os.ReadFile(abs)
	if err != nil || bytes.IndexByte(data[:min(len(data), sniffSize)], 0) >= 0 {
		return
	}
	shown := shownPath(s.env.Dir, abs)
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	matches := []int{}
	for i, line := range lines {
		if i%1024 == 1023 && s.ctx.Err() != nil {
			return
		}
		if s.re.Match(bytes.TrimSuffix(line, []byte("\r"))) {
			matches = append(matches, i)
			if s.mode == "files" {
				break
			}
		}
	}
	if len(matches) == 0 {
		return
	}
	wasMatched := s.matched
	s.matched = true
	switch s.mode {
	case "files":
		s.emit(shown)
	case "count":
		s.emit(shown + ":" + strconv.Itoa(len(matches)))
	default:
		s.content(shown, lines, matches, wasMatched)
	}
}

// content shows the matching lines and those around them, as grep does: path:line:text
// for a match, path-line-text beside one, -- between groups.
func (s *grepSearch) content(shown string, lines [][]byte, matches []int, separate bool) {
	isMatch := make(map[int]bool, len(matches))
	for _, m := range matches {
		isMatch[m] = true
	}
	next := 0 // the first line not yet shown
	for gi, m := range matches {
		from := max(m-s.before, next)
		gap := gi == 0 && separate || gi > 0 && from > next
		if gap && (s.before > 0 || s.after > 0) && !s.emit("--") {
			return
		}
		to := min(m+s.after, len(lines)-1)
		for i := from; i <= to; i++ {
			sep := "-"
			if isMatch[i] {
				sep = ":"
			}
			if !s.emit(s.lineText(shown, i, sep, lines[i])) {
				return
			}
		}
		next = max(next, to+1)
	}
}

func (s *grepSearch) lineText(shown string, i int, sep string, line []byte) string {
	text := strings.ToValidUTF8(string(bytes.TrimSuffix(line, []byte("\r"))), "\uFFFD")
	text = clip(text, grepLineMost, "…")
	if s.numbers {
		return shown + sep + strconv.Itoa(i+1) + sep + text
	}
	return shown + sep + text
}

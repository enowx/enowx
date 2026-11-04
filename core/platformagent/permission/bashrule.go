package permission

import (
	"regexp"
	"strings"
)

// splitCommand cuts a shell command into the simple commands it runs. ok is false when
// the text can't be judged by its words: a command substitution, a subshell, a redirect,
// a quote left open. Such a command matches no rule, and asks.
func splitCommand(command string) (parts []string, ok bool) {
	var part strings.Builder
	end := func() {
		if s := strings.TrimSpace(part.String()); s != "" {
			parts = append(parts, s)
		}
		part.Reset()
	}
	const (
		bare = iota
		single
		double
	)
	state := bare
	for i := 0; i < len(command); i++ {
		c := command[i]
		var next byte
		if i+1 < len(command) {
			next = command[i+1]
		}
		switch state {
		case single:
			if c == '\'' {
				state = bare
			}
		case double:
			switch {
			case c == '\\':
				part.WriteByte(c)
				i++
				c = next
			case c == '"':
				state = bare
			case c == '`', c == '$' && (next == '(' || next == '{'):
				return nil, false
			}
		default:
			switch {
			case c == '\\':
				// A line continued is a newline to the rule's eye, and bash's own business.
				if next == 0 || next == '\n' || next == '\r' {
					return nil, false
				}
				part.WriteByte(c)
				i++
				c = next
			case c == '\'':
				state = single
			case c == '"':
				state = double
			case c == '`', c == '<', c == '>', c == '(', c == ')':
				return nil, false
			case c == '$' && (next == '(' || next == '{' || next == '\'' || next == '"'):
				return nil, false
			case c == '#' && (part.Len() == 0 || isSpace(command[i-1])):
				// A comment's quotes aren't quotes, which could hide the next line from us.
				return nil, false
			case c == ';', c == '&', c == '|', c == '\n', c == '\r':
				end()
				continue
			}
		}
		part.WriteByte(c)
	}
	if state != bare {
		return nil, false
	}
	end()
	return parts, len(parts) > 0
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' }

// bashPatterns are the patterns of rules like Bash(git *).
func bashPatterns(rules []string) []string {
	var patterns []string
	for _, rule := range rules {
		if inner, ok := strings.CutPrefix(rule, bashTool+"("); ok {
			if inner, ok = strings.CutSuffix(inner, ")"); ok && inner != "" {
				patterns = append(patterns, inner)
			}
		}
	}
	return patterns
}

// bashRulesCover says every part of command matches one of the rules' patterns.
func bashRulesCover(rules []string, command string) bool {
	patterns := bashPatterns(rules)
	if len(patterns) == 0 {
		return false
	}
	parts, ok := splitCommand(command)
	if !ok {
		return false
	}
	for _, part := range parts {
		covered := false
		for _, pattern := range patterns {
			if patternMatches(pattern, part) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

// patternMatches reads a trailing * as a prefix on word boundaries, so "git *" takes
// "git" and "git status" but not "gitx"; without one the command must equal it.
func patternMatches(pattern, part string) bool {
	if prefix, ok := strings.CutSuffix(pattern, " *"); ok {
		return (part == prefix || strings.HasPrefix(part, prefix+" ")) && !reachesOut(prefix, part)
	}
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(part, prefix)
	}
	return part == pattern
}

var programWord = regexp.MustCompile(`^[A-Za-z0-9_./+-]+$`)

// boundedCommands are the only commands kept as a prefix: what they do is plainly bounded,
// whatever arguments follow, but for the flags listed, which run other programs or write
// where they say. Anything else is kept as the exact command: too many programs run others
// with the right arguments (setsid, stdbuf, flock, busybox, uv run, npm exec, docker run,
// go run, ...) for a list of those to be complete.
var boundedCommands = map[string][]string{
	"git status": nil,
	"git diff":   gitReachesOut,
	"git log":    gitReachesOut,
	"git show":   gitReachesOut,
	"go test":    goReachesOut,
	"go build":   goReachesOut,
	"go vet":     goReachesOut,
	"npm test":   {"--script-shell"},
	"make":       {"-f", "--file", "--makefile", "-C", "--directory", "--eval", "-E", "-I", "--include-dir"},
	"ls":         nil,
	"pwd":        nil,
}

var (
	gitReachesOut = []string{"--output", "--ext-diff", "--textconv"}
	goReachesOut  = []string{"-exec", "--exec", "-toolexec", "--toolexec", "-vettool", "--vettool", "-o", "--o", "-overlay", "--overlay", "-modfile", "--modfile"}
)

// reachesOut says part, kept under prefix, uses a flag the prefix doesn't vouch for, or for
// make sets a variable, which can be SHELL.
func reachesOut(prefix, part string) bool {
	fields := strings.Fields(part)
	for _, arg := range fields[min(len(strings.Fields(prefix)), len(fields)):] {
		if prefix == "make" && !strings.HasPrefix(arg, "-") && strings.Contains(arg, "=") {
			return true
		}
		for _, flag := range boundedCommands[prefix] {
			short := len(flag) == 2 // -fFILE is -f FILE
			if arg == flag || strings.HasPrefix(arg, flag+"=") || short && strings.HasPrefix(arg, flag) {
				return true
			}
		}
	}
	return false
}

// suggestBashRule is the narrowest useful rule for command: Bash(git status *) for what
// boundedCommands hold, else the exact command; nothing when a rule can't stand for it
// (several commands in one, a command that can't be judged). It never suggests a pattern
// that allows every command.
func suggestBashRule(command string) string {
	parts, ok := splitCommand(command)
	if !ok || len(parts) != 1 {
		return ""
	}
	part := parts[0]
	fields := strings.Fields(part)
	if !programWord.MatchString(fields[0]) {
		return "" // an assignment, a variable, a quoted name: what runs isn't what it says
	}
	for n := min(2, len(fields)); n > 0; n-- {
		prefix := strings.Join(fields[:n], " ")
		if _, bounded := boundedCommands[prefix]; bounded && !reachesOut(prefix, part) {
			return bashTool + "(" + prefix + " *)"
		}
	}
	return exactRule(part)
}

// exactRule keeps part as it is, unless a * in it would read as a wildcard ("rm *").
func exactRule(part string) string {
	if strings.ContainsAny(part, "*\n\r") {
		return ""
	}
	return bashTool + "(" + part + ")"
}

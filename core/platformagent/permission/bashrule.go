package permission

import (
	"path"
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
		return part == prefix || strings.HasPrefix(part, prefix+" ")
	}
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(part, prefix)
	}
	return part == pattern
}

var (
	programWord    = regexp.MustCompile(`^[A-Za-z0-9_./+-]+$`)
	subcommandWord = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:_-]*$`)
)

// programsWithSubcommands are kept with their subcommand: "git status" says what's being
// allowed, "git" all there is.
var programsWithSubcommands = wordSet("apt apt-get brew bun bundle cargo composer deno docker dotnet gh git go gradle helm kubectl make mvn npm npx pip pip3 pnpm poetry rails systemctl terraform uv yarn")

// programsAnyArgument are kept only as the exact command: with any arguments they run
// other programs, delete or overwrite anything, or send data out, so a prefix would allow
// all of it.
var programsAnyArgument = wordSet(". awk bash builtin chmod chown command cp curl dash dd doas env eval exec find fish ksh mkfs mv nc ncat nice node nohup perl php python python2 python3 rm rsync ruby scp sed sh ssh sudo su tee time timeout watch wget xargs zsh")

func wordSet(words string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(words) {
		set[w] = true
	}
	return set
}

// suggestBashRule is the narrowest useful rule for command: Bash(git status *) or
// Bash(ls *), the exact command for a program whose arguments can do anything, nothing
// when a rule can't stand for it (several commands in one, a command that can't be
// judged). It never suggests a pattern that allows every command.
func suggestBashRule(command string) string {
	parts, ok := splitCommand(command)
	if !ok || len(parts) != 1 {
		return ""
	}
	part := parts[0]
	fields := strings.Fields(part)
	program := fields[0]
	if !programWord.MatchString(program) {
		return "" // an assignment, a variable, a quoted name: what runs isn't what it says
	}
	switch base := path.Base(program); {
	case programsAnyArgument[base]:
		return exactRule(part)
	case programsWithSubcommands[base]:
		if len(fields) > 1 && subcommandWord.MatchString(fields[1]) {
			return bashTool + "(" + program + " " + fields[1] + " *)"
		}
		return exactRule(part)
	}
	return bashTool + "(" + program + " *)"
}

// exactRule keeps part as it is, unless a * in it would read as a wildcard ("rm *").
func exactRule(part string) string {
	if strings.ContainsAny(part, "*\n\r") {
		return ""
	}
	return bashTool + "(" + part + ")"
}

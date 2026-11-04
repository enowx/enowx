package hooks

import (
	"regexp"
	"strings"
)

// What trimming a command's output may wrap. The wrapper runs a command in a
// shell of its own, so what it changes of the agent's shell would be lost, and it
// holds the output until the command ends, so what never ends would show nothing.

// shellLead is a statement before a command that changes the agent's shell: it stays
// outside the wrapper, for the shell to keep what it does. A plain variable isn't one:
// outside, the command in the wrapper wouldn't see it.
var shellLead = regexp.MustCompile(`^\s*(cd|pushd|export)\s`)

// changesShell finds statements changing the shell further on in a command.
var changesShell = regexp.MustCompile(`(^|[;&|(\n]\s*|\b(do|then|else)\s+)(cd|pushd|popd|export|unset|source|\.|alias|unalias|set|ulimit|umask|exec|eval)(\s|$)`)

// neverEnds finds what runs on, is sent to the background, or reads the terminal.
var neverEnds = regexp.MustCompile(`(?m)&\s*$|\bnohup\b|\bdisown\b|\btail\s+-[a-zA-Z]*[fF]|\b(npm|pnpm|yarn|bun)\s+(run\s+)?(dev|start|serve|watch)\b|\b(docker\s+compose|docker-compose)\s+up\b|\bvite\b|\bnext\s+dev\b|\bwatch\b|\bserve\b|\bvim?\b|\bnano\b|\bless\b|\bmore\b|\btop\b|\bhtop\b|\bssh\b|\bpython3?\s*$|\bnode\s*$`)

// exactRead finds commands whose whole output the agent asked for: a file's
// lines, a change. Theirs is cut only when the judge is very sure.
var exactRead = regexp.MustCompile(`^\s*(cat|bat|sed\s+-n|head|tail|nl|xxd|od|wc|git\s+(diff|show|blame)|gh\s+pr\s+diff|jq|yq|awk)\b`)

// ExactRead says command asks for exact content.
func ExactRead(command string) bool {
	_, tail := splitLead(command)
	return exactRead.MatchString(tail)
}

// splitLead parts a command's leading statements that change the shell from what
// follows them. A statement is read as the shell reads it, quotes kept whole: only a
// plain one on the first line is a lead, so a heredoc's lines or a loop's never part.
func splitLead(command string) (lead, tail string) {
	tail = command
	for {
		n := leadLen(tail)
		if n == 0 {
			return lead, tail
		}
		lead += tail[:n]
		tail = tail[n:]
	}
}

// leadLen is how long the plain statement starting s is, with the `;` or `&&` after it
// and the spaces up to what follows; 0 when s doesn't start with one.
func leadLen(s string) int {
	if !shellLead.MatchString(s) {
		return 0
	}
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case c == '\\':
			i++
		case quote == '"':
			if c == '"' {
				quote = 0
			} else if c == '`' || c == '$' && i+1 < len(s) && s[i+1] == '(' {
				return 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == ';' || c == '&' && i+1 < len(s) && s[i+1] == '&':
			if c == '&' {
				i++
			}
			i++
			for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
				i++
			}
			return i
		case strings.IndexByte("\n&|<>()`#{}", c) >= 0 || c == '$' && i+1 < len(s) && s[i+1] == '(':
			return 0
		}
	}
	return 0
}

// WrapTrim is command with its output trimmed by bin, what changes the agent's
// shell left outside; false when wrapping it would change what it does.
func WrapTrim(bin, command string) (string, bool) {
	if strings.Contains(command, trimPrefix) {
		return "", false
	}
	lead, tail := splitLead(command)
	if strings.TrimSpace(tail) == "" || !quotable(tail) || changesShell.MatchString(tail) || neverEnds.MatchString(tail) {
		return "", false
	}
	return lead + shellWord(bin) + trimPrefix + shellQuote(tail), true
}

// plainWord is what a shell reads as it is.
var plainWord = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

// shellWord is s as one word of a command line: as it is where that's safe, so that the
// command an agent's log shows reads as the one it ran.
func shellWord(s string) string {
	if plainWord.MatchString(s) {
		return s
	}
	return shellQuote(s)
}

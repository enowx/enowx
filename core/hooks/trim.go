package hooks

import (
	"regexp"
	"strings"
)

// What trimming a command's output may wrap. The wrapper runs a command in a
// shell of its own, so what it changes of the agent's shell would be lost, and it
// holds the output until the command ends, so what never ends would show nothing.

// shellLead are the statements before a command that change the agent's shell:
// they stay outside the wrapper, for the shell to keep what they do.
var shellLead = regexp.MustCompile(`^\s*((cd|pushd)\s+[^;&|]+|export\s+[^;&|]+|[A-Za-z_][A-Za-z0-9_]*=[^\s;&|]*)\s*(&&|;)\s*`)

// changesShell finds statements changing the shell further on in a command.
var changesShell = regexp.MustCompile(`(^|[;&|(]\s*)(cd|pushd|popd|export|unset|source|\.|alias|unalias|set|ulimit|umask|exec|eval)(\s|$)`)

// neverEnds finds what runs on, is sent to the background, or reads the terminal.
var neverEnds = regexp.MustCompile(`&\s*$|\bnohup\b|\bdisown\b|\btail\s+-[a-zA-Z]*[fF]|\b(npm|pnpm|yarn|bun)\s+(run\s+)?(dev|start|serve|watch)\b|\b(docker\s+compose|docker-compose)\s+up\b|\bvite\b|\bnext\s+dev\b|\bwatch\b|\bserve\b|\bvim?\b|\bnano\b|\bless\b|\bmore\b|\btop\b|\bhtop\b|\bssh\b|\bpython3?\s*$|\bnode\s*$`)

// exactRead finds commands whose whole output the agent asked for: a file's
// lines, a change. Theirs is cut only when the judge is very sure.
var exactRead = regexp.MustCompile(`^\s*(cat|bat|sed\s+-n|head|tail|nl|xxd|od|wc|git\s+(diff|show|blame)|gh\s+pr\s+diff|jq|yq|awk)\b`)

// ExactRead says command asks for exact content.
func ExactRead(command string) bool {
	_, tail := splitLead(command)
	return exactRead.MatchString(tail)
}

// splitLead parts a command's leading statements that change the shell from
// what follows them.
func splitLead(command string) (lead, tail string) {
	tail = command
	for {
		m := shellLead.FindString(tail)
		if m == "" {
			return lead, tail
		}
		lead += m
		tail = tail[len(m):]
	}
}

// WrapTrim is command with its output trimmed by bin, what changes the agent's
// shell left outside; false when wrapping it would change what it does.
func WrapTrim(bin, command string) (string, bool) {
	if strings.Contains(command, trimPrefix) {
		return "", false
	}
	lead, tail := splitLead(command)
	if strings.TrimSpace(tail) == "" || changesShell.MatchString(tail) || neverEnds.MatchString(tail) {
		return "", false
	}
	return lead + shellQuote(bin) + trimPrefix + shellQuote(tail), true
}

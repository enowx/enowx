package accounts

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// What agents say when an account can't go on: a usage limit reached
// (until it resets), credits run out, a sign-in or key refused. Read from
// the end of a session's screen, where the agent says it.

// Limit is what an account hit: Reason (proto.Account*), the Line that
// said so, and Until when it resets, when said.
type Limit struct {
	Reason string
	Line   string
	Until  time.Time
}

var limitSigns = []struct {
	reason string
	re     *regexp.Regexp
}{
	{proto.AccountCredits, regexp.MustCompile(`(?i)credit balance is too low|insufficient (credits|balance|funds|quota)|out of credits|\b402\b|payment required|billing (limit|hard limit)`)},
	{proto.AccountAuth, regexp.MustCompile(`(?i)invalid api key|please run /login|oauth token (has )?expired|\b401\b.*unauthori[sz]ed|authentication_error|not logged in|token (is )?(invalid|expired|revoked)|sign in again`)},
	{proto.AccountLimit, regexp.MustCompile(`(?i)usage limit (reached|exceeded)|(hit|reached) (your|the) (usage |weekly |daily |session |5-hour )?limit|limit reached.{0,40}reset|quota (exceeded|reached)|exceeded retry limit, last status: 429|\b429\b.{0,40}(too many|rate)|rate.?limit(ed| exceeded| reached)|resource_exhausted`)},
}

var (
	resetClock = regexp.MustCompile(`(?i)resets?\s+(?:at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)`)
	resetIn    = regexp.MustCompile(`(?i)(?:resets?|try again|again)\s+in\s+(?:(\d+)\s*(?:hours?|hrs?|h)\b)?\s*(?:and\s+)?(?:(\d+)\s*(?:minutes?|mins?|m)\b)?`)
)

// tailLines is how much of the end of a screen is read.
const tailLines = 30

// Classify reads screen, the text a session shows, for a limit its agent
// says it hit, now being when: the last such line of its end.
func Classify(screen string, now time.Time) (Limit, bool) {
	lines := strings.Split(screen, "\n")
	if len(lines) > tailLines {
		lines = lines[len(lines)-tailLines:]
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		for _, s := range limitSigns {
			if s.re.MatchString(line) {
				l := Limit{Reason: s.reason, Line: cut(MaskSecrets(line), 200)}
				if s.reason == proto.AccountLimit {
					l.Until = resetTime(strings.Join(lines[i:min(len(lines), i+3)], " "), now)
				}
				return l, true
			}
		}
	}
	return Limit{}, false
}

// LimitLines counts the lines of screen saying an agent hit a limit.
func LimitLines(screen string) int {
	n := 0
	for line := range strings.SplitSeq(screen, "\n") {
		if readsAsLimit(line) {
			n++
		}
	}
	return n
}

// WithoutLimitLines is screen with its first n lines saying so taken out.
func WithoutLimitLines(screen string, n int) string {
	if n <= 0 {
		return screen
	}
	lines := strings.Split(screen, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if n > 0 && readsAsLimit(line) {
			n--
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func readsAsLimit(line string) bool {
	for _, s := range limitSigns {
		if s.re.MatchString(line) {
			return true
		}
	}
	return false
}

// resetTime is when text says a limit resets: at a clock time (the next
// one), or in so long; zero when it doesn't say.
func resetTime(text string, now time.Time) time.Time {
	if m := resetClock.FindStringSubmatch(text); m != nil {
		h, _ := strconv.Atoi(m[1])
		mins, _ := strconv.Atoi(m[2])
		if h == 12 {
			h = 0
		}
		if strings.EqualFold(m[3], "pm") {
			h += 12
		}
		t := time.Date(now.Year(), now.Month(), now.Day(), h, mins, 0, 0, now.Location())
		if !t.After(now) {
			t = t.Add(24 * time.Hour)
		}
		return t
	}
	if m := resetIn.FindStringSubmatch(text); m != nil && (m[1] != "" || m[2] != "") {
		h, _ := strconv.Atoi(m[1])
		mins, _ := strconv.Atoi(m[2])
		return now.Add(time.Duration(h)*time.Hour + time.Duration(mins)*time.Minute)
	}
	return time.Time{}
}

func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// secretLike are what reads as a key or token in a line an agent printed.
var secretLike = regexp.MustCompile(`\b(?:sk|pk|rk|ak|key|tok)[-_][A-Za-z0-9_-]{8,}|\bxox[abpr]-[A-Za-z0-9-]{8,}|\b(?:gh[pousr]|glpat)[-_][A-Za-z0-9_]{12,}|\b[A-Za-z0-9+/_-]{32,}={0,2}`)

// MaskSecrets is line with what reads as a key or token cut down to its
// last four characters.
func MaskSecrets(line string) string {
	return secretLike.ReplaceAllStringFunc(line, func(m string) string {
		return "[REDACTED …" + m[max(0, len(m)-4):] + "]"
	})
}

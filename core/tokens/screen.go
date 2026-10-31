package tokens

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/proto"
)

// The screen layer: an agent that keeps no record, and whose traffic
// isn't metered, may print what each answer took, as Aider does ("Tokens:
// 4.4k sent, 1.2k cache write, 3.3k cache hit, 120 received."). Those
// lines are read off the session's screen as it runs; the counts are as
// rounded as the agent prints them.

var screenLine = regexp.MustCompile(`(?i)tokens:\s*([\d.,]+[km]?)\s*sent(?:,\s*([\d.,]+[km]?)\s*cache write)?(?:,\s*([\d.,]+[km]?)\s*cache hit)?,\s*([\d.,]+[km]?)\s*received`)

// Screen reads sessions' screens for the lines agents print their
// tokens in, each line counted once even as the screen is read again.
// Safe from any goroutine.
type Screen struct {
	mu   sync.Mutex
	seen map[string]map[string]int // session → line → how many times counted
}

// Read returns the calls new on session's screen text since the last
// read.
func (s *Screen) Read(session, text string, now time.Time) []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen == nil {
		s.seen = map[string]map[string]int{}
	}
	seen := s.seen[session]
	if seen == nil {
		seen = map[string]int{}
		s.seen[session] = seen
	}
	count := map[string]int{}
	var out []Call
	for _, line := range strings.Split(text, "\n") {
		m := screenLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := strings.TrimSpace(m[0])
		count[key]++
		if count[key] <= seen[key] {
			continue
		}
		seen[key] = count[key]
		sum := sha256.Sum256([]byte(key))
		out = append(out, Call{ID: "screen:" + session + ":" + hex.EncodeToString(sum[:6]) + ":" + strconv.Itoa(count[key]),
			Time: now, Session: session, Layer: proto.TokenLayerScreen,
			Input: rounded(m[1]), CacheWrite: rounded(m[2]), CacheRead: rounded(m[3]), Output: rounded(m[4])})
	}
	return out
}

// Forget drops a session's lines, once it is gone.
func (s *Screen) Forget(session string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.seen, session)
}

// rounded reads "4.4k", "1,234", "2m" as a count.
func rounded(s string) int64 {
	s = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), ",", ""))
	if s == "" {
		return 0
	}
	mul := 1.0
	switch {
	case strings.HasSuffix(s, "k"):
		mul, s = 1e3, strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		mul, s = 1e6, strings.TrimSuffix(s, "m")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f*mul + 0.5)
}

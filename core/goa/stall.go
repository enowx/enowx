package goa

import (
	"fmt"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// How long a task may not move before GOA is told, in seconds, until the user
// picks otherwise.
const (
	StallBlank = 60  // a screen still empty after the task
	StallIdle  = 120 // no agent at work, with a task said to run
	StallStuck = 600 // working, nothing shown, nothing using the CPU
)

// A limit is at least a second, as watching is no finer, and at most a day,
// past which a stalled task may as well go untold.
const (
	stallLeast = 1
	stallMost  = 24 * 60 * 60
)

// StallOr is l with each limit not set (0 or less) at its default.
func StallOr(l proto.GOAStall) proto.GOAStall {
	if l.Blank <= 0 {
		l.Blank = StallBlank
	}
	if l.Idle <= 0 {
		l.Idle = StallIdle
	}
	if l.Stuck <= 0 {
		l.Stuck = StallStuck
	}
	return l
}

// CheckStall says what is wrong with limits to keep, their defaults filled in.
func CheckStall(l proto.GOAStall) error {
	l = StallOr(l)
	for _, v := range []int{l.Blank, l.Idle, l.Stuck} {
		if v < stallLeast || v > stallMost {
			return fmt.Errorf("goa: a stall limit is from %s to %s, not %s", StallText(stallLeast), StallText(stallMost), StallText(v))
		}
	}
	return nil
}

// ParseStall is a limit as written ("90s", "10m", "1h"), in seconds; 0 for
// nothing written or "0", which mean its default.
func ParseStall(text string) (int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("goa: %q isn't a time: write 90s, 10m or 1h", text)
	}
	if d%time.Second != 0 {
		return 0, fmt.Errorf("goa: %q isn't in whole seconds", text)
	}
	if d < 0 {
		return 0, fmt.Errorf("goa: %q is before now", text)
	}
	return int(d / time.Second), nil
}

// StallText is a limit of seconds as it is written.
func StallText(seconds int) string {
	s := (time.Duration(seconds) * time.Second).String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

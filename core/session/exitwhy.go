package session

import (
	"fmt"
	"time"
)

// quickExit is how soon after starting an ending says the program never got
// going, whatever it was.
const quickExit = 5 * time.Second

// exitSignals names the signals shells report as 128 + their number.
var exitSignals = map[int]string{
	1:  "a hang-up (SIGHUP): its terminal closed under it, or something outside ended it",
	2:  "an interrupt (SIGINT)",
	6:  "an abort (SIGABRT): it crashed",
	9:  "a kill (SIGKILL): something outside ended it, maybe the system, out of memory",
	11: "a segmentation fault (SIGSEGV): it crashed",
	15: "a stop (SIGTERM): something outside ended it",
}

// ExitWhy says in a line why a program ended with code after running for
// lived: "" when there's nothing to say.
func ExitWhy(code int, lived time.Duration) string {
	if code <= 0 {
		return ""
	}
	why := fmt.Sprintf("It ended with an error (code %d).", code)
	if code > 128 {
		if s, ok := exitSignals[code-128]; ok {
			why = fmt.Sprintf("It ended on %s (code %d).", s, code)
		}
	}
	if lived < quickExit {
		why += fmt.Sprintf(" That was %.1fs after it started, so it never got going: the daemon's log has more (Settings → Diagnostics).", lived.Seconds())
	}
	return why
}

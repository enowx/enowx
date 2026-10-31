package tokenview

import (
	"fmt"
	"strconv"

	"github.com/enowx/enowx/proto"
)

// Count writes a token count short: 950, 14k, 1.2M, 3.4B.
func Count(n int64) string {
	switch {
	case n >= 1e9:
		return strconv.FormatFloat(float64(n)/1e9, 'f', 1, 64) + "B"
	case n >= 1e6:
		return strconv.FormatFloat(float64(n)/1e6, 'f', 1, 64) + "M"
	case n >= 1e4:
		return strconv.FormatFloat(float64(n)/1e3, 'f', 0, 64) + "k"
	case n >= 1e3:
		return strconv.FormatFloat(float64(n)/1e3, 'f', 1, 64) + "k"
	}
	return strconv.FormatInt(n, 10)
}

// Dollars writes a cost: $0.42, $12.30, $1,204.
func Dollars(x float64) string {
	switch {
	case x == 0:
		return "$0"
	case x < 0.01:
		return "<$0.01"
	case x >= 1000:
		return "$" + strconv.FormatFloat(x, 'f', 0, 64)
	}
	return fmt.Sprintf("$%.2f", x)
}

// Spent is a session's use in a line, for its header and tooltips: its
// tokens and cost, API-equivalent marked.
func Spent(t proto.TokenCounts) string {
	if t.Calls == 0 {
		return ""
	}
	line := Count(t.Total()) + " tokens"
	switch {
	case t.Cost > 0 && t.Equivalent >= t.Cost:
		line += " · ≈" + Dollars(t.Cost) + " API-equivalent"
	case t.Cost > 0:
		line += " · " + Dollars(t.Cost)
	}
	return line
}

// SpentTip says a session's use in full, for a tooltip.
func SpentTip(t proto.TokenCounts) string {
	if t.Calls == 0 {
		return ""
	}
	return fmt.Sprintf("Tokens so far: %s in, %s cached, %s written to the cache, %s out, %d calls. %s",
		Count(t.Input), Count(t.CacheRead), Count(t.CacheWrite), Count(t.Output), t.Calls, Spent(t))
}

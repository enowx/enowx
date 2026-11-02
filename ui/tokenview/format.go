package tokenview

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/enowx/enowx/proto"
)

// Count writes a token count short: 950, 14k, 1.2M, 4M, 3.4B.
func Count(n int64) string {
	switch {
	case n >= 1e9:
		return strings.TrimSuffix(strconv.FormatFloat(float64(n)/1e9, 'f', 1, 64), ".0") + "B"
	case n >= 1e6:
		return strings.TrimSuffix(strconv.FormatFloat(float64(n)/1e6, 'f', 1, 64), ".0") + "M"
	case n >= 1e4:
		return strconv.FormatFloat(float64(n)/1e3, 'f', 0, 64) + "k"
	case n >= 1e3:
		return strconv.FormatFloat(float64(n)/1e3, 'f', 1, 64) + "k"
	}
	return strconv.FormatInt(n, 10)
}

// Whole writes a whole number with its thousands marked: 5,140.
func Whole(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0 && s[i-1] != '-'; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// Dollars writes a cost: $0.0022, $0.42, $12.30, $1,204.
func Dollars(x float64) string {
	switch {
	case x == 0:
		return "$0"
	case x < 0.0001:
		return "<$0.0001"
	case x < 0.01:
		return fmt.Sprintf("$%.4f", x)
	case x >= 1000:
		return "$" + Whole(int(math.Round(x)))
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
	return fmt.Sprintf("Tokens so far: %s in, %s out, %s cache write, and %s cache read not counted in them, %d calls. %s",
		Count(t.Input), Count(t.Output), Count(t.CacheWrite), Count(t.CacheRead), t.Calls, Spent(t))
}

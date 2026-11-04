package record

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseBudget reads dollars as the user writes them, "$5" or "5", 0 being no limit. NaN and
// infinity parse as floats but no budget can be, and JSON can't keep them.
func ParseBudget(text string) (float64, error) {
	usd, err := strconv.ParseFloat(strings.TrimPrefix(text, "$"), 64)
	if err != nil || usd < 0 || math.IsNaN(usd) || math.IsInf(usd, 0) {
		return 0, fmt.Errorf("%q isn't a number of dollars", text)
	}
	return usd, nil
}

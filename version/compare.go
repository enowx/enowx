package version

import (
	"strconv"
	"strings"
)

// Older says version a is older than b: their numbers compared in turn, what follows a
// "-" or "+" (a development build's mark) left out. A version that isn't one of numbers
// is older than none, and none older than it, as nothing can be told.
func Older(a, b string) bool {
	x, okA := numbers(a)
	y, okB := numbers(b)
	if !okA || !okB {
		return false
	}
	for i := range max(len(x), len(y)) {
		var p, q int
		if i < len(x) {
			p = x[i]
		}
		if i < len(y) {
			q = y[i]
		}
		if p != q {
			return p < q
		}
	}
	return false
}

func numbers(v string) ([]int, bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, part := range strings.Split(v, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, len(out) > 0
}

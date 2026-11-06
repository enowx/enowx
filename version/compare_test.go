package version

import "testing"

func TestOlder(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.7.0", "0.7.1", true}, {"0.7.1", "0.7.0", false}, {"0.7.1", "0.7.1", false},
		{"0.6.10", "0.7.0", true}, {"0.7.9", "0.7.10", true}, {"0.7", "0.7.1", true},
		{"0.7.1-dev", "0.7.1", false}, {"v0.7.0", "0.7.1", true}, {"dev", "0.7.1", false}, {"0.7.1", "dev", false},
	} {
		if got := Older(c.a, c.b); got != c.want {
			t.Errorf("Older(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

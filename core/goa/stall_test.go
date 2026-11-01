package goa

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

// A limit reads back as it was written, nothing written meaning its default,
// and what can't be a limit is refused.
func TestStallLimits(t *testing.T) {
	for _, c := range []struct {
		text string
		want int
		back string
	}{{"90s", 90, "1m30s"}, {"10m", 600, "10m"}, {"1h", 3600, "1h"}, {"1h30m", 5400, "1h30m"}, {"", 0, "0s"}, {"0", 0, "0s"}, {" 45s ", 45, "45s"}} {
		got, err := ParseStall(c.text)
		if err != nil || got != c.want || StallText(got) != c.back {
			t.Errorf("%q: %d (%s), %v", c.text, got, StallText(got), err)
		}
	}
	for _, bad := range []string{"soon", "10", "-1m", "1500ms"} {
		if got, err := ParseStall(bad); err == nil {
			t.Errorf("%q taken: %d", bad, got)
		}
	}
	if got := StallOr(proto.GOAStall{Idle: 30}); got != (proto.GOAStall{Blank: StallBlank, Idle: 30, Stuck: StallStuck}) {
		t.Errorf("filled in: %+v", got)
	}
	if err := CheckStall(proto.GOAStall{}); err != nil {
		t.Errorf("the defaults refused: %v", err)
	}
	if err := CheckStall(proto.GOAStall{Stuck: stallMost + 1}); err == nil {
		t.Error("over a day taken")
	}
}

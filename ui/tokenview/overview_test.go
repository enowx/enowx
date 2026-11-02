package tokenview

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

// Input and output show apart, the cache's two as their own figures, and cache reads are
// neither in the Tokens figure nor in a day's bar.
func TestUsageShowsInputAndOutputApart(t *testing.T) {
	counts := proto.TokenCounts{Input: 1000, Output: 2000, CacheWrite: 4000, CacheRead: 9_000_000, Calls: 3}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(900).Children(func() { totals(c, counts) })
	}, 900, 300)
	tt.Frame()
	for _, want := range []string{"Input", "Output", "Cache read", "Cache write", Count(7000), Count(9_000_000)} {
		if !slices.Contains(tt.Texts(), want) {
			t.Errorf("no %q: %q", want, tt.Texts())
		}
	}
	since := time.Now().AddDate(0, 0, -1)
	bars := dayBars(proto.TokensSummary{Days: []proto.TokenDay{{Day: time.Now().Format(time.DateOnly), TokenCounts: counts}}}, since)
	last := bars[len(bars)-1]
	if len(last.Parts) != 3 || last.Parts[0]+last.Parts[1]+last.Parts[2] != 7000 {
		t.Errorf("a day's bar: %+v", last.Parts)
	}
	if !strings.Contains(last.Tip, "cache read") {
		t.Errorf("its tip: %q", last.Tip)
	}
	if line := splitLine(counts); !strings.Contains(line, "Input") || !strings.Contains(line, "Output") || !strings.Contains(line, "Cache read") {
		t.Errorf("a split: %q", line)
	}
}

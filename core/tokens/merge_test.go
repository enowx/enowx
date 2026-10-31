package tokens

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

func TestMergeSumsEveryHost(t *testing.T) {
	a := proto.TokensSummary{Host: "mac", HostID: "h1", On: true, Total: proto.TokenCounts{Input: 10, Calls: 1},
		Days:   []proto.TokenDay{{Day: "2026-10-30", TokenCounts: proto.TokenCounts{Input: 10, Calls: 1}}},
		Groups: []proto.TokenGroup{{Key: "claude", TokenCounts: proto.TokenCounts{Input: 10, Calls: 1}}}}
	b := proto.TokensSummary{Host: "vps", HostID: "h2", Total: proto.TokenCounts{Input: 30, Calls: 2},
		Days:   []proto.TokenDay{{Day: "2026-10-30", TokenCounts: proto.TokenCounts{Input: 5, Calls: 1}}, {Day: "2026-10-31", TokenCounts: proto.TokenCounts{Input: 25, Calls: 1}}},
		Groups: []proto.TokenGroup{{Key: "claude", TokenCounts: proto.TokenCounts{Input: 5, Calls: 1}}, {Key: "codex", TokenCounts: proto.TokenCounts{Input: 25, Calls: 1}}}}
	m := Merge(proto.TokensQuery{By: proto.TokensByAgent}, []proto.TokensSummary{a, b})
	if m.Total.Input != 40 || len(m.Days) != 2 || m.Days[0].Input != 15 || len(m.Groups) != 2 || m.Groups[0].Key != "codex" || m.Groups[1].Input != 15 || !m.On {
		t.Fatalf("by agent: %+v", m)
	}
	h := Merge(proto.TokensQuery{By: proto.TokensByHost}, []proto.TokensSummary{a, b})
	if len(h.Groups) != 2 || h.Groups[0].Label != "vps" || h.Groups[0].Input != 30 {
		t.Fatalf("by host: %+v", h.Groups)
	}
}

package ui

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

// An agent with no other config stays one click; one with some offers Default and each,
// as New session does, the preferred marked.
func TestTabMenuConfigs(t *testing.T) {
	if got := tabConfigsOf(nil, ""); got != nil {
		t.Fatalf("no configs: %v", got)
	}
	got := tabConfigsOf([]proto.Account{{ID: "a", Name: "DeepSeek Flash"}, {ID: "b", Name: "Work"}}, "b")
	want := []tabConfig{{"", "Default"}, {"a", "DeepSeek Flash"}, {"b", "Work (preferred)"}}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: %v, want %v", i, got[i], want[i])
		}
	}
}

package textfmt

import (
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

// A session says whether its rules arrived and which version, never that they
// did when nothing shows it.
func TestSessionRules(t *testing.T) {
	cases := []struct {
		r    *proto.SessionRules
		want string
	}{
		{nil, ""},
		{&proto.SessionRules{For: proto.RulesForProject}, ""},
		{&proto.SessionRules{For: proto.RulesForProject, Version: "aa", Current: "aa", Via: proto.RulesViaStart}, "rules aa arrived"},
		{&proto.SessionRules{For: proto.RulesForProject, Version: "aa", Current: "aa", Via: proto.RulesViaHook, Hook: true}, "rules aa arrived"},
		{&proto.SessionRules{For: proto.RulesForProject, Version: "aa", Current: "aa", Via: proto.RulesViaAsked}, "rules aa arrived"},
		{&proto.SessionRules{For: proto.RulesForProject, Version: "aa", Current: "aa", Via: proto.RulesViaFile}, "not confirmed"},
		{&proto.SessionRules{For: proto.RulesForProject, Current: "bb"}, "rules bb not confirmed"},
		{&proto.SessionRules{For: proto.RulesForProject, Version: "aa", Current: "bb", Hook: true}, "wait for its next prompt"},
		{&proto.SessionRules{For: proto.RulesForProject, Version: "aa", Current: "bb"}, "rules changed to bb, it has aa"},
		{&proto.SessionRules{For: proto.RulesForGOA, Version: "aa", Current: "aa", Hook: true, Again: true}, "given again"},
		{&proto.SessionRules{For: proto.RulesForGOA, Version: "aa", Hook: true}, "told with its next prompt"},
		{&proto.SessionRules{For: proto.RulesForGOA, Version: "aa"}, "it still has aa"},
	}
	for _, c := range cases {
		got := SessionRules(c.r)
		if c.want == "" && got != "" || !strings.Contains(got, c.want) {
			t.Fatalf("%+v: %q, want %q", c.r, got, c.want)
		}
	}
}

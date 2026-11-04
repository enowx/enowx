package vault

import (
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

func TestAgentKey(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	cred := func(name string, f func(*proto.CredentialInfo)) proto.CredentialInfo {
		c := proto.CredentialInfo{Name: name, ID: "id-" + name, Platform: "anthropic"}
		if f != nil {
			f(&c)
		}
		return c
	}
	scoped := cred("work", func(c *proto.CredentialInfo) { c.Projects = []string{"/code/work"} })
	def := cred("mine", func(c *proto.CredentialInfo) { c.Default = true })
	plain := cred("plain", nil)
	cases := []struct {
		name    string
		list    []proto.CredentialInfo
		project string
		have    []string
		pick    string
		has     bool
	}{
		{"none", nil, "", nil, "", false},
		{"another platform's", []proto.CredentialInfo{{Name: "o", Platform: "openai"}}, "", nil, "", false},
		{"the only one", []proto.CredentialInfo{plain}, "", nil, "plain", true},
		{"the default first", []proto.CredentialInfo{plain, def}, "", nil, "mine", true},
		{"the project's first", []proto.CredentialInfo{def, scoped}, "/code/work", nil, "work", true},
		{"another project's isn't", []proto.CredentialInfo{scoped}, "/code/play", nil, "", false},
		{"it has one", []proto.CredentialInfo{plain, def}, "", []string{"plain"}, "", true},
		{"it has one by ID", []proto.CredentialInfo{plain}, "", []string{"id-plain"}, "", true},
		{"every session has it", []proto.CredentialInfo{cred("auto", func(c *proto.CredentialInfo) { c.Auto = true })}, "", nil, "", true},
		{"revoked", []proto.CredentialInfo{cred("r", func(c *proto.CredentialInfo) { c.Revoked = true })}, "", nil, "", false},
		{"expired", []proto.CredentialInfo{cred("e", func(c *proto.CredentialInfo) { c.Expires = now.Add(-time.Hour) })}, "", nil, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pick, has := AgentKey(tc.list, "anthropic", tc.project, tc.have, now)
			if pick != tc.pick || has != tc.has {
				t.Fatalf("pick %q, has %v", pick, has)
			}
		})
	}
}

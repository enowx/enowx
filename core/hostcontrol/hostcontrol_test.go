package hostcontrol

import (
	"testing"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/proto"
)

// On a teammate's host a viewer reads and a member changes; the vault is
// out of reach for everyone.
func TestRolesOnASharedHost(t *testing.T) {
	cases := []struct {
		method, role string
		ok           bool
	}{
		{proto.MethodSessionList, api.RoleViewer, true},
		{proto.MethodSessionInput, api.RoleViewer, false},
		{proto.MethodSessionInput, api.RoleMember, true},
		{proto.MethodSessionKill, api.RoleViewer, false},
		{proto.MethodSessionKill, "", true},
		{proto.MethodVaultGet, "", false},
		{proto.MethodHostCall, "", false},
	}
	for _, c := range cases {
		if _, err := Check(c.method, c.role); (err == nil) != c.ok {
			t.Errorf("%s as %q: %v", c.method, c.role, err)
		}
	}
}

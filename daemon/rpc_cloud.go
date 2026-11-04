package daemon

import "github.com/enowx/enowx/proto"

// The host's sign-in to a team cloud, and requests to it as the host's
// device (cloud.go).

var cloudHandlers = map[string]handler{
	proto.MethodCloudStatus: bare(func(c *conn) (any, error) { return c.srv.cloudStatus() }),
	proto.MethodCloudSetURL: with(func(c *conn, p proto.CloudSetURLParams) (any, error) { return c.srv.cloudSetURL(p.URL) }),
	proto.MethodCloudSignIn: with(func(c *conn, p proto.CloudSignInParams) (any, error) { return c.srv.cloudSignIn(p) }),
	proto.MethodCloudCancel: bare(func(c *conn) (any, error) {
		c.srv.cloudCancel()
		return c.srv.cloudStatus()
	}),
	proto.MethodCloudSignOut:     bare(func(c *conn) (any, error) { return c.srv.cloudSignOut() }),
	proto.MethodCloudRequest:     with(func(c *conn, p proto.CloudRequest) (any, error) { return c.srv.cloudRequest(p) }),
	proto.MethodCloudRecoveryNew: bare(func(c *conn) (any, error) { return c.srv.newRecovery() }),
	proto.MethodCloudRecoveryUse: with(func(c *conn, p proto.CloudRecovery) (any, error) { return c.srv.useRecovery(p.Key) }),
	proto.MethodSpaces:           bare(func(c *conn) (any, error) { return c.srv.spaces(), nil }),
	proto.MethodSpaceSync:        with(func(c *conn, p proto.SpaceRef) (any, error) { return c.srv.syncSpace(p.Team) }),
	proto.MethodTeamKit:          bare(func(c *conn) (any, error) { return c.srv.teamKit(), nil }),
	proto.MethodTeamKitAdd:       with(func(c *conn, p proto.TeamKitAddParams) (any, error) { return c.srv.addToKit(p) }),
	proto.MethodTeamKitSave: with(func(c *conn, p proto.TeamKitSaveParams) (any, error) {
		return c.srv.saveToKit(p)
	}),
	proto.MethodTeamKitRemove: with(func(c *conn, p proto.TeamKitRef) (any, error) {
		return ok, c.srv.removeFromKit(p)
	}),
	proto.MethodProjectRules: with(func(c *conn, p proto.ProjectRulesRef) (any, error) { return c.srv.projectRules(p.Path) }),
	proto.MethodRulesHistory: with(func(c *conn, p proto.RulesDraftParams) (any, error) { return c.srv.rulesHistory(p), nil }),
	proto.MethodRulesDraft:   with(func(c *conn, p proto.RulesDraftParams) (any, error) { return c.srv.draftRules(p) }),
	proto.MethodProjectRulesSave: with(func(c *conn, p proto.ProjectRules) (any, error) {
		r, err := c.srv.saveProjectRules(p)
		if err == nil {
			c.srv.refreshRules()
		}
		return r, err
	}),
	proto.MethodTeamProjects: bare(func(c *conn) (any, error) { return c.srv.teamProjects(), nil }),
	proto.MethodTeamProjectAdd: with(func(c *conn, p proto.TeamProjectAddParams) (any, error) {
		return c.srv.addTeamProject(p)
	}),
	proto.MethodTeamProjectRemove: with(func(c *conn, p proto.TeamProjectRef) (any, error) {
		return ok, c.srv.removeTeamProject(p)
	}),
}

package daemon

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/server"
	"github.com/enowx/enowx/cloud/store/storetest"
	"github.com/enowx/enowx/proto"
)

// A project's rules reach every session in it: the user's own project's
// from their settings, a team's project's from the team's space, the same
// on every member's host, edited by members or only admins as the admins
// say, never holding a secret, each change in the team's audit log.
func TestProjectRulesReachItsSessions(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	cloud := httptest.NewServer(server.New(server.Config{URL: "http://cloud.test"}, db, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(cloud.Close)
	ada := signInHost(t, db, cloud.URL, 1, "ada")
	bob := signInHost(t, db, cloud.URL, 2, "bob")

	mine := gitRepo(t)
	if _, err := ada.c.SaveProjectRules(proto.ProjectRules{Path: mine, Rules: "Use tabs."}); err != nil {
		t.Fatal(err)
	}
	if g := ada.srv.projectGuide(mine); !strings.Contains(g, "the user's rules") || !strings.Contains(g, "Use tabs.") {
		t.Fatalf("the guide of the user's own project: %q", g)
	}
	if _, err := ada.c.SaveProjectRules(proto.ProjectRules{Path: mine, Rules: "key AKIAABCDEFGHIJKLMNOP"}); err == nil {
		t.Fatal("rules holding a secret were kept")
	}

	if _, err := ada.c.SaveProjectRules(proto.ProjectRules{Path: mine}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("saved empty over rules: %v", err)
	}
	if _, err := ada.c.SaveProjectRules(proto.ProjectRules{Path: mine, Rules: "Use spaces."}); err != nil {
		t.Fatal(err)
	}
	if kept, err := ada.c.RulesHistory(proto.RulesDraftParams{For: proto.RulesForProject, Path: mine}); err != nil || len(kept) != 1 || kept[0].Rules != "Use tabs.\n" && kept[0].Rules != "Use tabs." {
		t.Fatalf("the rules saved over: %+v %v", kept, err)
	}
	if g := ada.srv.projectGuide(mine); !strings.Contains(g, "Use spaces.") {
		t.Fatalf("the guide: %q", g)
	}

	team, _ := db.CreateTeam(ctx, ada.acc, "Design")
	inv, _ := db.CreateInvite(ctx, team.ID, ada.acc, api.InviteParams{Role: api.RoleMember})
	db.AcceptInvite(ctx, inv.Token, bob.acc)
	ada.srv.learnTeams(ada.acc, []api.Team{{ID: team.ID, Name: "Design", Role: api.RoleOwner}})
	bob.srv.learnTeams(bob.acc, []api.Team{{ID: team.ID, Name: "Design", Role: api.RoleMember}})
	ada.c.AddTeamProject(proto.TeamProjectAddParams{Team: team.ID, Repo: "https://github.com/enowx/app"})
	clone := func() string {
		dir := gitRepo(t)
		exec.Command("git", "-C", dir, "remote", "add", "origin", "https://github.com/enowx/app.git").Run()
		return dir
	}
	adaApp, bobApp := clone(), clone()
	ada.sync(t, team.ID)
	bob.sync(t, team.ID) // the team's project reaches bob's host
	r, err := bob.c.SaveProjectRules(proto.ProjectRules{Path: bobApp, Rules: "Every change gets a test."})
	if err != nil || r.Team != team.ID || !r.CanEdit {
		t.Fatalf("a member wrote the team's project's rules: %+v %v", r, err)
	}
	bob.sync(t, team.ID)
	ada.sync(t, team.ID)
	if g := ada.srv.projectGuide(adaApp); !strings.Contains(g, "the team's rules") || !strings.Contains(g, "Every change gets a test.") {
		t.Fatalf("the guide on another member's host: %q", g)
	}
	if _, err := ada.c.SaveProjectRules(proto.ProjectRules{Path: adaApp, Rules: "Every change gets a test.", Editors: proto.RulesByAdmins}); err != nil {
		t.Fatal(err)
	}
	ada.sync(t, team.ID)
	bob.sync(t, team.ID)
	if r, _ := bob.c.ProjectRules(bobApp); r.CanEdit || r.Editors != proto.RulesByAdmins {
		t.Fatalf("admins only, as the member's host says: %+v", r)
	}
	if _, err := bob.c.SaveProjectRules(proto.ProjectRules{Path: bobApp, Rules: "No tests."}); err == nil {
		t.Fatal("a member changed rules only admins edit")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var n int
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit WHERE team_id = $1 AND action = $2`, team.ID, api.AuditRulesChanged).Scan(&n)
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the rules' change isn't in the team's audit log")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

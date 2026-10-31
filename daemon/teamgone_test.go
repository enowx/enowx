package daemon

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	cloudclient "github.com/enowx/enowx/cloud/client"
	"github.com/enowx/enowx/cloud/server"
	"github.com/enowx/enowx/cloud/store/storetest"
	"github.com/enowx/enowx/proto"
)

// The owner deletes a team from their host: its space leaves that host at
// once, unannounced. A member's host, at its next sync, drops its copy,
// stops sharing itself with the team (its listener closes, its visitor is
// let go) and keeps a notice that the team was deleted until it is seen.
func TestDeletedTeamLeavesMembersHosts(t *testing.T) {
	t.Setenv(proto.EnvTeamListen, "127.0.0.1:0")
	ctx := context.Background()
	db := storetest.Open(t)
	cloud := httptest.NewServer(server.New(server.Config{URL: "http://cloud.test"}, db, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(cloud.Close)
	ada := signInHost(t, db, cloud.URL, 1, "ada")
	bob := signInHost(t, db, cloud.URL, 2, "bob")
	team, _ := db.CreateTeam(ctx, ada.acc, "Design")
	inv, _ := db.CreateInvite(ctx, team.ID, ada.acc, api.InviteParams{Role: api.RoleAdmin})
	db.AcceptInvite(ctx, inv.Token, bob.acc)
	ada.srv.learnTeams(ada.acc, []api.Team{{ID: team.ID, Name: "Design", Role: api.RoleOwner}})
	bob.srv.learnTeams(bob.acc, []api.Team{{ID: team.ID, Name: "Design", Role: api.RoleAdmin}})
	if _, err := ada.c.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Roadmap", Team: team.ID}, Body: "one\n"}); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.c.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Bob's own"}}); err != nil {
		t.Fatal(err)
	}

	// Bob shares a project of his host with the team; Ada visits it.
	repo := gitRepo(t)
	if _, err := bob.c.AddProject(repo); err != nil {
		t.Fatal(err)
	}
	sh, err := bob.c.SetHostShare(team.ID, []string{repo})
	if err != nil || sh.Addr == "" {
		t.Fatalf("shared: %+v %v", sh, err)
	}
	ada.sync(t, team.ID)
	bob.sync(t, team.ID)
	ada.sync(t, team.ID)
	if _, ok := bob.noteTitled(t, "Roadmap"); !ok {
		t.Fatal("the team's note didn't reach bob's host")
	}
	adaDev, _ := ada.srv.cloud.store.Get()
	ac, _ := cloudclient.New(cloud.URL, adaDev.Token)
	bobDev, _ := bob.srv.cloud.store.Get()
	tk, err := ac.HostTicket(ctx, team.ID, bobDev.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	nc, err := net.Dial("tcp", sh.Addr)
	if err != nil {
		t.Fatal(err)
	}
	visitor, err := client.New(nc, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { visitor.Close() })
	if _, err := visitor.TeamAuth(tk.Ticket); err != nil {
		t.Fatal(err)
	}

	// An admin can't delete it; its owner can.
	tp := strings.Replace(api.PathTeam, "{team}", team.ID, 1)
	if err := bob.c.CloudCall("DELETE", tp, nil, nil); err == nil || !strings.Contains(err.Error(), "role") {
		t.Fatalf("an admin deleted the team: %v", err)
	}
	if err := ada.c.CloudCall("DELETE", tp, nil, nil); err != nil {
		t.Fatal(err)
	}
	if sp, _ := ada.c.Spaces(); len(sp) != 0 {
		t.Fatalf("the owner's host kept the space: %+v", sp)
	}
	if gone, _ := ada.c.SpacesGone(); len(gone) != 0 {
		t.Fatalf("the owner's host was told what it did: %+v", gone)
	}

	// Bob's host, at its next sync.
	if _, err := bob.srv.syncSpace(team.ID); err == nil || !strings.Contains(err.Error(), "was deleted by its owner") {
		t.Fatalf("a deleted team's sync: %v", err)
	}
	if sp, _ := bob.c.Spaces(); len(sp) != 0 {
		t.Fatalf("the member's host kept the space: %+v", sp)
	}
	if _, err := os.Stat(filepath.Join(bob.srv.teamsDir(), team.ID)); !os.IsNotExist(err) {
		t.Fatalf("the space's copy stayed on disk: %v", err)
	}
	if _, ok := bob.noteTitled(t, "Roadmap"); ok {
		t.Fatal("the team's note stayed")
	}
	if _, ok := bob.noteTitled(t, "Bob's own"); !ok {
		t.Fatal("bob's own note went with the team")
	}
	if sh, _ := bob.c.HostShare(); len(sh.Teams) != 0 || sh.Addr != "" {
		t.Fatalf("still shared with the deleted team: %+v", sh)
	}
	time.Sleep(50 * time.Millisecond)
	if c, err := net.Dial("tcp", sh.Addr); err == nil {
		c.Close()
		t.Fatal("the team listener stayed open")
	}
	if _, err := visitor.Projects(); err == nil {
		t.Fatal("the team's visitor wasn't let go")
	}
	gone, err := bob.c.SpacesGone()
	if err != nil || len(gone) != 1 || gone[0].Team != team.ID || gone[0].Name != "Design" || !gone[0].Shared {
		t.Fatalf("the notice: %+v %v", gone, err)
	}
	if err := bob.c.SpaceGoneSeen(team.ID); err != nil {
		t.Fatal(err)
	}
	if gone, _ := bob.c.SpacesGone(); len(gone) != 0 {
		t.Fatalf("seen, the notice stayed: %+v", gone)
	}
}

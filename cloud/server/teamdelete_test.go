package server

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/blobstore"
)

// A team is deleted by its owner alone: an admin or a member is refused;
// deleted, all it held is gone from the cloud (items, invites, keys,
// files), its former members are told it was deleted, and one never in it
// is told nothing more than that it isn't there.
func TestOwnerDeletesTeam(t *testing.T) {
	ctx := context.Background()
	files := blobstore.Dir(t.TempDir())
	e := newTeamEnv(t, Config{Blobs: files})
	own := e.signIn(1, "own", "")
	adm := e.signIn(2, "adm", "")
	mem := e.signIn(3, "mem", "")
	out := e.signIn(4, "out", "")
	team := e.team(own, "Design")
	e.join(own, team, api.RoleAdmin, adm)
	e.join(own, team, api.RoleMember, mem)
	tp := e.url + "/v1/teams/" + team

	// What the team holds: an item (its chat, notes, canvases, kit and
	// hosts are items), an open invite, a file.
	const id = "0123456789abcdef0123456789abcdef"
	if code, msg := send(t, "PUT", tp+"/items/"+id, mem.token, api.PutItemParams{Kind: api.ItemNote, Version: 1, Epoch: 1, Sealed: []byte("x")}, nil); code != http.StatusOK {
		t.Fatalf("item: %d %s", code, msg)
	}
	if code, msg := send(t, "POST", tp+"/invites", own.token, api.InviteParams{Role: api.RoleMember}, nil); code != http.StatusOK {
		t.Fatalf("invite: %d %s", code, msg)
	}
	req, _ := http.NewRequest("PUT", tp+"/blobs/"+id, bytes.NewReader([]byte("sealed image")))
	req.Header.Set("Authorization", "Bearer "+mem.token)
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode/100 != 2 {
		t.Fatalf("file: %v %v", res, err)
	}

	for _, who := range []signedInMember{adm, mem, out} {
		want := http.StatusForbidden
		if who.acc.ID == out.acc.ID {
			want = http.StatusNotFound
		}
		if code, _ := send(t, "DELETE", tp, who.token, nil, nil); code != want {
			t.Fatalf("%s deleting the team: %d, want %d", who.acc.Name, code, want)
		}
	}
	if code, msg := send(t, "DELETE", tp, own.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("the owner deleting the team: %d %s", code, msg)
	}

	for _, table := range []string{"teams", "members", "items", "invites", "team_keys", "blobs"} {
		col := "team_id"
		if table == "teams" {
			col = "id"
		}
		var n int
		if err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+col+` = $1`, team).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s kept %d rows of the deleted team (%v)", table, n, err)
		}
	}
	if _, err := files.Get(ctx, blobKey(team, id)); err == nil {
		t.Fatal("the team's file stayed in the file store")
	}

	// Its former members' hosts, syncing, learn it was deleted.
	for _, who := range []signedInMember{own, adm, mem} {
		code, msg := send(t, "GET", tp+"/items?after=0", who.token, nil, nil)
		if code != http.StatusNotFound || msg != api.ErrTeamDeleted {
			t.Fatalf("%s reading the deleted team: %d %s", who.acc.Name, code, msg)
		}
	}
	if code, msg := send(t, "GET", tp+"/items?after=0", out.token, nil, nil); code != http.StatusNotFound || msg == api.ErrTeamDeleted {
		t.Fatalf("one never in the team learned of its deletion: %d %s", code, msg)
	}
	var list []api.Team
	if code, _ := send(t, "GET", e.url+api.PathTeams, mem.token, nil, &list); code != http.StatusOK || len(list) != 0 {
		t.Fatalf("the deleted team still listed: %d %+v", code, list)
	}
}

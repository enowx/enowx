package store_test

// What is used once stays used once when several ask at the same moment.
// Postgres (READ COMMITTED) is where a check and the write after it can
// interleave; SQLite's single writer serializes them, so these run only
// against Postgres.

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
	"github.com/enowx/enowx/cloud/store/storetest"
)

const raceRounds = 30

func openPostgres(t *testing.T) *store.DB {
	if os.Getenv(storetest.Env) == "" {
		t.Skip("needs " + storetest.Env + " (Postgres)")
	}
	db := storetest.Open(t)
	db.SetMaxOpenConns(0) // these need calls at the same moment
	return db
}

func raceAccount(t *testing.T, db *store.DB, gh int64) (api.Account, api.Device) {
	t.Helper()
	ctx := context.Background()
	acc, err := db.GitHubAccount(ctx, gh, fmt.Sprint("u", gh), "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := db.AddDevice(ctx, acc.ID, "dev", api.DeviceApp, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	return acc, d
}

// together runs f n times at once, released together, and counts the
// calls that succeeded.
func together(n int, f func(i int) error) int {
	var start, done sync.WaitGroup
	var ok atomic.Int32
	start.Add(1)
	for i := range n {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			if f(i) == nil {
				ok.Add(1)
			}
		}()
	}
	start.Done()
	done.Wait()
	return int(ok.Load())
}

// Of several writes of an item's same version at once, one is kept and the
// others get a conflict: no change is lost unseen.
func TestItemVersionWrittenOnce(t *testing.T) {
	db := openPostgres(t)
	ctx := context.Background()
	acc, dev := raceAccount(t, db, 1)
	team, err := db.CreateTeam(ctx, acc.ID, "t")
	if err != nil {
		t.Fatal(err)
	}
	for round := range raceRounds {
		id := fmt.Sprintf("%032x", round+1)
		ok := together(8, func(i int) error {
			_, err := db.PutItem(ctx, team.ID, acc.ID, dev.ID, id, api.PutItemParams{Kind: api.ItemNote, Version: 1, Epoch: 1, Sealed: []byte{byte(i)}})
			return err
		})
		if ok != 1 {
			t.Fatalf("round %d: %d writes of version 1 kept", round, ok)
		}
	}
}

// An invite accepted by several accounts at once lets one in.
func TestInviteAcceptedOnce(t *testing.T) {
	db := openPostgres(t)
	ctx := context.Background()
	owner, _ := raceAccount(t, db, 1)
	team, err := db.CreateTeam(ctx, owner.ID, "t")
	if err != nil {
		t.Fatal(err)
	}
	var accs []api.Account
	for i := range 8 {
		a, _ := raceAccount(t, db, int64(100+i))
		accs = append(accs, a)
	}
	for round := range raceRounds {
		in, err := db.CreateInvite(ctx, team.ID, owner.ID, api.InviteParams{Role: api.RoleAdmin})
		if err != nil {
			t.Fatal(err)
		}
		ok := together(len(accs), func(i int) error {
			_, err := db.AcceptInvite(ctx, in.Token, accs[i].ID)
			return err
		})
		if ok != 1 {
			t.Fatalf("round %d: one invite let %d in", round, ok)
		}
		for _, a := range accs {
			db.ExecContext(ctx, `DELETE FROM members WHERE team_id = $1 AND account_id = $2`, team.ID, a.ID)
		}
	}
}

// A host ticket redeemed several times at once lets one visitor in.
func TestTicketRedeemedOnce(t *testing.T) {
	db := openPostgres(t)
	ctx := context.Background()
	owner, ownerDev := raceAccount(t, db, 1)
	team, err := db.CreateTeam(ctx, owner.ID, "t")
	if err != nil {
		t.Fatal(err)
	}
	host, _, err := db.AddDevice(ctx, owner.ID, "host", api.DeviceHost, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	for round := range raceRounds {
		tk, err := db.IssueTicket(ctx, team.ID, owner.ID, ownerDev.ID, host.ID)
		if err != nil {
			t.Fatal(err)
		}
		ok := together(8, func(int) error {
			_, err := db.RedeemTicket(ctx, tk.Ticket, host.ID)
			return err
		})
		if ok != 1 {
			t.Fatalf("round %d: one ticket redeemed %d times", round, ok)
		}
	}
}

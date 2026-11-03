package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/enowx/enowx/cloud/store"
	"github.com/enowx/enowx/cloud/store/storetest"
)

// One GitHub account links to one Discord account, and a member may change theirs.
func TestDiscordLinks(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	if err := db.LinkDiscord(ctx, "d1", 42); err != nil {
		t.Fatal(err)
	}
	if err := db.LinkDiscord(ctx, "d2", 42); !errors.Is(err, store.ErrGitHubLinked) {
		t.Fatalf("a second member: %v", err)
	}
	if err := db.LinkDiscord(ctx, "d1", 42); err != nil {
		t.Fatalf("linking again: %v", err)
	}
	if err := db.LinkDiscord(ctx, "d1", 43); err != nil {
		t.Fatalf("another account of theirs: %v", err)
	}
	if id, err := db.DiscordOfGitHub(ctx, 42); err != nil || id != "" {
		t.Fatalf("the account left: %q %v", id, err)
	}
	if id, err := db.DiscordOfGitHub(ctx, 43); err != nil || id != "d1" {
		t.Fatalf("the account linked: %q %v", id, err)
	}
	if err := db.UnlinkDiscord(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	if on, err := db.DiscordLinked(ctx, "d1"); err != nil || on {
		t.Fatalf("still linked: %v %v", on, err)
	}
	if err := db.UnlinkDiscord(ctx, "d1"); err != nil {
		t.Fatalf("nothing to remove: %v", err)
	}
}

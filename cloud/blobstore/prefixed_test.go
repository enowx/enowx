package blobstore

import (
	"context"
	"errors"
	"testing"
)

// A prefixed store reaches what's under its prefix and nothing else, whatever
// key it's given: a team's files in the same store stay out of its reach.
func TestPrefixedStaysInItsPrefix(t *testing.T) {
	ctx := context.Background()
	shared := Dir(t.TempDir())
	if err := shared.Put(ctx, "teams/t1/secret", []byte("a team's file")); err != nil {
		t.Fatal(err)
	}
	lib, err := Prefixed(shared, "library")
	if err != nil {
		t.Fatal(err)
	}
	if err := lib.Put(ctx, "items/i1/v1/doc", []byte("doc")); err != nil {
		t.Fatal(err)
	}
	if got, err := shared.Get(ctx, "library/items/i1/v1/doc"); err != nil || string(got) != "doc" {
		t.Fatalf("not under the prefix: %q %v", got, err)
	}
	for _, key := range []string{"../teams/t1/secret", "/teams/t1/secret", "items/../../teams/t1/secret", "..", "", "ITEMS/x", "a//b", "a/", `a\b`, "a b", "a%2f..%2fb"} {
		if _, err := lib.Get(ctx, key); err == nil {
			t.Errorf("Get(%q) was taken", key)
		}
		if err := lib.Put(ctx, key, []byte("x")); err == nil {
			t.Errorf("Put(%q) was taken", key)
		}
		if err := lib.Delete(ctx, key); err == nil {
			t.Errorf("Delete(%q) was taken", key)
		}
	}
	// The same key names something else inside: a team's path isn't reached.
	if _, err := lib.Get(ctx, "teams/t1/secret"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reached a team's file: %v", err)
	}
	if err := lib.Delete(ctx, "teams/t1/secret"); err != nil && !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if got, err := shared.Get(ctx, "teams/t1/secret"); err != nil || string(got) != "a team's file" {
		t.Fatalf("a team's file changed: %q %v", got, err)
	}
	if _, err := Prefixed(shared, "../up"); err == nil {
		t.Fatal("a prefix leaving the store was taken")
	}
}

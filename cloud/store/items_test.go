package store_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store/storetest"
)

// A page stops at its size in bytes and at its count, and paging through them
// gives every item once, in order, however large.
func TestItemPages(t *testing.T) {
	db := storetest.Open(t)
	ctx := context.Background()
	acc, err := db.GitHubAccount(ctx, 7, "pages", "Pages", "", "")
	if err != nil {
		t.Fatal(err)
	}
	dev, _, err := db.AddDevice(ctx, acc.ID, "d", api.DeviceApp, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	team, err := db.CreateTeam(ctx, acc.ID, "t")
	if err != nil {
		t.Fatal(err)
	}
	put := func(i, size int) {
		t.Helper()
		id := fmt.Sprintf("%032x", i+1)
		if _, err := db.PutItem(ctx, team.ID, acc.ID, dev.ID, id, api.PutItemParams{Kind: api.ItemNote, Version: 1, Epoch: 1, Sealed: bytes.Repeat([]byte{1}, size)}); err != nil {
			t.Fatal(err)
		}
	}
	// 3 MB each: two fit in 8 MB's page, the third doesn't.
	const big = 5
	for i := range big {
		put(i, 3<<20)
	}
	// Small ones then fill pages by count.
	const small = 450
	for i := range small {
		put(big+i, 8)
	}
	var sizes []int
	seen, after := 0, int64(0)
	for {
		page, err := db.Items(ctx, team.ID, acc.ID, after)
		if err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, len(page.Items))
		for _, it := range page.Items {
			if want := fmt.Sprintf("%032x", seen+1); it.ID != want {
				t.Fatalf("item %d is %s, not %s", seen, it.ID, want)
			}
			if it.Seq <= after {
				t.Fatalf("item %s at seq %d, after %d", it.ID, it.Seq, after)
			}
			after = it.Seq
			seen++
		}
		if page.Seq != after {
			t.Fatalf("the page's seq is %d, its last item's %d", page.Seq, after)
		}
		if !page.More {
			break
		}
	}
	if seen != big+small {
		t.Fatalf("%d items read of %d", seen, big+small)
	}
	if want := []int{2, 2, 200, 200, 51}; !slices.Equal(sizes, want) {
		t.Fatalf("pages of %v, want %v", sizes, want)
	}
	// An item past the page's size alone still comes, by itself.
	put(big+small, 9<<20)
	page, err := db.Items(ctx, team.ID, acc.ID, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.More {
		t.Fatalf("%d items, more %v, want the one large item", len(page.Items), page.More)
	}
}

package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/enowx/enowx/cloud/api"
)

// What a moderator took down stays down: a version prepared before the takedown
// can't bring it back, by the API or by what was already on its way in the store.
func TestLibraryTakedownStays(t *testing.T) {
	e := newLibEnv(t, Config{})
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	id := e.live(ada, "First") // ada is trusted from here
	p := publishParams("Second")
	p.ItemID = id
	var made api.LibraryPublished
	if code, msg := send(t, "POST", e.url+api.PathLibraryItems, ada.token, p, &made); code != http.StatusOK {
		t.Fatalf("v2: %d %s", code, msg)
	}
	if code, msg := e.put(ada.token, partPath(id, made.Version, api.PartSource), bundle(t, `<p>2</p>`)); code != http.StatusNoContent {
		t.Fatalf("source: %d %s", code, msg)
	}
	if code, msg := e.decide(boss, id, 0, api.DecisionRemove, "breaks the rules"); code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", code, msg)
	}
	if code, msg := e.put(ada.token, partPath(id, made.Version, api.PartThumb), thumb(t)); code != http.StatusGone {
		t.Errorf("a part after the takedown: %d %s", code, msg)
	}
	if code, msg := send(t, "POST", e.url+submitPath(id, made.Version), ada.token, nil, nil); code != http.StatusGone {
		t.Errorf("submit after the takedown: %d %s", code, msg)
	}
	ctx := context.Background()
	for _, status := range []string{api.LibraryLive, api.LibraryReview} {
		if err := e.db.SubmitLibraryVersion(ctx, ada.acc.ID, id, made.Version, status, "[]"); err == nil {
			t.Errorf("the store took a version as %s on what was taken down", status)
		}
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryItem, id), "", nil, nil); code != http.StatusGone {
		t.Errorf("it shows again: %d", code)
	}
}

package server

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/enowx/enowx/cloud/api"
)

func picturePath(id string, part string) string {
	return strings.NewReplacer("{id}", id, "{version}", strconv.Itoa(1), "{part}", part).Replace(api.PathLibraryPicture)
}

// Anyone gets a live version's picture, and nothing else from this route: not
// before it shows, not once it's taken back, and never its document.
func TestLibraryPicturesArePublicDocumentsAreNot(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "ada@secret.example")
	sub := e.publish(ada, publishParams("First"), `<h1>hi</h1>`)
	pic := e.url + picturePath(sub.ID, api.PartThumb)
	if code, _ := send(t, "GET", pic, "", nil, nil); code != http.StatusNotFound {
		t.Fatalf("in review: %d", code)
	}
	boss := e.webSession(e.admin.acc)
	if code, msg := e.decide(boss, sub.ID, 1, api.DecisionApprove, "fine"); code != http.StatusNoContent {
		t.Fatalf("approve: %d %s", code, msg)
	}
	res, err := http.Get(pic)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(res.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("live: %d %v", res.StatusCode, res.Header)
	}
	for _, part := range []string{api.PartDoc, api.PartDocPhone, api.PartSource, "../../doc"} {
		if code, _ := send(t, "GET", e.url+picturePath(sub.ID, part), "", nil, nil); code == http.StatusOK {
			t.Fatalf("%s through the pictures' route: %d", part, code)
		}
	}
	if code, _ := send(t, "GET", e.url+partPath(sub.ID, 1, api.PartDoc), "", nil, nil); code != http.StatusForbidden {
		t.Fatalf("a document without the website's secret: %d", code)
	}
	if code, _ := send(t, "DELETE", e.url+itemPath(api.PathLibraryItem, sub.ID), ada.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("unpublish: %d", code)
	}
	if code, _ := send(t, "GET", pic, "", nil, nil); code != http.StatusGone {
		t.Fatalf("taken back: %d", code)
	}
}

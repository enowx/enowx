package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/core/designshare"
	"github.com/enowx/enowx/proto"
)

// connectApp connects as the app says it is.
func connectApp(t *testing.T, srv *Server, onEvent func(proto.Event)) *client.Client {
	t.Helper()
	was := client.Kind
	client.Kind = "app"
	defer func() { client.Kind = was }()
	return connect(t, srv, onEvent)
}

// sentItem is a design of this account sent to the cloud, in review.
func (e *libraryEnv) sentItem(t *testing.T, frame, title, license string) proto.LibraryDesign {
	t.Helper()
	data := []byte(`{"format":"` + designshare.Format + `","frame":{"name":"Home","width":800,"height":600,"html":"<h1>Hi</h1>"}}`)
	id := designshare.DraftID("canvas", frame)
	d := libraryDraft{Form: proto.LibraryForm{Title: title}}
	d.ID, d.Canvas, d.Frame = id, "canvas", frame
	d.Prepared = proto.LibraryPrepared{Width: 800, Height: 600, MadeWith: "agent"}
	if err := e.srv.putLibraryDraft(d, map[string][]byte{"bundle.json": data, "thumb.webp": libraryThumb(t)}); err != nil {
		t.Fatal(err)
	}
	got, err := e.srv.libraryPublish(proto.LibraryPublishParams{DraftID: id, Form: proto.LibraryForm{Kind: api.LibraryPage, License: license, Title: title, Description: "A page",
		Category: "dashboard", Prompt: "Build it", PromptApproved: true, Rights: true}})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// liveItem is a design of this account that shows: a moderator approves the
// first, and an account so trusted goes live directly.
func (e *libraryEnv) liveItem(t *testing.T, frame, title, license string) proto.LibraryDesign {
	t.Helper()
	got := e.sentItem(t, frame, title, license)
	if got.Status == api.LibraryLive {
		return got
	}
	if err := e.db.DecideLibraryVersion(context.Background(), e.acc.ID, got.ItemID, got.Version, true, "fine"); err != nil {
		t.Fatal(err)
	}
	return got
}

// Search, an item, its prompt and its picture, through the protocol: only what's
// live, the license gate kept, and no email in any answer.
func TestLibraryBrowseThroughTheProtocol(t *testing.T) {
	e := newLibraryEnv(t, true)
	waiting := e.sentItem(t, "waiting", "Not yet", api.LicenseMIT) // before the account is trusted
	open := e.liveItem(t, "open", "Harbor ops", api.LicenseMIT)
	closed := e.liveItem(t, "closed", "Quiet page", api.LicenseViewOnly)
	c := connect(t, e.srv, nil)

	res, err := c.LibrarySearch(proto.LibrarySearchParams{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.On || len(res.Items) != 2 {
		t.Fatalf("search: %+v", res)
	}
	found, err := c.LibrarySearch(proto.LibrarySearchParams{Query: "Harbor", Category: "dashboard"})
	if err != nil || len(found.Items) != 1 || found.Items[0].ItemID != open.ItemID {
		t.Fatalf("narrowed: %v %+v", err, found)
	}
	it := found.Items[0]
	if !strings.HasPrefix(it.URL, "https://ui.example.com/d/"+open.ItemID+"/") || it.Handle == "" || !it.Actions.CopyPrompt {
		t.Fatalf("an item: %+v", it)
	}
	raw, _ := json.Marshal(res)
	if strings.Contains(string(raw), "ada@example.com") {
		t.Fatalf("an email in an answer: %s", raw)
	}

	if _, err := c.LibraryItem(waiting.ItemID); err == nil {
		t.Fatal("an item in review, to its creator's token: a host's agent reads only what's live")
	}
	if _, err := c.LibraryPrompt(waiting.ItemID); err == nil {
		t.Fatal("a prompt in review")
	}

	got, err := c.LibraryPrompt(open.ItemID)
	if err != nil || !strings.Contains(got.Prompt, "Build it") || !strings.Contains(got.Prompt, "License: mit") {
		t.Fatalf("a prompt: %v %+v", err, got)
	}
	if _, err := c.LibraryPrompt(closed.ItemID); err == nil || !strings.Contains(err.Error(), "view only") {
		t.Fatalf("a view-only design's prompt: %v", err)
	} else if strings.Contains(err.Error(), "Quiet page") {
		t.Fatalf("other people's text in an error: %v", err)
	}
	if viewOnly, err := c.LibraryItem(closed.ItemID); err != nil || viewOnly.Actions != (proto.LibraryActions{}) {
		t.Fatalf("view only: %v %+v", err, viewOnly)
	}

	pic, err := c.LibraryPicture(proto.LibraryPictureParams{ItemID: open.ItemID, Version: it.Version})
	if err != nil || len(pic) == 0 {
		t.Fatalf("a picture: %v %d", err, len(pic))
	}
	if _, err := c.LibraryPicture(proto.LibraryPictureParams{ItemID: waiting.ItemID, Version: waiting.Version}); err == nil {
		t.Fatal("a picture in review")
	}

	if _, err := e.srv.libraryWithdraw(proto.LibraryItemRef{ItemID: open.ItemID}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.LibraryItem(open.ItemID); err == nil || !strings.Contains(err.Error(), "taken down or unpublished") {
		t.Fatalf("taken back: %v", err)
	}
}

// With no library, or no cloud, browsing shows nothing and isn't an error.
func TestLibraryBrowseOffWithoutACloudThatHasIt(t *testing.T) {
	for name, srv := range map[string]*Server{"without one": newLibraryEnv(t, false).srv, "no cloud": startServer(t)} {
		res, err := connect(t, srv, nil).LibrarySearch(proto.LibrarySearchParams{Query: "x"})
		if err != nil || res.On || len(res.Items) != 0 {
			t.Errorf("%s: %v %+v", name, err, res)
		}
	}
}

// library.publish is the app form's: a command's connection is refused, and
// host.call, from whatever connection, has no such method. The draft stays and
// nothing reaches the cloud.
func TestLibraryPublishOnlyFromTheApp(t *testing.T) {
	e := newLibraryEnv(t, true)
	data := []byte(`{"format":"` + designshare.Format + `","frame":{"name":"Home","width":800,"height":600,"html":"<h1>Hi</h1>"}}`)
	id := designshare.DraftID("canvas", "frame")
	d := libraryDraft{Form: proto.LibraryForm{Title: "Home"}}
	d.ID, d.Canvas, d.Frame = id, "canvas", "frame"
	d.Prepared = proto.LibraryPrepared{Width: 800, Height: 600}
	if err := e.srv.putLibraryDraft(d, map[string][]byte{"bundle.json": data, "thumb.webp": libraryThumb(t)}); err != nil {
		t.Fatal(err)
	}
	form := proto.LibraryForm{Kind: api.LibraryPage, License: api.LicenseMIT, Title: "Home", Category: "other", Prompt: "x", PromptApproved: true, Rights: true}
	p := proto.LibraryPublishParams{DraftID: id, Form: form, AcceptWarnings: true}

	if _, err := connect(t, e.srv, nil).LibraryPublish(p); err == nil || !strings.Contains(err.Error(), "app's form") {
		t.Fatalf("a command's connection: %v", err)
	}
	app := connectApp(t, e.srv, nil)
	raw, _ := json.Marshal(p)
	for _, c := range []*client.Client{connect(t, e.srv, nil), app} {
		var out proto.LibraryDesign
		err := c.Call(proto.MethodHostCall, proto.HostCallParams{Method: proto.MethodLibraryPublish, Params: raw, From: "session"}, &out)
		if err == nil {
			t.Fatalf("through host.call: %+v", out)
		}
	}
	if st, err := e.srv.libraryStatus(); err != nil || len(st.Designs) != 0 {
		t.Fatalf("something reached the cloud: %v %+v", err, st)
	}
	sent, err := app.LibraryPublish(p)
	if err != nil || sent.ItemID == "" {
		t.Fatalf("from the app: %v %+v", err, sent)
	}
}

// Asking for the form tells the app and does nothing else: no draft, nothing sent.
func TestLibraryAskPublishOnlyShowsTheForm(t *testing.T) {
	e := newLibraryEnv(t, true)
	asked := make(chan proto.LibraryPublishAsked, 1)
	app := connectApp(t, e.srv, func(ev proto.Event) {
		if ev.Name == proto.EventLibraryPublishAsked {
			var a proto.LibraryPublishAsked
			if jsonOK(ev.Data, &a) {
				asked <- a
			}
		}
	})
	cv, err := app.CreateCanvas(proto.CanvasCreateParams{Title: "Ridgeline", Kind: proto.CanvasBoard})
	if err != nil {
		t.Fatal(err)
	}
	c := connect(t, e.srv, nil)
	got, err := c.LibraryAskPublish(proto.LibraryAskPublishParams{Canvas: cv.ID, Frame: cv.Frames[0].ID, From: "session"})
	if err != nil {
		t.Fatal(err)
	}
	if got.AppOpen {
		t.Fatal("no app has said it serves yet")
	}
	if err := app.Call(proto.MethodAppServe, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got, err = c.LibraryAskPublish(proto.LibraryAskPublishParams{Canvas: cv.ID, Frame: "1", From: "session"}); err != nil || !got.AppOpen || got.FrameName != cv.Frames[0].Name {
		t.Fatalf("asked: %v %+v", err, got)
	}
	select {
	case a := <-asked:
		if a.Canvas != cv.ID || a.CanvasTitle != "Ridgeline" || a.From != "session" {
			t.Fatalf("the event: %+v", a)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the app wasn't told")
	}
	if ids := e.srv.libraryDrafts().IDs(); len(ids) != 0 {
		t.Fatalf("a draft was made: %v", ids)
	}
	if st, err := e.srv.libraryStatus(); err != nil || len(st.Designs) != 0 {
		t.Fatalf("something was sent: %v %+v", err, st)
	}
	if _, err := c.LibraryAskPublish(proto.LibraryAskPublishParams{Canvas: "nope", Frame: "1"}); err == nil {
		t.Fatal("asked about no canvas")
	}
	if _, err := connect(t, newLibraryEnv(t, false).srv, nil).LibraryAskPublish(proto.LibraryAskPublishParams{Canvas: cv.ID}); err == nil {
		t.Fatal("asked with no library")
	}
}

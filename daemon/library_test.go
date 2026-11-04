package daemon

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/blobstore"
	"github.com/enowx/enowx/cloud/hostauth"
	"github.com/enowx/enowx/cloud/seal"
	"github.com/enowx/enowx/cloud/server"
	"github.com/enowx/enowx/cloud/store/storetest"
	"github.com/enowx/enowx/core/designshare"
	"github.com/enowx/enowx/core/designshare/thumb"
	"github.com/enowx/enowx/proto"
)

// onePixelPNG stands in for what the app renders.
var onePixelPNG, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")

type libraryEnv struct {
	srv   *Server
	blobs blobstore.Dir
	cloud string
}

// newLibraryEnv is a daemon signed in to a cloud that has a library. An app
// stands by to render pictures. library false is a cloud without one.
func newLibraryEnv(t *testing.T, library bool) *libraryEnv {
	t.Helper()
	db := storetest.Open(t)
	blobs := blobstore.Dir(t.TempDir())
	cfg := server.Config{}
	if library {
		cfg.Library = &server.LibraryConfig{Blobs: blobs, PublicURL: "https://ui.example.com"}
	}
	web := httptest.NewServer(server.New(cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(web.Close)
	ctx := context.Background()
	acc, err := db.GitHubAccount(ctx, 7, "ada", "Ada", "ada@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	k, _ := seal.NewDeviceKey()
	_, token, err := db.AddDevice(ctx, acc.ID, "host", api.DeviceHost, k.Public())
	if err != nil {
		t.Fatal(err)
	}
	srv := startServer(t)
	if err := srv.cloud.store.Put(hostauth.State{URL: web.URL, Token: token}); err != nil {
		t.Fatal(err)
	}
	return &libraryEnv{srv: srv, blobs: blobs, cloud: web.URL}
}

// The whole way from a draft to the cloud, the bundle holding only the design:
// comments and the other designs of the canvas don't reach the cloud's bucket.
func TestLibraryDraftReachesTheCloudHoldingOnlyTheDesign(t *testing.T) {
	e := newLibraryEnv(t, true)
	data := []byte(`{"format":"` + designshare.Format + `","frame":{"name":"Home","width":800,"height":600,"html":"<h1>Hi</h1>"}}`)
	id := designshare.DraftID("canvas", "frame")
	d := libraryDraft{Form: proto.LibraryForm{Title: "Home"}}
	d.ID, d.Canvas, d.Frame = id, "canvas", "frame"
	d.Prepared = proto.LibraryPrepared{Width: 800, Height: 600, MadeWith: "agent"}
	webpBytes := libraryThumb(t)
	if err := e.srv.putLibraryDraft(d, map[string][]byte{"bundle.json": data, "thumb.webp": webpBytes}); err != nil {
		t.Fatal(err)
	}
	form := proto.LibraryForm{Kind: api.LibraryPage, License: api.LicenseMIT, Title: "Home", Description: "A page", Category: "other", Prompt: "Build it", PromptApproved: true, Rights: true}
	if _, err := e.srv.libraryPublish(proto.LibraryPublishParams{DraftID: id, Form: proto.LibraryForm{Title: "Home"}}); err == nil {
		t.Fatal("published without the rights statement")
	}
	got, err := e.srv.libraryPublish(proto.LibraryPublishParams{DraftID: id, Form: form})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != api.LibraryReview || got.ItemID == "" || !strings.Contains(got.URL, "https://ui.example.com/d/"+got.ItemID+"/home") {
		t.Fatalf("published: %+v", got)
	}
	if _, err := e.blobs.Get(context.Background(), "library/items/"+got.ItemID+"/v1/doc"); err != nil {
		t.Fatalf("no document: %v", err)
	}
	if s := e.srv.libraryDrafts().File(id, "bundle.json"); s != nil {
		t.Fatal("a sent draft stayed")
	}
	if e.srv.libraryDrafts().ItemOf("canvas", "frame") != got.ItemID {
		t.Fatal("the design isn't linked to its item")
	}
	st, err := e.srv.libraryStatus()
	if err != nil || !st.On || !st.SignedIn || len(st.Designs) != 1 || st.Designs[0].Status != api.LibraryReview || st.Designs[0].Canvas != "canvas" {
		t.Fatalf("status: %v %+v", err, st)
	}
	if _, err := e.srv.libraryWithdraw(proto.LibraryItemRef{ItemID: got.ItemID}); err != nil {
		t.Fatal(err)
	}
	if e.srv.libraryDrafts().ItemOf("canvas", "frame") != "" {
		t.Fatal("still linked once withdrawn")
	}
}

// From the canvas through the daemon's protocol: a design with its picture
// (shapes need no app to draw) is prepared, a phone design of the same page goes
// with it, and what the canvas holds around them stays behind.
func TestLibraryPrepareAndPublishThroughTheProtocol(t *testing.T) {
	e := newLibraryEnv(t, true)
	c := connect(t, e.srv, nil)
	cv, err := c.CreateCanvas(proto.CanvasCreateParams{Title: "Ridgeline", Kind: proto.CanvasBoard})
	if err != nil {
		t.Fatal(err)
	}
	main := cv.Frames[0]
	page := "Home"
	for _, ed := range []proto.CanvasEdit{
		{ID: cv.ID, Op: proto.CanvasOpFrameMeta, Frame: main.ID, Page: &page},
		{ID: cv.ID, Op: proto.CanvasOpFrameAdd, FrameInfo: &proto.CanvasFrame{ID: "phone", Name: "Phone", Width: 390, Height: 844}},
	} {
		if cv, err = c.EditCanvas(ed); err != nil {
			t.Fatal(err)
		}
	}
	phone := cv.Frames[1]
	if cv, err = c.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpFrameMeta, Frame: phone.ID, Page: &page}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CommentCanvas(proto.CanvasCommentParams{ID: cv.ID, Op: proto.CanvasCommentAdd, Comment: proto.CanvasComment{Frame: main.ID, Text: "PRIVATE COMMENT"}}); err != nil {
		t.Fatal(err)
	}
	if main.Width <= phone.Width {
		t.Skipf("the first frame is %d wide", main.Width)
	}
	got, err := c.LibraryPrepare(proto.LibraryPrepareParams{Canvas: cv.ID, Frame: main.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Phone != phone.ID || !got.PhoneOffered || got.Form.License != api.LicenseCCBY || got.Form.Prompt == "" {
		t.Fatalf("prepared: %+v", got)
	}
	alone, err := c.LibraryPrepare(proto.LibraryPrepareParams{Canvas: cv.ID, Frame: main.ID, NoPhone: true})
	if err != nil || alone.Phone != "" || !alone.PhoneOffered {
		t.Fatalf("without the phone design: %v %+v", err, alone)
	}
	got, err = c.LibraryPrepare(proto.LibraryPrepareParams{Canvas: cv.ID, Frame: main.ID})
	if err != nil {
		t.Fatal(err)
	}
	form := got.Form
	form.PromptApproved, form.Rights = true, true
	form.Title = "A saved title"
	if err := c.LibrarySaveDraft(proto.LibraryDraftParams{DraftID: got.DraftID, Form: form}); err != nil {
		t.Fatal(err)
	}
	st, err := c.LibraryStatus()
	if err != nil || len(st.Designs) != 1 || st.Designs[0].Status != proto.LibraryStatusDraft || st.Designs[0].Title != "A saved title" {
		t.Fatalf("a draft: %v %+v", err, st)
	}
	again, err := c.LibraryPrepare(proto.LibraryPrepareParams{Canvas: cv.ID, Frame: main.ID})
	if err != nil || again.Form.Title != "A saved title" {
		t.Fatalf("the draft comes back: %v %+v", err, again.Form)
	}
	sent, err := c.LibraryPublish(proto.LibraryPublishParams{DraftID: got.DraftID, Form: form, AcceptWarnings: true})
	if err != nil {
		t.Fatal(err)
	}
	if sent.Status != api.LibraryReview {
		t.Fatalf("sent: %+v", sent)
	}
	ctx := context.Background()
	for _, part := range []string{"source", "doc", "doc-phone", "thumb", "thumb-phone"} {
		if _, err := e.blobs.Get(ctx, "library/items/"+sent.ItemID+"/v1/"+part); err != nil {
			t.Errorf("%s: %v", part, err)
		}
	}
	raw, err := e.blobs.Get(ctx, "library/items/"+sent.ItemID+"/v1/source")
	if err != nil {
		t.Fatal(err)
	}
	if text := gunzipForTest(t, raw); strings.Contains(text, "PRIVATE COMMENT") || strings.Contains(text, "Ridgeline") {
		t.Fatalf("the bundle holds what's around the design: %.200s", text)
	}
	// A new version keeps the address.
	next, err := c.LibraryPrepare(proto.LibraryPrepareParams{Canvas: cv.ID, Frame: main.ID})
	if err != nil || next.ItemID != sent.ItemID || next.Form.Title != "A saved title" || next.Form.Prompt == "" || next.Form.Rights {
		t.Fatalf("an update: %v %+v", err, next)
	}
	form.PromptApproved, form.Rights = true, true
	updated, err := c.LibraryPublish(proto.LibraryPublishParams{DraftID: next.DraftID, Form: form, AcceptWarnings: true})
	if err != nil || updated.ItemID != sent.ItemID || updated.Version != 2 {
		t.Fatalf("updated: %v %+v", err, updated)
	}
	if _, err := c.LibraryWithdraw(sent.ItemID); err != nil {
		t.Fatal(err)
	}
}

func gunzipForTest(t *testing.T, data []byte) string {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// A secret never leaves the host: refused before the first request to the cloud.
func TestLibraryRefusesSecretsOnTheHost(t *testing.T) {
	e := newLibraryEnv(t, true)
	secret := "ghp_" + strings.Repeat("a1B2", 10)
	data := []byte(`{"format":"` + designshare.Format + `","frame":{"name":"Home","width":800,"height":600,"html":"<p>` + secret + `</p>"}}`)
	id := designshare.DraftID("canvas", "secret")
	d := libraryDraft{}
	d.ID, d.Canvas, d.Frame = id, "canvas", "secret"
	if err := e.srv.putLibraryDraft(d, map[string][]byte{"bundle.json": data}); err != nil {
		t.Fatal(err)
	}
	form := proto.LibraryForm{Kind: api.LibraryPage, License: api.LicenseMIT, Title: "Home", Category: "other", Prompt: "x", PromptApproved: true, Rights: true}
	got, err := e.srv.libraryPublish(proto.LibraryPublishParams{DraftID: id, Form: form, AcceptWarnings: true})
	if err == nil {
		t.Fatal("a secret was sent")
	}
	if strings.Contains(err.Error(), secret) || len(got.Findings) == 0 || strings.Contains(got.Findings[0].Where+got.Findings[0].Kind, secret) {
		t.Fatalf("the value shows: %v %+v", err, got)
	}
	st, _ := e.srv.libraryStatus()
	if len(st.Designs) != 0 {
		t.Fatalf("something reached the cloud: %+v", st.Designs)
	}
}

// Warnings wait for the creator's word.
func TestLibraryWarningsNeedAccepting(t *testing.T) {
	e := newLibraryEnv(t, true)
	data := []byte(`{"format":"` + designshare.Format + `","frame":{"name":"Home","width":800,"height":600,"html":"<p>ada@example.com</p>"}}`)
	id := designshare.DraftID("canvas", "warn")
	d := libraryDraft{}
	d.ID, d.Canvas, d.Frame = id, "canvas", "warn"
	d.Prepared = proto.LibraryPrepared{Width: 800, Height: 600}
	e.srv.putLibraryDraft(d, map[string][]byte{"bundle.json": data, "thumb.webp": libraryThumb(t)})
	form := proto.LibraryForm{Kind: api.LibraryPage, License: api.LicenseMIT, Title: "Home", Category: "other", Prompt: "x", PromptApproved: true, Rights: true}
	if _, err := e.srv.libraryPublish(proto.LibraryPublishParams{DraftID: id, Form: form}); err == nil {
		t.Fatal("sent without accepting the warnings")
	}
	if _, err := e.srv.libraryPublish(proto.LibraryPublishParams{DraftID: id, Form: form, AcceptWarnings: true}); err != nil {
		t.Fatal(err)
	}
}

// A cloud without a library, or none at all, shows nothing and asks nothing.
func TestLibraryOffWithoutACloudThatHasIt(t *testing.T) {
	e := newLibraryEnv(t, false)
	st, err := e.srv.libraryStatus()
	if err != nil || st.On {
		t.Fatalf("a cloud without one: %v %+v", err, st)
	}
	st, err = startServer(t).libraryStatus()
	if err != nil || st.On {
		t.Fatalf("no cloud: %v %+v", err, st)
	}
}

func libraryThumb(t *testing.T) []byte {
	t.Helper()
	data, err := thumb.WebP(onePixelPNG)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

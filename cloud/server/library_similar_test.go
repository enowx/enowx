package server

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"math/rand"
	"net/http"
	"testing"

	"github.com/gen2brain/webp"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/proto"
)

// layout draws a page-like picture of a seed's blocks.
func layout(seed int64, w, h int) image.Image {
	r := rand.New(rand.NewSource(seed))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{245, 245, 240, 255}), image.Point{}, draw.Src)
	for range 14 {
		x, y := r.Float64(), r.Float64()
		bw, bh := 0.1+r.Float64()*0.4, 0.05+r.Float64()*0.3
		shade := uint8(r.Intn(180))
		rect := image.Rect(int(x*float64(w)), int(y*float64(h)), int((x+bw)*float64(w)), int((y+bh)*float64(h)))
		draw.Draw(img, rect, image.NewUniform(color.RGBA{shade, shade, shade, 255}), image.Point{}, draw.Src)
	}
	return img
}

func webpOf(t *testing.T, img image.Image, quality int) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := webp.Encode(&out, img, webp.Options{Quality: quality}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// A moderator sees designs that look close beside the one under review, and that
// is all that happens: nothing is refused, held or changed by it.
func TestLibrarySimilarIsAdviceForModerators(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.trusted(1, "ada-orig")
	original := e.publishFrame(ada, publishParams("The original"), proto.CanvasFrame{Name: "Home", Width: 800, Height: 600, HTML: "<p>x</p>"}, webpOf(t, layout(7, 800, 500), 80))
	e.publishFrame(ada, publishParams("Something else"), proto.CanvasFrame{Name: "Home", Width: 800, Height: 600, HTML: "<p>y</p>"}, webpOf(t, layout(99, 800, 500), 80))

	// Someone new sends the same picture, smaller and worse.
	bob := e.signIn(2, "bob", "bob@example.com")
	copied := e.publishFrame(bob, publishParams("Totally mine"), proto.CanvasFrame{Name: "Home", Width: 800, Height: 600, HTML: "<p>z</p>"}, webpOf(t, layout(7, 480, 300), 40))
	if copied.Status != api.LibraryReview {
		t.Fatalf("status: %s", copied.Status)
	}
	// And someone a blank picture, which says nothing.
	blank := e.signIn(3, "cleo", "cleo@example.com")
	plain := e.publish(blank, publishParams("Blank"), "<p>b</p>")

	entriesBefore := e.logCount()
	var q api.LibraryQueue
	if code, msg := send(t, "GET", e.url+api.PathLibraryQueue, e.admin.token, nil, &q); code != http.StatusOK {
		t.Fatalf("queue: %d %s", code, msg)
	}
	var got, gotBlank *api.LibraryQueueItem
	for i := range q.Items {
		switch q.Items[i].ID {
		case copied.ID:
			got = &q.Items[i]
		case plain.ID:
			gotBlank = &q.Items[i]
		}
	}
	if got == nil || gotBlank == nil {
		t.Fatalf("queue: %+v", q.Items)
	}
	if !got.SimilarChecked || len(got.Similar) != 1 || got.Similar[0].ItemID != original.ID || got.Similar[0].Title != "The original" || got.Similar[0].Creator == "" || got.Similar[0].SameCreator {
		t.Fatalf("near matches: %+v", got)
	}
	if got.Similar[0].Distance > 10 {
		t.Fatalf("distance: %d", got.Similar[0].Distance)
	}
	if gotBlank.SimilarChecked || len(gotBlank.Similar) != 0 {
		t.Fatalf("a blank picture: %+v", gotBlank)
	}

	// Advice only: both still stand as they did, the log has no new line.
	var it api.LibraryItem
	send(t, "GET", e.url+itemPath(api.PathLibraryItem, original.ID), "", nil, &it)
	if it.Status != api.LibraryLive {
		t.Fatalf("the original: %s", it.Status)
	}
	if after := e.logCount(); after != entriesBefore {
		t.Fatalf("log: %d, then %d", entriesBefore, after)
	}
	// A trusted account's lookalike isn't held either.
	if s := e.publishFrame(ada, publishParams("Her own again"), proto.CanvasFrame{Name: "Home", Width: 800, Height: 600, HTML: "<p>x</p>"}, webpOf(t, layout(7, 800, 500), 80)); s.Status != api.LibraryLive {
		t.Fatalf("a trusted lookalike: %s", s.Status)
	}
	// Nobody else reads the queue.
	if code, _ := send(t, "GET", e.url+api.PathLibraryQueue, bob.token, nil, nil); code != http.StatusForbidden {
		t.Fatalf("queue as a user: %d", code)
	}
}

func (e *libEnv) logCount() int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM library_log`).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

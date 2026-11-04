package daemon

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/core/designshare"
	"github.com/enowx/enowx/proto"
)

// silentMP4 stands in for what the app's encoder makes: the boxes the library reads.
func silentMP4() []byte {
	box := func(kind string, parts ...[]byte) []byte {
		size := 8
		for _, p := range parts {
			size += len(p)
		}
		out := append(binary.BigEndian.AppendUint32(nil, uint32(size)), kind...)
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	mvhd := make([]byte, 20)
	binary.BigEndian.PutUint32(mvhd[12:], 1000)
	binary.BigEndian.PutUint32(mvhd[16:], 3000)
	hdlr := append(append(make([]byte, 8), "vide"...), make([]byte, 12)...)
	return append(box("ftyp", []byte("isom")), box("moov", box("mvhd", mvhd), box("trak", box("mdia", box("hdlr", hdlr))))...)
}

// libraryVideoEnv is a moving design as a draft, and an app answering for videos
// with answer, recording what it was asked.
func libraryVideoEnv(t *testing.T, answer func(proto.CanvasVideoRequest) proto.CanvasVideoReply) (*libraryEnv, string, chan proto.CanvasVideoRequest) {
	t.Helper()
	e := newLibraryEnv(t, true)
	asked := make(chan proto.CanvasVideoRequest, 4)
	var app *client.Client
	app = connect(t, e.srv, func(ev proto.Event) {
		var req proto.CanvasVideoRequest
		if ev.Name != proto.EventCanvasVideo || json.Unmarshal(ev.Data, &req) != nil {
			return
		}
		asked <- req
		go func() {
			r := answer(req)
			r.ID = req.ID
			_ = app.CanvasVideoReply(r)
		}()
	})
	if err := app.ServeBrowsers(); err != nil {
		t.Fatal(err)
	}
	cv, err := app.CreateCanvas(proto.CanvasCreateParams{Title: "Moving", Kind: proto.CanvasBoard})
	if err != nil {
		t.Fatal(err)
	}
	frame := cv.Frames[0]
	const page = `<style>@keyframes a{to{opacity:0}}h1{animation:a 1s infinite}</style><h1>Hi</h1>`
	if _, err := app.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLSet, Frame: frame.ID, HTML: page}); err != nil {
		t.Fatal(err)
	}
	data, err := designshare.Build(proto.CanvasFrame{Name: "Home", Width: 1440, Height: 900, HTML: "<h1>Hi</h1>"}, nil, nil).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	id := designshare.DraftID(cv.ID, frame.ID)
	d := libraryDraft{Form: proto.LibraryForm{Title: "Moving"}}
	d.ID, d.Canvas, d.Frame = id, cv.ID, frame.ID
	d.Prepared = proto.LibraryPrepared{Width: 1440, Height: 900, MadeWith: "hand", HasMotion: true}
	files := map[string][]byte{"bundle.json": data, "thumb.webp": libraryThumb(t), libraryVideoFile: e.srv.libraryVideo(cv.ID, frame.ID, 1440)}
	if err := e.srv.putLibraryDraft(d, files); err != nil {
		t.Fatal(err)
	}
	return e, id, asked
}

func movingForm() proto.LibraryForm {
	return proto.LibraryForm{Kind: api.LibraryPage, License: api.LicenseMIT, Title: "Moving", Description: "It moves", Category: "motion", Prompt: "Build it", PromptApproved: true, Rights: true}
}

// A design that moves has the app make its short video, small and left in no folder,
// with its draft, and that reaches the cloud.
func TestLibraryPublishCarriesAVideo(t *testing.T) {
	video := silentMP4()
	e, id, asked := libraryVideoEnv(t, func(proto.CanvasVideoRequest) proto.CanvasVideoReply {
		return proto.CanvasVideoReply{Result: proto.CanvasVideoResult{Data: video}}
	})
	got, err := e.srv.libraryPublish(proto.LibraryPublishParams{DraftID: id, Form: movingForm()})
	if err != nil {
		t.Fatal(err)
	}
	req := <-asked
	if !req.Preview || req.OutWidth != libraryVideoWidth || req.To != libraryVideoMs {
		t.Fatalf("what the app was asked: %+v", req)
	}
	kept, err := e.blobs.Get(context.Background(), "library/items/"+got.ItemID+"/v1/video")
	if err != nil || string(kept) != string(video) {
		t.Fatalf("the video in the bucket: %d bytes, %v", len(kept), err)
	}
}

// Without ffmpeg (the app says so), with what isn't a silent MP4, or when the form
// says not to, the design goes without one and is published all the same.
func TestLibraryPublishGoesOnWithoutAVideo(t *testing.T) {
	for name, c := range map[string]struct {
		reply   proto.CanvasVideoReply
		noVideo bool
	}{
		"no ffmpeg":     {reply: proto.CanvasVideoReply{Error: "MP4 needs ffmpeg, which isn't installed here."}},
		"not an MP4":    {reply: proto.CanvasVideoReply{Result: proto.CanvasVideoResult{Data: []byte("<html>")}}},
		"too big":       {reply: proto.CanvasVideoReply{Result: proto.CanvasVideoResult{Data: make([]byte, api.MaxLibraryVideo+1)}}},
		"no data":       {reply: proto.CanvasVideoReply{Result: proto.CanvasVideoResult{Path: "/somewhere.mp4"}}},
		"form says not": {noVideo: true},
	} {
		t.Run(name, func(t *testing.T) {
			e, id, asked := libraryVideoEnv(t, func(proto.CanvasVideoRequest) proto.CanvasVideoReply { return c.reply })
			form := movingForm()
			form.NoVideo = c.noVideo
			got, err := e.srv.libraryPublish(proto.LibraryPublishParams{DraftID: id, Form: form})
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != api.LibraryReview {
				t.Fatalf("published: %+v", got)
			}
			_ = asked // the draft asks in any case: what the form says is known at send
			if _, err := e.blobs.Get(context.Background(), "library/items/"+got.ItemID+"/v1/video"); err == nil {
				t.Fatal("a video went")
			}
		})
	}
}

// A host with no app serving publishes too.
func TestLibraryPublishWithNoAppForAVideo(t *testing.T) {
	e := newLibraryEnv(t, true)
	data, err := designshare.Build(proto.CanvasFrame{Name: "Home", Width: 800, Height: 600, HTML: "<h1>Hi</h1>"}, nil, nil).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	id := designshare.DraftID("canvas", "frame")
	d := libraryDraft{Form: proto.LibraryForm{Title: "Moving"}}
	d.ID, d.Canvas, d.Frame = id, "canvas", "frame"
	d.Prepared = proto.LibraryPrepared{Width: 800, Height: 600, MadeWith: "hand", HasMotion: true}
	if err := e.srv.putLibraryDraft(d, map[string][]byte{"bundle.json": data, "thumb.webp": libraryThumb(t)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.srv.libraryPublish(proto.LibraryPublishParams{DraftID: id, Form: movingForm()}); err != nil {
		t.Fatal(err)
	}
}

// The video is of what the creator saw in the form: it's made with the draft, not
// when it's sent, by which time the canvas may show something else.
func TestLibraryVideoIsMadeWithTheDraft(t *testing.T) {
	e := newLibraryEnv(t, true)
	video := silentMP4()
	var asked atomic.Int32
	var app *client.Client
	app = connectApp(t, e.srv, func(ev proto.Event) {
		switch ev.Name {
		case proto.EventCanvasVideo:
			var req proto.CanvasVideoRequest
			if json.Unmarshal(ev.Data, &req) == nil {
				asked.Add(1)
				go func() {
					_ = app.CanvasVideoReply(proto.CanvasVideoReply{ID: req.ID, Result: proto.CanvasVideoResult{Data: video}})
				}()
			}
		case proto.EventCanvasRender:
			var req proto.CanvasRenderRequest
			if json.Unmarshal(ev.Data, &req) == nil {
				go func() { _ = app.CanvasRenderReply(proto.CanvasRenderReply{ID: req.ID, Data: onePixelPNG}) }()
			}
		}
	})
	if err := app.ServeBrowsers(); err != nil {
		t.Fatal(err)
	}
	cv, err := app.CreateCanvas(proto.CanvasCreateParams{Title: "Moving", Kind: proto.CanvasBoard})
	if err != nil {
		t.Fatal(err)
	}
	frame := cv.Frames[0]
	const moving = `<style>@keyframes a{to{opacity:0}}h1{animation:a 1s infinite}</style><h1>Hi</h1>`
	if _, err := app.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLSet, Frame: frame.ID, HTML: moving}); err != nil {
		t.Fatal(err)
	}
	prepared, err := app.LibraryPrepare(proto.LibraryPrepareParams{Canvas: cv.ID, Frame: frame.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := asked.Load(); got != 1 {
		t.Fatalf("made %d videos with the draft", got)
	}
	if _, err := app.EditCanvas(proto.CanvasEdit{ID: cv.ID, Op: proto.CanvasOpHTMLSet, Frame: frame.ID, HTML: "<h1>Something private since</h1>"}); err != nil {
		t.Fatal(err)
	}
	got, err := app.LibraryPublish(proto.LibraryPublishParams{DraftID: prepared.DraftID, Form: movingForm(), AcceptWarnings: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := asked.Load(); n != 1 {
		t.Fatalf("the canvas was recorded again at send: %d", n)
	}
	kept, err := e.blobs.Get(context.Background(), "library/items/"+got.ItemID+"/v1/video")
	if err != nil || string(kept) != string(video) {
		t.Fatalf("the video in the bucket: %d bytes, %v", len(kept), err)
	}
}

package ui

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/communityview"
)

// pageCommunity is the community library, reached from the Canvas page's list
// when the host's cloud has one.
const pageCommunity = "community"

// communityPage is the page, begun anew when the host changed: another host may
// be set to another cloud.
func (a *App) communityPage(c *ui.Context) {
	if a.community == nil || a.communityHost != a.host.ID {
		a.community, a.communityHost = &communityview.View{}, a.host.ID
	}
	a.community.Draw(c, a.communityOptions())
}

func (a *App) communityOptions() communityview.Options {
	cl := a.client
	return communityview.Options{
		Search: func(p proto.LibrarySearchParams, done func(proto.LibraryResults, error)) {
			go func() {
				res, err := cl.LibrarySearch(p)
				a.onMain(func() { done(res, explained(err)) })
			}()
		},
		Prompt: func(id string, done func(string, error)) {
			go func() {
				got, err := cl.LibraryPrompt(id)
				a.onMain(func() { done(got.Prompt, explained(err)) })
			}()
		},
		Picture: func(it proto.LibraryListing, done func(*ui.Bitmap, error)) {
			go func() {
				data, err := cl.LibraryPicture(proto.LibraryPictureParams{ItemID: it.ItemID, Version: it.Version})
				var bmp *ui.Bitmap
				if err == nil {
					bmp, err = communityview.DecodePicture(data)
				}
				a.onMain(func() { done(bmp, err) })
			}()
		},
		OpenURL: func(url string) { go func() { _ = mygo.Shell.OpenExternal(url) }() },
		Back:    func() { a.page = pageCanvas },
	}
}

// explained is err as the page says it.
func explained(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(strings.TrimPrefix(client.Explain(err), "library: "))
}

// libraryAsked is an agent or a command asking to show the user a design's
// publish form (library publish). It moves nothing: a card says so, and only a
// click on it opens the form, where the user fills it in and sends it, or doesn't.
func (a *App) libraryAsked(data json.RawMessage) func() {
	var ask proto.LibraryPublishAsked
	if json.Unmarshal(data, &ask) != nil {
		return nil
	}
	return func() {
		text := "Asked to share a design to the Community Library"
		if ask.From != "" {
			text = "An agent asks you to share a design to the Community Library"
		}
		if ask.FrameName != "" {
			text += ": " + ask.FrameName
		}
		a.toastAction("library:"+ask.Canvas+":"+ask.Frame, ask.CanvasTitle, text+". Nothing is sent until you do it in the form.", func() {
			hc := a.canvasesOf()
			a.page, hc.open, hc.frame, hc.compare = pageCanvas, ask.Canvas, ask.Frame, nil
			a.canvases.ShareDesign(a.canvasOptions(), ask.Canvas, ask.Frame)
		})
	}
}

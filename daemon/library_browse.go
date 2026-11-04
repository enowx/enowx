package daemon

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/enowx/enowx/cloud/api"
	cloudclient "github.com/enowx/enowx/cloud/client"
	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
)

// Reading the community library: only ever the cloud the user set, with no other
// address tried. Only what's live: a moderator's or a creator's token may see
// more of the cloud, and an agent on their host doesn't get that by asking here.

// libraryReader is a client of a cloud that has a library, and its website.
func (s *Server) libraryReader(ctx context.Context) (*cloudclient.Client, string, error) {
	c, _, err := s.libraryClient()
	if err != nil {
		return nil, "", err
	}
	on, site, err := c.LibraryHas(ctx)
	if err != nil {
		return nil, "", err
	}
	if !on {
		return nil, "", errors.New("library: this cloud has no community library")
	}
	return c, site, nil
}

func libraryListing(it api.LibraryItem, site string) proto.LibraryListing {
	a := it.Actions
	return proto.LibraryListing{ItemID: it.ID, Kind: it.Kind, License: it.License, Title: it.Title, Description: it.Description, Category: it.Category, Tags: it.Tags,
		HasMotion: it.HasMotion, MadeWith: it.MadeWith, Version: it.LiveVersion, Width: it.Width, Height: it.Height, HasPhone: it.HasPhone, Featured: it.Featured,
		Handle: it.Creator.Handle, Creator: it.Creator.Name, URL: libraryURL(site, it.ID, it.Slug), Updated: it.Updated,
		Actions: proto.LibraryActions{CopyPrompt: a.CopyPrompt, UseComponent: a.UseComponent, UseReference: a.UseReference, OpenInEnowX: a.OpenInEnowX}}
}

// librarySearch is a page of the library. No cloud, or one without a library,
// isn't an error: nothing shows.
func (s *Server) librarySearch(p proto.LibrarySearchParams) (proto.LibraryResults, error) {
	var out proto.LibraryResults
	c, _, err := s.libraryClient()
	if err != nil {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), libraryTimeout)
	defer cancel()
	on, site, err := c.LibraryHas(ctx)
	if err != nil {
		return out, err
	}
	out.On, out.SiteURL = on, site
	if !on {
		return out, nil
	}
	q := url.Values{}
	for k, v := range map[string]string{"q": p.Query, "category": p.Category, "kind": p.Kind, "license": p.License, "sort": p.Sort, "cursor": p.Cursor} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	page, err := c.LibrarySearch(ctx, q)
	if err != nil {
		return out, err
	}
	for _, it := range page.Items {
		out.Items = append(out.Items, libraryListing(it, site))
	}
	out.Next = page.Next
	return out, nil
}

// liveItem is item id if it shows to anyone now.
func (s *Server) liveItem(ctx context.Context, c *cloudclient.Client, id, site string) (proto.LibraryListing, error) {
	it, err := c.LibraryItem(ctx, id)
	if cloudclient.IsStatus(err, http.StatusGone) {
		return proto.LibraryListing{}, errors.New("library: this design was taken down or unpublished")
	}
	if err != nil {
		return proto.LibraryListing{}, err
	}
	if it.Status != api.LibraryLive {
		return proto.LibraryListing{}, errors.New("library: no such design is live")
	}
	return libraryListing(it, site), nil
}

func (s *Server) libraryItem(p proto.LibraryItemRef) (proto.LibraryListing, error) {
	ctx, cancel := context.WithTimeout(context.Background(), libraryTimeout)
	defer cancel()
	c, site, err := s.libraryReader(ctx)
	if err != nil {
		return proto.LibraryListing{}, err
	}
	return s.liveItem(ctx, c, p.ItemID, site)
}

// libraryPrompt is an item's prompt. A view-only item's isn't asked for, and the
// cloud would refuse it anyway.
func (s *Server) libraryPrompt(p proto.LibraryItemRef) (proto.LibraryPromptAnswer, error) {
	var out proto.LibraryPromptAnswer
	ctx, cancel := context.WithTimeout(context.Background(), libraryTimeout)
	defer cancel()
	c, site, err := s.libraryReader(ctx)
	if err != nil {
		return out, err
	}
	if out.Item, err = s.liveItem(ctx, c, p.ItemID, site); err != nil {
		return out, err
	}
	if !out.Item.Actions.CopyPrompt {
		// No title here: an error isn't in the block that labels other people's text.
		return out, errors.New("library: this design is view only: its prompt isn't given, and it may only be looked at")
	}
	out.Prompt, err = c.LibraryPrompt(ctx, p.ItemID)
	return out, err
}

// libraryPicture is a live version's picture, through the host so the app needs
// no way of its own to the cloud.
func (s *Server) libraryPicture(p proto.LibraryPictureParams) (proto.LibraryPicture, error) {
	c, _, err := s.libraryClient()
	if err != nil {
		return proto.LibraryPicture{}, err
	}
	part := api.PartThumb
	if p.Phone {
		part = api.PartThumbPhone
	}
	ctx, cancel := context.WithTimeout(context.Background(), libraryTimeout)
	defer cancel()
	data, err := c.LibraryPicture(ctx, p.ItemID, p.Version, part)
	if err != nil {
		return proto.LibraryPicture{}, err
	}
	if len(data) > api.MaxLibraryThumb {
		return proto.LibraryPicture{}, errors.New("library: this picture is larger than the library allows")
	}
	return proto.LibraryPicture{Data: data}, nil
}

// libraryAskPublish shows the user a design's form, for an agent or a command.
// It makes no draft and sends nothing: what's in the form, and whether it's sent,
// are the user's.
func (s *Server) libraryAskPublish(p proto.LibraryAskPublishParams) (proto.LibraryPublishAsked, error) {
	var out proto.LibraryPublishAsked
	ctx, cancel := context.WithTimeout(context.Background(), libraryTimeout)
	defer cancel()
	if _, _, err := s.libraryReader(ctx); err != nil {
		return out, err
	}
	if err := s.mayShare(p.Canvas); err != nil {
		return out, err
	}
	cv, err := s.canvasFor(p.Canvas).Get(p.Canvas)
	if err != nil {
		return out, err
	}
	i, err := canvas.FrameIndex(cv, p.Frame)
	if err != nil {
		return out, err
	}
	out = proto.LibraryPublishAsked{Canvas: cv.ID, Frame: cv.Frames[i].ID, CanvasTitle: cv.Title, FrameName: cv.Frames[i].Name, From: p.From}
	s.appReqs.mu.Lock()
	out.AppOpen = len(s.appReqs.serving) > 0
	s.appReqs.mu.Unlock()
	s.broadcast(proto.EventLibraryPublishAsked, out)
	return out, nil
}

// errPublishFromForm is what anything but the app's form gets from library.publish.
var errPublishFromForm = errors.New("library: a design is published from the enowX app's form, by the user: library publish opens it for them")

// fromTheApp says c may send a draft: a connection on this host that says it's
// the app. The app's form is its only caller. A command, an agent's tool, the
// daemon's own calls and a teammate's visit are refused here, and host.call has
// no such method (core/hostcontrol). A connection's word for what it is can be
// false: this keeps honest paths away from sending, it doesn't stand against a
// program running as the user, which can read the host's cloud token as well.
func (c *conn) fromTheApp() error {
	if c.self || c.remote || c.kind != "app" {
		return errPublishFromForm
	}
	return nil
}

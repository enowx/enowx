package daemon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
	cloudclient "github.com/enowx/enowx/cloud/client"
	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/core/designshare"
	"github.com/enowx/enowx/core/designshare/thumb"
	"github.com/enowx/enowx/proto"
)

// Sharing a canvas's design to the community library (docs/canvas.md): a draft
// on this host, checked and packed, then sent to the cloud once the user said
// so in the form. The cloud is the one the user set; nothing goes elsewhere.
// The canvas's comments, brief, history and chats aren't read here.

// libraryDraftLife is how long a draft nobody saved stays on the host.
const libraryDraftLife = 7 * 24 * time.Hour

// libraryTimeout bounds a call to the cloud that isn't a file's.
const libraryTimeout = 20 * time.Second

// libraryEmbedTimeout bounds fetching a design's libraries from their CDNs.
const libraryEmbedTimeout = 45 * time.Second

func (s *Server) libraryDrafts() *designshare.Drafts {
	return designshare.OpenDrafts(filepath.Join(s.dataDir, "library"))
}

// libraryDraft is a draft's record, its fields typed.
type libraryDraft struct {
	designshare.Draft
	Form     proto.LibraryForm     `json:"form"`
	Prepared proto.LibraryPrepared `json:"prepared"`
}

func (s *Server) putLibraryDraft(d libraryDraft, files map[string][]byte) error {
	rec := d.Draft
	rec.Form, rec.Prepared, rec.Title = d.Form, d.Prepared, d.Form.Title
	return s.libraryDrafts().Put(rec, files)
}

func (s *Server) libraryDraft(id string) (libraryDraft, error) {
	var d libraryDraft
	err := s.libraryDrafts().Get(id, &d)
	return d, err
}

// mayShare says whether the design of canvas id may leave its space: a team's
// only when its admins let members copy its items out.
func (s *Server) mayShare(id string) error {
	if team := s.spaceOf("canvas", id); team != "" {
		return s.mayMoveSpace(team, "")
	}
	return nil
}

// phoneFor is the design a phone would be shown instead of f: another of the
// same page, narrower.
func phoneFor(cv proto.Canvas, f proto.CanvasFrame) *proto.CanvasFrame {
	if f.Page == "" {
		return nil
	}
	for i := range cv.Frames {
		o := &cv.Frames[i]
		if o.ID != f.ID && o.Page == f.Page && o.Width < f.Width && o.Width <= 600 {
			return o
		}
	}
	return nil
}

// libraryPrepare checks and packs a design as a draft, for the form.
func (s *Server) libraryPrepare(p proto.LibraryPrepareParams) (proto.LibraryPrepared, error) {
	s.libraryDrafts().Prune(libraryDraftLife)
	var out proto.LibraryPrepared
	c, st, err := s.libraryClient()
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), libraryTimeout)
	defer cancel()
	on, site, err := c.LibraryHas(ctx)
	if err != nil {
		return out, err
	}
	if !on {
		return out, errors.New("library: this cloud has no community library")
	}
	if !st {
		return out, errors.New("library: sign in to the cloud first")
	}
	if err := s.mayShare(p.Canvas); err != nil {
		return out, err
	}
	store := s.canvasFor(p.Canvas)
	cv, err := store.Get(p.Canvas)
	if err != nil {
		return out, err
	}
	i, err := canvas.FrameIndex(cv, p.Frame)
	if err != nil {
		return out, err
	}
	frame := cv.Frames[i]
	phone := phoneFor(cv, frame)
	offered := phone != nil
	if p.NoPhone {
		phone = nil
	}
	assets := func(name string) ([]byte, error) { return store.Asset(cv.ID, name) }
	bundle := designshare.Build(frame, phone, assets)
	embed, stop := context.WithTimeout(context.Background(), libraryEmbedTimeout)
	defer stop()
	bundle.Frame.HTML = designshare.EmbedLibraries(embed, bundle.Frame.HTML, nil)
	if bundle.Phone != nil {
		bundle.Phone.HTML = designshare.EmbedLibraries(embed, bundle.Phone.HTML, nil)
	}
	data, err := bundle.Marshal()
	if err != nil {
		return out, err
	}
	if len(data) > api.MaxLibraryBundle {
		return out, fmt.Errorf("library: this design is %d MB with its files, and the library takes 10 MB", len(data)>>20)
	}
	files := map[string][]byte{"bundle.json": data}
	shot, err := s.canvasRender(proto.CanvasRenderParams{ID: cv.ID, Frame: frame.ID, Format: proto.RenderPNG})
	if err != nil {
		return out, fmt.Errorf("library: a picture of the design: %w", err)
	}
	if files["thumb.webp"], err = thumb.WebP(shot.Data); err != nil {
		return out, err
	}
	if phone != nil {
		pic, err := s.canvasRender(proto.CanvasRenderParams{ID: cv.ID, Frame: phone.ID, Format: proto.RenderPNG})
		if err != nil {
			return out, fmt.Errorf("library: a picture of the phone design: %w", err)
		}
		if files["thumb-phone.webp"], err = thumb.WebP(pic.Data); err != nil {
			return out, err
		}
	}
	id := designshare.DraftID(cv.ID, frame.ID)
	var old libraryDraft
	form := proto.LibraryForm{Kind: api.LibraryPage, License: api.LicenseCCBY, Title: orStr(frame.Name, cv.Title), Description: frame.Description, Category: "other"}
	item := s.libraryDrafts().ItemOf(cv.ID, frame.ID)
	mine, mineErr := c.LibraryMine(ctx)
	switch {
	case s.libraryDrafts().Get(id, &old) == nil && old.Form.Title != "":
		form = old.Form
	case item != "" && mineErr == nil:
		// An update starts from what shows; approval and rights are said again.
		for _, it := range mine.Items {
			if it.ID == item {
				form = proto.LibraryForm{Kind: it.Kind, License: it.License, Title: it.Title, Description: it.Description, Category: it.Category, Tags: it.Tags, Style: it.Style, SourceURL: it.SourceURL, Prompt: it.Latest.Prompt}
			}
		}
	}
	if form.Prompt == "" {
		form.Prompt = designshare.DraftPrompt(bundle, form.Title, form.Description, form.Kind)
	}
	out = proto.LibraryPrepared{DraftID: id, Canvas: cv.ID, Frame: frame.ID, FrameName: frame.Name, Width: frame.Width, Height: frame.Height,
		PhoneOffered: offered, Files: len(bundle.Assets), OtherDesigns: len(cv.Frames) - 1, Size: int64(len(data)), MadeWith: "hand",
		HasMotion: canvas.UsesMotion(frame.HTML), Form: form, ItemID: item, SiteURL: site}
	if phone != nil {
		out.Phone, out.PhoneName, out.PhoneWidth, out.PhoneHeight = phone.ID, phone.Name, phone.Width, phone.Height
		out.OtherDesigns--
	}
	if cv.Agent != "" {
		out.MadeWith = "agent"
	}
	out.Findings = findingsFor(designshare.Check(bundle, designshare.Text{Title: form.Title, Description: form.Description, Prompt: form.Prompt}))
	if mineErr == nil {
		out.Trust = mine.Trust
	}
	// Empty when there's none, so that one from an earlier look doesn't stay.
	files[libraryVideoFile] = nil
	if out.HasMotion {
		files[libraryVideoFile] = s.libraryVideo(cv.ID, frame.ID, frame.Width)
	}
	d := libraryDraft{Form: form, Prepared: out}
	d.ID, d.Canvas, d.Frame, d.Saved = id, cv.ID, frame.ID, old.Saved
	if err := s.putLibraryDraft(d, files); err != nil {
		return out, err
	}
	return out, nil
}

func findingsFor(found []designshare.Finding) []proto.LibraryFinding {
	out := make([]proto.LibraryFinding, len(found))
	for i, f := range found {
		out[i] = proto.LibraryFinding{Severity: f.Severity, Kind: f.Kind, Where: f.Where}
	}
	return out
}

// libraryClient is a client of the host's cloud, and whether it's signed in.
func (s *Server) libraryClient() (*cloudclient.Client, bool, error) {
	st, err := s.cloud.store.Get()
	if err != nil {
		return nil, false, err
	}
	if st.URL == "" {
		return nil, false, errors.New("library: no cloud is set up on this host")
	}
	c, err := cloudclient.New(st.URL, st.Token)
	if err != nil {
		return nil, false, err
	}
	return c, st.SignedIn(), nil
}

// librarySaveDraft keeps what the form holds, on this host alone.
func (s *Server) librarySaveDraft(p proto.LibraryDraftParams) error {
	d, err := s.libraryDraft(p.DraftID)
	if err != nil {
		return err
	}
	d.Form, d.Saved = p.Form, true
	return s.putLibraryDraft(d, nil)
}

// libraryPublish sends a draft: the checks once more, with what the form says
// now, then the objects, then submit. What a check refuses never leaves.
func (s *Server) libraryPublish(p proto.LibraryPublishParams) (proto.LibraryDesign, error) {
	var out proto.LibraryDesign
	d, err := s.libraryDraft(p.DraftID)
	if err != nil {
		return out, err
	}
	if !p.Form.Rights || !p.Form.PromptApproved {
		return out, errors.New("library: approve the prompt and say you have the right to share this design")
	}
	if err := s.mayShare(d.Canvas); err != nil {
		return out, err
	}
	data := s.libraryDrafts().File(p.DraftID, "bundle.json")
	bundle, err := designshare.Parse(data)
	if err != nil {
		return out, err
	}
	found := designshare.Check(bundle, designshare.Text{Title: p.Form.Title, Description: p.Form.Description, Prompt: p.Form.Prompt})
	out.Findings = findingsFor(found)
	if designshare.Refused(found) {
		return out, errors.New("library: the check found something that can't be shared: fix it first")
	}
	if len(found) > 0 && !p.AcceptWarnings {
		return out, errors.New("library: look at the check's warnings first")
	}
	c, signedIn, err := s.libraryClient()
	if err != nil {
		return out, err
	}
	if !signedIn {
		return out, errors.New("library: sign in to the cloud first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	made, err := c.LibraryCreate(ctx, api.LibraryPublishParams{ItemID: d.Prepared.ItemID, Kind: p.Form.Kind, License: p.Form.License, Title: p.Form.Title, Description: p.Form.Description,
		Category: p.Form.Category, Tags: p.Form.Tags, Style: p.Form.Style, HasMotion: d.Prepared.HasMotion, MadeWith: d.Prepared.MadeWith, SourceURL: p.Form.SourceURL, Prompt: p.Form.Prompt,
		PromptApproved: p.Form.PromptApproved, Rights: p.Form.Rights, Format: bundle.Format, Width: d.Prepared.Width, Height: d.Prepared.Height})
	if err != nil {
		return out, libraryError(err, &out)
	}
	var video []byte
	if d.Prepared.HasMotion && !p.Form.NoVideo {
		video = s.libraryDrafts().File(p.DraftID, libraryVideoFile)
	}
	parts := []struct{ part, file string }{{api.PartSource, "bundle.json"}, {api.PartThumb, "thumb.webp"}, {api.PartThumbPhone, "thumb-phone.webp"}}
	for _, part := range parts {
		content := s.libraryDrafts().File(p.DraftID, part.file)
		if len(content) == 0 {
			continue
		}
		if err := c.LibraryPut(ctx, made.ID, made.Version, part.part, content); err != nil {
			return out, libraryError(err, &out)
		}
	}
	// A cloud too old for a video, or one refusing it, costs the design nothing.
	if len(video) > 0 {
		if err := c.LibraryPut(ctx, made.ID, made.Version, api.PartVideo, video); err != nil {
			s.logger().Warn("library: a design's video stays behind", "err", err)
		}
	}
	sent, err := c.LibrarySubmit(ctx, made.ID, made.Version)
	if err != nil {
		return out, libraryError(err, &out)
	}
	if err := s.libraryDrafts().SetLink(d.Canvas, d.Frame, made.ID); err != nil {
		s.logger().Warn("library: link", "err", err)
	}
	if err := s.libraryDrafts().Delete(p.DraftID); err != nil {
		s.logger().Warn("library: a sent draft stays", "err", err)
	}
	_, site, _ := c.LibraryHas(ctx)
	return proto.LibraryDesign{Canvas: d.Canvas, Frame: d.Frame, ItemID: made.ID, Title: p.Form.Title, Status: sent.Status, Version: made.Version,
		URL: libraryURL(site, made.ID, designshare.Slug(p.Form.Title)), Updated: time.Now().UTC()}, nil
}

// The short video of a moving design: enough of it to see how it moves.
const (
	libraryVideoMs    = 6000
	libraryVideoWidth = 720
)

// libraryVideoFile is a draft's video, of what its form was shown.
const libraryVideoFile = "video.mp4"

// libraryVideo has the app render a design that moves as a short video, from the
// same render as its MP4 export, for its draft: what's sent has to be what the
// creator saw, and the canvas may have changed by then. It's a nicety: without an
// app serving, a web view or ffmpeg, or when it comes out too big, the design goes
// without one.
func (s *Server) libraryVideo(canvasID, frame string, width int) []byte {
	res, err := s.canvasVideo(proto.CanvasVideoParams{ID: canvasID, Frame: frame, Width: min(width, libraryVideoWidth), To: libraryVideoMs, Preview: true})
	if err != nil {
		s.logger().Warn("library: no video of this design", "err", err)
		return nil
	}
	if len(res.Data) == 0 || len(res.Data) > api.MaxLibraryVideo {
		s.logger().Warn("library: no video of this design: too big", "bytes", len(res.Data))
		return nil
	}
	if err := designshare.CheckVideo(res.Data, api.MaxLibraryVideoSeconds); err != nil {
		s.logger().Warn("library: no video of this design", "err", err)
		return nil
	}
	return res.Data
}

// libraryError keeps what the cloud's check found, for the form.
func libraryError(err error, out *proto.LibraryDesign) error {
	var r *cloudclient.LibraryRefused
	if errors.As(err, &r) {
		out.Findings = out.Findings[:0]
		for _, f := range r.Findings {
			out.Findings = append(out.Findings, proto.LibraryFinding{Severity: f.Severity, Kind: f.Kind, Where: f.Where})
		}
	}
	return err
}

func libraryURL(site, id, slug string) string {
	if site == "" {
		return ""
	}
	return strings.TrimSuffix(site, "/") + "/d/" + id + "/" + slug
}

// libraryStatus says whether the host's cloud has a library, and how this
// account's designs stand: those in the cloud, and the drafts kept here.
func (s *Server) libraryStatus() (proto.LibraryState, error) {
	var out proto.LibraryState
	c, signedIn, err := s.libraryClient()
	if err != nil {
		return out, nil // no cloud, no library: not an error to show
	}
	ctx, cancel := context.WithTimeout(context.Background(), libraryTimeout)
	defer cancel()
	on, site, err := c.LibraryHas(ctx)
	if err != nil {
		return out, nil // a cloud out of reach shows nothing for now
	}
	out.On, out.SignedIn, out.SiteURL = on, signedIn, site
	if !on {
		return out, nil
	}
	drafts := s.libraryDrafts()
	for _, id := range drafts.IDs() {
		d, err := s.libraryDraft(id)
		if err == nil && d.Saved {
			out.Designs = append(out.Designs, proto.LibraryDesign{Canvas: d.Canvas, Frame: d.Frame, DraftID: id, Title: d.Form.Title, Status: proto.LibraryStatusDraft, Updated: d.Updated})
		}
	}
	if !signedIn {
		return out, nil
	}
	mine, err := c.LibraryMine(ctx)
	if err != nil {
		if cloudclient.IsStatus(err, http.StatusUnauthorized) {
			out.SignedIn = false
			return out, nil
		}
		return out, err
	}
	out.Used, out.Limit = mine.Used, mine.Limit
	links := drafts.Links()
	for _, it := range mine.Items {
		design := proto.LibraryDesign{ItemID: it.ID, Title: it.Title, Status: it.Status, RejectReason: it.RejectReason, Version: it.Latest.Version, URL: libraryURL(site, it.ID, it.Slug), Updated: it.Updated}
		// A newer version waiting says more than what shows.
		if it.Latest.Status == api.LibraryReview || it.Latest.Status == api.LibraryRejected {
			design.Status = it.Latest.Status
		}
		if j := slices.IndexFunc(links, func(l designshare.Link) bool { return l.Item == it.ID }); j >= 0 {
			design.Canvas, design.Frame = links[j].Canvas, links[j].Frame
		}
		out.Designs = append(out.Designs, design)
	}
	return out, nil
}

// libraryWithdraw takes a published design back; copies people made stay theirs.
func (s *Server) libraryWithdraw(p proto.LibraryItemRef) (proto.LibraryDesign, error) {
	c, signedIn, err := s.libraryClient()
	if err != nil {
		return proto.LibraryDesign{}, err
	}
	if !signedIn {
		return proto.LibraryDesign{}, errors.New("library: sign in to the cloud first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), libraryTimeout)
	defer cancel()
	if err := c.LibraryWithdraw(ctx, p.ItemID); err != nil {
		return proto.LibraryDesign{}, err
	}
	drafts := s.libraryDrafts()
	for _, l := range drafts.Links() {
		if l.Item == p.ItemID {
			if err := drafts.SetLink(l.Canvas, l.Frame, ""); err != nil {
				s.logger().Warn("library: link", "err", err)
			}
		}
	}
	return proto.LibraryDesign{ItemID: p.ItemID, Status: api.LibraryUnpublished}, nil
}

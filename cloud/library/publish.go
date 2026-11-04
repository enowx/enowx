package library

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/image/webp"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/blobstore"
	"github.com/enowx/enowx/cloud/store"
	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/core/designshare"
)

// Publishing, from the app: an item is made, its objects put, then it's
// submitted. The server takes no host's word: it checks the bundle again and
// makes the document people see from it itself.

// findingsOf are a check's findings as the API says them.
func findingsOf(found []designshare.Finding) []api.LibraryFinding {
	out := make([]api.LibraryFinding, len(found))
	for i, f := range found {
		out[i] = api.LibraryFinding{Severity: f.Severity, Kind: f.Kind, Where: f.Where}
	}
	return out
}

// refuse answers that a check stopped a publish, with where and of what kind.
func refuse(w http.ResponseWriter, status int, msg string, found []designshare.Finding) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(struct {
		Error    string               `json:"error"`
		Findings []api.LibraryFinding `json:"findings,omitempty"`
	}{msg, findingsOf(found)})
}

var tagJunk = regexp.MustCompile(`[^a-z0-9-]+`)

func cleanTags(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.Trim(tagJunk.ReplaceAllString(strings.ToLower(strings.TrimSpace(t)), "-"), "-")
		if t == "" || len(t) > 32 || seen[t] || len(out) == 8 {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// webURL says u is an address a link may go to: nothing that runs.
func webURL(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "https" || p.Scheme == "http") && p.Host != ""
}

// bucketFull says the bucket's limits are reached, and answers; what's
// published goes on showing.
func (s *Service) bucketFull(w http.ResponseWriter, r *http.Request) bool {
	u, err := s.db.LibraryUsage(r.Context())
	if err != nil {
		s.answer(w, nil, err)
		return true
	}
	if u.BytesStored >= u.StoredLimit || u.BytesWritten >= u.WrittenLimit {
		fail(w, http.StatusInsufficientStorage, "the library can't take new designs now: its storage is full. Everything published still shows")
		return true
	}
	return false
}

func (s *Service) createItem(w http.ResponseWriter, r *http.Request) {
	var p api.LibraryPublishParams
	if !readJSON(w, r, &p) {
		return
	}
	accID := asker(r).account.ID
	me, err := s.db.LibraryProfile(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	p.Title, p.Description, p.Prompt = cleanText(p.Title, 120), cleanText(p.Description, 2000), cleanText(p.Prompt, 8000)
	bad := ""
	switch {
	case me.Trust == api.TrustBlocked:
		fail(w, http.StatusForbidden, "this account can't publish")
		return
	case !p.Rights:
		bad = "say that you have the right to share this design"
	case !p.PromptApproved || p.Prompt == "":
		bad = "approve the prompt people copy"
	case p.Title == "":
		bad = "a design needs a title"
	case p.Format != designshare.Format:
		bad = "this version of the app sends a format the library doesn't know"
	case p.Kind != api.LibraryComponent && p.Kind != api.LibraryPage:
		bad = "a design is a component or a full page"
	case p.License != api.LicenseMIT && p.License != api.LicenseCCBY && p.License != api.LicenseViewOnly:
		bad = "pick a license from the list"
	case !oneOf(api.LibraryCategories, p.Category):
		bad = "pick a category from the list"
	case p.SourceURL != "" && !webURL(p.SourceURL):
		bad = "a source link is a web address"
	case p.Width <= 0 || p.Height <= 0:
		bad = "a design needs its size"
	}
	if bad != "" {
		fail(w, http.StatusBadRequest, bad)
		return
	}
	if found := designshare.Check(designshare.Bundle{Format: designshare.Format}, designshare.Text{Title: p.Title, Description: p.Description, Prompt: p.Prompt}); designshare.Refused(found) {
		refuse(w, http.StatusUnprocessableEntity, "this holds a secret: take it out first", found)
		return
	}
	c, err := s.db.LibraryCounts(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	switch {
	case c.UploadsToday >= api.MaxLibraryUploadsDay:
		w.Header().Set("Retry-After", "3600")
		fail(w, http.StatusTooManyRequests, "that's 20 uploads for today: try again tomorrow")
		return
	case c.Used >= api.MaxLibraryAccount:
		s.answer(w, nil, store.ErrLibraryQuota)
		return
	case p.ItemID == "" && c.Live >= api.MaxLibraryLiveItems:
		fail(w, http.StatusConflict, "an account shows 50 designs at most: unpublish one first")
		return
	case p.ItemID == "" && me.Trust == api.TrustNew && c.InReview >= api.MaxLibraryNewReview:
		fail(w, http.StatusConflict, "you have 3 designs in review: wait for them first")
		return
	}
	if s.bucketFull(w, r) {
		return
	}
	meta, _ := json.Marshal(store.LibraryMeta{Title: p.Title, Description: p.Description, Category: p.Category, Style: styleOf(p.Style), MadeWith: madeWith(p.MadeWith), SourceURL: p.SourceURL, Tags: cleanTags(p.Tags), HasMotion: p.HasMotion})
	version := store.LibraryVersionRow{Kind: p.Kind, License: p.License, Format: p.Format, Prompt: p.Prompt, Width: p.Width, Height: p.Height, Meta: string(meta)}
	if p.ItemID == "" {
		id, err := s.db.AddLibraryItem(r.Context(), store.LibraryNewItem{Owner: accID, Slug: designshare.Slug(p.Title), Kind: p.Kind, License: p.License, Title: p.Title, Description: p.Description,
			Category: p.Category, Style: styleOf(p.Style), MadeWith: madeWith(p.MadeWith), SourceURL: p.SourceURL,
			Tags: cleanTags(p.Tags), HasMotion: p.HasMotion, Version: version})
		s.answer(w, api.LibraryPublished{ID: id, Version: 1}, err)
		return
	}
	it, err := s.ownItem(w, r, p.ItemID)
	if err != nil {
		return
	}
	if gone(it.Status) {
		fail(w, http.StatusGone, "this design was taken down or unpublished: publish it as a new one")
		return
	}
	versions, err := s.db.LibraryVersions(r.Context(), it.ID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if len(versions) >= api.MaxLibraryVersions && versions[0].Status != api.LibraryUploading && !s.dropOldest(r, versions) {
		fail(w, http.StatusConflict, "a design keeps 5 versions at most, and none of its can go now: wait for the one in review")
		return
	}
	n, err := s.db.AddLibraryVersion(r.Context(), it.ID, version)
	s.answer(w, api.LibraryPublished{ID: it.ID, Version: n}, err)
}

// dropOldest makes room for a new version, so that a design at its limit can
// still be updated: the oldest that neither shows nor waits goes, objects first.
// versions are the latest first.
func (s *Service) dropOldest(r *http.Request, versions []store.LibraryVersionRow) bool {
	for i := len(versions) - 1; i >= 0; i-- {
		v := versions[i]
		if v.Status != api.LibrarySuperseded && v.Status != api.LibraryRejected {
			continue
		}
		for _, part := range []string{api.PartSource, api.PartDoc, api.PartDocPhone, api.PartThumb, api.PartThumbPhone} {
			if err := s.objects.Delete(r.Context(), objectKey(v.ItemID, v.Version, part)); err != nil && !errors.Is(err, blobstore.ErrNotFound) {
				s.log.Warn("library: drop a version", "item", v.ItemID, "err", err)
				return false
			}
		}
		if err := s.db.ForgetLibraryVersion(r.Context(), store.LibraryPurge{ItemID: v.ItemID, Version: v.Version}); err != nil {
			s.log.Warn("library: drop a version", "item", v.ItemID, "err", err)
			return false
		}
		return true
	}
	return false
}

func madeWith(v string) string {
	if v == "agent" {
		return "agent"
	}
	return "hand"
}

// ownItem is item id when the asker owns it, or answers with what the error
// says.
func (s *Service) ownItem(w http.ResponseWriter, r *http.Request, id string) (api.LibraryItem, error) {
	it, err := s.db.LibraryItem(r.Context(), id)
	if err != nil {
		s.answer(w, nil, err)
		return it, err
	}
	if it.Creator.ID != asker(r).account.ID {
		// Not shown to exist, as what isn't yours and isn't public doesn't.
		fail(w, http.StatusNotFound, "not found")
		return it, errRefused
	}
	return it, nil
}

// uploading is the asker's version waiting for its objects, or answers.
func (s *Service) uploading(w http.ResponseWriter, r *http.Request) (store.LibraryVersionRow, bool) {
	n, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		fail(w, http.StatusBadRequest, "a version is a number")
		return store.LibraryVersionRow{}, false
	}
	it, err := s.ownItem(w, r, r.PathValue("id"))
	if err != nil {
		return store.LibraryVersionRow{}, false
	}
	if gone(it.Status) {
		fail(w, http.StatusGone, "this design was taken down or unpublished: publish it as a new one")
		return store.LibraryVersionRow{}, false
	}
	v, err := s.db.LibraryVersion(r.Context(), r.PathValue("id"), n)
	if err != nil {
		s.answer(w, nil, err)
		return v, false
	}
	if v.Status != api.LibraryUploading {
		fail(w, http.StatusConflict, "this version was submitted already")
		return v, false
	}
	return v, true
}

// putPart keeps an object of a version: the bundle, or a picture of it. The
// documents are the server's own to make.
func (s *Service) putPart(w http.ResponseWriter, r *http.Request) {
	part := r.PathValue("part")
	limit := int64(api.MaxLibraryThumb)
	switch part {
	case api.PartSource:
		limit = api.MaxLibraryBundle
	case api.PartThumb, api.PartThumbPhone:
	case api.PartVideo:
		limit = api.MaxLibraryVideo
	default:
		fail(w, http.StatusNotFound, "not found")
		return
	}
	v, ok := s.uploading(w, r)
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		fail(w, http.StatusRequestEntityTooLarge, "that's bigger than the library takes: a design is 10 MB at most, a picture 300 KB, a video 4 MB")
		return
	}
	if part == api.PartSource {
		if _, err := designshare.Parse(data); err != nil {
			fail(w, http.StatusBadRequest, "this isn't a design's bundle")
			return
		}
		if data, err = gzipped(data); err != nil {
			s.answer(w, nil, err)
			return
		}
	} else if part == api.PartVideo {
		var meta store.LibraryMeta
		if json.Unmarshal([]byte(v.Meta), &meta) != nil || !meta.HasMotion {
			fail(w, http.StatusBadRequest, "only a design that moves has a video")
			return
		}
		if err := designshare.CheckVideo(data, api.MaxLibraryVideoSeconds); err != nil {
			fail(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "designshare: "))
			return
		}
	} else {
		cfg, err := webp.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			fail(w, http.StatusBadRequest, "a picture is a WebP")
			return
		}
		if cfg.Width > api.MaxLibraryThumbWidth {
			fail(w, http.StatusBadRequest, "a picture is 1200 pixels wide at most")
			return
		}
	}
	if s.bucketFull(w, r) {
		return
	}
	if !s.keep(w, r, v, part, data) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// keep puts an object in the bucket, counted against its owner's account
// first: what doesn't fit is never written.
func (s *Service) keep(w http.ResponseWriter, r *http.Request, v store.LibraryVersionRow, part string, data []byte) bool {
	owner := asker(r).account.ID
	if err := s.db.SetLibraryPartSize(r.Context(), owner, v.ItemID, v.Version, part, int64(len(data)), api.MaxLibraryAccount); err != nil {
		s.answer(w, nil, err)
		return false
	}
	if err := s.objects.Put(r.Context(), objectKey(v.ItemID, v.Version, part), data); err != nil {
		s.log.Error("library: put", "item", v.ItemID, "err", err)
		_ = s.db.SetLibraryPartSize(r.Context(), owner, v.ItemID, v.Version, part, 0, api.MaxLibraryAccount)
		fail(w, http.StatusBadGateway, "the library's storage didn't answer: try again")
		return false
	}
	if err := s.db.AddLibraryUsage(r.Context(), int64(len(data))); err != nil {
		s.log.Warn("library: usage", "err", err)
	}
	return true
}

// documentOf is f's document zipped as it's kept, or answers: what would come to
// too much is never made.
func (s *Service) documentOf(w http.ResponseWriter, f designshare.Frame, assets canvas.Assets) ([]byte, bool) {
	page, err := canvas.FrameDocumentWithin(f.Canvas(), assets, api.MaxLibraryDocument)
	if errors.Is(err, canvas.ErrDocumentTooBig) {
		fail(w, http.StatusUnprocessableEntity, "this design comes to more than 32 MB with its files in place: show each file fewer times")
		return nil, false
	}
	if err != nil {
		s.answer(w, nil, err)
		return nil, false
	}
	doc, err := gzipped([]byte(page))
	if err != nil {
		s.answer(w, nil, err)
		return nil, false
	}
	return doc, true
}

// submit settles a version whose objects are there: checked again, its
// documents made, then in review or live by its owner's standing.
func (s *Service) submit(w http.ResponseWriter, r *http.Request) {
	v, ok := s.uploading(w, r)
	if !ok {
		return
	}
	if v.SourceSize == 0 || v.ThumbSize == 0 {
		fail(w, http.StatusConflict, "this version misses its design or its picture")
		return
	}
	accID := asker(r).account.ID
	me, err := s.db.LibraryProfile(r.Context(), accID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	if me.Trust == api.TrustBlocked {
		fail(w, http.StatusForbidden, "this account can't publish")
		return
	}
	raw, err := s.objects.Get(r.Context(), objectKey(v.ItemID, v.Version, api.PartSource))
	if err != nil {
		s.log.Error("library: get", "item", v.ItemID, "err", err)
		fail(w, http.StatusBadGateway, "the library's storage didn't answer: try again")
		return
	}
	data, err := gunzipped(raw, api.MaxLibraryBundle+1)
	if err != nil || len(data) > api.MaxLibraryBundle {
		fail(w, http.StatusBadRequest, "this isn't a design's bundle")
		return
	}
	b, err := designshare.Parse(data)
	if err != nil {
		fail(w, http.StatusBadRequest, "this isn't a design's bundle")
		return
	}
	it, err := s.db.LibraryItem(r.Context(), v.ItemID)
	if err != nil {
		s.answer(w, nil, err)
		return
	}
	found := designshare.Check(b, designshare.Text{Title: it.Title, Description: it.Description, Prompt: v.Prompt})
	if designshare.Refused(found) {
		refuse(w, http.StatusUnprocessableEntity, "the check stopped this design", found)
		return
	}
	if v.Kind == api.LibraryComponent {
		if err := designshare.CheckComponent(b); err != nil {
			fail(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	assets := b.AssetReader()
	doc, ok := s.documentOf(w, b.Frame, assets)
	if !ok {
		return
	}
	var phone []byte
	if b.Phone != nil {
		if phone, ok = s.documentOf(w, *b.Phone, assets); !ok {
			return
		}
	}
	if !s.keep(w, r, v, api.PartDoc, doc) {
		return
	}
	if phone != nil && !s.keep(w, r, v, api.PartDocPhone, phone) {
		return
	}
	status := api.LibraryReview
	if me.Trust == api.TrustTrusted && (it.LiveVersion == 0 || it.Kind == v.Kind) {
		status = api.LibraryLive
	}
	if err := s.readTraits(r.Context(), v, b); err != nil {
		s.answer(w, nil, err)
		return
	}
	scan, _ := json.Marshal(findingsOf(found))
	if err := s.db.SubmitLibraryVersion(r.Context(), accID, v.ItemID, v.Version, status, string(scan)); err != nil {
		s.answer(w, nil, err)
		return
	}
	reply(w, api.LibrarySubmitted{ID: v.ItemID, Version: v.Version, Status: status, Findings: findingsOf(found)})
}

package library

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store"
)

// Searching by meaning.

// mountSearch adds meaning search's route.
func (s *Service) mountSearch(mux *http.ServeMux) {
	mux.HandleFunc("GET "+api.PathLibraryMeaning, s.searchMeaning)
}

// Embedder makes text into vectors. It's what meaning search leaves to someone
// else: set by the owner, off when nil, and given nothing but a design's public
// title, description and tags, and what people search for.
type Embedder interface {
	// Model names what makes the vectors: vectors of different models don't compare.
	Model() string
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

const (
	// meaningPerMinute is how many searches by meaning an address may make: each
	// one is a call to the embedder, which may cost money.
	meaningPerMinute = 20
	// embedsAtOnce is the most calls to the embedder searches make at once. It
	// holds no database connection meanwhile.
	embedsAtOnce = 2
	// embedTime is the longest a search waits for the embedder.
	embedTime = 8 * time.Second
	// leastMeaning is the least similarity that counts as close. It isn't tuned
	// for any model: that takes the owner's choice of one.
	leastMeaning = 0.3
	// embedBatch is how many designs one go of embedding takes.
	embedBatch = 64
	// embedEvery is how often new and changed designs are embedded.
	embedEvery = 5 * time.Minute
)

// MeaningOn says searching by meaning works here: an embedder is set and the
// database can compare vectors.
func (s *Service) MeaningOn() bool { return s.vectors != nil }

// openMeaning turns meaning search on if it's set up and the database can. It
// never stops the server: without pgvector the library just has no meaning search.
func (s *Service) openMeaning() {
	if s.cfg.Embedder == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	vectors, err := s.db.OpenLibraryVectors(ctx)
	if err != nil {
		s.log.Warn("library: searching by meaning stays off", "err", err, "noPgvector", errors.Is(err, store.ErrNoVectors))
		return
	}
	s.vectors = vectors
}

func (s *Service) searchMeaning(w http.ResponseWriter, r *http.Request) {
	if s.vectors == nil {
		fail(w, http.StatusNotFound, "this library doesn't search by meaning")
		return
	}
	if !s.limits.allow("meaning "+s.addressOf(r), meaningPerMinute) {
		tooMany(w, "that's many searches from here: wait a minute")
		return
	}
	q := r.URL.Query()
	text := cleanText(q.Get("q"), 200)
	if text == "" {
		reply(w, api.LibraryItems{Items: []api.LibraryItem{}})
		return
	}
	select {
	case s.embeds <- struct{}{}:
	default:
		w.Header().Set("Retry-After", "5")
		fail(w, http.StatusServiceUnavailable, "the library is busy: try again")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), embedTime)
	defer cancel()
	vectors, err := s.cfg.Embedder.Embed(ctx, []string{text})
	<-s.embeds
	if err != nil || len(vectors) != 1 {
		s.log.Warn("library: embedder", "err", err)
		fail(w, http.StatusBadGateway, "searching by meaning isn't answering: try again")
		return
	}
	s.public(func(w http.ResponseWriter, r *http.Request) {
		query := queryOf(q)
		query.Limit, _ = strconv.Atoi(q.Get("limit"))
		if query.Limit == 0 {
			query.Limit = 12
		}
		ctx, cancel := context.WithTimeout(r.Context(), searchTime)
		defer cancel()
		items, err := s.vectors.Search(ctx, query, s.cfg.Embedder.Model(), vectors[0], leastMeaning)
		if err != nil {
			s.answer(w, nil, err)
			return
		}
		for i := range items {
			items[i] = shown(items[i])
		}
		reply(w, api.LibraryItems{Items: items})
	})(w, r)
}

// embedLoop keeps vectors up to date until ctx ends.
func (s *Service) embedLoop(ctx context.Context) {
	tick := time.NewTicker(embedEvery)
	defer tick.Stop()
	for {
		s.EmbedOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// EmbedOnce is one go of embedding: what's new or changed gets its vector, and
// what no longer shows loses it. Only public text leaves: never a prompt, a page or
// a picture.
func (s *Service) EmbedOnce(ctx context.Context) {
	if s.vectors == nil {
		return
	}
	if err := s.vectors.Prune(ctx); err != nil {
		s.log.Warn("library: embeddings", "err", err)
	}
	model := s.cfg.Embedder.Model()
	due, err := s.vectors.Due(ctx, model, embedBatch)
	if err != nil {
		s.log.Warn("library: embeddings", "err", err)
		return
	}
	if len(due) == 0 {
		return
	}
	texts := make([]string, len(due))
	for i, d := range due {
		texts[i] = embedText(d)
	}
	vectors, err := s.cfg.Embedder.Embed(ctx, texts)
	if err != nil || len(vectors) != len(due) {
		s.log.Warn("library: embedder", "err", err)
		return
	}
	for i, d := range due {
		if err := s.vectors.Put(ctx, d.ItemID, model, vectors[i]); err != nil {
			s.log.Warn("library: embeddings", "err", err)
		}
	}
}

func embedText(d store.LibraryEmbedding) string {
	return strings.TrimSpace(d.Title + "\n" + d.Description + "\n" + d.Tags)
}

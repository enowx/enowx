package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/library"
)

// fakeEmbedder makes the same vector of the same text, without any service. Words
// that mean the same land on the same place.
type fakeEmbedder struct {
	mu    sync.Mutex
	texts []string
	fail  bool
}

var fakeSynonyms = map[string]string{"boat": "harbor", "ship": "harbor", "vessel": "harbor", "money": "pricing", "cost": "pricing"}

func (f *fakeEmbedder) Model() string { return "fake-1" }

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return nil, io.ErrUnexpectedEOF
	}
	f.texts = append(f.texts, texts...)
	out := make([][]float32, len(texts))
	for i, text := range texts {
		v := make([]float32, 32)
		for _, w := range strings.Fields(strings.ToLower(text)) {
			w = strings.Trim(w, ".,")
			if s, ok := fakeSynonyms[w]; ok {
				w = s
			}
			n := 0
			for _, c := range w {
				n = n*31 + int(c)
			}
			v[(n%32+32)%32]++
		}
		out[i] = v
	}
	return out, nil
}

// pgvectorHere says the test's Postgres has the extension.
func pgvectorHere(t *testing.T, e *libEnv) bool {
	var n int
	if err := e.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM pg_available_extensions WHERE name = 'vector'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// Off unless an embedder is set: nothing in info, no route answering, nothing
// asked of anyone.
func TestLibraryMeaningOffByDefault(t *testing.T) {
	e := newLibEnv(t, Config{})
	var in api.Info
	send(t, "GET", e.url+api.PathInfo, "", nil, &in)
	if slices.Contains(in.Features, api.FeatureLibraryMeaning) {
		t.Fatalf("features: %v", in.Features)
	}
	if code, _ := send(t, "GET", e.url+api.PathLibraryMeaning+"?q=boat", "", nil, nil); code != http.StatusNotFound {
		t.Fatalf("route: %d", code)
	}
}

// With an embedder: on where the database can compare vectors, and where it can't
// (a Postgres without pgvector) the server starts all the same, without it.
func TestLibraryMeaning(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.trusted(1, "ada")
	publish := func(title, about string, tags ...string) string {
		p := publishParams(title)
		p.Description, p.Tags, p.Prompt = about, tags, "SECRET-PROMPT-TEXT"
		return e.publish(ada, p, "<p>HTML-OF-THE-PAGE</p>").ID
	}
	harbor := publish("Harbor", "harbor harbor harbor", "harbor")
	publish("Pricing", "pricing pricing pricing", "pricing")

	fake := &fakeEmbedder{}
	cfg := Config{URL: "https://cloud.example.com", Library: &library.Config{Blobs: e.blobs, WebSecret: libSecret, Embedder: fake}}
	e.url = e.on(cfg, e.db)
	svc, err := library.New(*cfg.Library, library.Host{ClientAddr: func(*http.Request) string { return "test" }}, e.db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	var in api.Info
	send(t, "GET", e.url+api.PathInfo, "", nil, &in)
	if onPostgres() && !pgvectorHere(t, e) {
		// The server is up, and answers as if nothing was set up.
		if slices.Contains(in.Features, api.FeatureLibraryMeaning) || svc.MeaningOn() {
			t.Fatalf("on without pgvector: %v", in.Features)
		}
		if !slices.Contains(in.Features, api.FeatureLibrary) {
			t.Fatalf("the library went with it: %v", in.Features)
		}
		if code, _ := send(t, "GET", e.url+api.PathLibraryMeaning+"?q=boat", "", nil, nil); code != http.StatusNotFound {
			t.Fatalf("route: %d", code)
		}
		svc.EmbedOnce(context.Background())
		if len(fake.texts) != 0 {
			t.Fatalf("embedded with nowhere to keep it: %v", fake.texts)
		}
		return
	}
	if !slices.Contains(in.Features, api.FeatureLibraryMeaning) {
		t.Fatalf("features: %v", in.Features)
	}

	ctx := context.Background()
	svc.EmbedOnce(ctx)
	if len(fake.texts) == 0 {
		t.Fatal("nothing embedded")
	}
	// Public text alone leaves: no prompt, no page.
	for _, text := range fake.texts {
		if strings.Contains(text, "SECRET-PROMPT-TEXT") || strings.Contains(text, "HTML-OF-THE-PAGE") {
			t.Fatalf("this left: %q", text)
		}
	}
	// What has its vector isn't asked for again.
	fake.texts = nil
	svc.EmbedOnce(ctx)
	if len(fake.texts) != 0 {
		t.Fatalf("embedded again: %v", fake.texts)
	}

	meaning := func(query string) []string {
		t.Helper()
		var out api.LibraryItems
		if code, msg := send(t, "GET", e.url+api.PathLibraryMeaning+"?"+query, "", nil, &out); code != http.StatusOK {
			t.Fatalf("%s: %d %s", query, code, msg)
		}
		titles := []string{}
		for _, it := range out.Items {
			if it.Actions == (api.LibraryActions{}) {
				t.Fatalf("no actions: %+v", it)
			}
			titles = append(titles, it.Title)
		}
		return titles
	}
	if got := meaning("q=boat"); len(got) == 0 || got[0] != "Harbor" || slices.Contains(got, "Pricing") {
		t.Fatalf("by meaning: %v", got)
	}
	if got := e.titles("q=boat"); len(got) != 0 {
		t.Fatalf("by the words, this wouldn't be found: %v", got)
	}
	if got := meaning("q=money"); len(got) == 0 || got[0] != "Pricing" {
		t.Fatalf("by meaning: %v", got)
	}
	if got := meaning("q=boat&kind=component"); len(got) != 0 {
		t.Fatalf("filters hold too: %v", got)
	}
	if got := meaning("q=boat&style=" + url.QueryEscape("' OR 1=1 --")); len(got) != 0 {
		t.Fatalf("a filter's value: %v", got)
	}
	if got := meaning("q="); len(got) != 0 {
		t.Fatalf("no words: %v", got)
	}

	// What's taken back stops showing, and its vector goes.
	if code, _ := send(t, "DELETE", e.url+itemPath(api.PathLibraryItem, harbor), ada.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("withdraw: %d", code)
	}
	if got := meaning("q=boat"); slices.Contains(got, "Harbor") {
		t.Fatalf("after taking it back: %v", got)
	}
	svc.EmbedOnce(ctx)
	var kept int
	if err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_embeddings WHERE item_id = $1`, harbor).Scan(&kept); err != nil || kept != 0 {
		t.Fatalf("its vector stayed: %d, %v", kept, err)
	}

	// The embedder away: says so, and the words go on working.
	fake.fail = true
	if code, _ := send(t, "GET", e.url+api.PathLibraryMeaning+"?q=boat", "", nil, nil); code != http.StatusBadGateway {
		t.Fatalf("away: %d", code)
	}
	if code, _ := send(t, "GET", e.url+api.PathLibraryItems+"?q=pricing", "", nil, nil); code != http.StatusOK {
		t.Fatalf("words: %d", code)
	}
}

// Searches by meaning call a service that may cost money: fewer per address.
func TestLibraryMeaningIsLimitedPerAddress(t *testing.T) {
	e := newLibEnv(t, Config{})
	if onPostgres() && !pgvectorHere(t, e) {
		t.Skip("needs pgvector here")
	}
	fake := &fakeEmbedder{}
	e.url = e.on(Config{URL: "https://cloud.example.com", Library: &library.Config{Blobs: e.blobs, WebSecret: libSecret, Embedder: fake}}, e.db)
	last := 0
	for i := 0; i < 25; i++ {
		last, _ = send(t, "GET", e.url+api.PathLibraryMeaning+"?q=boat", "", nil, nil)
		if last == http.StatusTooManyRequests {
			break
		}
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("no limit: %d", last)
	}
	if len(fake.texts) > 20 {
		t.Fatalf("embedded %d", len(fake.texts))
	}
}

// The embedder the owner may name: what it sends, what it reads. A local fake: no
// test reaches a real one.
func TestHTTPEmbedder(t *testing.T) {
	var got struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	key := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		key = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"data":[{"index":1,"embedding":[0,1]},{"index":0,"embedding":[1,0]}]}`))
	}))
	defer srv.Close()
	vectors, err := library.NewHTTPEmbedder(srv.URL+"/v1/", "m", "k").Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "m" || !slices.Equal(got.Input, []string{"a", "b"}) || key != "Bearer k" {
		t.Fatalf("sent %+v, %q", got, key)
	}
	if vectors[0][0] != 1 || vectors[1][1] != 1 {
		t.Fatalf("order: %v", vectors)
	}
	if _, err := library.NewHTTPEmbedder(srv.URL+"/nope", "m", "").Embed(context.Background(), []string{"a"}); err == nil {
		t.Fatal("a 404 read as vectors")
	}
}

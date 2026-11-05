package rag

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/enowx/enowx/proto"
)

// fakeEmbeddings is an OpenAI-compatible embeddings endpoint on this
// machine: a text's words hashed into 128 buckets.
func fakeEmbeddings(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Input []string `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		type item struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		var out struct {
			Data  []item         `json:"data"`
			Usage map[string]int `json:"usage"`
		}
		for i, text := range in.Input {
			v := make([]float32, 128)
			for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) {
				h := fnv.New32a()
				h.Write([]byte(w))
				v[h.Sum32()%128]++
			}
			out.Data = append(out.Data, item{Embedding: v, Index: i})
		}
		out.Usage = map[string]int{"total_tokens": 10 * len(in.Input)}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1"
}

// A project indexed in the built-in store is found by meaning, its hits
// pointing at file and lines; its notes and memories too, by scope;
// a change re-embeds only what changed; what is left out isn't indexed.
func TestIndexAndSearch(t *testing.T) { indexAndSearch(t, proto.RAGBuiltin) }

// The same in PostgreSQL with pgvector, when ENOWX_RAG_TEST_POSTGRES names
// one (its URL).
func TestIndexAndSearchInPostgres(t *testing.T) {
	if os.Getenv("ENOWX_RAG_TEST_POSTGRES") == "" {
		t.Skip("ENOWX_RAG_TEST_POSTGRES=postgres://… runs it")
	}
	indexAndSearch(t, proto.RAGPostgres)
}

func indexAndSearch(t *testing.T, store string) {
	ctx := context.Background()
	dir := t.TempDir()
	write := func(name, body string) {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
	}
	write("billing/invoice.go", "package billing\n\n// Total sums an invoice's lines with tax.\nfunc Total(lines []Line, tax float64) float64 {\n\treturn 0\n}\n")
	write("auth/login.go", "package auth\n\n// Login checks a password and starts a session cookie.\nfunc Login(user, password string) error {\n\treturn nil\n}\n")
	write("docs/deploy.md", "# Deploy\n\nPush a tag and the release workflow ships it.\n")
	write("vendor/huge.go", "package vendor\n\nfunc Login() {}\n")
	write(".env", "SECRET=1\n")
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git: %s", out)
	}
	home := t.TempDir()
	st := OpenSettings(home)
	if err := st.Set(proto.RAGSettings{Embedder: proto.RAGOpenAI, Model: "fake", BaseURL: fakeEmbeddings(t), Dim: 128, Store: store, PostgresCred: "pg"}); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(home, st)
	e.KeyOf = func(cred string) (string, error) { return os.Getenv("ENOWX_RAG_TEST_POSTGRES"), nil }
	p := proto.RAGProject{On: true, Exclude: []string{"vendor/**"}}
	plan := e.Plan(dir, p)
	if plan.Files != 3 || !strings.Contains(plan.Host, "this device") {
		t.Fatalf("plan: %+v", plan)
	}
	mem, _ := st.Remember(ProjectID(dir), "Invoices are rounded half up, never banker's rounding.", "claude")
	extra := []Doc{{ID: "memory:" + mem.ID, Text: mem.Text, Source: proto.RAGMemo}}
	res, err := e.Index(ctx, dir, p, extra)
	if err != nil {
		t.Fatal(err)
	}
	if res.Chunks < 4 || res.Added < 4 {
		t.Fatalf("indexed: %+v", res)
	}
	hits, err := e.Search(ctx, dir, "password session cookie login", 3, nil)
	if err != nil || len(hits) == 0 || hits[0].File != "auth/login.go" || hits[0].Start != 1 || hits[0].Source != proto.RAGCode {
		t.Fatalf("search: %+v %v", hits, err)
	}
	for _, h := range hits {
		if strings.HasPrefix(h.File, "vendor/") || h.File == ".env" {
			t.Fatalf("left out but indexed: %s", h.File)
		}
	}
	if docs, _ := e.Search(ctx, dir, "release workflow tag", 2, []string{proto.RAGDocs}); len(docs) == 0 || docs[0].File != "docs/deploy.md" {
		t.Fatalf("docs: %+v", docs)
	}
	if m, _ := e.Search(ctx, dir, "how are invoices rounded", 1, []string{proto.RAGMemo}); len(m) != 1 || m[0].Memory != mem.ID {
		t.Fatalf("memory: %+v", m)
	}
	// Nothing changed: nothing embedded again.
	again, err := e.Index(ctx, dir, p, extra)
	if err != nil || again.Added != 0 || again.Skipped < 3 {
		t.Fatalf("again: %+v %v", again, err)
	}
	write("auth/login.go", "package auth\n\n// Login checks a password.\nfunc Login(user, password string) error {\n\treturn nil\n}\n")
	if changed, _ := e.Index(ctx, dir, p, extra); changed.Added != 1 || changed.Removed != 1 {
		t.Fatalf("one file changed: %+v", changed)
	}
	// Lines added above a chunk move it, not embed it again.
	write("billing/invoice.go", "package billing\n\nimport \"fmt\"\n\nvar _ = fmt.Sprint\n\n// Total sums an invoice's lines with tax.\nfunc Total(lines []Line, tax float64) float64 {\n\treturn 0\n}\n")
	moved, _ := e.Index(ctx, dir, p, extra)
	if hits, _ := e.Search(ctx, dir, "invoice lines tax total", 1, []string{proto.RAGCode}); len(hits) == 0 || hits[0].File != "billing/invoice.go" || hits[0].Start != 1 {
		t.Fatalf("after moving: %+v %+v", moved, hits)
	}
	if err := e.Drop(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if s, _ := e.Status(ctx, dir); s.Chunks != 0 {
		t.Fatalf("dropped: %+v", s)
	}
}

// Every request to the embedder is counted, the search's query too: an
// index's and a prompt's, told apart by the project they were about.
func TestEveryEmbedderCallIsCounted(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "invoice.go"),
		[]byte("package shop\n\n// Total sums an invoice's lines with tax.\nfunc Total(tax float64) float64 {\n\treturn 0\n}\n"), 0o644)
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git: %s", out)
	}
	home := t.TempDir()
	st := OpenSettings(home)
	if err := st.Set(proto.RAGSettings{Embedder: proto.RAGOpenAI, Model: "fake", BaseURL: fakeEmbeddings(t), Dim: 128}); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(home, st)
	var mu sync.Mutex
	counted := map[string]int64{}
	e.OnEmbed = func(project string, n int64) {
		mu.Lock()
		counted[project] += n
		mu.Unlock()
	}
	if _, err := e.Index(context.Background(), dir, proto.RAGProject{On: true}, nil); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	indexed := counted[dir]
	mu.Unlock()
	if indexed <= 0 {
		t.Fatalf("the index counted %d tokens", indexed)
	}
	if _, err := e.Search(context.Background(), dir, "invoice tax total", 3, nil); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	after := counted[dir]
	mu.Unlock()
	if after <= indexed {
		t.Fatalf("the query wasn't counted: %d after the index, %d after a search", indexed, after)
	}
}

// With no embedder set nothing is indexed.
func TestNotReady(t *testing.T) {
	home := t.TempDir()
	e := NewEngine(home, OpenSettings(home))
	if _, err := e.Index(context.Background(), t.TempDir(), proto.RAGProject{On: true}, nil); err != ErrNotReady {
		t.Fatalf("err %v", err)
	}
}

// The embedder's key typed in the app is used, the vault never asked; the
// settings say a key is set, never what it is.
func TestTypedEmbedderKey(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"embedding": []float32{1, 0, 0}, "index": 0}}})
	}))
	defer srv.Close()
	home := t.TempDir()
	st := OpenSettings(home)
	e := NewEngine(home, st)
	e.KeyOf = func(string) (string, error) { t.Fatal("the vault was asked"); return "", nil }
	set := proto.RAGSettings{Embedder: proto.RAGOpenAI, Model: "m", BaseURL: srv.URL + "/v1"}
	if err := e.SetKey(proto.RAGKeyEmbedder, "sk-typed"); err != nil {
		t.Fatal(err)
	}
	if dim, err := e.Test(context.Background(), set); err != nil || dim != 3 || auth != "Bearer sk-typed" {
		t.Fatalf("test %d %v, auth %q", dim, err, auth)
	}
	st.Set(set)
	if w := e.WithKeys(st.Get()); !w.HasKey || w.HasPostgres {
		t.Fatalf("settings %+v", w)
	}
	data, _ := os.ReadFile(filepath.Join(home, "settings", "rag.json"))
	if strings.Contains(string(data), "sk-typed") {
		t.Fatal("the key is in the synced settings")
	}
}

// Files like google-services.json stay out by their names, and a Firebase key
// in any file by its look; a file on one long line is cut, not one huge
// chunk, and what tools generate stays out.
func TestLeftOutAndCut(t *testing.T) {
	for _, name := range []string{"android/app/google-services.json", "ios/GoogleService-Info.plist", "keys/firebase-adminsdk-x1.json"} {
		if !IsSecret(name) {
			t.Errorf("%s isn't kept out", name)
		}
	}
	if IsSecret("android/app/build.gradle") {
		t.Error("an ordinary file kept out")
	}
	if !HoldsSecret("\"current_key\": \"AIzaSyA1234567890abcdefghijklmnopqrstuv\"") {
		t.Error("a Google API key isn't seen")
	}
	for _, name := range []string{"public/app.js.map", "dist/x.min.js", "package-lock.json"} {
		if !IsGenerated(name) {
			t.Errorf("%s isn't seen as generated", name)
		}
	}
	long := strings.Repeat("{\"a\":\"é\"},", 50000)
	parts := chunkFile(long, "data.json", chunkChars)
	if len(parts) < 100 {
		t.Fatalf("%d chunks of a long line", len(parts))
	}
	joined := ""
	for _, p := range parts {
		if len(p.text) > chunkChars {
			t.Fatalf("a chunk of %d", len(p.text))
		}
		if !utf8.ValidString(p.text) {
			t.Fatal("a character cut in two")
		}
		joined += p.text
	}
	if joined != long {
		t.Fatal("text lost in cutting")
	}
}

// A batch stays within what an embedder takes, however many long texts come.
func TestEmbedBatchesBySize(t *testing.T) {
	var most atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Input []string `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		size := 0
		data := []map[string]any{}
		for i, s := range in.Input {
			size += len(s)
			data = append(data, map[string]any{"embedding": []float32{1}, "index": i})
		}
		if int64(size) > most.Load() {
			most.Store(int64(size))
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()
	h := &httpEmbedder{kind: proto.RAGOpenAI, url: srv.URL + "/embeddings", model: "m", batch: 96}
	texts := make([]string, 200)
	for i := range texts {
		texts[i] = strings.Repeat("x", 10000)
	}
	vecs, err := h.embed(t.Context(), texts, false, nil)
	if err != nil || len(vecs) != len(texts) {
		t.Fatalf("%d vectors: %v", len(vecs), err)
	}
	if most.Load() > embedBatchBytes {
		t.Fatalf("a batch of %d bytes", most.Load())
	}
}

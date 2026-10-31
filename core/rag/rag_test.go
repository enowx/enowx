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
	"testing"

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

// With no embedder set nothing is indexed.
func TestNotReady(t *testing.T) {
	home := t.TempDir()
	e := NewEngine(home, OpenSettings(home))
	if _, err := e.Index(context.Background(), t.TempDir(), proto.RAGProject{On: true}, nil); err != ErrNotReady {
		t.Fatalf("err %v", err)
	}
}

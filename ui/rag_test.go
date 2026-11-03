package ui

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// The Index tool turns a session's project on only once the user agreed
// to what goes to the embedder, indexes it, finds a part by meaning, and
// keeps a memory.
func TestIndexTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell session")
	}
	emb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Input []string `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		type item struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		var out struct {
			Data []item `json:"data"`
		}
		for i, text := range in.Input {
			v := make([]float32, 64)
			for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return r < 'a' || r > 'z' }) {
				h := fnv.New32a()
				h.Write([]byte(w))
				v[h.Sum32()%64]++
			}
			out.Data = append(out.Data, item{Embedding: v, Index: i})
		}
		json.NewEncoder(w).Encode(out)
	}))
	defer emb.Close()

	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	app := New(Options{})
	cl, err := client.New(b1, func(ev proto.Event) { app.ragEvent(ev) })
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	proj := t.TempDir()
	os.WriteFile(filepath.Join(proj, "invoice.go"), []byte("package shop\n\n// Total sums an invoice's lines with tax.\nfunc Total(lines []float64, tax float64) float64 {\n\treturn 0\n}\n"), 0o644)
	os.WriteFile(filepath.Join(proj, "README.md"), []byte("# Shop\n\nOrders and refunds.\n"), 0o644)
	if out, err := exec.Command("git", "-C", proj, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git: %s", out)
	}
	if _, err := cl.SetRAGSettings(proto.RAGSettings{Embedder: proto.RAGOpenAI, Model: "fake", BaseURL: emb.URL + "/v1"}); err != nil {
		t.Fatal(err)
	}
	s, err := cl.CreateSession(proto.SessionCreateParams{Project: proj, Agent: "shell"})
	if err != nil {
		t.Fatal(err)
	}

	posted := make(chan func(), 64)
	app.onMain = func(fn func()) { posted <- fn }
	app.client, app.agents = cl, agent.List()
	app.host.Name = "this Mac"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(460).Height(900).Background(theme.Background).TextColor(theme.Text).Children(func() {
			app.indexTab(c, s)
			app.confirmDialog(c)
		})
	}, 460, 900)
	frames := func(until func() bool, what string) {
		deadline := time.Now().Add(15 * time.Second)
		for !until() {
			select {
			case fn := <-posted:
				fn()
			case <-time.After(30 * time.Millisecond):
			}
			tt.Frame()
			if time.Now().After(deadline) {
				t.Fatalf("never: %s: %q", what, tt.Texts())
			}
		}
	}
	tt.Frame()
	frames(func() bool { return hasText(tt, "isn't indexed") }, "offered to index")
	if err := tt.Click("Index this project…"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool { return hasText(tt, "Agree and index") }, "the consent")
	if !hasText(tt, "Files") {
		t.Fatalf("no plan: %q", tt.Texts())
	}
	if st, _ := cl.RAGStatus(proto.RAGRef{Project: proj}); st.Settings.On {
		t.Fatal("on before agreeing")
	}
	if err := tt.Click("Agree and index"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool { return app.ragPanel.st.Chunks >= 2 && !app.ragPanel.st.Indexing }, "the index")
	app.ragPanel.query = "invoice tax total"
	frames(func() bool { return hasText(tt, "invoice.go:") }, "a result")
	if os.Getenv("SHOTS") != "" {
		shot(t, tt, "index-tool.png")
	}
	var hit proto.RAGHit
	for _, h := range app.ragPanel.hits {
		if h.File == "invoice.go" {
			hit = h
		}
	}
	if err := tt.Click(fmt.Sprintf("invoice.go:%d  %s", hit.Start, hit.Symbol)); err != nil {
		t.Fatal(err)
	}
	if d := app.docs[s.ID+"\x00invoice.go"]; d == nil || app.centerOf(s.ID).front != fileTabPrefix+"invoice.go" {
		t.Fatalf("the result didn't open its file: %q", app.centerOf(s.ID).front)
	}

	app.ragPanel.newMem = "Invoices round half up"
	tt.Frame()
	if err := tt.Click("Keep"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool { return hasText(tt, "Invoices round half up") && len(app.ragPanel.mems) == 1 }, "the memory")

	if err := tt.Click("Turn off and delete the index"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Turn off"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool { return hasText(tt, "isn't indexed") }, "off again")
}

// A key typed on the host offers to go to the vault; with no vault to take
// it, it says so and stays.
func TestRAGSectionOffersTheVault(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	app := New(Options{})
	cl, err := client.New(b1, func(proto.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	posted := make(chan func(), 64)
	app.onMain = func(fn func()) { posted <- fn }
	app.client, app.host.Name = cl, "this Mac"
	if _, err := cl.SetRAGKey(proto.RAGKeyEmbedder, "typed-key"); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(760).Height(800).Background(theme.Background).TextColor(theme.Text).Children(func() { app.ragSection(c) })
	}, 760, 800)
	frames := func(until func() bool, what string) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for !until() {
			select {
			case fn := <-posted:
				fn()
			case <-time.After(30 * time.Millisecond):
			}
			tt.Frame()
			if time.Now().After(deadline) {
				t.Fatalf("never: %s: %q", what, tt.Texts())
			}
		}
	}
	frames(func() bool { return hasText(tt, "Keep them in the vault") }, "the offer")
	shot(t, tt, "rag-keys-to-vault.png")
	if err := tt.Click("Keep them in the vault"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool { return hasText(tt, "stays here") }, "why it stayed")
	if st, _ := cl.RAGSettings(); !st.HasKey {
		t.Fatal("the key went with nowhere to go")
	}
}

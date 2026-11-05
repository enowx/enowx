package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/core/tokens"
	"github.com/enowx/enowx/proto"
)

// ragProject is a project with one file to find, indexed and agreed to.
func ragProject(t *testing.T, srv *Server, embedder string, p proto.RAGProject) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "billing"), 0o755)
	os.WriteFile(filepath.Join(dir, "billing", "invoice.go"),
		[]byte("package billing\n\n// Total sums an invoice's lines with tax.\nfunc Total(lines []float64, tax float64) float64 {\n\treturn 0\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Shop\n\nOrders and refunds.\n"), 0o644)
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git: %s", out)
	}
	if err := srv.ragSettings.Set(proto.RAGSettings{Embedder: proto.RAGOpenAI, Model: "fake", BaseURL: embedder}); err != nil {
		t.Fatal(err)
	}
	st, _ := srv.setRAGProject(proto.RAGProjectParams{Project: dir, Settings: p})
	p.Consent = st.Embedder
	srv.setRAGProject(proto.RAGProjectParams{Project: dir, Settings: p})
	if _, err := srv.ragIndex(dir); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the index", func() bool { s := srv.ragStatus(dir); return !s.Indexing && s.Chunks >= 2 })
	return dir
}

// ragPromptSession is a session in dir, its hooks running as its agent's.
func ragPromptSession(t *testing.T, srv *Server, dir string) {
	t.Helper()
	s, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: dir, Cols: 80, Rows: 24, Meta: session.Meta{Project: dir}})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(session.EnvSession, s.ID())
}

// ragOwn are the calls counted as RAG's own.
func ragOwn(srv *Server) []tokens.Call {
	var out []tokens.Call
	for _, c := range srv.tok.store.Calls(time.Now().Add(-time.Hour), time.Now().Add(time.Minute)) {
		if c.Own == proto.TokensOwnRAG {
			out = append(out, c)
		}
	}
	return out
}

// Every prompt of a session whose project is indexed gets the parts that
// bear on it, whatever the project's switch says and whatever the judge
// would: the switch is no longer read, and the judge isn't asked.
func TestEveryPromptGetsTheProjectsParts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	t.Setenv("HOME", t.TempDir())
	judgeLater(t, srv) // the judge would drop what doesn't bear on the prompt
	dir := ragProject(t, srv, ragEmbeddings(t), proto.RAGProject{On: true, Context: false})
	ragPromptSession(t, srv, dir)

	_, ctx, _ := hookOf(`{"prompt":"make the colors of the total and tax screens warmer"}`, "--event", proto.HookPromptStart)
	if !strings.Contains(ctx, "billing/invoice.go") {
		t.Fatalf("the switch off skipped the search: %q", ctx)
	}
	if acted := srv.judge.Log(proto.DecideLogParams{Use: proto.DecideContext}); len(acted) != 0 {
		t.Fatalf("the judge was asked which parts to keep: %+v", acted)
	}
}

// A prompt's search embeds its query: it is counted as RAG's own use, one
// call a prompt, beside the index's.
func TestPromptSearchIsCounted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	if _, err := srv.tok.settings.Set(proto.TokensSettings{On: true}); err != nil {
		t.Fatal(err)
	}
	dir := ragProject(t, srv, ragEmbeddings(t), proto.RAGProject{On: true})
	ragPromptSession(t, srv, dir)
	indexed := len(ragOwn(srv))
	if indexed == 0 {
		t.Fatal("the index's embeddings weren't counted")
	}
	hookOf(`{"prompt":"fix the invoice total tax"}`, "--event", proto.HookPromptStart)
	if got := len(ragOwn(srv)); got != indexed+1 {
		t.Fatalf("%d calls after a prompt, %d after the index: the query wasn't counted", got, indexed)
	}
	hookOf(`{"prompt":"what does the refund flow do"}`, "--event", proto.HookPromptStart)
	if got := len(ragOwn(srv)); got != indexed+2 {
		t.Fatalf("%d calls after two prompts: %d", got, indexed)
	}
}

// An embedder that fails doesn't hold the prompt up: it goes on within the
// search's timeout, with what the index's words find, marked as such, and
// the failure shows in the RAG settings.
func TestPromptSearchFallsBackWhenTheEmbedderFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv, _ := hookServer(t)
	var broken atomic.Bool
	inner := ragEmbeddings(t)
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if broken.Load() {
			http.Error(w, "the embedder is down", http.StatusInternalServerError)
			return
		}
		resp, err := http.Post(inner+"/embeddings", "application/json", r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		var out json.RawMessage
		json.NewDecoder(resp.Body).Decode(&out)
		w.Header().Set("Content-Type", "application/json")
		w.Write(out)
	}))
	defer srv2.Close()
	dir := ragProject(t, srv, srv2.URL+"/v1", proto.RAGProject{On: true})
	ragPromptSession(t, srv, dir)

	broken.Store(true)
	start := time.Now()
	_, ctx, _ := hookOf(`{"prompt":"fix the invoice total tax"}`, "--event", proto.HookPromptStart)
	took := time.Since(start)
	if took > ragContextTimeout+5*time.Second {
		t.Fatalf("the prompt waited %s on a failing embedder", took)
	}
	if !strings.Contains(ctx, "billing/invoice.go") || !strings.Contains(ctx, "found by words") {
		t.Fatalf("no word fallback: %q", ctx)
	}
	if st := srv.ragStatus(dir); !strings.Contains(st.ContextError, "500") {
		t.Fatalf("the failure isn't in the status: %+v", st)
	}
}

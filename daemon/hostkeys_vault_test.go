package daemon

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/enowx/enowx/proto"
)

// A key typed on one host moves to the vault without being typed again: the
// settings, as they sync, then work on another host, which asks the vault;
// nothing of the key is left in the host's files, or in what it answers. If
// the user refuses, all stays as it was.
func TestTypedKeysMoveToTheVault(t *testing.T) {
	const embedKey, judgeKey, pgURL = "emb-key-aaaaaaaaaaaaaaaaaaaaaaaa", "judge-key-bbbbbbbbbbbbbbbbbbbbbb", "postgres://rag:pw-cccccccccccccc@db.example:5432/rag"
	// A model API and an embedder that take their keys alone.
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		switch {
		case strings.HasSuffix(r.URL.Path, "/embeddings") && auth == "Bearer "+embedKey:
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"embedding": []float32{1, 0, 0}, "index": 0}}})
		case strings.HasSuffix(r.URL.Path, "/chat/completions") && auth == "Bearer "+judgeKey:
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{
				"content": `{"verdict":"allow","confidence":0.9,"why":"fine"}`}}}})
		default:
			http.Error(w, "no", http.StatusUnauthorized)
		}
	}))
	defer api.Close()

	// The vault, on the user's device: what it keeps, what it is asked.
	var mu sync.Mutex
	kept := map[string]map[string]string{}
	refuse := true
	var asks []proto.VaultRequest
	answer := func(r proto.VaultRequest) (proto.VaultLend, error) {
		mu.Lock()
		defer mu.Unlock()
		asks = append(asks, r)
		switch r.Action {
		case proto.VaultAdd:
			if refuse {
				return proto.VaultLend{}, errors.New("vault: the user refused")
			}
			kept[r.Name] = r.Fields
			return proto.VaultLend{Message: "kept"}, nil
		case proto.VaultEnv:
			if f, ok := kept[r.Name]; ok {
				for _, v := range f {
					return proto.VaultLend{Env: map[string]string{"API_KEY": v}, Credentials: []string{r.Name}}, nil
				}
			}
		}
		return proto.VaultLend{}, errors.New("no such credential")
	}

	a := startServer(t)
	ca := connect(t, a, nil)
	vaultConn := serveVault(t, a, answer) // the app asks on the connection it answers on
	if _, err := ca.SetRAGSettings(proto.RAGSettings{Embedder: proto.RAGOpenAI, Model: "m", Dim: 3, BaseURL: api.URL + "/v1"}); err != nil {
		t.Fatal(err)
	}
	for which, v := range map[string]string{proto.RAGKeyEmbedder: embedKey, proto.RAGKeyPostgres: pgURL} {
		if _, err := ca.SetRAGKey(which, v); err != nil {
			t.Fatal(err)
		}
	}
	judge := proto.Judge{Kind: proto.JudgeAPI, Provider: proto.ProviderOpenAI, URL: api.URL + "/v1", Model: "small", Label: "Small one"}
	st, err := ca.SetDecideSettings(proto.DecideSettings{Policy: proto.DecideAnyProvider, Own: []proto.Judge{judge}})
	if err != nil {
		t.Fatal(err)
	}
	oldID := st.Own[0].ID
	st.Order = []string{oldID}
	st.Uses = map[string]proto.DecideUse{proto.DecideApprove: {On: true, Judge: oldID}}
	if _, err := ca.SetDecideSettings(st); err != nil {
		t.Fatal(err)
	}
	if err := a.judge.SetKey(oldID, judgeKey); err != nil {
		t.Fatal(err)
	}

	// The user refuses: all as it was, and they are told.
	res, err := vaultConn.KeysToVault()
	if err != nil || len(res.Moved) != 0 || len(res.Failed) != 3 {
		t.Fatalf("refused: %+v %v", res, err)
	}
	if s := a.ragSettings.Get(); s.KeyCred != "" || s.PostgresCred != "" || a.rag.TypedKey(proto.RAGKeyEmbedder) != embedKey || a.judge.TypedKeys()[0].Key != judgeKey {
		t.Fatalf("changed though refused: %+v", s)
	}

	mu.Lock()
	refuse = false
	mu.Unlock()
	res, err = vaultConn.KeysToVault()
	if err != nil || len(res.Moved) != 3 || len(res.Failed) != 0 {
		t.Fatalf("moved: %+v %v", res, err)
	}
	raw, _ := json.Marshal(res)
	for _, secret := range []string{embedKey, judgeKey, "pw-cccccccccccccc"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("a key in the answer")
		}
		filepath.WalkDir(a.dataDir, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if data, _ := os.ReadFile(path); strings.Contains(string(data), secret) {
					t.Errorf("a key is left in %s", path)
				}
			}
			return nil
		})
	}
	s := a.ragSettings.Get()
	if s.KeyCred != "enowx-rag-embedder" || s.PostgresCred != "enowx-rag-postgres" {
		t.Fatalf("RAG's settings: %+v", s)
	}
	if f := kept["enowx-rag-postgres"]; f["url"] != pgURL {
		t.Fatal("the URL isn't kept as the PostgreSQL platform's field")
	}
	moved := a.judge.Settings().Get()
	newID := moved.Own[0].ID
	if moved.Own[0].Cred != "enowx-judge-small-one" || newID == oldID || moved.Order[0] != newID || moved.Uses[proto.DecideApprove].Judge != newID {
		t.Fatalf("the judge's settings: %+v", moved)
	}
	if len(a.judge.TypedKeys()) != 0 || a.rag.TypedKey(proto.RAGKeyEmbedder) != "" || a.rag.TypedKey(proto.RAGKeyPostgres) != "" {
		t.Fatal("a typed key stayed")
	}
	// Moving again finds nothing, and asks nothing.
	mu.Lock()
	asks = nil
	mu.Unlock()
	if res, err := ca.KeysToVault(); err != nil || len(res.Moved)+len(res.Failed) != 0 || len(asks) != 0 {
		t.Fatalf("again: %+v %v, %d asks", res, err, len(asks))
	}

	// Another host: the settings as they sync, no key of its own.
	b := startServer(t)
	cb := connect(t, b, nil)
	if err := os.MkdirAll(filepath.Join(b.dataDir, "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"rag.json", "decide.json"} {
		data, err := os.ReadFile(filepath.Join(a.dataDir, "settings", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(b.dataDir, "settings", f), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// With no app, it says so rather than working on nothing.
	if j := b.judge.Test(t.Context(), newID); len(j) == 0 || j[0].OK {
		t.Fatalf("no vault to ask, and it passed: %+v", j)
	}
	serveVault(t, b, answer)
	synced := b.ragSettings.Get()
	synced.Store = proto.RAGBuiltin // the test's PostgreSQL is no server
	var tested proto.RAGTestResult
	if err := cb.Call(proto.MethodRAGTest, synced, &tested); err != nil || tested.Error != "" {
		t.Fatalf("RAG's test on the other host: %+v %v", tested, err)
	}
	found := false
	for _, j := range b.judge.Test(t.Context(), newID) {
		if j.ID == newID {
			found = j.OK
			if !j.OK {
				t.Errorf("the judge's test on the other host: %s", j.Error)
			}
		}
	}
	if !found {
		t.Fatal("the judge wasn't tested on the other host")
	}
}

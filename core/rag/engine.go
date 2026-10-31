package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/projectlinks"
	"github.com/enowx/enowx/proto"
)

// ErrNotReady is an engine with no embedder set: nothing is indexed.
var ErrNotReady = errors.New("rag: no embedder set: Settings → RAG")

// defaultMaxFiles bounds a project's files indexed.
const defaultMaxFiles = 20000

// maxFileSize is the largest file indexed.
const maxFileSize = 500 << 10

// Doc is something of a project indexed besides its files: a memory, an
// attached note.
type Doc struct {
	ID     string // "memory:<id>", "note:<id>"
	Title  string
	Text   string
	Source string // proto.RAGMemo, RAGNotes
}

// Engine is a host's RAG: its embedder and store built from the settings
// as they change, keys asked of the vault (KeyOf). Safe from any goroutine.
type Engine struct {
	dir      string
	settings *Settings
	// KeyOf is the value of the vault's credential named cred.
	KeyOf func(cred string) (string, error)

	mu    sync.Mutex
	sig   string
	emb   embedder
	store store
}

// NewEngine is the RAG of the daemon in dir, by s.
func NewEngine(dir string, s *Settings) *Engine { return &Engine{dir: dir, settings: s} }

// signature is what the embedder and store are built of: the settings but
// the projects'.
func signature(st proto.RAGSettings) string {
	st.Projects = nil
	data, _ := json.Marshal(st)
	return string(data)
}

// embedderKey tells one embedder's vectors from another's in a store.
func embedderKey(st proto.RAGSettings) string {
	sum := sha256.Sum256([]byte(st.Embedder + "|" + st.Model + "|" + st.BaseURL + "|" + fmt.Sprint(st.Dim)))
	return hex.EncodeToString(sum[:4])
}

// indexKey is the project in dir's index in the store: its ID and the
// embedder's, as vectors of two embedders never compare.
func indexKey(dir string, st proto.RAGSettings) string {
	return ProjectID(dir) + "@" + embedderKey(st)
}

// ready is the embedder and store for the settings now, built again when
// they changed.
func (e *Engine) ready(ctx context.Context) (embedder, store, proto.RAGSettings, error) {
	st := e.settings.Get()
	switch st.Embedder {
	case proto.RAGVoyage, proto.RAGOpenAI, proto.RAGTEI:
	default:
		return nil, nil, st, ErrNotReady
	}
	sig := signature(st)
	e.mu.Lock()
	defer e.mu.Unlock()
	if sig == e.sig && e.store != nil {
		return e.emb, e.store, st, nil
	}
	if e.store != nil {
		e.store.close()
	}
	e.emb, e.store, e.sig = nil, nil, ""
	emb, err := e.embedderOf(st)
	if err != nil {
		return nil, nil, st, err
	}
	s, err := e.storeOf(ctx, st)
	if err != nil {
		return nil, nil, st, err
	}
	e.emb, e.store, e.sig = emb, s, sig
	return emb, s, st, nil
}

func (e *Engine) embedderOf(st proto.RAGSettings) (embedder, error) {
	key, err := e.secret(st.KeyCred)
	if err != nil {
		return nil, err
	}
	return newEmbedder(st, key)
}

func (e *Engine) storeOf(ctx context.Context, st proto.RAGSettings) (store, error) {
	if st.Store == proto.RAGPostgres {
		if st.PostgresCred == "" {
			return nil, errors.New("rag: pick the vault's credential with the PostgreSQL URL")
		}
		u, err := e.secret(st.PostgresCred)
		if err != nil {
			return nil, err
		}
		return openPostgres(ctx, u)
	}
	return openSQLite(filepath.Join(e.dir, "rag", "index.db"))
}

// secret is the vault's credential cred, "" for none.
func (e *Engine) secret(cred string) (string, error) {
	if cred == "" {
		return "", nil
	}
	if e.KeyOf == nil {
		return "", errors.New("rag: no vault to read " + cred + " from")
	}
	return e.KeyOf(cred)
}

// Test embeds a line with settings st (not kept) and, for PostgreSQL,
// connects to it; it says the vector's size.
func (e *Engine) Test(ctx context.Context, st proto.RAGSettings) (int, error) {
	emb, err := e.embedderOf(st)
	if err != nil {
		return 0, err
	}
	v, err := emb.embed(ctx, []string{"enowX checks the embedder"}, true)
	if err != nil {
		return 0, err
	}
	if len(v) == 0 || len(v[0]) == 0 {
		return 0, errors.New("rag: the embedder gave no vector")
	}
	if st.Store == proto.RAGPostgres {
		s, err := e.storeOf(ctx, st)
		if err != nil {
			return len(v[0]), err
		}
		s.close()
	}
	return len(v[0]), nil
}

// EmbedderHost is where st's embedder sends text, for the user to agree
// to: a host, or "this device".
func (e *Engine) EmbedderHost(st proto.RAGSettings) string {
	switch st.Embedder {
	case proto.RAGVoyage:
		return "api.voyageai.com"
	case proto.RAGOpenAI:
		return hostOf(orString(st.BaseURL, "https://api.openai.com/v1"), "api.openai.com")
	case proto.RAGTEI:
		return hostOf(st.BaseURL, "a TEI server")
	}
	return st.Embedder
}

// ProjectHost is where the project in dir's text goes, for the user to
// agree to.
func (e *Engine) ProjectHost(string) string { return e.EmbedderHost(e.settings.Get()) }

// hostOf is the host of address raw, "this device" for a local one.
func hostOf(raw, fallback string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		if h := u.Hostname(); h == "localhost" || h == "127.0.0.1" || h == "::1" {
			return "this device (" + u.Host + ")"
		}
		return u.Host
	}
	return fallback
}

// Index brings the project in dir's index up to date as p says, with its
// docs: a chunk whose text is unchanged keeps its vector (moved with its
// file), a new one is embedded, one gone is removed.
func (e *Engine) Index(ctx context.Context, dir string, p proto.RAGProject, docs []Doc) (proto.RAGStatus, error) {
	emb, s, st, err := e.ready(ctx)
	if err != nil {
		return proto.RAGStatus{}, err
	}
	key := indexKey(dir, st)
	out := proto.RAGStatus{Project: dir, ID: ProjectID(dir)}
	have, err := s.ids(ctx, key)
	if err != nil {
		return out, err
	}
	var all []point
	for _, f := range Files(dir, p.Exclude, orInt(p.MaxFiles, defaultMaxFiles)) {
		pts, ok := filePoints(dir, f)
		if ok {
			out.Files++
			all = append(all, pts...)
		}
	}
	for _, d := range docs {
		all = append(all, docPoints(d)...)
	}
	var fresh, moved []point
	keep := map[string]bool{}
	for _, pt := range all {
		keep[pt.id] = true
		was, ok := have[pt.id]
		switch {
		case !ok:
			fresh = append(fresh, pt)
		case was.start != pt.start || was.end != pt.end || was.symbol != pt.symbol:
			moved = append(moved, pt) // vec nil: keeps its own
		default:
			out.Skipped++
		}
	}
	before := emb.tokens()
	added, err := embedAll(ctx, emb, s, key, fresh)
	out.Added, out.Tokens = added, emb.tokens()-before
	if err != nil {
		return out, err
	}
	if err := s.put(ctx, key, moved); err != nil {
		return out, err
	}
	var gone []string
	for id := range have {
		if !keep[id] {
			gone = append(gone, id)
		}
	}
	if err := s.remove(ctx, key, gone); err != nil {
		return out, err
	}
	out.Removed = len(gone)
	out.Chunks, _ = s.count(ctx, key)
	return out, nil
}

// embedWorkers are the batches embedded at once: the embedder's latency,
// not its throughput, bounds a first index.
const embedWorkers = 4

// embedAll embeds pts in batches, a few at once, and keeps each batch as
// it comes; it says how many were kept.
func embedAll(ctx context.Context, emb embedder, s store, key string, pts []point) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	batches := make(chan []point)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	added := 0
	fail := func(err error) {
		mu.Lock()
		if first == nil {
			first = err
			cancel()
		}
		mu.Unlock()
	}
	for range embedWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for part := range batches {
				texts := make([]string, len(part))
				for j, pt := range part {
					texts[j] = header(pt) + pt.text
				}
				vecs, err := emb.embed(ctx, texts, false)
				if err == nil {
					for j := range part {
						part[j].vec = vecs[j]
					}
					err = s.put(ctx, key, part)
				}
				if err != nil {
					fail(err)
					continue
				}
				mu.Lock()
				added += len(part)
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < len(pts) && ctx.Err() == nil; i += 128 {
		batches <- pts[i:min(len(pts), i+128)]
	}
	close(batches)
	wg.Wait()
	return added, first
}

// filePoints are file f of dir's chunks, unless it is no text to index.
func filePoints(dir, f string) ([]point, bool) {
	if IsSecret(f) || !textFile(f) {
		return nil, false
	}
	full := filepath.Join(dir, filepath.FromSlash(f))
	info, err := os.Stat(full)
	if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > maxFileSize {
		return nil, false
	}
	data, err := os.ReadFile(full)
	if err != nil || slices.Contains(data, 0) {
		return nil, false
	}
	source := SourceOf(f)
	var out []point
	seen := map[string]int{}
	for _, c := range chunkFile(string(data), f, chunkChars) {
		if secretText.MatchString(c.text) {
			continue // a key in the code: never sent to an embedder
		}
		out = append(out, point{id: pointID(f, c.text, seen), file: f, start: c.start, end: c.end, symbol: c.symbol, source: source, text: c.text})
	}
	return out, true
}

// docPoints are doc d's chunks.
func docPoints(d Doc) []point {
	var out []point
	seen := map[string]int{}
	for _, c := range chunkFile(d.Text, d.ID+".md", chunkChars) {
		if secretText.MatchString(c.text) {
			continue
		}
		symbol := c.symbol
		if symbol == "" {
			symbol = d.Title
		}
		out = append(out, point{id: pointID(d.ID, c.text, seen), file: d.ID, symbol: symbol, source: d.Source, text: c.text})
	}
	return out
}

// pointID is a chunk's ID: where it is from and its text's hash, a count
// after it for the same text twice.
func pointID(from, text string, seen map[string]int) string {
	sum := sha256.Sum256([]byte(text))
	h := hex.EncodeToString(sum[:8])
	seen[h]++
	if n := seen[h]; n > 1 {
		return fmt.Sprintf("%s#%s-%d", from, h, n)
	}
	return from + "#" + h
}

// header is what a chunk is embedded with ahead of its text: where it is.
func header(p point) string {
	switch {
	case p.start > 0 && p.symbol != "":
		return fmt.Sprintf("File: %s:%d-%d (%s)\n\n", p.file, p.start, p.end, p.symbol)
	case p.start > 0:
		return fmt.Sprintf("File: %s:%d-%d\n\n", p.file, p.start, p.end)
	case p.source == proto.RAGMemo:
		return "Memory of this project:\n\n"
	}
	return "Note “" + p.symbol + "” attached to this project:\n\n"
}

// Search finds query in the project in dir: k results (5 when 0), of
// scope's sources (all when none), by meaning and by words together.
func (e *Engine) Search(ctx context.Context, dir, query string, k int, scope []string) ([]proto.RAGHit, error) {
	if k <= 0 {
		k = 5
	}
	emb, s, st, err := e.ready(ctx)
	if err != nil {
		return nil, err
	}
	key := indexKey(dir, st)
	want := k * 4
	if len(scope) > 0 {
		want = k * 12
	}
	q, err := emb.embed(ctx, []string{query}, true)
	if err != nil {
		return nil, err
	}
	near, err := s.nearest(ctx, key, normalize(q[0]), want)
	if err != nil {
		return nil, err
	}
	byWords, err := s.keyword(ctx, key, query, want)
	if err != nil {
		byWords = nil // a query FTS can't take: meaning alone
	}
	ranked := fuse(near, byWords)
	cos := map[string]float64{}
	for _, n := range near {
		cos[n.id] = n.score
	}
	ids := make([]string, len(ranked))
	for i, r := range ranked {
		ids[i] = r.id
	}
	pts, err := s.get(ctx, key, ids)
	if err != nil {
		return nil, err
	}
	var out []proto.RAGHit
	for _, r := range ranked {
		pt, ok := pts[r.id]
		if !ok || len(scope) > 0 && !slices.Contains(scope, pt.source) {
			continue
		}
		out = append(out, hitOf(pt, cos[r.id]))
		if len(out) == k {
			break
		}
	}
	return out, nil
}

// hitOf is point p as a result, score its cosine with the query.
func hitOf(p point, score float64) proto.RAGHit {
	text := p.text
	if len(text) > 800 {
		text = text[:800] + "…"
	}
	h := proto.RAGHit{File: p.file, Start: p.start, End: p.end, Symbol: p.symbol, Snippet: text, Score: score, Source: p.source}
	if p.start == 0 {
		h.File = "" // a memory's or note's
	}
	if p.source == proto.RAGMemo {
		h.Memory = strings.TrimPrefix(p.file, "memory:")
	}
	return h
}

// SourceOf is what a project's file is: docs (Markdown, its docs folder,
// the brief) or code.
func SourceOf(file string) string {
	l := strings.ToLower(file)
	switch {
	case l == "":
		return ""
	case strings.HasSuffix(l, ".md") || strings.HasSuffix(l, ".mdx") || strings.HasSuffix(l, ".txt"),
		strings.HasPrefix(l, "docs/"), l == ".enowx/brief.md":
		return proto.RAGDocs
	}
	return proto.RAGCode
}

// Status is the index of the project in dir: its chunks.
func (e *Engine) Status(ctx context.Context, dir string) (proto.RAGStatus, error) {
	out := proto.RAGStatus{Project: dir, ID: ProjectID(dir)}
	_, s, st, err := e.ready(ctx)
	if err != nil {
		return out, err
	}
	out.Ready = true
	out.Chunks, _ = s.count(ctx, indexKey(dir, st))
	return out, nil
}

// Drop deletes the index of the project in dir.
func (e *Engine) Drop(ctx context.Context, dir string) error {
	_, s, st, err := e.ready(ctx)
	if err != nil {
		return err
	}
	return s.drop(ctx, indexKey(dir, st))
}

// Files are the files of the project in root to index, relative with
// slashes: git's (its ignores honored), else a walk; but those matching
// exclude, at most max.
func Files(root string, exclude []string, max int) []string {
	var files []string
	if out, err := git.Run(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard"); err == nil {
		for _, f := range strings.Split(out, "\x00") {
			if f != "" {
				files = append(files, f)
			}
		}
	} else {
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			name := d.Name()
			if d.IsDir() && (strings.HasPrefix(name, ".") && p != root || name == "node_modules" || name == "vendor" || name == "dist" || name == "build" || name == "target") {
				return filepath.SkipDir
			}
			if !d.IsDir() {
				if rel, err := filepath.Rel(root, p); err == nil {
					files = append(files, filepath.ToSlash(rel))
				}
			}
			return nil
		})
	}
	out := files[:0]
	for _, f := range files {
		if slices.ContainsFunc(exclude, func(pattern string) bool { return projectlinks.Match(pattern, f) }) {
			continue
		}
		out = append(out, f)
		if max > 0 && len(out) == max {
			break
		}
	}
	return out
}

// Plan is what indexing the project in dir sends: its files and roughly
// their tokens, and what never goes.
func (e *Engine) Plan(dir string, p proto.RAGProject) proto.RAGPlan {
	st := e.settings.Get()
	out := proto.RAGPlan{Embedder: st.Embedder, Host: e.ProjectHost(dir)}
	secrets, excluded := 0, len(Files(dir, nil, 0))
	files := Files(dir, p.Exclude, orInt(p.MaxFiles, defaultMaxFiles))
	excluded -= len(files)
	var bytes int64
	for _, f := range files {
		if IsSecret(f) {
			secrets++
			continue
		}
		if !textFile(f) {
			continue
		}
		if fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err == nil && fi.Size() <= maxFileSize {
			out.Files++
			bytes += fi.Size()
		}
	}
	out.Tokens = int(bytes / 4)
	out.Left = []string{fmt.Sprintf("%d files that hold secrets (.env, keys, certificates), and any chunk holding a token", secrets),
		"what .gitignore ignores, and files over 500 KB or not text"}
	if excluded > 0 {
		out.Left = append(out.Left, fmt.Sprintf("%d files you left out", excluded))
	}
	return out
}

// textFile says file is one indexed, by its kind.
func textFile(file string) bool {
	switch strings.ToLower(path.Ext(file)) {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java", ".kt", ".swift", ".c", ".cpp", ".h", ".hpp", ".cs", ".rb", ".php",
		".vue", ".svelte", ".sql", ".sh", ".bash", ".zsh", ".yml", ".yaml", ".toml", ".json", ".xml", ".html", ".css", ".scss",
		".md", ".txt", ".cfg", ".ini", ".conf", ".dockerfile", ".proto", ".graphql", ".gql":
		return true
	}
	switch strings.ToLower(path.Base(file)) {
	case "dockerfile", "makefile", "license", ".gitignore":
		return true
	}
	return false
}

func orString(s, or string) string {
	if s == "" {
		return or
	}
	return s
}

func orInt(n, or int) int {
	if n <= 0 {
		return or
	}
	return n
}

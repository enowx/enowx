package rag

import (
	"cmp"
	"context"
	"encoding/binary"
	"math"
	"slices"
	"strings"
	"unicode"
)

// point is a piece of a project in its index: a file's chunk, a memory, or
// part of an attached note.
type point struct {
	id         string // its source and text's hash: unchanged text keeps it
	file       string
	start, end int
	symbol     string
	source     string // proto.RAGCode, RAGDocs, RAGNotes, RAGMemo
	text       string // without the header it was embedded with
	vec        []float32
}

// lines are where a point is, kept as its file changes around it.
type lines struct {
	start, end int
	symbol     string
}

// scored is a point's ID and how near a search found it.
type scored struct {
	id    string
	score float64
}

// store keeps a project's points (the project an index key: its ID and
// the embedder's, vectors of two never compared). Safe from any goroutine.
type store interface {
	// ids are the project's points and where each is.
	ids(ctx context.Context, project string) (map[string]lines, error)
	// put adds points, or moves them: a point without its vector keeps the
	// one it has, its lines changed.
	put(ctx context.Context, project string, pts []point) error
	remove(ctx context.Context, project string, ids []string) error
	drop(ctx context.Context, project string) error
	count(ctx context.Context, project string) (int, error)
	// nearest are the k points nearest q (normalized), by cosine.
	nearest(ctx context.Context, project string, q []float32, k int) ([]scored, error)
	// keyword are the k points best matching query's words.
	keyword(ctx context.Context, project, query string, k int) ([]scored, error)
	get(ctx context.Context, project string, ids []string) (map[string]point, error)
	close() error
}

// normalize makes v of length 1, so a dot product is the cosine.
func normalize(v []float32) []float32 {
	var n float64
	for _, x := range v {
		n += float64(x) * float64(x)
	}
	if n == 0 {
		return v
	}
	s := float32(1 / math.Sqrt(n))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x * s
	}
	return out
}

func dot(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var s float32
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// vecBytes is v as little-endian float32s, as SQLite keeps it.
func vecBytes(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
	}
	return b
}

func bytesVec(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v
}

// words are query's words to match, at most 24, in lower case.
func words(query string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' }) {
		if len(w) < 2 || seen[w] || stopWords[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
		if len(out) == 24 {
			break
		}
	}
	return out
}

var stopWords = map[string]bool{"the": true, "a": true, "an": true, "of": true, "to": true, "in": true, "is": true, "it": true, "and": true, "or": true,
	"how": true, "what": true, "where": true, "when": true, "does": true, "do": true, "are": true, "for": true, "with": true, "on": true, "by": true, "its": true}

// fuse ranks the points of several rankings together (reciprocal rank
// fusion): near the top of either, near the top of both.
func fuse(rankings ...[]scored) []scored {
	const k = 60
	sum := map[string]float64{}
	var order []string
	for _, r := range rankings {
		for i, s := range r {
			if _, ok := sum[s.id]; !ok {
				order = append(order, s.id)
			}
			sum[s.id] += 1 / float64(k+i+1)
		}
	}
	out := make([]scored, len(order))
	for i, id := range order {
		out[i] = scored{id, sum[id]}
	}
	sortScored(out)
	return out
}

func sortScored(s []scored) {
	slices.SortStableFunc(s, func(a, b scored) int { return cmp.Compare(b.score, a.score) })
}

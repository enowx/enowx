package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/enowx/enowx/proto"
)

// embedder turns texts into vectors: documents to index, or a query to
// search them with (some models embed the two apart).
type embedder interface {
	embed(ctx context.Context, texts []string, query bool) ([][]float32, error)
	// tokens is what it was sent since made, as its service counted.
	tokens() int64
}

// newEmbedder is st's embedder, key its key (from the vault).
func newEmbedder(st proto.RAGSettings, key string) (embedder, error) {
	switch st.Embedder {
	case proto.RAGVoyage:
		return &httpEmbedder{kind: proto.RAGVoyage, url: "https://api.voyageai.com/v1/embeddings", key: key,
			model: orString(st.Model, "voyage-code-3"), dim: st.Dim, batch: 64}, nil
	case proto.RAGOpenAI:
		if st.Model == "" {
			return nil, errors.New("rag: name the embedding model")
		}
		base := strings.TrimRight(orString(st.BaseURL, "https://api.openai.com/v1"), "/")
		return &httpEmbedder{kind: proto.RAGOpenAI, url: base + "/embeddings", key: key, model: st.Model, dim: st.Dim, batch: 96}, nil
	case proto.RAGTEI:
		if st.BaseURL == "" {
			return nil, errors.New("rag: give the TEI server's address")
		}
		return &httpEmbedder{kind: proto.RAGTEI, url: strings.TrimRight(st.BaseURL, "/") + "/embed", key: key, batch: 32}, nil
	}
	return nil, ErrNotReady
}

// httpEmbedder calls an embedding service: Voyage's, an OpenAI-compatible
// one, or Text Embeddings Inference.
type httpEmbedder struct {
	kind, url, key, model string
	dim, batch            int
	used                  atomic.Int64
}

func (h *httpEmbedder) tokens() int64 { return h.used.Load() }

func (h *httpEmbedder) embed(ctx context.Context, texts []string, query bool) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += h.batch {
		part := texts[i:min(len(texts), i+h.batch)]
		vecs, err := h.call(ctx, part, query)
		if err != nil {
			return nil, err
		}
		if len(vecs) != len(part) {
			return nil, fmt.Errorf("rag: the embedder gave %d vectors for %d texts", len(vecs), len(part))
		}
		out = append(out, vecs...)
	}
	return out, nil
}

// call embeds one batch, again after a moment when the service is busy.
func (h *httpEmbedder) call(ctx context.Context, texts []string, query bool) ([][]float32, error) {
	var body any
	switch h.kind {
	case proto.RAGVoyage:
		kind := "document"
		if query {
			kind = "query"
		}
		b := map[string]any{"input": texts, "model": h.model, "input_type": kind}
		if h.dim > 0 {
			b["output_dimension"] = h.dim
		}
		body = b
	case proto.RAGOpenAI:
		b := map[string]any{"input": texts, "model": h.model}
		if h.dim > 0 {
			b["dimensions"] = h.dim
		}
		body = b
	default:
		body = map[string]any{"inputs": texts, "truncate": true}
	}
	data, _ := json.Marshal(body)
	var last error
	wait := time.Second
	for try := range 8 {
		if try > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			wait = min(wait*2, 20*time.Second)
		}
		raw, status, after, err := h.post(ctx, data)
		switch {
		case err != nil:
			last = err
			continue
		case status == http.StatusTooManyRequests || status >= 500:
			// Busy, or over its rate: as long as it asks, else longer each time.
			last = fmt.Errorf("rag: the embedder: %d %s", status, short(raw))
			wait = max(wait, after)
			continue
		case status >= 300:
			return nil, fmt.Errorf("rag: the embedder: %d %s", status, short(raw))
		}
		return h.parse(raw, texts)
	}
	return nil, last
}

// post sends data, and says the answer, its status and how long the
// service asked to wait before trying again.
func (h *httpEmbedder) post(ctx context.Context, data []byte) ([]byte, int, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.key != "" {
		req.Header.Set("Authorization", "Bearer "+h.key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("rag: the embedder: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	var after time.Duration
	if secs, e := strconv.Atoi(resp.Header.Get("Retry-After")); e == nil && secs > 0 {
		after = min(time.Duration(secs)*time.Second, time.Minute)
	}
	return raw, resp.StatusCode, after, err
}

func (h *httpEmbedder) parse(raw []byte, texts []string) ([][]float32, error) {
	if h.kind == proto.RAGTEI {
		var out [][]float32
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("rag: the embedder's answer: %w", err)
		}
		n := 0
		for _, t := range texts {
			n += len(t) / 4 // TEI doesn't count: about 4 characters a token
		}
		h.used.Add(int64(n))
		return out, nil
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Usage struct {
			Total int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("rag: the embedder's answer: %w", err)
	}
	vecs := make([][]float32, len(out.Data))
	for i, d := range out.Data {
		at := d.Index
		if at < 0 || at >= len(vecs) {
			at = i
		}
		vecs[at] = d.Embedding
	}
	h.used.Add(out.Usage.Total)
	return vecs, nil
}

// short is the start of a service's error, for a message.
func short(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

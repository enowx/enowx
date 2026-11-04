package library

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// httpEmbedder asks an OpenAI-compatible /embeddings endpoint: what most hosted
// embedders and self-hosted servers speak. It exists for the owner to choose one
// by the environment; nothing makes one unless they do.
type httpEmbedder struct {
	url, model, key string
	client          *http.Client
}

// NewHTTPEmbedder is an embedder at base (…/v1) with model; key may be empty for a
// server of one's own.
func NewHTTPEmbedder(base, model, key string) Embedder {
	return &httpEmbedder{url: strings.TrimRight(base, "/") + "/embeddings", model: model, key: key, client: &http.Client{Timeout: 30 * time.Second}}
}

func (h *httpEmbedder) Model() string { return h.model }

func (h *httpEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"model": h.model, "input": texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("library: embedder: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if h.key != "" {
		req.Header.Set("Authorization", "Bearer "+h.key)
	}
	res, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("library: embedder: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("library: embedder: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("library: embedder answered %d", res.StatusCode)
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("library: embedder: %w", err)
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("library: embedder gave %d vectors for %d texts", len(out.Data), len(texts))
	}
	vectors := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vectors) || len(d.Embedding) == 0 {
			return nil, fmt.Errorf("library: embedder gave a bad vector")
		}
		vectors[d.Index] = d.Embedding
	}
	for _, v := range vectors {
		if v == nil {
			return nil, fmt.Errorf("library: embedder gave a bad vector")
		}
	}
	return vectors, nil
}

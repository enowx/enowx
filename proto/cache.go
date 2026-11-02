package proto

import "time"

// Clean cache: what a host keeps that can go, and taking it away.
const (
	MethodCacheList  = "cache.list"  // CacheListParams → CacheReport
	MethodCacheClean = "cache.clean" // CacheCleanParams → CacheCleanResult
)

// CacheListParams says how old things must be to count.
type CacheListParams struct {
	OlderDays int `json:"olderDays,omitempty"` // 0 is 30
}

// CacheReport is what a host's folder takes, and what of it can go.
type CacheReport struct {
	Total int64       `json:"total"` // all of the host's enowX folder, bytes
	Kinds []CacheKind `json:"kinds"`
}

// CacheKind is a kind of thing that can go.
type CacheKind struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	About   string `json:"about"`             // what cleaning it does
	Warning string `json:"warning,omitempty"` // what is lost for good
	Bytes   int64  `json:"bytes"`             // what cleaning all of it frees
	// Default says it's picked until the user says otherwise. PerItem that its items
	// are picked one by one. Info that it can't be cleaned, only seen.
	Default bool        `json:"default,omitempty"`
	PerItem bool        `json:"perItem,omitempty"`
	Info    bool        `json:"info,omitempty"`
	Items   []CacheItem `json:"items,omitempty"`
}

// CacheItem is one thing of a kind.
type CacheItem struct {
	Path  string    `json:"path"` // under the host's enowX folder, with slashes
	Name  string    `json:"name"`
	Bytes int64     `json:"bytes"`
	When  time.Time `json:"when,omitzero"`
	Note  string    `json:"note,omitempty"` // its state, for the user to weigh
	// Blocked says why it can't go now.
	Blocked string `json:"blocked,omitempty"`
}

// CacheCleanParams says what to clean: whole kinds, and items of per-item kinds.
type CacheCleanParams struct {
	Kinds     []string `json:"kinds,omitempty"`
	Items     []string `json:"items,omitempty"`
	OlderDays int      `json:"olderDays,omitempty"`
}

// CacheCleanResult says what happened, item by item.
type CacheCleanResult struct {
	Freed  int64         `json:"freed"`
	Done   int           `json:"done"`
	Failed []CacheFailed `json:"failed,omitempty"`
}

// CacheFailed is an item that didn't go, and why.
type CacheFailed struct {
	Path string `json:"path"`
	Why  string `json:"why"`
}

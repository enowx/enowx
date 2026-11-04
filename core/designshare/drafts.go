package designshare

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// Drafts are what the host keeps of designs being shared: the bundle and what
// the form holds, a folder each, and which item each design became. A draft
// stays on the host: only its owner sees it, as nothing sends it until they do.
type Drafts struct {
	dir string
}

// linksMu keeps links.json whole: handlers run on connections' goroutines, each
// with a Drafts of its own.
var linksMu sync.Mutex

// OpenDrafts is the drafts kept in dir.
func OpenDrafts(dir string) *Drafts { return &Drafts{dir: dir} }

// Draft is a draft's record; its bundle and pictures are files beside it.
type Draft struct {
	ID       string    `json:"id"`
	Canvas   string    `json:"canvas"`
	Frame    string    `json:"frame"`
	Form     any       `json:"form,omitempty"`
	Prepared any       `json:"prepared,omitempty"`
	Saved    bool      `json:"saved,omitempty"` // the creator asked to keep it
	Title    string    `json:"title,omitempty"`
	Updated  time.Time `json:"updated"`
}

var draftID = regexp.MustCompile(`^[0-9a-f]{16}$`)

// DraftID is the one draft of design frame of canvas.
func DraftID(canvas, frame string) string {
	h := sha256.Sum256([]byte(canvas + "\x00" + frame))
	return hex.EncodeToString(h[:8])
}

func (d *Drafts) folder(id string) (string, error) {
	if !draftID.MatchString(id) {
		return "", errors.New("designshare: no such draft")
	}
	return filepath.Join(d.dir, id), nil
}

// Put keeps a draft's record and its files by name, replacing the files named.
func (d *Drafts) Put(dr Draft, files map[string][]byte) error {
	dir, err := d.folder(dr.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("designshare: draft: %w", err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return fmt.Errorf("designshare: draft: %w", err)
		}
	}
	dr.Updated = time.Now().UTC()
	data, err := json.MarshalIndent(dr, "", " ")
	if err != nil {
		return fmt.Errorf("designshare: draft: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "draft.json"), data, 0o600); err != nil {
		return fmt.Errorf("designshare: draft: %w", err)
	}
	return nil
}

// Get reads a draft's record into out, whose Form and Prepared fields the
// caller types.
func (d *Drafts) Get(id string, out any) error {
	dir, err := d.folder(id)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, "draft.json"))
	if err != nil {
		return fmt.Errorf("designshare: draft: %w", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("designshare: draft: %w", err)
	}
	return nil
}

// File reads a draft's file; nil when it has none.
func (d *Drafts) File(id, name string) []byte {
	dir, err := d.folder(id)
	if err != nil {
		return nil
	}
	data, _ := os.ReadFile(filepath.Join(dir, name))
	return data
}

// Delete forgets a draft.
func (d *Drafts) Delete(id string) error {
	dir, err := d.folder(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// IDs are the drafts kept.
func (d *Drafts) IDs() []string {
	entries, _ := os.ReadDir(d.dir)
	var out []string
	for _, e := range entries {
		if e.IsDir() && draftID.MatchString(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

// Link is what a design became in the library.
type Link struct {
	Canvas string `json:"canvas"`
	Frame  string `json:"frame"`
	Item   string `json:"item"`
}

func (d *Drafts) readLinks() []Link {
	var links []Link
	data, err := os.ReadFile(filepath.Join(d.dir, "links.json"))
	if err == nil {
		json.Unmarshal(data, &links)
	}
	return links
}

// Links are the designs that became items.
func (d *Drafts) Links() []Link {
	linksMu.Lock()
	defer linksMu.Unlock()
	return d.readLinks()
}

// ItemOf is the item design frame of canvas became, "" for none.
func (d *Drafts) ItemOf(canvas, frame string) string {
	for _, l := range d.Links() {
		if l.Canvas == canvas && l.Frame == frame {
			return l.Item
		}
	}
	return ""
}

// SetLink records that design frame of canvas is item; "" forgets it.
func (d *Drafts) SetLink(canvas, frame, item string) error {
	linksMu.Lock()
	defer linksMu.Unlock()
	links := []Link{}
	for _, l := range d.readLinks() {
		if l.Canvas != canvas || l.Frame != frame {
			links = append(links, l)
		}
	}
	if item != "" {
		links = append(links, Link{Canvas: canvas, Frame: frame, Item: item})
	}
	data, err := json.MarshalIndent(links, "", " ")
	if err != nil {
		return fmt.Errorf("designshare: links: %w", err)
	}
	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		return fmt.Errorf("designshare: links: %w", err)
	}
	if err := os.WriteFile(filepath.Join(d.dir, "links.json"), data, 0o600); err != nil {
		return fmt.Errorf("designshare: links: %w", err)
	}
	return nil
}

// Prune deletes drafts nobody asked to keep, untouched for longer than age: each
// is a whole design, packed for a form that was closed.
func (d *Drafts) Prune(age time.Duration) {
	for _, id := range d.IDs() {
		var dr struct {
			Saved   bool      `json:"saved"`
			Updated time.Time `json:"updated"`
		}
		if d.Get(id, &dr) == nil && !dr.Saved && time.Since(dr.Updated) > age {
			d.Delete(id)
		}
	}
}

package usersync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Base is what two hosts had at their last sync: each file's hash, and
// for the text files small enough, its content to merge against.
type Base struct {
	Peer  string              `json:"peer"`
	Last  time.Time           `json:"last"`
	Files map[string]BaseFile `json:"files"`
}

// BaseFile is a file as both hosts had it.
type BaseFile struct {
	Hash string `json:"hash"`
	Data []byte `json:"data,omitempty"`
}

func (b *Base) set(p, hash string, data []byte) {
	if b.Files == nil {
		b.Files = map[string]BaseFile{}
	}
	if hash == "" {
		delete(b.Files, p)
		return
	}
	f := BaseFile{Hash: hash}
	if mergeable(p, data) {
		f.Data = data
	} else if old, ok := b.Files[p]; ok && old.Hash == hash {
		f.Data = old.Data
	}
	b.Files[p] = f
}

func mergeable(p string, data []byte) bool {
	// Merged without a base; a conflict, two whole versions, isn't merged at all, and
	// every base is read and written whole each sync.
	if AppendOnly(p) || strings.HasPrefix(p, ConflictsDir) {
		return false
	}
	switch path.Ext(p) {
	case ".md", ".json", ".txt", ".html", ".css", ".svg":
		return len(data) <= mergeMax && utf8.Valid(data)
	}
	return false
}

// LoadBase reads the base of the sync with peer from dir, empty at first.
func LoadBase(dir, peer string) (*Base, error) {
	b := &Base{Peer: peer, Files: map[string]BaseFile{}}
	data, err := os.ReadFile(baseFile(dir, peer))
	if errors.Is(err, fs.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, fmt.Errorf("usersync: %w", err)
	}
	if err := json.Unmarshal(data, b); err != nil {
		return nil, fmt.Errorf("usersync: base %s: %w", peer, err)
	}
	if b.Files == nil {
		b.Files = map[string]BaseFile{}
	}
	for p, file := range b.Files {
		if file.Data != nil && !mergeable(p, file.Data) { // kept by an older build
			file.Data = nil
			b.Files[p] = file
		}
	}
	return b, nil
}

// Save writes b to dir.
func (b *Base) Save(dir string) error {
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	return writeAtomic(baseFile(dir, b.Peer), data)
}

// ForgetBase drops the base with peer: the next sync starts over.
func ForgetBase(dir, peer string) error {
	err := os.Remove(baseFile(dir, peer))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func baseFile(dir, peer string) string {
	return filepath.Join(dir, "base-"+filepath.Base(peer)+".json")
}

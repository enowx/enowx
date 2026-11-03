package ui

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Pictures of canvases' designs, kept on this device so they show at once, a
// restart included. A picture's key holds its HTML's hash and its size, so a design
// that changed misses it. It is a cache: what fails here only costs a new render.

// canvasThumbsMost bounds the folder, the longest unused going first: an agent
// writing a design leaves a picture per version.
const canvasThumbsMost = 2000

func canvasThumbDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "enowx", "canvas-previews")
}

func canvasThumbFile(dir, key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".png")
}

// readCanvasThumb is the picture kept as key, nil for none; reading it counts as
// its use.
func readCanvasThumb(dir, key string) []byte {
	if dir == "" {
		return nil
	}
	path := canvasThumbFile(dir, key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	return data
}

// keepCanvasThumb keeps data as key, whole or not at all.
func keepCanvasThumb(dir, key string, data []byte) {
	if dir == "" || os.MkdirAll(dir, 0o700) != nil {
		return
	}
	path := canvasThumbFile(dir, key)
	tmp, err := os.CreateTemp(dir, "new-*")
	if err != nil {
		return
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	pruneCanvasThumbs(dir, canvasThumbsMost)
}

// pruneCanvasThumbs leaves the most pictures last used.
func pruneCanvasThumbs(dir string, most int) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= most {
		return
	}
	type kept struct {
		name string
		at   time.Time
	}
	var all []kept
	for _, e := range entries {
		if info, err := e.Info(); err == nil && strings.HasSuffix(e.Name(), ".png") {
			all = append(all, kept{e.Name(), info.ModTime()})
		}
	}
	slices.SortFunc(all, func(a, b kept) int { return cmp.Compare(b.at.UnixNano(), a.at.UnixNano()) })
	for _, k := range all[min(most, len(all)):] {
		_ = os.Remove(filepath.Join(dir, k.name))
	}
}

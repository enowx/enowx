// Package canvas keeps a host's canvases: mockups of UIs in HTML and CSS,
// wireframes, diagrams and boards of shapes, each a folder of its own
// (its frames, every version of them, comments and images) that the user
// and agents change an element at a time.
package canvas

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/proto"
)

// Store is the canvases in a folder: <id>/canvas.json, its versions in
// <id>/history, comments.json and assets/ beside them.
type Store struct {
	dir string
	mu  sync.Mutex // one change at a time
}

// Open is the store in dir, made when the first canvas is.
func Open(dir string) *Store { return &Store{dir: dir} }

var idPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)

// Devices are the sizes a frame is made at, and previewed at.
var Devices = []struct {
	Name          string
	Width, Height int
}{{"Desktop", 1440, 900}, {"Tablet", 768, 1024}, {"Mobile", 390, 844}}

// Device is the size of device name ("desktop", "tablet", "mobile").
func Device(name string) (w, h int, ok bool) {
	for _, d := range Devices {
		if strings.EqualFold(d.Name, name) {
			return d.Width, d.Height, true
		}
	}
	return 0, 0, false
}

func (s *Store) folder(id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("canvas: no canvas %q", id)
	}
	return filepath.Join(s.dir, id), nil
}

// Dir is canvas id's folder, where its assets are.
func (s *Store) Dir(id string) (string, error) { return s.folder(id) }

// NewID is a new canvas's ID, as Create gives it.
func NewID() string { return newID(6) }

func newID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// writeFile writes data to path whole or not at all.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// List is every canvas without its frames, the latest changed first.
func (s *Store) List() ([]proto.CanvasInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []proto.CanvasInfo{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("canvas: %w", err)
	}
	out := []proto.CanvasInfo{}
	for _, e := range entries {
		if !e.IsDir() || !idPattern.MatchString(e.Name()) {
			continue
		}
		c, err := s.Get(e.Name())
		if err != nil {
			continue // half made
		}
		out = append(out, c.CanvasInfo)
	}
	slices.SortStableFunc(out, func(a, b proto.CanvasInfo) int { return b.Updated.Compare(a.Updated) })
	return out, nil
}

// Get is canvas id with its frames.
func (s *Store) Get(id string) (proto.Canvas, error) {
	dir, err := s.folder(id)
	if err != nil {
		return proto.Canvas{}, err
	}
	var c proto.Canvas
	if err := readJSON(filepath.Join(dir, "canvas.json"), &c); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return proto.Canvas{}, fmt.Errorf("canvas: no canvas %q", id)
		}
		return proto.Canvas{}, fmt.Errorf("canvas: %s: %w", id, err)
	}
	return c, nil
}

// Find is the canvas named by ref: its ID, else its title (ignoring
// case), else the only one whose title starts with it.
func (s *Store) Find(ref string) (proto.CanvasInfo, error) {
	list, err := s.List()
	if err != nil {
		return proto.CanvasInfo{}, err
	}
	var prefixed []proto.CanvasInfo
	for _, c := range list {
		if c.ID == ref || strings.EqualFold(c.Title, ref) {
			return c, nil
		}
		if ref != "" && strings.HasPrefix(strings.ToLower(c.Title), strings.ToLower(ref)) {
			prefixed = append(prefixed, c)
		}
	}
	if len(prefixed) == 1 {
		return prefixed[0], nil
	}
	return proto.CanvasInfo{}, fmt.Errorf("canvas: no canvas %q", ref)
}

// Create makes a canvas, its first version.
func (s *Store) Create(p proto.CanvasCreateParams) (proto.Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kind := p.Kind
	switch kind {
	case "":
		kind = proto.CanvasMockup
	case proto.CanvasMockup, proto.CanvasWireframe, proto.CanvasDiagram, proto.CanvasBoard:
	default:
		return proto.Canvas{}, fmt.Errorf("canvas: no kind %q: mockup, wireframe, diagram or board", kind)
	}
	title := strings.Join(strings.Fields(p.Title), " ")
	if title == "" {
		title = "Untitled canvas"
	}
	now := time.Now().UTC()
	c := proto.Canvas{CanvasInfo: proto.CanvasInfo{
		ID: newID(6), Title: title, Kind: kind, Group: strings.TrimSpace(p.Group), Projects: p.Projects,
		Created: now, Updated: now,
	}}
	for _, f := range p.Frames {
		c.Frames = append(c.Frames, newFrame(f, kind))
	}
	if len(c.Frames) == 0 {
		c.Frames = []proto.CanvasFrame{newFrame(proto.CanvasFrame{}, kind)}
	}
	if err := s.commit(&c, proto.CanvasVersion{Op: "create"}); err != nil {
		return proto.Canvas{}, err
	}
	return c, nil
}

// newFrame fills in what f leaves out: an ID, a name and the kind's size.
func newFrame(f proto.CanvasFrame, kind string) proto.CanvasFrame {
	if f.ID == "" {
		f.ID = "f" + newID(3)
	}
	for _, d := range Devices {
		if strings.EqualFold(d.Name, f.Name) {
			f.Name = d.Name
		}
	}
	if f.Width <= 0 || f.Height <= 0 {
		if w, h, ok := Device(f.Name); ok {
			f.Width, f.Height = w, h
		} else if kind == proto.CanvasDiagram || kind == proto.CanvasBoard {
			f.Width, f.Height = 1600, 1000
		} else {
			f.Width, f.Height = 1440, 900
		}
	}
	if f.Name == "" {
		switch kind {
		case proto.CanvasDiagram:
			f.Name = "Diagram"
		case proto.CanvasBoard:
			f.Name = "Board"
		default:
			f.Name = "Desktop"
		}
	}
	return f
}

// Delete deletes canvas id, its versions, comments and assets.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.folder(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "canvas.json")); err != nil {
		return fmt.Errorf("canvas: no canvas %q", id)
	}
	return os.RemoveAll(dir)
}

// FrameIndex is the index of c's design ref: its ID or name, its number
// from 1 ("3", "design-3", "desain-3", "#3"), "" for the first.
func FrameIndex(c proto.Canvas, ref string) (int, error) {
	if len(c.Frames) == 0 {
		return -1, errors.New("canvas: it has no frames")
	}
	if ref == "" {
		return 0, nil
	}
	for i, f := range c.Frames {
		if f.ID == ref || strings.EqualFold(f.Name, ref) {
			return i, nil
		}
	}
	if n, ok := DesignNumber(ref); ok {
		if n >= 1 && n <= len(c.Frames) {
			return n - 1, nil
		}
		return -1, fmt.Errorf("canvas: no design %d: it has %d", n, len(c.Frames))
	}
	return -1, fmt.Errorf("canvas: no frame %q", ref)
}

// DesignNumber reads a design's number: "3", "#3", "design-3", "desain-3".
func DesignNumber(ref string) (int, bool) {
	r := strings.ToLower(strings.TrimSpace(ref))
	for _, prefix := range []string{"design-", "desain-", "design ", "desain ", "#"} {
		r = strings.TrimPrefix(r, prefix)
	}
	n, err := strconv.Atoi(r)
	return n, err == nil
}

// AddAsset keeps data as canvas id's file name (only its base), for
// images to show.
func (s *Store) AddAsset(id, name string, data []byte) (string, error) {
	dir, err := s.folder(id)
	if err != nil {
		return "", err
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("canvas: bad asset name %q", name)
	}
	const maxAsset = 20 << 20
	if len(data) > maxAsset {
		return "", errors.New("canvas: an asset is 20 MB at most")
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o700); err != nil {
		return "", err
	}
	return name, os.WriteFile(filepath.Join(dir, "assets", name), data, 0o600)
}

// Asset reads canvas id's file name.
func (s *Store) Asset(id, name string) ([]byte, error) {
	dir, err := s.folder(id)
	if err != nil {
		return nil, err
	}
	if name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return nil, fmt.Errorf("canvas: bad asset name %q", name)
	}
	return os.ReadFile(filepath.Join(dir, "assets", name))
}

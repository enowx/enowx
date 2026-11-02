package canvas

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/enowx/enowx/proto"
)

// Every change makes a version, so the user goes back to any and undoes an agent's
// work as their own (history_frames.go says how they're kept). Undo and redo move
// between versions; a change after an undo forgets those undone. Versions are this
// host's own, as they aren't synced: a change that came from elsewhere is a version
// here once it is undone from.

// maxVersions is how many versions a canvas keeps, the oldest going first, those
// with a label last: a redesign's passes are what a user goes back to.
const maxVersions = 400

// pruneEvery is how many versions pass between looks for frames none needs.
const pruneEvery = 64

func (s *Store) historyFile(id string) string { return filepath.Join(s.dir, id, "history.json") }

func (s *Store) versionFile(id string, n int) string {
	return filepath.Join(s.dir, id, "history", strconv.Itoa(n)+".json")
}

func (s *Store) history(id string) (proto.CanvasHistory, error) {
	var h proto.CanvasHistory
	err := readJSON(s.historyFile(id), &h)
	if os.IsNotExist(err) {
		return proto.CanvasHistory{}, nil
	}
	return h, err
}

// commit writes c as its new version, made as v says. The caller holds
// s.mu.
func (s *Store) commit(c *proto.Canvas, v proto.CanvasVersion) error {
	h, err := s.history(c.ID)
	if err != nil {
		return fmt.Errorf("canvas: history: %w", err)
	}
	// Those undone go.
	for len(h.Versions) > 0 && h.Versions[len(h.Versions)-1].N > h.Head {
		_ = os.Remove(s.versionFile(c.ID, h.Versions[len(h.Versions)-1].N))
		h.Versions = h.Versions[:len(h.Versions)-1]
	}
	v.N = max(h.Head, c.Version) + 1
	v.Time = time.Now().UTC()
	h.Head = v.N
	h.Versions = append(h.Versions, v)
	for len(h.Versions) > maxVersions {
		i := slices.IndexFunc(h.Versions, func(x proto.CanvasVersion) bool { return x.Label == "" })
		if i < 0 {
			i = 0
		}
		_ = os.Remove(s.versionFile(c.ID, h.Versions[i].N))
		h.Versions = slices.Delete(h.Versions, i, i+1)
	}
	c.Version, c.Updated = v.N, v.Time
	if err := s.writeVersion(*c, v.N); err != nil {
		return fmt.Errorf("canvas: %w", err)
	}
	if v.N%pruneEvery == 0 {
		s.pruneFrames(c.ID, h.Versions)
	}
	if err := writeJSON(filepath.Join(s.dir, c.ID, "canvas.json"), c); err != nil {
		return fmt.Errorf("canvas: %w", err)
	}
	return writeJSON(s.historyFile(c.ID), h)
}

// History is canvas id's versions.
func (s *Store) History(id string) (proto.CanvasHistory, error) {
	if _, err := s.folder(id); err != nil {
		return proto.CanvasHistory{}, err
	}
	return s.history(id)
}

// Version is canvas id as it was at version n.
func (s *Store) Version(id string, n int) (proto.Canvas, error) {
	if _, err := s.folder(id); err != nil {
		return proto.Canvas{}, err
	}
	c, err := s.readVersion(id, n)
	if err != nil {
		return proto.Canvas{}, fmt.Errorf("canvas: no version %d of %s", n, id)
	}
	return c, nil
}

// Undo shows canvas id's version before the one shown.
func (s *Store) Undo(id string) (proto.Canvas, error) { return s.step(id, -1) }

// Redo shows the version undone last.
func (s *Store) Redo(id string) (proto.Canvas, error) { return s.step(id, +1) }

func (s *Store) step(id string, by int) (proto.Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.folder(id); err != nil {
		return proto.Canvas{}, err
	}
	h, err := s.history(id)
	if err != nil {
		return proto.Canvas{}, err
	}
	cur, err := s.Get(id)
	if err != nil {
		return proto.Canvas{}, err
	}
	if cur.Version != h.Head && len(h.Versions) > 0 {
		// A change that came from another host: it must be there to come back to.
		if err := s.commit(&cur, proto.CanvasVersion{Op: "synced"}); err != nil {
			return proto.Canvas{}, err
		}
		if h, err = s.history(id); err != nil {
			return proto.Canvas{}, err
		}
	}
	at := slices.IndexFunc(h.Versions, func(v proto.CanvasVersion) bool { return v.N == h.Head })
	if at < 0 || at+by < 0 || at+by >= len(h.Versions) {
		if by < 0 {
			return proto.Canvas{}, fmt.Errorf("canvas: nothing to undo")
		}
		return proto.Canvas{}, fmt.Errorf("canvas: nothing to redo")
	}
	n := h.Versions[at+by].N
	c, err := s.Version(id, n)
	if err != nil {
		return proto.Canvas{}, err
	}
	// What isn't the design stays as it is now.
	c.Title, c.Group, c.Projects = cur.Title, cur.Group, cur.Projects
	c.Version, c.Updated = n, time.Now().UTC()
	h.Head = n
	if err := writeJSON(filepath.Join(s.dir, id, "canvas.json"), c); err != nil {
		return proto.Canvas{}, err
	}
	return c, writeJSON(s.historyFile(id), h)
}

// Restore makes canvas id as it was at version n its newest version.
func (s *Store) Restore(id string, n int) (proto.Canvas, error) {
	old, err := s.Version(id, n)
	if err != nil {
		return proto.Canvas{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.Get(id)
	if err != nil {
		return proto.Canvas{}, err
	}
	c.Frames = old.Frames
	c.Kind = old.Kind
	return c, s.commit(&c, proto.CanvasVersion{Op: "restore " + strconv.Itoa(n)})
}

// Label names canvas id's latest version label, and returns its number.
func (s *Store) Label(id, label string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.history(id)
	if err != nil {
		return 0, fmt.Errorf("canvas: history: %w", err)
	}
	for i := range h.Versions {
		if h.Versions[i].N == h.Head {
			h.Versions[i].Label = label
			return h.Head, writeJSON(s.historyFile(id), h)
		}
	}
	return h.Head, nil
}

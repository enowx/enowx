package canvas

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/enowx/enowx/proto"
)

// A version used to be a whole copy of its canvas, so a prototype of a hundred designs
// came to hundreds of megabytes, though a change touches one design. Now a version
// names its frames by their content, and each frame's content is kept once,
// compressed, in <id>/history/frames.

// storedVersion is a version as kept: its canvas without its frames, and those frames'
// names. An older version file has no FrameRefs, and its frames inside it.
type storedVersion struct {
	proto.Canvas
	FrameRefs []string `json:"frameRefs,omitempty"`
}

// historyFormat marks a canvas whose older versions have all been converted.
const historyFormat = "format"

func (s *Store) framesDir(id string) string { return filepath.Join(s.dir, id, "history", "frames") }

func (s *Store) frameFile(id, ref string) string {
	return filepath.Join(s.framesDir(id), ref+".json.gz")
}

// keepFrames keeps frames not kept yet, and gives their names.
func (s *Store) keepFrames(id string, frames []proto.CanvasFrame) ([]string, error) {
	refs := make([]string, len(frames))
	for i, f := range frames {
		data, err := json.Marshal(f)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		refs[i] = hex.EncodeToString(sum[:16])
		path := s.frameFile(id, refs[i])
		if _, err := os.Stat(path); err == nil {
			continue
		}
		var zipped bytes.Buffer
		zw := gzip.NewWriter(&zipped)
		if _, err := zw.Write(data); err != nil {
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
		if err := writeFile(path, zipped.Bytes()); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

// readFrame is the frame kept as ref.
func (s *Store) readFrame(id, ref string) (proto.CanvasFrame, error) {
	var f proto.CanvasFrame
	file, err := os.Open(s.frameFile(id, ref))
	if err != nil {
		return f, err
	}
	defer file.Close()
	zr, err := gzip.NewReader(file)
	if err != nil {
		return f, err
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		return f, err
	}
	return f, json.Unmarshal(data, &f)
}

// writeVersion keeps c as version n.
func (s *Store) writeVersion(c proto.Canvas, n int) error {
	refs, err := s.keepFrames(c.ID, c.Frames)
	if err != nil {
		return err
	}
	c.Frames = nil
	return writeJSON(s.versionFile(c.ID, n), storedVersion{Canvas: c, FrameRefs: refs})
}

// readVersion is version n of canvas id, in either form.
func (s *Store) readVersion(id string, n int) (proto.Canvas, error) {
	var v storedVersion
	if err := readJSON(s.versionFile(id, n), &v); err != nil {
		return proto.Canvas{}, err
	}
	if v.FrameRefs == nil {
		return v.Canvas, nil
	}
	v.Canvas.Frames = make([]proto.CanvasFrame, len(v.FrameRefs))
	for i, ref := range v.FrameRefs {
		f, err := s.readFrame(id, ref)
		if err != nil {
			return proto.Canvas{}, fmt.Errorf("canvas: version %d: %w", n, err)
		}
		v.Canvas.Frames[i] = f
	}
	return v.Canvas, nil
}

// pruneFrames removes the frames no version names any more. The caller holds s.mu.
func (s *Store) pruneFrames(id string, versions []proto.CanvasVersion) {
	used := map[string]bool{}
	for _, v := range versions {
		var sv storedVersion
		if err := readJSON(s.versionFile(id, v.N), &sv); err != nil {
			return // can't tell what's in use: nothing goes
		}
		for _, ref := range sv.FrameRefs {
			used[ref] = true
		}
	}
	entries, err := os.ReadDir(s.framesDir(id))
	if err != nil {
		return
	}
	for _, e := range entries {
		if ref, ok := strings.CutSuffix(e.Name(), ".json.gz"); ok && !used[ref] {
			_ = os.Remove(filepath.Join(s.framesDir(id), e.Name()))
		}
	}
}

// historyBackupDone marks a backup as whole, in its folder.
const historyBackupDone = "complete"

// BackupHistory keeps canvas id's versions as they are, in folder dest, for its conversion:
// what's there isn't touched again, so a conversion that stopped partway keeps the originals.
// Files are linked rather than copied where the system can: a conversion replaces a version's
// file rather than writing in it, so the link keeps the old content, and costs nothing until
// then. It says whether there's anything to convert: nothing to keep means nothing to do.
func (s *Store) BackupHistory(id, dest string) (bool, error) {
	if _, err := s.folder(id); err != nil {
		return false, err
	}
	dir := filepath.Join(s.dir, id, "history")
	if _, err := os.Stat(filepath.Join(dir, historyFormat)); err == nil {
		return false, nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(entries) == 0 {
		return false, nil
	}
	if _, err := os.Stat(filepath.Join(dest, historyBackupDone)); err == nil {
		return true, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := keepFiles(dir, filepath.Join(dest, "history")); err != nil {
		return false, err
	}
	if err := keepFile(s.historyFile(id), filepath.Join(dest, "history.json")); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, writeFile(filepath.Join(dest, historyBackupDone), []byte("complete\n"))
}

// keepFiles keeps the files of folder from in folder to, the folders in it too.
func keepFiles(from, to string) error {
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(to, 0o700); err != nil {
		return err
	}
	for _, e := range entries {
		a, b := filepath.Join(from, e.Name()), filepath.Join(to, e.Name())
		if e.IsDir() {
			err = keepFiles(a, b)
		} else {
			err = keepFile(a, b)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// keepFile links or copies a file, and checks that what's there is as long as the original.
func keepFile(from, to string) error {
	info, err := os.Stat(from)
	if err != nil {
		return err
	}
	if have, err := os.Stat(to); err == nil && have.Size() == info.Size() {
		return nil // from a backup that stopped partway
	}
	_ = os.Remove(to)
	if os.Link(from, to) != nil {
		data, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		if err := os.WriteFile(to, data, 0o600); err != nil {
			return err
		}
	}
	have, err := os.Stat(to)
	if err != nil {
		return err
	}
	if have.Size() != info.Size() {
		return fmt.Errorf("the copy of %s isn't whole", filepath.Base(from))
	}
	return nil
}

// ConvertHistory turns canvas id's versions still kept whole into versions naming their
// frames, one at a time, each read back before its whole copy goes, and says how many
// bytes that freed. It stops at a version it can't vouch for, which stays as it was.
func (s *Store) ConvertHistory(id string) (int64, error) {
	if _, err := s.folder(id); err != nil {
		return 0, err
	}
	dir := filepath.Join(s.dir, id, "history")
	if _, err := os.Stat(filepath.Join(dir, historyFormat)); err == nil {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var freed int64
	for _, e := range entries {
		n, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil || e.IsDir() {
			continue
		}
		saved, err := s.convertVersion(id, n)
		if err != nil {
			return freed, fmt.Errorf("canvas: convert %s version %d: %w", id, n, err)
		}
		freed += saved
	}
	return freed, writeFile(filepath.Join(dir, historyFormat), []byte("2\n"))
}

// convertVersion converts version n if it is still whole, under s.mu as a change is.
func (s *Store) convertVersion(id string, n int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.versionFile(id, n)
	info, err := os.Stat(path)
	if err != nil {
		return 0, nil // gone meanwhile
	}
	var v storedVersion
	if err := readJSON(path, &v); err != nil {
		return 0, err
	}
	if v.FrameRefs != nil || len(v.Canvas.Frames) == 0 {
		return 0, nil
	}
	whole, err := json.Marshal(v.Canvas.Frames)
	if err != nil {
		return 0, err
	}
	refs, err := s.keepFrames(id, v.Canvas.Frames)
	if err != nil {
		return 0, err
	}
	back := make([]proto.CanvasFrame, len(refs))
	for i, ref := range refs {
		if back[i], err = s.readFrame(id, ref); err != nil {
			return 0, err
		}
	}
	again, err := json.Marshal(back)
	if err != nil {
		return 0, err
	}
	if !bytes.Equal(whole, again) {
		return 0, fmt.Errorf("what was kept doesn't read back the same")
	}
	c := v.Canvas
	c.Frames = nil
	if err := writeJSON(path, storedVersion{Canvas: c, FrameRefs: refs}); err != nil {
		return 0, err
	}
	now, err := os.Stat(path)
	if err != nil {
		return 0, nil
	}
	return info.Size() - now.Size(), nil
}

// ThinHistory takes canvas id down to its latest keep versions, those with a label and
// the one shown, and says how many bytes that freed. The design itself stays.
func (s *Store) ThinHistory(id string, keep int) (int64, error) {
	if _, err := s.folder(id); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before := dirSize(filepath.Join(s.dir, id, "history"))
	h, err := s.history(id)
	if err != nil {
		return 0, err
	}
	var kept []proto.CanvasVersion
	for i, v := range h.Versions {
		if i >= len(h.Versions)-keep || v.Label != "" || v.N == h.Head {
			kept = append(kept, v)
			continue
		}
		_ = os.Remove(s.versionFile(id, v.N))
	}
	h.Versions = kept
	if err := writeJSON(s.historyFile(id), h); err != nil {
		return 0, err
	}
	s.pruneFrames(id, h.Versions)
	return before - dirSize(filepath.Join(s.dir, id, "history")), nil
}

// dirSize is what the files under dir take.
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

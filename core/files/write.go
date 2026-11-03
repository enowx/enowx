package files

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/enowx/enowx/proto"
)

// ReadWhole is Read up to proto.MaxEdit, for a file to edit. An absolute path reads where it
// is, as ReadAbs does.
func ReadWhole(root, rel string, abs bool, unless time.Time) (proto.FileContent, error) {
	if abs {
		if !filepath.IsAbs(rel) {
			return proto.FileContent{}, fmt.Errorf("files: %s is not an absolute path", rel)
		}
		clean := filepath.Clean(rel)
		return read(clean, clean, unless, proto.MaxEdit)
	}
	path, clean, err := resolve(root, rel)
	if err != nil {
		return proto.FileContent{}, err
	}
	return read(path, clean, unless, proto.MaxEdit)
}

// Write saves p in root (or where it says, when absolute): a whole new file moved over the
// old, so that nothing reads it half written, with the old one's mode; through a link, to
// what it points at, which has to stay in root. A file changed since p.Unless isn't written
// over unless p.Force says so.
func Write(root string, p proto.FileWrite) (proto.FileWritten, error) {
	if len(p.Text) > proto.MaxEdit {
		return proto.FileWritten{}, fmt.Errorf("files: %s is too long to save here", p.Path)
	}
	path := filepath.Clean(p.Path)
	if p.Abs {
		if !filepath.IsAbs(p.Path) {
			return proto.FileWritten{}, fmt.Errorf("files: %s is not an absolute path", p.Path)
		}
	} else {
		abs, _, err := resolve(root, p.Path)
		if err != nil {
			return proto.FileWritten{}, err
		}
		path = abs
	}
	mode := os.FileMode(0o644)
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if !p.Force {
			return proto.FileWritten{Changed: true}, nil // it went: the user says what to do
		}
	case err != nil:
		return proto.FileWritten{}, fmt.Errorf("files: %w", err)
	case info.IsDir():
		return proto.FileWritten{}, fmt.Errorf("files: %s is a folder", p.Path)
	case info.Mode().Perm()&0o200 == 0:
		return proto.FileWritten{}, fmt.Errorf("files: %s is read-only", p.Path)
	default:
		mode = info.Mode().Perm()
		if !p.Force && !p.Unless.IsZero() && !info.ModTime().Equal(p.Unless) {
			return proto.FileWritten{Modified: info.ModTime(), Changed: true}, nil
		}
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return proto.FileWritten{}, fmt.Errorf("files: %w", err)
		}
		if !p.Abs {
			if real, err = Contained(root, path); err != nil {
				return proto.FileWritten{}, err
			}
		}
		path = real
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.saving")
	if err != nil {
		return proto.FileWritten{}, fmt.Errorf("files: %w", err)
	}
	defer os.Remove(tmp.Name()) // gone already once it's moved
	_, err = tmp.WriteString(p.Text)
	if err == nil {
		err = tmp.Chmod(mode)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		return proto.FileWritten{}, fmt.Errorf("files: %w", err)
	}
	st, err := os.Stat(path)
	if err != nil {
		return proto.FileWritten{}, fmt.Errorf("files: %w", err)
	}
	return proto.FileWritten{Modified: st.ModTime()}, nil
}

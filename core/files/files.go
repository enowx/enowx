// Package files lists and reads the files of a session's folder, for the
// side panel's Files tool. Paths are relative to the folder and can't
// reach out of it.
package files

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/proto"
)

// Limits keep what the app receives small: a folder lists its first
// maxEntries, and a file shows its first maxRead bytes.
const (
	maxEntries = 2000
	maxRead    = 1 << 20
)

// resolve returns the absolute path of rel in root, refusing paths that
// leave it.
func resolve(root, rel string) (string, string, error) {
	if root == "" {
		return "", "", errors.New("files: the session has no folder")
	}
	clean := path.Clean("/" + filepath.ToSlash(rel))
	if strings.Contains(clean, "/../") || clean == "/.." {
		return "", "", fmt.Errorf("files: %s is outside the session's folder", rel)
	}
	clean = strings.TrimPrefix(clean, "/")
	return filepath.Join(root, filepath.FromSlash(clean)), clean, nil
}

// Folder returns the absolute path of folder rel of root, making it if
// missing; it refuses paths that leave root.
func Folder(root, rel string) (string, error) {
	abs, _, err := resolve(root, rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("files: %w", err)
	}
	return abs, nil
}

// List lists the folder rel of root, marking what git says changed or
// ignores when root is in a repository.
func List(root, rel string) (proto.FileListing, error) {
	abs, clean, err := resolve(root, rel)
	if err != nil {
		return proto.FileListing{}, err
	}
	dirents, err := os.ReadDir(abs)
	if err != nil {
		return proto.FileListing{}, fmt.Errorf("files: %w", err)
	}
	out := proto.FileListing{Path: clean, Root: root}
	for _, d := range dirents {
		if d.Name() == ".git" {
			continue
		}
		e := proto.FileEntry{Name: d.Name(), Dir: d.IsDir()}
		if info, err := d.Info(); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				if target, err := os.Stat(filepath.Join(abs, d.Name())); err == nil {
					info = target
					e.Dir = target.IsDir()
				}
			}
			if !e.Dir {
				e.Size = info.Size()
			}
		}
		out.Entries = append(out.Entries, e)
	}
	slices.SortFunc(out.Entries, func(a, b proto.FileEntry) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	if len(out.Entries) > maxEntries {
		out.Entries, out.Truncated = out.Entries[:maxEntries], true
	}
	markGit(root, clean, out.Entries)
	return out, nil
}

// markGit marks the entries of folder rel with git's status, and those git
// ignores. Outside a repository it marks nothing.
func markGit(root, rel string, entries []proto.FileEntry) {
	if len(entries) == 0 {
		return
	}
	top, err := git.TopLevel(root)
	if err != nil {
		return
	}
	// git status names paths from the repository's top, which root may be
	// below.
	under := ""
	if r, err := filepath.Rel(top, root); err == nil && r != "." {
		under = filepath.ToSlash(r) + "/"
	}
	prefix := ""
	if rel != "" {
		prefix = rel + "/"
	}
	pathspec := rel
	if pathspec == "" {
		pathspec = "."
	}
	// Untrimmed: a status line may start with a space.
	status, _ := git.Command(root, "-c", "core.quotePath=false", "status", "--porcelain", "--untracked-files=all", "--", pathspec).Output()
	changed := map[string]string{}
	for _, line := range strings.Split(string(status), "\n") {
		if len(line) < 4 {
			continue
		}
		code, p := strings.TrimSpace(line[:2]), line[3:]
		if _, to, ok := strings.Cut(p, " -> "); ok {
			p = to
		}
		if code == "??" {
			code = "?"
		}
		if p, ok := strings.CutPrefix(p, under); ok && code != "" {
			changed[p] = code[:1]
		}
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = prefix + e.Name
	}
	ignored := map[string]bool{}
	cmd := git.Command(root, "check-ignore", "--stdin")
	cmd.Stdin = strings.NewReader(strings.Join(names, "\n") + "\n")
	if out, err := cmd.Output(); err == nil || len(out) > 0 {
		for _, p := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			ignored[strings.TrimSuffix(p, "/")] = true
		}
	}
	for i := range entries {
		e, name := &entries[i], names[i]
		e.Ignored = ignored[name]
		if !e.Dir {
			e.Status = changed[name]
			continue
		}
		for p := range changed {
			if strings.HasPrefix(p, name+"/") {
				e.Status = "M"
				break
			}
		}
	}
}

// imageExts are the image formats Read sends whole, for the app to show.
var imageExts = map[string]bool{"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true, "bmp": true, "svg": true}

// Read reads the file rel of root: an image's bytes (up to
// proto.MaxImage), else its first maxRead bytes, unless it looks binary.
// A file last changed at unless (when set) reads as Unchanged.
func Read(root, rel string, unless time.Time) (proto.FileContent, error) {
	abs, clean, err := resolve(root, rel)
	if err != nil {
		return proto.FileContent{}, err
	}
	return read(abs, clean, unless, maxRead)
}

// ReadWithin is Read refusing a file that leads out of root through a
// link: root and the file are resolved, links and all, and the file must
// still be in root. It reads what it resolved.
func ReadWithin(root, rel string, unless time.Time) (proto.FileContent, error) {
	abs, clean, err := resolve(root, rel)
	if err != nil {
		return proto.FileContent{}, err
	}
	real, err := Contained(root, abs)
	if err != nil {
		return proto.FileContent{}, err
	}
	return read(real, clean, unless, maxRead)
}

// Contained is path abs resolved, links and all, when it stays in root,
// resolved too; an error when it leads out of it or doesn't exist.
func Contained(root, abs string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("files: %w", err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("files: %w", err)
	}
	if real != realRoot && !strings.HasPrefix(real, strings.TrimSuffix(realRoot, string(filepath.Separator))+string(filepath.Separator)) {
		return "", fmt.Errorf("files: %s leads out of the folder", abs)
	}
	return real, nil
}

// ReadAbs reads the file at the absolute path abs, as Read does: a file a
// session's program printed, wherever it is.
func ReadAbs(abs string, unless time.Time) (proto.FileContent, error) {
	if !filepath.IsAbs(abs) {
		return proto.FileContent{}, fmt.Errorf("files: %s is not an absolute path", abs)
	}
	abs = filepath.Clean(abs)
	return read(abs, abs, unless, maxRead)
}

// ReadPart reads up to proto.BytesPart of the file rel of root from offset,
// as it is; abs reads rel as an absolute path on the host instead.
func ReadPart(root, rel string, abs bool, offset int64) (proto.FileBytes, error) {
	full := filepath.Clean(rel)
	if abs {
		if !filepath.IsAbs(rel) {
			return proto.FileBytes{}, fmt.Errorf("files: %s is not an absolute path", rel)
		}
	} else {
		var err error
		if full, _, err = resolve(root, rel); err != nil {
			return proto.FileBytes{}, err
		}
	}
	f, err := os.Open(full)
	if err != nil {
		return proto.FileBytes{}, fmt.Errorf("files: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return proto.FileBytes{}, fmt.Errorf("files: %w", err)
	}
	if info.IsDir() {
		return proto.FileBytes{}, fmt.Errorf("files: %s is a folder", rel)
	}
	out := proto.FileBytes{Size: info.Size(), Modified: info.ModTime()}
	if offset >= info.Size() {
		return out, nil
	}
	buf := make([]byte, min(int64(proto.BytesPart), info.Size()-offset))
	n, err := f.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return proto.FileBytes{}, fmt.Errorf("files: %w", err)
	}
	out.Data = buf[:n]
	return out, nil
}

// read reads the file at abs, named clean in what it returns.
func read(abs, clean string, unless time.Time, most int64) (proto.FileContent, error) {
	f, err := os.Open(abs)
	if err != nil {
		return proto.FileContent{}, fmt.Errorf("files: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return proto.FileContent{}, fmt.Errorf("files: %w", err)
	}
	if info.IsDir() {
		return proto.FileContent{}, fmt.Errorf("files: %s is a folder", clean)
	}
	out := proto.FileContent{Path: clean, Size: info.Size(), Modified: info.ModTime()}
	if !unless.IsZero() && info.ModTime().Equal(unless) {
		out.Unchanged = true
		return out, nil
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(clean), "."))
	if imageExts[ext] && info.Size() <= proto.MaxImage {
		data, err := io.ReadAll(f)
		if err != nil {
			return proto.FileContent{}, fmt.Errorf("files: %w", err)
		}
		if ext == "jpg" {
			ext = "jpeg"
		}
		out.Image, out.Data = ext, data
		return out, nil
	}
	data, err := io.ReadAll(io.LimitReader(f, most))
	if err != nil {
		return proto.FileContent{}, fmt.Errorf("files: %w", err)
	}
	out.Truncated = info.Size() > most
	out.ReadOnly = info.Mode().Perm()&0o200 == 0
	if out.Truncated {
		// Cut at a whole character, not inside one: the part shown and
		// copied stays valid text.
		for i := 0; i < utf8.UTFMax-1 && len(data) > 0; i++ {
			if r, size := utf8.DecodeLastRune(data); r != utf8.RuneError || size > 1 {
				break
			}
			data = data[:len(data)-1]
		}
	}
	if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
		out.Binary = true
		return out, nil
	}
	out.Text = string(data)
	return out, nil
}

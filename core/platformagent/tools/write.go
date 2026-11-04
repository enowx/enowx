package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	// As much of a change the chat's diff keeps, as it does of other agents'.
	diffClip = 3000
	// Over this a file's old text isn't read for the diff.
	diffReadMost = 4 << 20
)

const writeSchema = `{"type":"object","properties":{"file_path":{"type":"string","description":"The file to write, absolute or relative to the working folder"},"content":{"type":"string","description":"The file's whole new content"}},"required":["file_path","content"],"additionalProperties":false}`

type writeTool struct{}

func (writeTool) Name() string { return "Write" }
func (writeTool) Description() string {
	return "Write a file whole, replacing what it holds and creating missing folders. To change part of a file, use Edit."
}
func (writeTool) Schema() json.RawMessage { return json.RawMessage(writeSchema) }
func (writeTool) ReadOnly() bool          { return false }
func (writeTool) Edits() bool             { return true }

type writeInput struct {
	FilePath string  `json:"file_path"`
	Content  *string `json:"content"`
}

func (writeTool) Detail(input json.RawMessage) string {
	in, err := decode[writeInput](input)
	if err != nil {
		return "Write"
	}
	return oneLine(in.FilePath, 200)
}

func (writeTool) Run(_ context.Context, input json.RawMessage, env Env) Result {
	in, err := decode[writeInput](input)
	if err != nil {
		return failf("%v", err)
	}
	if in.FilePath == "" || in.Content == nil {
		return failf("file_path and content are required")
	}
	abs := ResolvePath(env.Dir, in.FilePath)
	shown := shownPath(env.Dir, abs)
	target, info, err := writeTarget(abs)
	if err != nil {
		return failf("write %s: %v", shown, err)
	}
	old, verb := "", "Created"
	if info != nil {
		verb = "Wrote"
		if info.Size() <= diffReadMost {
			if data, err := os.ReadFile(target); err == nil && utf8.Valid(data) {
				old = string(data)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return failf("write %s: %v", shown, err)
	}
	if err := writeAtomic(target, []byte(*in.Content), info); err != nil {
		return failf("write %s: %v", shown, err)
	}
	added, removed := lineChanges(old, *in.Content)
	return Result{
		Text: fmt.Sprintf("%s %s (%d lines)", verb, shown, countLines(*in.Content)),
		Old:  clip(old, diffClip, "\n…"), New: clip(*in.Content, diffClip, "\n…"),
		Added: added, Removed: removed,
	}
}

// writeTarget is the file a write to abs lands in, through a link rather than over it,
// and what it is now: nil when new.
func writeTarget(abs string) (string, os.FileInfo, error) {
	info, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return abs, nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	target := abs
	if info.Mode()&os.ModeSymlink != 0 {
		if target, err = filepath.EvalSymlinks(abs); err != nil {
			return "", nil, fmt.Errorf("it's a link that leads nowhere: %w", err)
		}
		if info, err = os.Stat(target); err != nil {
			return "", nil, err
		}
	}
	switch {
	case info.IsDir():
		return "", nil, errors.New("it's a folder")
	case !info.Mode().IsRegular():
		return "", nil, errors.New("it isn't a regular file")
	}
	return target, info, nil
}

// writeAtomic puts data in path all at once: a temporary file beside it, renamed over
// it, so a crash or a reader never sees half of it. It keeps the mode of the file it
// replaces (was); a new one gets what the umask leaves.
func writeAtomic(path string, data []byte, was os.FileInfo) (err error) {
	f, err := createTemp(filepath.Dir(path))
	if err != nil {
		return err
	}
	temp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(temp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if was != nil {
		if err = f.Chmod(was.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)); err != nil && !errors.Is(err, errors.ErrUnsupported) {
			return err
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// createTemp makes a new file in dir with the mode the umask leaves, which
// os.CreateTemp's 0600 would hide.
func createTemp(dir string) (*os.File, error) {
	for try := 0; ; try++ {
		name := filepath.Join(dir, fmt.Sprintf(".enowx-write-%016x", rand.Uint64()))
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if err == nil || !errors.Is(err, os.ErrExist) || try == 8 {
			return f, err
		}
	}
}

// lineChanges counts the lines repl adds to old and those it drops, equal lines
// matching whatever their order, as the chat counts other agents' edits.
func lineChanges(old, repl string) (added, removed int) {
	have := map[string]int{}
	if old != "" {
		for _, l := range strings.Split(old, "\n") {
			have[l]++
		}
	}
	if repl != "" {
		for _, l := range strings.Split(repl, "\n") {
			if have[l] > 0 {
				have[l]--
			} else {
				added++
			}
		}
	}
	for _, n := range have {
		removed += n
	}
	return added, removed
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

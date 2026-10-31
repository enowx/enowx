package tokens

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
)

// Env is where agents' records are looked for: the user's home, and the
// folders of the host's projects and sessions (for agents that keep their
// records in the project, as Aider does).
type Env struct {
	Home string
	Dirs []string
}

// FileState is how far a record file was read, and what its earlier lines
// said that later ones don't repeat (the model, the folder, the
// conversation).
type FileState struct {
	Size   int64     `json:"size"`
	Mod    time.Time `json:"mod"`
	Offset int64     `json:"offset,omitempty"`
	Model  string    `json:"model,omitempty"`
	// Provider is the provider the records last named.
	Provider string `json:"provider,omitempty"`
	Dir      string `json:"dir,omitempty"`
	Conv     string `json:"conv,omitempty"`
	// Mark is an adapter's own place: OpenCode's last update read.
	Mark int64 `json:"mark,omitempty"`
}

// adapter reads one agent's records (and those of agents sharing them).
type adapter struct {
	agent string
	files func(env Env) []string
	read  func(path string, st *FileState) ([]Call, error)
}

// Scanner reads the agents' records that changed since its last scan,
// each file from where it left off. Its state is kept in file. Scan runs
// on one goroutine at a time (mu).
type Scanner struct {
	file string

	mu    sync.Mutex
	files map[string]*FileState
}

// NewScanner is a scanner keeping its state in file.
func NewScanner(file string) *Scanner {
	sc := &Scanner{file: file, files: map[string]*FileState{}}
	if data, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(data, &sc.files)
	}
	return sc
}

// Scan reads what changed since the last scan in the records of the
// agents only allows (nil: all), skipping files untouched since since.
// It returns the calls read and how many files it read.
func (sc *Scanner) Scan(env Env, since time.Time, only func(agent string) bool) ([]Call, int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	var out []Call
	read := 0
	for _, a := range adapters {
		if only != nil && !only(a.agent) {
			continue
		}
		for _, path := range a.files(env) {
			size, mod, ok := fileStamp(path)
			if !ok || mod.Before(since) {
				continue
			}
			st := sc.files[path]
			if st != nil && st.Size == size && st.Mod.Equal(mod) {
				continue
			}
			if st == nil || size < st.Offset {
				st = &FileState{}
			}
			calls, err := a.read(path, st)
			if err != nil {
				continue
			}
			st.Size, st.Mod = size, mod
			sc.files[path] = st
			read++
			for _, c := range calls {
				if c.Agent == "" {
					c.Agent = a.agent
				}
				if c.Layer == "" {
					c.Layer = proto.TokenLayerRecords
				}
				if c.Provider == "" {
					c.Provider = ProviderOf(c.Model)
				}
				if c.Time.IsZero() {
					c.Time = mod // a record without its time: when its file last changed
				}
				if !c.Empty() && !judgeDir(c.Dir) {
					out = append(out, c)
				}
			}
		}
	}
	if read > 0 {
		sc.saveLocked()
	}
	return out, read
}

// Reset forgets how far each file was read, so the next scan reads them
// all again (a backfill); the store counts each call once anyway.
func (sc *Scanner) Reset() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.files = map[string]*FileState{}
	sc.saveLocked()
}

func (sc *Scanner) saveLocked() {
	data, err := json.Marshal(sc.files)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(sc.file), 0o700)
	_ = envsync.WriteFileAtomic(sc.file, data, 0o600)
}

// judgeDir reports whether dir is a folder the judge ran an agent in
// (core/decide): its tokens are counted as enowX's own, from the judge.
func judgeDir(dir string) bool {
	return dir != "" && strings.HasPrefix(filepath.Base(dir), "enowx-judge-")
}

// fileStamp is a file's size and last change, its SQLite write-ahead log
// counted with it.
func fileStamp(path string) (int64, time.Time, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0, time.Time{}, false
	}
	size, mod := info.Size(), info.ModTime()
	if wal, err := os.Stat(path + "-wal"); err == nil {
		size += wal.Size()
		if wal.ModTime().After(mod) {
			mod = wal.ModTime()
		}
	}
	return size, mod, true
}

// readLines calls fn with each whole line of path from st.Offset on, and
// moves st.Offset past them; a line still being written waits for the
// next read.
func readLines(path string, st *FileState, fn func(line []byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(st.Offset, io.SeekStart); err != nil {
		return err
	}
	br := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := br.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return nil // a partial line: read again next time
		}
		if err != nil {
			return err
		}
		st.Offset += int64(len(line))
		if line = bytes.TrimSpace(line); len(line) > 0 {
			fn(line)
		}
	}
}

// walk lists the files under root whose name ends in ext.
func walk(root, ext string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && filepath.Ext(d.Name()) == ext {
			out = append(out, path)
		}
		return nil
	})
	return out
}

package goa

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/proto"
)

// boardKept is how many tasks the board keeps, the oldest going first.
const boardKept = 500

// Board is GOA's task board, kept in a file, which it reads once and
// keeps: the daemon is its one writer. Safe from any goroutine; mu guards
// tasks.
type Board struct {
	path   string
	mu     sync.Mutex
	loaded bool
	tasks  []proto.GOATask
}

// OpenBoard is the board kept at path.
func OpenBoard(path string) *Board { return &Board{path: path} }

// read is a copy of the tasks, read from the file the first time.
func (b *Board) read() []proto.GOATask {
	if !b.loaded {
		b.loaded = true
		if data, err := os.ReadFile(b.path); err == nil {
			_ = json.Unmarshal(data, &b.tasks)
		}
	}
	return slices.Clone(b.tasks)
}

func (b *Board) write(tasks []proto.GOATask) error {
	if len(tasks) > boardKept {
		tasks = slices.Clone(tasks[len(tasks)-boardKept:])
	}
	b.tasks = tasks
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o700); err != nil {
		return fmt.Errorf("goa: board: %w", err)
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("goa: board: %w", err)
	}
	return os.Rename(tmp, b.path)
}

// List is every task, the oldest first.
func (b *Board) List() []proto.GOATask {
	b.mu.Lock()
	defer b.mu.Unlock()
	tasks := b.read()
	if tasks == nil {
		tasks = []proto.GOATask{}
	}
	return tasks
}

// Add puts t on the board, working, with an ID of its own.
func (b *Board) Add(t proto.GOATask) (proto.GOATask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := make([]byte, 4)
	_, _ = rand.Read(id)
	t.ID = "t" + hex.EncodeToString(id)
	t.Created, t.Updated = time.Now(), time.Now()
	if t.Status == "" {
		t.Status = proto.GOATaskWorking
	}
	if t.Title == "" {
		t.Title = TitleOf(t.Prompt)
	}
	return t, b.write(append(b.read(), t))
}

// Update changes task id (or the start of one) with change, and returns
// it.
func (b *Board) Update(id string, change func(*proto.GOATask)) (proto.GOATask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	tasks := b.read()
	i, err := find(tasks, id)
	if err != nil {
		return proto.GOATask{}, err
	}
	change(&tasks[i])
	tasks[i].Updated = time.Now()
	return tasks[i], b.write(tasks)
}

// Of updates the unsettled tasks of session with change, and returns
// those it changed. Session IDs are unique across hosts.
func (b *Board) Of(session string, change func(*proto.GOATask) bool) []proto.GOATask {
	b.mu.Lock()
	defer b.mu.Unlock()
	tasks := b.read()
	var out []proto.GOATask
	for i := range tasks {
		t := &tasks[i]
		if session == "" || t.Session != session || Settled(t.Status) {
			continue
		}
		if change(t) {
			t.Updated = time.Now()
			out = append(out, *t)
		}
	}
	if len(out) > 0 {
		_ = b.write(tasks)
	}
	return out
}

// Has says whether session has a task not settled.
func (b *Board) Has(session string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.read()
	return slices.ContainsFunc(b.tasks, func(t proto.GOATask) bool {
		return session != "" && t.Session == session && !Settled(t.Status)
	})
}

// Working are the tasks not settled yet.
func (b *Board) Working() []proto.GOATask {
	return slices.DeleteFunc(b.List(), func(t proto.GOATask) bool { return Settled(t.Status) })
}

// Settled says whether a task in status is over.
func Settled(status string) bool {
	return status == proto.GOATaskDone || status == proto.GOATaskFailed || status == proto.GOATaskStopped
}

// WorkerStatus is a task's status as its session info says, "" for no
// change (a hibernated session comes back).
func WorkerStatus(info proto.SessionInfo) string {
	switch {
	case info.Exited && info.Hibernated:
		return ""
	case info.Exited && info.ExitCode != 0:
		return proto.GOATaskFailed
	case info.Exited:
		return proto.GOATaskStopped
	case info.State == proto.StateWaiting:
		return proto.GOATaskWaiting
	case info.State == proto.StateDone:
		return proto.GOATaskTurnEnded // over or mid-work: GOA's to say
	case info.State == proto.StateWorking:
		return proto.GOATaskWorking
	}
	return ""
}

func find(tasks []proto.GOATask, id string) (int, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return -1, errors.New("goa: which task?")
	}
	found := -1
	for i, t := range tasks {
		if t.ID == id {
			return i, nil
		}
		if strings.HasPrefix(t.ID, id) {
			if found >= 0 {
				return -1, fmt.Errorf("goa: %q names several tasks", id)
			}
			found = i
		}
	}
	if found < 0 {
		return -1, fmt.Errorf("goa: no task %q on the board", id)
	}
	return found, nil
}

// TitleOf is a task's title from its prompt: its first line, short.
func TitleOf(prompt string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(prompt), "\n")
	if r := []rune(line); len(r) > 72 {
		line = string(r[:71]) + "…"
	}
	return line
}

// Later is the later of two times.
func Later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

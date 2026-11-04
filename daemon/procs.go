package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/enowx/enowx/core/procstat"
)

// What each session started, followed from outside. A hang-up reaches a session's
// program and its foreground, and the environment tag reap goes by can't be read on
// recent macOS, so an agent's servers and helpers in groups of their own outlived it,
// with nothing left to stop them. The daemon writes down each session's process tree
// as it goes: once a parent has ended, its children can't be found any other way. Kept
// in a file, as what a crash left is found at the next start. Only what is on this
// list is ever stopped, each by its PID and start together, so never a process that
// was given a PID freed meanwhile, nor one no session started.

const (
	procsEvery = 15 * time.Second
	// procsHangup is how long a hung-up program has to end by itself and take its
	// children along, as the terminal gives it.
	procsHangup = 2 * time.Second
	// reapGrace is how long processes get to exit once asked.
	reapGrace = 3 * time.Second
)

// startedProc is a process a session started.
type startedProc struct {
	PID     int    `json:"pid"`
	Start   string `json:"start"`
	Name    string `json:"name,omitempty"`
	Session string `json:"session"`
	// Left says it is no longer under its session's program.
	Left bool `json:"left,omitempty"`
}

// procTracker is that list. path "" follows nothing, as in a server under test.
type procTracker struct {
	mu    sync.Mutex
	path  string
	known map[int]startedProc
	// agentSeen is when each background agent was first seen: backgroundLocked.
	agentSeen map[startedProc]time.Time
}

func (t *procTracker) load(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.path, t.known = path, map[int]startedProc{}
	var list []startedProc
	if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &list) == nil {
		for _, p := range list {
			t.known[p.PID] = p
		}
	}
}

// saveLocked keeps the list. The caller holds t.mu.
func (t *procTracker) saveLocked() error {
	list := make([]startedProc, 0, len(t.known))
	for _, p := range t.known {
		list = append(list, p)
	}
	slices.SortFunc(list, func(x, y startedProc) int { return x.PID - y.PID })
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return writeFileAtomic(t.path, data)
}

// mergeTrees is known updated by a look at the processes: what each session's root
// (rootOf, "" for none) has under it now goes on, what no longer runs goes off, and
// what left its tree stays, marked. Neither this program nor what started it (spare,
// should a session have started the daemon) ever counts.
func mergeTrees(known map[int]startedProc, list []procstat.Proc, self int, rootOf func(pid int) string, spare map[int]bool) (map[int]startedProc, bool) {
	byPID := make(map[int]procstat.Proc, len(list))
	kids := make(map[int][]int, len(list))
	for _, p := range list {
		byPID[p.PID] = p
		kids[p.PPID] = append(kids[p.PPID], p.PID)
	}
	out := make(map[int]startedProc, len(known))
	for pid, was := range known {
		if p, ok := byPID[pid]; ok && p.Start == was.Start {
			was.Left = true // until found under its session, next
			out[pid] = was
		}
	}
	for _, root := range kids[self] {
		id := rootOf(root)
		if id == "" {
			continue
		}
		queue := []int{root}
		for len(queue) > 0 {
			pid := queue[0]
			queue = queue[1:]
			if pid == self || spare[pid] {
				continue
			}
			p := byPID[pid]
			out[pid] = startedProc{PID: pid, Start: p.Start, Name: p.Name, Session: id}
			queue = append(queue, kids[pid]...)
		}
	}
	changed := len(out) != len(known)
	for pid, p := range out {
		if known[pid] != p {
			changed = true
		}
	}
	return out, changed
}

// ancestors are the processes above pid.
func ancestors(list []procstat.Proc, pid int) map[int]bool {
	parent := make(map[int]int, len(list))
	for _, p := range list {
		parent[p.PID] = p.PPID
	}
	out := map[int]bool{}
	for at := parent[pid]; at > 1 && !out[at]; at = parent[at] {
		out[at] = true
	}
	return out
}

// procsLook updates the list from a look at the processes now.
func (s *Server) procsLook() {
	t := &s.procs
	t.mu.Lock()
	followed := t.path != ""
	t.mu.Unlock()
	if !followed {
		return
	}
	list, err := procstat.Snapshot()
	if err != nil {
		return
	}
	self := os.Getpid()
	spare := ancestors(list, self)
	roots := sessionOf // by its name where it runs under one, else by its environment
	t.mu.Lock()
	defer t.mu.Unlock()
	was := t.known
	known, changed := mergeTrees(t.known, list, self, roots, spare)
	if adoptDetached(known, list, procstat.Env, s.isSession) {
		changed = true
	}
	t.known = known
	found := t.backgroundLocked(list, was, procstat.Args, time.Now())
	if changed {
		if err := t.saveLocked(); err != nil {
			s.logger().Warn("procs: keep what sessions started", "err", err)
		}
	}
	s.noteBackground(found)
}

// watchProcs follows what sessions start until done closes.
func (s *Server) watchProcs(done <-chan struct{}) {
	tick := time.NewTicker(procsEvery)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
			s.procsLook()
		}
	}
}

// procsTake takes off the list what match says, and gives it.
func (t *procTracker) procsTake(match func(startedProc) bool) []startedProc {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []startedProc
	for pid, p := range t.known {
		if match(p) {
			out = append(out, p)
			delete(t.known, pid)
		}
	}
	if len(out) > 0 && t.path != "" {
		_ = t.saveLocked()
	}
	return out
}

// stillRuns are those of procs running now, each the same process it was.
func stillRuns(procs []startedProc) []startedProc {
	list, err := procstat.Snapshot()
	if err != nil {
		return nil
	}
	starts := make(map[int]string, len(list))
	for _, p := range list {
		starts[p.PID] = p.Start
	}
	return slices.DeleteFunc(slices.Clone(procs), func(p startedProc) bool { return starts[p.PID] != p.Start })
}

// endProcs stops procs: asked first, then forced after reapGrace if they ignore it.
func (s *Server) endProcs(procs []startedProc) {
	left := stillRuns(procs)
	for _, p := range left {
		stopProcess(p.PID, false)
	}
	if len(left) == 0 {
		return
	}
	time.Sleep(reapGrace)
	for _, p := range stillRuns(left) {
		s.logger().Info("procs: forced to end", "pid", p.PID, "name", p.Name, "session", p.Session)
		stopProcess(p.PID, true)
	}
}

// endSessionProcs ends all session id started, on a goroutine of its own, once what
// was hung up has had its moment to end by itself.
func (s *Server) endSessionProcs(id string) {
	procs := s.procs.procsTake(func(p startedProc) bool { return p.Session == id })
	if len(procs) == 0 {
		return
	}
	go func() {
		time.Sleep(procsHangup)
		s.endProcs(procs)
	}()
}

// endAllProcs ends what sessions started, returning once it is gone: the daemon is
// about to exit, its sessions come back with the next, and nothing of theirs should
// run twice.
func (s *Server) endAllProcs() {
	s.endProcs(s.procs.procsTake(func(startedProc) bool { return true }))
}

// endProcsLeftBefore ends what a daemon that ended without seeing to it left behind,
// at the start of this one, whose sessions start theirs again.
func (s *Server) endProcsLeftBefore() {
	procs := s.procs.procsTake(func(startedProc) bool { return true })
	if left := stillRuns(procs); len(left) > 0 {
		s.logger().Info("procs: ending what the last daemon left running", "processes", len(left))
		s.endProcs(left)
	}
}

func procsFile(dir string) string { return filepath.Join(dir, "session-procs.json") }

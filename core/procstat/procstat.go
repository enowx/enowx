// Package procstat measures memory and CPU: of a process with all its
// descendants, and of the whole machine. The app shows it so people see
// what their agents cost.
package procstat

import (
	"os"
	"runtime"
	"sync"
	"time"
)

// Proc is a process as the system reports it.
type Proc struct {
	PID, PPID int
	RSS       uint64        // resident memory, bytes
	CPUTime   time.Duration // user and system time used so far
	Name      string        // its program's name
	Start     string        // when it started: with PID it names this process, as a PID is used again
}

// Snapshot lists the processes now, an error where the system can't say.
func Snapshot() ([]Proc, error) { return processes() }

// Usage is what a process tree uses.
type Usage struct {
	RSS   uint64  // bytes
	CPU   float64 // percent of one core, since the previous sample
	Procs int
}

// Host is the machine's memory and CPU.
type Host struct {
	CPUs     int
	MemTotal uint64
	MemUsed  uint64
	CPU      float64 // percent of all cores, since the previous sample
}

// Sampler turns successive process lists into CPU percentages. It is safe
// for concurrent use.
type Sampler struct {
	mu    sync.Mutex
	prev  map[int]time.Duration
	at    time.Time
	procs map[int]Proc
	kids  map[int][]int
	cpu   map[int]float64
	total float64
}

// Sample reads every process. CPU figures cover the time since the
// previous call, so the first call reports none.
func (s *Sampler) Sample() error {
	list, err := processes()
	if err != nil {
		return err
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	wall := now.Sub(s.at)
	s.procs, s.kids, s.cpu, s.total = map[int]Proc{}, map[int][]int{}, map[int]float64{}, 0
	next := make(map[int]time.Duration, len(list))
	for _, p := range list {
		s.procs[p.PID] = p
		s.kids[p.PPID] = append(s.kids[p.PPID], p.PID)
		next[p.PID] = p.CPUTime
		if before, ok := s.prev[p.PID]; ok && wall > 0 && p.CPUTime >= before {
			pct := 100 * float64(p.CPUTime-before) / float64(wall)
			s.cpu[p.PID] = pct
			s.total += pct
		}
	}
	s.prev, s.at = next, now
	return nil
}

// Tree sums what pid and its descendants use, as of the last sample.
func (s *Sampler) Tree(pid int) Usage {
	s.mu.Lock()
	defer s.mu.Unlock()
	var u Usage
	var walk func(int)
	walk = func(p int) {
		if proc, ok := s.procs[p]; ok {
			u.RSS += proc.RSS
			u.CPU += s.cpu[p]
			u.Procs++
		}
		for _, k := range s.kids[p] {
			if k != p {
				walk(k)
			}
		}
	}
	walk(pid)
	return u
}

// One is what pid alone uses, as of the last sample.
func (s *Sampler) One(pid int) Usage {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.procs[pid]
	if !ok {
		return Usage{}
	}
	return Usage{RSS: p.RSS, CPU: s.cpu[pid], Procs: 1}
}

// Descendants returns pid and every process under it, as of the last
// sample.
func (s *Sampler) Descendants(pid int) []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []int{pid}
	for i := 0; i < len(out); i++ {
		for _, k := range s.kids[out[i]] {
			if k != out[i] {
				out = append(out, k)
			}
		}
	}
	return out
}

// Children returns the direct children of pid, as of the last sample.
func (s *Sampler) Children(pid int) []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.kids[pid]...)
}

// Host returns the machine's memory and CPU use.
func (s *Sampler) Host() Host {
	h := Host{CPUs: runtime.NumCPU()}
	h.MemTotal, h.MemUsed = memory()
	s.mu.Lock()
	h.CPU = s.total / float64(h.CPUs)
	s.mu.Unlock()
	return h
}

// Tagged lists the processes, other than this one, whose environment holds
// key=value: those a session or a display started, even ones that left
// its process tree (tmux, nohup, setsid). Only processes whose environment
// this one may read count, which are its user's; on systems that hide it
// (recent macOS), none.
func Tagged(key, value string) []int {
	procs, err := processes()
	if err != nil {
		return nil
	}
	self := os.Getpid()
	var out []int
	for _, p := range procs {
		if p.PID != self && p.PID > 1 && Env(p.PID, key) == value {
			out = append(out, p.PID)
		}
	}
	return out
}

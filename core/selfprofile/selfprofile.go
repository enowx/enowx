// Package selfprofile measures the process it runs in, changing nothing: what the system
// says it holds, what the Go runtime holds, which code holds the heap and runs the
// goroutines. `enowx-app profile` and diagnostics bundles show it.
package selfprofile

import (
	"bytes"
	"runtime"
	"runtime/metrics"
	"runtime/pprof"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// topCosts is how many lines of each cost a profile keeps.
const topCosts = 25

var started = time.Now()

// Options says what to measure of this process.
type Options struct {
	Kind, Version string
	Params        proto.ProfileParams
	Areas         func() []proto.ProfileArea // what its parts keep; may be nil
}

// Take measures this process. With Params.CPUSeconds it waits that long.
func Take(o Options) proto.ProcessProfile {
	out := proto.ProcessProfile{
		Kind: o.Kind, Version: o.Version, OS: runtime.GOOS, Arch: runtime.GOARCH,
		PID: pid(), UptimeSeconds: int64(time.Since(started).Seconds()),
	}
	window := min(max(o.Params.CPUSeconds, 0), 60)
	var cpu bytes.Buffer
	before := cpuTime()
	began := time.Now()
	if window > 0 {
		capturing := o.Params.Raw && pprof.StartCPUProfile(&cpu) == nil
		time.Sleep(time.Duration(window) * time.Second)
		if capturing {
			pprof.StopCPUProfile()
		}
	}
	used := cpuTime()
	out.CPUSeconds = used.Seconds()
	if window > 0 {
		out.CPUWindow = time.Since(began).Seconds()
		out.CPUPercent = 100 * (used - before).Seconds() / out.CPUWindow
	}
	out.Memory = pages()
	out.Go = goMemory()
	out.HeapBy = heapCosts()
	out.GoroutinesBy = goroutineCosts()
	if o.Areas != nil {
		out.Areas = o.Areas()
	}
	if o.Params.Raw {
		out.Raw = raw(cpu.Bytes())
	}
	return out
}

func goMemory() proto.ProfileGo {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	settings := []metrics.Sample{{Name: "/gc/gogc:percent"}, {Name: "/gc/gomemlimit:bytes"}}
	metrics.Read(settings) // unlike debug.SetGCPercent, reading it doesn't stop the program
	out := proto.ProfileGo{
		HeapAlloc: m.HeapAlloc, HeapInuse: m.HeapInuse, HeapIdle: m.HeapIdle,
		HeapReleased: m.HeapReleased, HeapSys: m.HeapSys, NextGC: m.NextGC,
		StackInuse: m.StackInuse, Sys: m.Sys, Objects: m.HeapObjects, NumGC: m.NumGC,
		PauseMillis: m.PauseTotalNs / uint64(time.Millisecond), GCPercent: int(settings[0].Value.Uint64()),
		Goroutines: runtime.NumGoroutine(), GCCPUPercent: 100 * m.GCCPUFraction,
	}
	if p := pprof.Lookup("threadcreate"); p != nil {
		out.Threads = p.Count()
	}
	if limit := settings[1].Value.Uint64(); limit != 1<<63-1 { // MaxInt64 is "none"
		out.MemoryLimit = int64(limit)
	}
	return out
}

// heapCosts sums the heap's live sampled allocations by the code of ours that made them.
// The runtime samples them from the process's start, about one per 512 KiB.
func heapCosts() []proto.ProfileCost {
	records := make([]runtime.MemProfileRecord, 4096)
	n, ok := runtime.MemProfile(records, false)
	if !ok {
		records = make([]runtime.MemProfileRecord, n+256)
		n, _ = runtime.MemProfile(records, false)
	}
	rate := uint64(max(runtime.MemProfileRate, 1))
	sums := map[string]*proto.ProfileCost{}
	for _, r := range records[:min(n, len(records))] {
		live := r.InUseBytes()
		if live <= 0 {
			continue
		}
		objects := r.InUseObjects()
		size := uint64(live) / uint64(objects)
		// The same scaling pprof applies: a small object is less likely to be sampled.
		scale := 1.0
		if size < rate {
			scale = 1 / (1 - expNeg(float64(size)/float64(rate)))
		}
		name := owner(r.Stack())
		c := sums[name]
		if c == nil {
			c = &proto.ProfileCost{Name: name}
			sums[name] = c
		}
		c.Bytes += uint64(float64(live) * scale)
		c.Count += int64(float64(objects) * scale)
	}
	return sorted(sums, func(c *proto.ProfileCost) uint64 { return c.Bytes })
}

// expNeg is e^-x, accurate enough for scaling a sample.
func expNeg(x float64) float64 {
	sum, term := 1.0, 1.0
	for i := 1; i < 16; i++ {
		term *= -x / float64(i)
		sum += term
	}
	return sum
}

func goroutineCosts() []proto.ProfileCost {
	records := make([]runtime.StackRecord, runtime.NumGoroutine()+64)
	n, ok := runtime.GoroutineProfile(records)
	if !ok {
		return nil
	}
	sums := map[string]*proto.ProfileCost{}
	for _, r := range records[:n] {
		name := owner(r.Stack())
		c := sums[name]
		if c == nil {
			c = &proto.ProfileCost{Name: name}
			sums[name] = c
		}
		c.Count++
	}
	return sorted(sums, func(c *proto.ProfileCost) uint64 { return uint64(c.Count) })
}

func sorted(sums map[string]*proto.ProfileCost, by func(*proto.ProfileCost) uint64) []proto.ProfileCost {
	out := make([]proto.ProfileCost, 0, len(sums))
	for _, c := range sums {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b proto.ProfileCost) int {
		if x, y := by(&a), by(&b); x != y {
			if x > y {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out[:min(len(out), topCosts)]
}

// owner names the code a stack belongs to: its innermost function of this project,
// which says more than the runtime's or a library's function all of them end in;
// else its innermost function outside the runtime.
func owner(stack []uintptr) string {
	frames := runtime.CallersFrames(stack)
	first := ""
	for {
		f, more := frames.Next()
		if name, ok := strings.CutPrefix(f.Function, "github.com/enowx/enowx/"); ok {
			return name
		}
		if first == "" && f.Function != "" && !strings.HasPrefix(f.Function, "runtime.") {
			first = f.Function
		}
		if !more {
			break
		}
	}
	if first == "" {
		return "runtime"
	}
	return first
}

func raw(cpu []byte) *proto.ProfileRaw {
	lookup := func(name string) []byte {
		var b bytes.Buffer
		if p := pprof.Lookup(name); p != nil {
			_ = p.WriteTo(&b, 0)
		}
		return b.Bytes()
	}
	return &proto.ProfileRaw{Heap: lookup("heap"), Goroutines: lookup("goroutine"), CPU: cpu}
}

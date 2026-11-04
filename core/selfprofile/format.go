package selfprofile

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// Write prints a profile for people.
func Write(w io.Writer, p proto.Profile) {
	WriteProcess(w, p.Daemon)
	if p.App != nil {
		fmt.Fprintln(w)
		WriteProcess(w, *p.App)
	} else if p.AppError != "" {
		fmt.Fprintf(w, "\napp: not measured: %s\n", p.AppError)
	}
	if len(p.Programs) > 0 {
		fmt.Fprintln(w, "\nprograms sessions run (RSS, counts clean code pages and shared memory more than once):")
		for _, g := range p.Programs {
			fmt.Fprintf(w, "  %9s  %3dx  %s\n", mb(g.RSS), g.Count, g.Name)
		}
	}
}

// WriteProcess prints one process's profile.
func WriteProcess(w io.Writer, p proto.ProcessProfile) {
	fmt.Fprintf(w, "%s %s (%s/%s) pid %d, up %s\n", p.Kind, p.Version, p.OS, p.Arch, p.PID, (time.Duration(p.UptimeSeconds) * time.Second).String())
	fmt.Fprintf(w, "cpu: %.1f s in all", p.CPUSeconds)
	if p.CPUWindow > 0 {
		fmt.Fprintf(w, ", %.1f%% of a core over the last %.0f s", p.CPUPercent, p.CPUWindow)
	}
	fmt.Fprintln(w)
	m := p.Memory
	fmt.Fprintf(w, "memory: rss %s", mb(m.RSS))
	if m.Footprint > 0 {
		fmt.Fprintf(w, ", footprint %s (peak %s)", mb(m.Footprint), mb(m.FootprintPeak))
	}
	if m.PSS > 0 {
		fmt.Fprintf(w, ", pss %s, private dirty %s, private clean %s, shared clean %s, anon %s, swap %s, peak rss %s",
			mb(m.PSS), mb(m.PrivateDirty), mb(m.PrivateClean), mb(m.SharedClean), mb(m.Anon), mb(m.Swap), mb(m.PeakRSS))
	}
	fmt.Fprintln(w)
	if m.Note != "" {
		fmt.Fprintf(w, "  (%s)\n", m.Note)
	}
	g := p.Go
	held := g.Sys - min(g.Sys, g.HeapReleased)
	fmt.Fprintf(w, "go: heap live %s, in spans %s, idle %s, returned %s, next gc %s; stacks %s; held from the system %s, address space ever %s\n",
		mb(g.HeapAlloc), mb(g.HeapInuse), mb(g.HeapIdle-min(g.HeapIdle, g.HeapReleased)), mb(g.HeapReleased), mb(g.NextGC), mb(g.StackInuse), mb(held), mb(g.Sys))
	if m.Anon > 0 {
		fmt.Fprintf(w, "    outside go: about %s of anonymous memory (native libraries)\n", mb(m.Anon-min(m.Anon, held)))
	}
	limit := "none"
	if g.MemoryLimit > 0 {
		limit = mb(uint64(g.MemoryLimit))
	}
	fmt.Fprintf(w, "    %d objects, %d goroutines, %d threads, %d GCs, %d ms paused, %.2f%% cpu in gc, GOGC %d, memory limit %s\n",
		g.Objects, g.Goroutines, g.Threads, g.NumGC, g.PauseMillis, g.GCCPUPercent, g.GCPercent, limit)
	if len(m.Regions) > 0 {
		fmt.Fprintln(w, "mappings (rss, private dirty):")
		for _, r := range m.Regions {
			fmt.Fprintf(w, "  %9s %9s  %s\n", mb(r.RSS), mb(r.PrivateDirty), r.Name)
		}
	}
	if len(p.Areas) > 0 {
		fmt.Fprintln(w, "kept:")
		for _, a := range p.Areas {
			size := ""
			if a.Bytes > 0 {
				size = mb(a.Bytes)
			}
			fmt.Fprintf(w, "  %9s %5d  %s\n", size, a.Count, a.Name)
		}
	}
	if len(p.HeapBy) > 0 {
		fmt.Fprintln(w, "heap by code (sampled, after the last gc):")
		for _, c := range p.HeapBy {
			fmt.Fprintf(w, "  %9s %9d  %s\n", mb(c.Bytes), c.Count, c.Name)
		}
	}
	if len(p.AllocBy) > 0 {
		fmt.Fprintln(w, "allocated by code since it started (sampled), what makes it collect:")
		for _, c := range p.AllocBy[:min(len(p.AllocBy), 15)] {
			fmt.Fprintf(w, "  %9s %9d  %s\n", mb(c.Bytes), c.Count, c.Name)
		}
	}
	if len(p.GoroutinesBy) > 0 {
		fmt.Fprintln(w, "goroutines by code:")
		for _, c := range p.GoroutinesBy[:min(len(p.GoroutinesBy), 12)] {
			fmt.Fprintf(w, "  %5d  %s\n", c.Count, c.Name)
		}
	}
}

// Text is a profile as Write prints it.
func Text(p proto.Profile) string {
	var b strings.Builder
	Write(&b, p)
	return b.String()
}

// TextOf is one process's profile as WriteProcess prints it.
func TextOf(p proto.ProcessProfile) string {
	var b strings.Builder
	WriteProcess(&b, p)
	return b.String()
}

func mb(n uint64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }

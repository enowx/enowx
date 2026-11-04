package selfprofile

import (
	"runtime"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

var keep [][]byte

// The profile says what the process holds, and which code of ours holds it.
func TestTakeNamesTheCodeHoldingTheHeap(t *testing.T) {
	for range 64 {
		keep = append(keep, make([]byte, 1<<20))
	}
	runtime.GC()
	runtime.GC()
	p := Take(Options{Kind: "test", Version: "1", Areas: func() []proto.ProfileArea {
		return []proto.ProfileArea{{Name: "pages", Count: 3}}
	}})
	if p.Go.HeapAlloc < 64<<20 || p.Go.Goroutines < 1 || p.Go.GCPercent == 0 {
		t.Fatalf("%+v", p.Go)
	}
	if runtime.GOOS == "linux" && (p.Memory.RSS == 0 || p.Memory.PSS == 0 || p.Memory.Anon == 0) {
		t.Fatalf("%+v", p.Memory)
	}
	found := false
	for _, c := range p.HeapBy {
		if strings.Contains(c.Name, "core/selfprofile.TestTakeNamesTheCodeHoldingTheHeap") && c.Bytes >= 32<<20 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no line for this test's 64 MiB in %+v", p.HeapBy)
	}
	text := TextOf(p)
	for _, want := range []string{"heap live", "pages", "selfprofile.Test"} {
		if !strings.Contains(text, want) {
			t.Fatalf("no %q in:\n%s", want, text)
		}
	}
}

func TestCPUWindowCountsWork(t *testing.T) {
	done := make(chan struct{})
	go func() {
		// Work for the window to count: spins on purpose, until told to stop.
		for n := 0; ; n++ {
			if n%1024 == 0 {
				select {
				case <-done:
					return
				default:
				}
			}
		}
	}()
	defer close(done)
	p := Take(Options{Params: proto.ProfileParams{CPUSeconds: 1, Raw: true}})
	if p.CPUPercent < 20 || p.CPUWindow < 0.9 {
		t.Fatalf("cpu %v%% over %v s", p.CPUPercent, p.CPUWindow)
	}
	if p.Raw == nil || len(p.Raw.Heap) == 0 || len(p.Raw.Goroutines) == 0 || len(p.Raw.CPU) == 0 {
		t.Fatal("raw profiles missing")
	}
}

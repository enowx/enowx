package watchdog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A thread that stops answering leaves a report with the stacks and what it showed.
func TestStallLeavesAReport(t *testing.T) {
	dir := t.TempDir()
	calls := make(chan func(), 1)
	block := make(chan struct{})
	go func() {
		for fn := range calls {
			fn()
			<-block // the thread is stuck from its first answer on
		}
	}()
	w := Start(dir, "main", func(fn func()) { calls <- fn }, func() string { return "the Notes page" })
	defer w.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		files, _ := filepath.Glob(filepath.Join(dir, "main-stall-*.txt"))
		if len(files) > 0 {
			data, _ := os.ReadFile(files[0])
			for _, want := range []string{"the Notes page", "goroutine ", "heap="} {
				if !strings.Contains(string(data), want) {
					t.Errorf("no %q in\n%s", want, data)
				}
			}
			close(block)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	close(block)
	t.Fatal("no report")
}

// Only the newest reports stay, and the log isn't one of them.
func TestTrimReports(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"main-lag-20260101-000000.txt", "main-stall-20260103-000000.txt", "daemon-lag-20260102-000000.txt", "main.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	trimReports(dir, 2)
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	got := strings.Join(names, " ")
	if strings.Contains(got, "20260101") || !strings.Contains(got, "main.log") || len(names) != 3 {
		t.Fatalf("left: %s", got)
	}
}

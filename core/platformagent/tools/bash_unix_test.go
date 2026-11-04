//go:build !windows

package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// gone waits for pid to end; a zombie counts, as nothing runs in it.
func gone(pid int) bool {
	for range 100 {
		if err := syscall.Kill(pid, 0); err != nil {
			return true
		}
		out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		if err != nil || strings.HasPrefix(strings.TrimSpace(string(out)), "Z") {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// progressPIDs reads the process IDs a command prints, once it has printed n.
type progressPIDs struct {
	mu   sync.Mutex
	text string
}

func (p *progressPIDs) add(s string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.text += s
}

func (p *progressPIDs) wait(t *testing.T, n int) []int {
	t.Helper()
	for range 200 {
		p.mu.Lock()
		fields := strings.Fields(p.text)
		p.mu.Unlock()
		if len(fields) >= n {
			pids := make([]int, n)
			for i := range n {
				pid, err := strconv.Atoi(fields[i])
				if err != nil {
					t.Fatal(err)
				}
				pids[i] = pid
			}
			return pids
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("no %d PIDs in %q", n, p.text)
	return nil
}

func TestBashOutputExitAndDir(t *testing.T) {
	dir := t.TempDir()
	var live strings.Builder
	input, _ := json.Marshal(map[string]any{"command": `pwd; echo out; echo err >&2; echo more; exit 3`})
	r := bashTool{}.Run(context.Background(), input, Env{Dir: dir, Environ: os.Environ(), Progress: func(s string) { live.WriteString(s) }})
	if !r.Failed || !strings.Contains(r.Text, "out\nerr\nmore\n") || !strings.Contains(r.Text, "exit code 3") {
		t.Fatalf("%+v", r)
	}
	if !strings.HasPrefix(live.String(), resolved(t, dir)) && !strings.HasPrefix(live.String(), dir) {
		t.Fatalf("ran elsewhere, or no progress: %q", live.String())
	}
	if r = call(t, bashTool{}, dir, map[string]any{"command": "true"}); r.Failed || r.Text != "(no output)" {
		t.Fatalf("no output: %+v", r)
	}
}

func resolved(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("sh", "-c", "cd "+dir+" && pwd -P").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestBashKeepsTheTail(t *testing.T) {
	r := call(t, bashTool{}, t.TempDir(), map[string]any{"command": `head -c 200000 /dev/zero | tr '\0' 'x'; echo; echo THE-END`})
	if r.Failed || !strings.HasSuffix(r.Text, "THE-END\n") || !strings.Contains(r.Text, "bytes of earlier output cut") || len(r.Text) > bashOutputMost+100 {
		t.Fatalf("failed=%v, %d bytes", r.Failed, len(r.Text))
	}
}

// Cancelling kills the shell and what it started, a background child and a child in a
// subshell alike, with nothing left behind.
func TestBashCancelKillsTheGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var pids progressPIDs
	input, _ := json.Marshal(map[string]any{"command": `sleep 300 & echo $!; (sleep 301 & echo $!; wait) & echo $$; wait`})
	done := make(chan Result, 1)
	start := time.Now()
	go func() {
		done <- bashTool{}.Run(ctx, input, Env{Dir: t.TempDir(), Environ: os.Environ(), Progress: pids.add})
	}()
	found := pids.wait(t, 3)
	cancel()
	select {
	case r := <-done:
		if !r.Failed || !strings.Contains(r.Text, "interrupted") {
			t.Fatalf("%+v", r)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelling didn't end the call")
	}
	if took := time.Since(start); took > 8*time.Second {
		t.Fatalf("took %v", took)
	}
	for _, pid := range found {
		if !gone(pid) {
			t.Errorf("process %d outlived the call", pid)
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

func TestBashTimeoutKillsTheGroup(t *testing.T) {
	var pids progressPIDs
	input, _ := json.Marshal(map[string]any{"command": `sleep 300 & echo $!; wait`, "timeout": 300})
	start := time.Now()
	r := bashTool{}.Run(context.Background(), input, Env{Dir: t.TempDir(), Environ: os.Environ(), Progress: pids.add})
	if !r.Failed || !strings.Contains(r.Text, "timed out") {
		t.Fatalf("%+v", r)
	}
	if took := time.Since(start); took > 8*time.Second {
		t.Fatalf("took %v", took)
	}
	for _, pid := range pids.wait(t, 1) {
		if !gone(pid) {
			t.Errorf("process %d outlived the timeout", pid)
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

// A dev server left in the background ends the call, not itself: it keeps running
// through writes to the pipe the call no longer reads.
func TestBashLeavesWhatItBackgrounded(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	r := call(t, bashTool{}, dir, map[string]any{"command": `(while :; do echo tick; sleep 0.05; done) & echo $!`})
	if time.Since(start) > 5*time.Second || r.Failed || !strings.Contains(r.Text, "background") {
		t.Fatalf("%+v", r)
	}
	pid, err := strconv.Atoi(strings.Fields(r.Text)[0])
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	time.Sleep(300 * time.Millisecond)
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil || strings.HasPrefix(strings.TrimSpace(string(out)), "Z") {
		t.Fatalf("it died of its next write: %q, %v", out, err)
	}
}

func TestBashEnvironIsUsedAsIs(t *testing.T) {
	const fake = "not-a-real-key-123"
	environ := CleanEnviron(append(os.Environ(), "ANTHROPIC_API_KEY="+fake, "ANTHROPIC_AUTH_TOKEN="+fake, "CLAUDE_CODE_OAUTH_TOKEN="+fake, "ENOWX_BIN=/opt/enowx"))
	input, _ := json.Marshal(map[string]any{"command": "env"})
	r := bashTool{}.Run(context.Background(), input, Env{Dir: t.TempDir(), Environ: environ})
	if strings.Contains(r.Text, fake) {
		t.Fatal("a Bash call's env shows the key")
	}
	if !strings.Contains(r.Text, "ENOWX_BIN=/opt/enowx") {
		t.Fatal("a Bash call lost ENOWX_BIN")
	}
	// No environment given means the agent's, cleaned, never the agent's as it is.
	t.Setenv("ANTHROPIC_API_KEY", fake)
	if r := (bashTool{}).Run(context.Background(), input, Env{Dir: t.TempDir()}); strings.Contains(r.Text, fake) {
		t.Fatal("a Bash call without Environ shows the key")
	}
}

func TestBashTimeoutBounds(t *testing.T) {
	for ms, want := range map[int]time.Duration{0: 2 * time.Minute, -1: 2 * time.Minute, 5000: 5 * time.Second, 999999999: 10 * time.Minute} {
		if got := bashTimeout(ms); got != want {
			t.Errorf("%d ms: %v", ms, got)
		}
	}
}

func TestWholeCharsHoldsACutCharacter(t *testing.T) {
	text, held := wholeChars(nil, []byte("ab\xc3"))
	if text != "ab" || len(held) != 1 {
		t.Fatalf("%q, held %d", text, len(held))
	}
	if text, held = wholeChars(held, []byte("\xa9c")); text != "éc" || len(held) != 0 {
		t.Fatalf("%q, held %d", text, len(held))
	}
}

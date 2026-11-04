//go:build !windows

package mcpclient

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"testing"
)

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func pidIn(t *testing.T, log, word string) int {
	t.Helper()
	b, _ := os.ReadFile(log)
	m := regexp.MustCompile(word + ` (\d+)`).FindSubmatch(b)
	if m == nil {
		return 0
	}
	pid, _ := strconv.Atoi(string(m[1]))
	return pid
}

// Close takes the server and what it started; no orphans.
func TestCloseKillsTheProgramAndItsChildren(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	s, err := Connect(context.Background(), fakeSpec(t, map[string]string{"fake": "child"}, log), nil)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the child to start", func() bool { return pidIn(t, log, "child") != 0 })
	server, child := pidIn(t, log, "start"), pidIn(t, log, "child")
	if !alive(child) {
		t.Fatal("the child isn't running")
	}
	_ = s.Close()
	waitFor(t, "the server to go", func() bool { return !alive(server) })
	waitFor(t, "the child to go", func() bool { return !alive(child) })
}

// A server that crashes doesn't leave what it started behind either.
func TestCrashKillsItsChildren(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	s, err := Connect(context.Background(), fakeSpec(t, map[string]string{"fake": "child"}, log), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	waitFor(t, "the child to start", func() bool { return pidIn(t, log, "child") != 0 })
	child := pidIn(t, log, "child")
	if r := run(find(t, s, "mcp__fake__crash"), `{}`); !r.Failed {
		t.Fatalf("crash: %+v", r)
	}
	waitFor(t, "the child to go", func() bool { return !alive(child) })
}

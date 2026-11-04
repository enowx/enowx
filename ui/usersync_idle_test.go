//go:build !windows

package ui

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/internal/rpccount"
	"github.com/enowx/enowx/proto"
)

// A sync with nothing to do reads no conflicts or trash and no base: this is what an idle
// app does every 30 seconds with each host.
func TestIdleSyncAsksLittle(t *testing.T) {
	hubDir, peerDir := t.TempDir(), t.TempDir()
	body := []byte(strings.Repeat(strings.Repeat("x", 199)+"\n", (270<<10)/200))
	write := func(rel string, data []byte) {
		t.Helper()
		file := filepath.Join(hubDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 29 {
		c := proto.SyncConflict{ID: fmt.Sprintf("%012x", i), Path: fmt.Sprintf("settings/big%d.txt", i), Created: time.Now().UTC(),
			Versions: []proto.SyncVersion{{Host: "mac", Data: body}, {Host: "enowxlabs", Data: body}}}
		data, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		write("sync/conflicts/"+c.ID+".json", data)
	}
	for i := range 200 {
		write(fmt.Sprintf("notes/%04d.md", i), []byte("---\ntitle: n\n---\n"+strings.Repeat("a line\n", 1500)))
	}
	dial := func(dir string) (*client.Client, *rpccount.Conn, func()) {
		t.Helper()
		srv, err := daemon.NewServer(dir)
		if err != nil {
			t.Fatal(err)
		}
		x, y := net.Pipe()
		go srv.ServeConn(x)
		counted := rpccount.Wrap(y)
		cl, err := client.New(counted, nil)
		if err != nil {
			t.Fatal(err)
		}
		return cl, counted, func() { cl.Close(); srv.Close() }
	}
	hub, hubCount, closeHub := dial(hubDir)
	defer closeHub()
	peer, _, closePeer := dial(peerDir)
	defer closePeer()

	a, run := watchedApp(t)
	a.host, a.client = LocalHost(), hub
	a.watches = map[string]*hostWatch{"p": {host: hosts.Host{ID: "p", Name: "peer"}, cl: peer}}
	cycle := func() {
		t.Helper()
		a.syncData()
		run(func() bool { return !a.dsync.running }, "a sync to end", 2*time.Minute)
		if st := a.dsync.hosts["p"]; st.state != syncIdle {
			t.Fatalf("sync: %s: %s", st.state, st.err)
		}
	}
	cycle() // brings the peer up to date
	cycle() // finds it so
	var m0, m1 runtime.MemStats
	const rounds = 5
	lists, bytes := hubCount.Calls(proto.MethodSyncConflicts), hubCount.BytesRead()
	runtime.GC()
	runtime.ReadMemStats(&m0)
	for range rounds {
		cycle()
	}
	runtime.ReadMemStats(&m1)
	lists = hubCount.Calls(proto.MethodSyncConflicts) - lists
	read := hubCount.BytesRead() - bytes
	allocated := (m1.TotalAlloc - m0.TotalAlloc) / rounds
	t.Logf("an idle sync, at 2 a minute: %d sync.conflicts in %d syncs; %d bytes from the hub a sync; %d bytes allocated a sync, %.1f MB a minute",
		lists, rounds, read/rounds, allocated, float64(allocated*2)/(1<<20))
	if lists != 0 {
		t.Fatalf("sync.conflicts called %d times with nothing changed", lists)
	}
	if len(a.dsync.conflicts) != 29 {
		t.Fatalf("%d conflicts shown", len(a.dsync.conflicts))
	}
	// A change is seen at once, and lists follow.
	write("sync/conflicts/new.json", []byte(`{"id":"new","path":"settings/n.txt","created":"2026-11-01T00:00:00Z","versions":[{"host":"a","data":"eA=="}]}`))
	cycle()
	if len(a.dsync.conflicts) != 30 {
		t.Fatalf("%d conflicts shown after one more", len(a.dsync.conflicts))
	}
}

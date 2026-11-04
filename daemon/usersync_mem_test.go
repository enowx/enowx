package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/core/usersync"
	"github.com/enowx/enowx/proto"
)

// conflictFixture writes n conflicts of two versions of size bytes each into srv's
// folder, the size the owner's Mac had: 29 of 270 KB took 20.9 MB to list.
func conflictFixture(t *testing.T, srv *Server, n, size int) {
	t.Helper()
	line := strings.Repeat("x", 199) + "\n"
	body := []byte(strings.Repeat(line, size/len(line)))
	for i := range n {
		c := proto.SyncConflict{ID: fmt.Sprintf("%012x", i), Path: fmt.Sprintf("settings/big%d.txt", i), Created: time.Now().UTC(),
			Versions: []proto.SyncVersion{{Host: "mac", Data: body}, {Host: "enowxlabs", Data: body}}}
		data, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeSyncFile(filepath.Join(srv.dataDir, filepath.FromSlash(usersync.ConflictsDir), c.ID+".json"), data); err != nil {
			t.Fatal(err)
		}
	}
}

// allocatedBy is what fn allocated in this process, both ends of the connection here.
func allocatedBy(fn func()) uint64 {
	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	fn()
	runtime.ReadMemStats(&b)
	return b.TotalAlloc - a.TotalAlloc
}

// Listing conflicts costs what naming them does, however large the files are.
func TestConflictListCarriesNoBodies(t *testing.T) {
	srv := startServer(t)
	conflictFixture(t, srv, 29, 270<<10)
	c := connect(t, srv, nil)
	var reply json.RawMessage
	first := allocatedBy(func() {
		if err := c.Call(proto.MethodSyncConflicts, nil, &reply); err != nil {
			t.Fatal(err)
		}
	})
	size := len(reply)
	const rounds = 5
	var total uint64
	for range rounds {
		total += allocatedBy(func() {
			if _, err := c.SyncConflicts(); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Logf("sync.conflicts of 29, 270 KB a version: reply %d bytes, allocated %d bytes the first time, %d a call after", size, first, total/rounds)
	if size > 20<<10 {
		t.Fatalf("reply of %d bytes", size)
	}
	if after := total / rounds; after > 1<<20 {
		t.Fatalf("%d bytes allocated a call", after)
	}
	// What an app asks for on demand has it all.
	v, err := c.SyncConflictVersion("000000000003", 2)
	if err != nil {
		t.Fatal(err)
	}
	if v.Host != "enowxlabs" || v.Size == 0 || int64(len(v.Data)) != v.Size {
		t.Fatalf("version: %s, %d bytes, says %d", v.Host, len(v.Data), v.Size)
	}
	list, err := c.SyncConflicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 29 || len(list[0].Versions) != 2 || list[0].Versions[0].Data != nil || list[0].Versions[0].Size == 0 {
		t.Fatalf("list: %+v", list[0])
	}
	if err := c.SyncResolve(list[0].ID, "1"); err != nil { // it settles from the file, not the list
		t.Fatal(err)
	}
}

func writeSyncFile(file string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	return os.WriteFile(file, data, 0o600)
}

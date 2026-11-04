package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/usersync"
	"github.com/enowx/enowx/proto"
)

func syncFrameFixture(t *testing.T, srv *Server) ([]string, []proto.SyncChange) {
	t.Helper()
	var paths []string
	var changes []proto.SyncChange
	for i := 0; i < 6; i++ {
		path := fmt.Sprintf("canvas/test/assets/%d.json", i)
		data := bytes.Repeat([]byte{byte('a' + i)}, 3<<20)
		file := filepath.Join(srv.dataDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, data, 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
		changes = append(changes, proto.SyncChange{Path: path, Data: data})
	}
	return paths, changes
}

func TestSyncReadLargerThanFrame(t *testing.T) {
	srv := startServer(t)
	paths, changes := syncFrameFixture(t, srv)
	c := connect(t, srv, nil)
	got, err := (client.SyncSide{C: c}).Read(paths)
	if err != nil {
		t.Fatalf("read 18 MiB of revisions: %v (connection: %v)", err, c.Err())
	}
	for _, change := range changes {
		if !bytes.Equal(got[change.Path], change.Data) {
			t.Fatalf("bytes differ for %s", change.Path)
		}
	}
	if _, err := c.Info(); err != nil {
		t.Fatalf("connection closed after read: %v", err)
	}
}

func TestSyncWriteLargerThanFrame(t *testing.T) {
	_, changes := syncFrameFixture(t, startServer(t))
	srv := startServer(t)
	c := connect(t, srv, nil)
	applied, stale, err := (client.SyncSide{C: c}).Write(changes)
	if err != nil || len(applied) != len(changes) || len(stale) != 0 {
		t.Fatalf("write 18 MiB of revisions: applied=%d stale=%d error=%v", len(applied), len(stale), err)
	}
	for _, change := range changes {
		got, err := os.ReadFile(filepath.Join(srv.dataDir, filepath.FromSlash(change.Path)))
		if err != nil || !bytes.Equal(got, change.Data) {
			t.Fatalf("bytes differ for %s: %v", change.Path, err)
		}
	}
	if _, err := c.Info(); err != nil {
		t.Fatalf("connection closed after write: %v", err)
	}
}

func TestUserSyncLargerThanFrame(t *testing.T) {
	source, destination := startServer(t), startServer(t)
	paths, changes := syncFrameFixture(t, source)
	a, b := connect(t, source, nil), connect(t, destination, nil)
	base := &usersync.Base{}
	rep, err := usersync.Sync(client.SyncSide{C: a}, client.SyncSide{C: b}, base)
	if err != nil || rep.Sent < len(paths) {
		t.Fatalf("sync 18 MiB between daemons: sent=%d error=%v", rep.Sent, err)
	}
	for _, change := range changes {
		got, err := os.ReadFile(filepath.Join(destination.dataDir, filepath.FromSlash(change.Path)))
		if err != nil || !bytes.Equal(got, change.Data) {
			t.Fatalf("bytes differ for %s: %v", change.Path, err)
		}
	}
	rep, err = usersync.Sync(client.SyncSide{C: a}, client.SyncSide{C: b}, base)
	if err != nil || rep.Sent != 0 || rep.Received != 0 {
		t.Fatalf("second sync should be unchanged: %+v, %v", rep, err)
	}
	for _, c := range []*client.Client{a, b} {
		if _, err := c.Info(); err != nil {
			t.Fatalf("connection closed after sync: %v", err)
		}
	}
}

func TestOversizedSyncReplyKeepsConnection(t *testing.T) {
	srv := startServer(t)
	paths, _ := syncFrameFixture(t, srv)
	// A client that takes parts is sent all of it.
	c := connect(t, srv, nil)
	var result proto.SyncReadResult
	if err := c.Call(proto.MethodSyncRead, proto.SyncReadParams{Paths: paths}, &result); err != nil {
		t.Fatalf("read: %v (connection: %v)", err, c.Err())
	}
	// One from before them gets a normal size error.
	nc, frames := oldApp(t, srv)
	if r := oldCall(t, nc, frames, 1, proto.MethodSyncRead, proto.SyncReadParams{Paths: paths}); !strings.Contains(r.Error, "frame limit") {
		t.Fatalf("expected a normal size error, got %q", r.Error)
	}
	if r := oldCall(t, nc, frames, 2, proto.MethodDaemonInfo, struct{}{}); r.Error != "" {
		t.Fatalf("oversized response closed the connection: %s", r.Error)
	}
}

func TestSyncLargeFileAndStaleWrites(t *testing.T) {
	srv := startServer(t)
	c := connect(t, srv, nil)
	side := client.SyncSide{C: c, From: `host <one> "two"`}
	data := bytes.Repeat([]byte("x"), 9<<20)
	a, b := "canvas/test/a.json", "canvas/test/b.json"
	applied, stale, err := side.Write([]proto.SyncChange{{Path: a, Data: data}})
	if err != nil || len(applied) != 1 || len(stale) != 0 {
		t.Fatalf("a single 9 MiB file fits: %v, %v, %v", applied, stale, err)
	}
	got, err := side.Read([]string{a})
	if err != nil || !bytes.Equal(got[a], data) {
		t.Fatalf("read a single 9 MiB file: %v", err)
	}
	applied, stale, err = side.Write([]proto.SyncChange{
		{Path: a, Data: data, Expect: "changed since the manifest"},
		{Path: b, Data: data},
	})
	if err != nil || len(applied) != 1 || applied[0] != b || len(stale) != 1 || stale[0] != a {
		t.Fatalf("batched stale writes: %v, %v, %v", applied, stale, err)
	}
}

func TestSyncSingleOversizedFileKeepsConnection(t *testing.T) {
	srv := startServer(t)
	c := connect(t, srv, nil)
	side := client.SyncSide{C: c}
	data := bytes.Repeat([]byte("x"), 13<<20)
	path := "canvas/test/large.json"
	file := filepath.Join(srv.dataDir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := side.Read([]string{path}); err == nil || !strings.Contains(err.Error(), "frame limit") {
		t.Fatalf("expected a size error before requesting the file: %v", err)
	}
	applied, _, err := side.Write([]proto.SyncChange{{Path: "canvas/test/small.json", Data: []byte("small")}, {Path: path, Data: data}})
	if err == nil || len(applied) != 0 {
		t.Fatalf("oversized write should fail preflight without applying files: %v, %v", applied, err)
	}
	if _, err := c.Info(); err != nil {
		t.Fatalf("oversized file closed the connection: %v", err)
	}
}

type syncCountingConn struct {
	net.Conn
	reads, writes atomic.Int32
}

func (c *syncCountingConn) Write(p []byte) (int, error) {
	if len(p) >= 9 && p[4] == byte(proto.TypeRequest) {
		var req proto.Request
		if json.Unmarshal(p[9:], &req) == nil {
			switch req.Method {
			case proto.MethodSyncRead:
				c.reads.Add(1)
			case proto.MethodSyncWrite:
				c.writes.Add(1)
			}
		}
	}
	return c.Conn.Write(p)
}

func TestSmallSyncFilesStayBatched(t *testing.T) {
	srv := startServer(t)
	server, peer := net.Pipe()
	go srv.ServeConn(server)
	counted := &syncCountingConn{Conn: peer}
	c, err := client.New(counted, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	side := client.SyncSide{C: c}
	var changes []proto.SyncChange
	var paths []string
	for i := 0; i < 50; i++ {
		path := fmt.Sprintf("canvas/test/%d.json", i)
		paths = append(paths, path)
		changes = append(changes, proto.SyncChange{Path: path, Data: []byte("small")})
	}
	if _, _, err := side.Write(changes); err != nil {
		t.Fatal(err)
	}
	if _, err := side.Read(paths); err != nil {
		t.Fatal(err)
	}
	if counted.reads.Load() != 1 || counted.writes.Load() != 1 {
		t.Fatalf("small files lost batching: %d reads, %d writes", counted.reads.Load(), counted.writes.Load())
	}
}

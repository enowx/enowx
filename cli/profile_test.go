package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

func TestProfileShowsTheRunningDaemon(t *testing.T) {
	srv := newServer(t)
	c := dialServer(t, srv, nil)
	var out bytes.Buffer
	if err := (runner{c: c, out: &out}).profile(nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"daemon ", "heap live", "app: not measured"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("no %q in:\n%s", want, out.String())
		}
	}
	out.Reset()
	if err := (runner{c: c, out: &out}).profile([]string{"--json"}); err != nil {
		t.Fatal(err)
	}
	var got proto.Profile
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.Daemon.Kind != "daemon" {
		t.Fatalf("%v: %s", err, out.String())
	}
	dir := t.TempDir()
	if err := (runner{c: c, out: &out}).profile([]string{"--out", dir}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"daemon-heap.pprof", "daemon-goroutine.pprof", "profile.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
}

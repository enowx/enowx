package daemon

import (
	"encoding/json"
	"os"
	"testing"
)

// A line for GOA stays until it has gone in: over a restart, and put back ahead
// of the rest when typing it fails.
func TestGOALinesWaitToBeDelivered(t *testing.T) {
	srv := startServer(t)
	srv.goa.mu.Lock()
	srv.goa.tell = []goaLine{{Text: "a report"}}
	srv.goa.mu.Unlock()
	srv.saveGOATell()

	srv.goa.mu.Lock()
	srv.goa.tell = nil
	srv.goa.mu.Unlock()
	srv.loadGOATell() // as a daemon starting again does

	srv.goaFlush("no-such-session")
	srv.goa.mu.Lock()
	back := srv.goa.tell
	srv.goa.mu.Unlock()
	if len(back) != 1 || back[0].Text != "a report" {
		t.Fatalf("after a failed typing: %+v", back)
	}
	data, err := os.ReadFile(srv.goa.tellFile)
	if err != nil {
		t.Fatal(err)
	}
	var kept []goaLine
	if err := json.Unmarshal(data, &kept); err != nil || len(kept) != 1 {
		t.Fatalf("kept %s: %v", data, err)
	}
}

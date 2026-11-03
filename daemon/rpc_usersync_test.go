package daemon

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

// An app that hasn't said it keeps a canvas's versions on their host gets no manifest: it
// would take their not being listed here for their deletion, and delete them elsewhere.
func TestOlderAppGetsNoManifest(t *testing.T) {
	c := connect(t, startServer(t), nil)
	var m proto.SyncManifest
	if err := c.Call(proto.MethodSyncManifest, nil, &m); err == nil {
		t.Fatal("an older app got a manifest")
	}
	if _, err := c.SyncManifest(); err != nil {
		t.Fatal(err)
	}
}

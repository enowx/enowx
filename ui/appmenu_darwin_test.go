package ui

import (
	"testing"

	"github.com/egoist/mygo"
)

// Cmd+W is the tab's, so it can't close the only window and quit the app.
func TestCmdWClosesATabNotTheWindow(t *testing.T) {
	items := fileMenuItems(New(Options{}))
	if items[0].Accelerator != "CmdOrCtrl+W" || items[0].Role != "" || items[0].Click == nil {
		t.Fatalf("Cmd+W: %+v", items[0])
	}
	if items[1].Role != mygo.RoleClose || items[1].Accelerator == "CmdOrCtrl+W" || items[1].Accelerator == "" {
		t.Fatalf("closing the window: %+v", items[1])
	}
}

package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/usersync"
	"github.com/enowx/enowx/proto"
)

// The window's menus and dialogs at the design's size, each with what must
// show, kept as pictures with SHOTS set: the composer's settings, the tools,
// the host switcher, the files' dialogs, GOA's menu and Sync with other hosts.
func TestWindowMenusAtTheDesignsSize(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	a := h.app
	auditHosts(t, a)
	a.state.RailWide = true
	dir := a.projects[0].Path
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# A project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chatAuditPages()[0].open(a)
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1440, 900)
	h.tt = tt
	settle := func() {
		for range 2 {
			h.settle("", func() bool { return true })
			tt.Advance(500 * time.Millisecond)
		}
	}
	settle()
	step := func(name string, do func() error, want ...string) {
		t.Helper()
		if err := do(); err != nil {
			t.Fatalf("%s: %v in %q", name, err, tt.Texts())
		}
		settle()
		shot(t, tt, filepath.Join("window", name+".png"))
		for _, w := range want {
			if !tt.HasText(w) {
				t.Errorf("%s: no %q in %q", name, w, tt.Texts())
			}
		}
		for _, p := range outOfParents(tt) {
			t.Errorf("%s: %s", name, p)
		}
	}
	esc := func() { tt.Key(0, ui.KeyEscape); settle() }
	click := func(label string) func() error { return func() error { return tt.Click(label) } }

	step("menu-model", click("Model"), "Opus 5.5")
	esc()
	step("menu-perm", click("Permissions"), "Plan")
	esc()
	step("menu-effort", click("Effort"), "High effort")
	esc()
	step("menu-tools", click("More tools"), "Ports")
	esc()
	_, state := a.connectionState()
	step("menu-host", click("Host: "+a.host.Name+", "+state), "Hosts", "Manage hosts…", "enowxlabs", "nas-box")
	esc()

	a.selectSession(a.sessions[0])
	a.state.Chats = nil
	a.openPanel(panelFiles)
	h.settle("the files", func() bool { return tt.HasText("README.md") })
	settle()
	step("file-menu", func() error { return tt.RightClick("README.md") }, "Properties")
	step("dialog-props", func() error {
		if err := tt.Click("Properties"); err != nil {
			return err
		}
		h.settle("the properties", func() bool { return tt.HasText("Permissions") })
		return nil
	}, "Permissions")
	step("dialog-props-closed", click("Close"))
	if err := tt.RightClick("README.md"); err != nil {
		t.Fatal(err)
	}
	settle()
	step("dialog-new-folder", click("New folder in the project folder…"), "New folder", "Create")
	step("dialog-new-folder-closed", click("Cancel"))

	a.state.Panel.Open = false
	a.dsync.hosts = map[string]hostSync{a.hostList[0].ID: {state: syncIdle, last: time.Now().Add(-2 * time.Minute), rep: usersync.Report{Sent: 3, Received: 5, Merged: 1}}, a.hostList[1].ID: {state: syncIdle, last: time.Now().Add(-40 * time.Minute), rep: usersync.Report{Received: 2}}}
	a.dsync.hub = proto.SyncStatus{Files: 184, Size: 12 << 20}
	a.dsync.conflicts = []proto.SyncConflict{{ID: "c", Path: "settings/notify.json", Created: time.Now(), Versions: []proto.SyncVersion{{Host: hosts.LocalID, Data: []byte("{}")}, {Host: a.hostList[0].ID, Data: []byte("{ }")}}}}
	a.dsync.trash = []proto.SyncTrashItem{{ID: "n", Path: "notes/old.md", Title: "Old release notes", Deleted: time.Now().Add(-24 * time.Hour)}, {ID: "m", Path: "notes/m.md", Title: "Meeting 12 Oct", Deleted: time.Now().Add(-9 * 24 * time.Hour)}}
	a.page, a.settingsSection = pageSettings, sectionSync
	step("settings-sync", func() error { return nil }, "enowxlabs", "settings/notify.json", "Old release notes")

	a.page = ""
	ovGOA(a)
	settle()
	step("menu-goa", click("More for GOD of Agents"), "Rules…", "Leave GOD of Agents…")
}

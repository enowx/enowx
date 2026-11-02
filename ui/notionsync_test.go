package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// A Notion token typed once goes to the vault when asked, is used from
// there, and an import asked again daily is kept by the credential's ID
// alone; when due, it runs on the host with the vault's token.
func TestNotionTokenFromTheVaultAndImportAgain(t *testing.T) {
	h := newVaultHarness(t)
	if err := h.app.vaultCreate("correct horse", false); err != nil {
		t.Fatal(err)
	}
	h.app.daemonInfo = proto.DaemonInfo{ID: "mac"}
	h.app.page = pageNotes
	h.app.openNoteImport(nil)
	f := h.app.noteImport
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1000).Height(900).Background(theme.Background).TextColor(theme.Text).Children(func() {
			h.app.noteImportDialog(c)
		})
	}, 1000, 900)
	h.settle("the dialog", func() bool { return hasFound(h.tt, "Notion (API)") })
	h.click("Notion (API)")
	h.settle("the token field", func() bool { return hasFound(h.tt, "Keep the token in the vault") })
	f.token = "ntn_fakeTOKEN0123456789"
	f.keep = true
	h.settle("how often", func() bool { return hasFound(h.tt, "Daily") })
	shot(t, h.tt, "notes-notion-token.png")
	h.click("Daily")
	h.app.runNoteImport(f, false)
	if f.err != "" {
		t.Fatal(f.err)
	}
	creds := h.app.notionCreds()
	if len(creds) != 1 || creds[0].Fields["token"] != "ntn_fakeTOKEN0123456789" || f.cred != creds[0].ID || f.token != "" {
		t.Fatalf("the vault's Notion credentials: %+v, picked %q", creds, f.cred)
	}
	syncs := h.app.state.NotionSyncs
	if len(syncs) != 1 || syncs[0].Hours != 24 || syncs[0].Cred != creds[0].ID || syncs[0].Host != "mac" {
		t.Fatalf("kept to import again: %+v", syncs)
	}
	if data, _ := json.Marshal(h.app.state); strings.Contains(string(data), "fakeTOKEN") {
		t.Fatal("the app's state holds the token")
	}
	h.settle("the vault's token offered", func() bool { return hasFound(h.tt, "Use "+creds[0].Name) })
	shot(t, h.tt, "notes-notion-vault.png")

	// Due: it runs again with the vault's token.
	h.app.state.NotionSyncs[0].Last = time.Now().Add(-25 * time.Hour)
	h.app.notionSyncTick()
	h.settle("imported again", func() bool {
		s := h.app.state.NotionSyncs[0]
		return time.Since(s.Last) < time.Minute && !h.app.notionRunning[s.ID]
	})
	if err := h.app.state.NotionSyncs[0].Err; err != "" {
		t.Fatalf("importing again: %s", err)
	}
	h.app.vaultLock()
	h.app.state.NotionSyncs[0].Last = time.Now().Add(-25 * time.Hour)
	h.app.notionSyncTick()
	h.settle("nothing while locked", func() bool { return true })
	if time.Since(h.app.state.NotionSyncs[0].Last) < time.Hour {
		t.Fatal("imported again with the vault locked")
	}
}

func hasFound(tt *ui.Tester, label string) bool {
	_, ok := tt.Find(label)
	return ok
}

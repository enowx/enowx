package ui

import (
	"archive/zip"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// Files dropped on the Notes page are sent to the host, previewed and
// imported; then all the notes are exported as folders of Markdown and
// saved on this device.
func TestImportAndExportInTheNotesPage(t *testing.T) {
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a, b := net.Pipe()
	go srv.ServeConn(a)
	app := New(Options{})
	var later []func()
	app.onMain = func(fn func()) { later = append(later, fn) }
	cl, err := client.New(b, func(ev proto.Event) {
		var apply func()
		switch ev.Name {
		case proto.EventNotesChanged:
			apply = app.notesEvent(ev.Data)
		case proto.EventNoteGroupsChanged:
			apply = app.groupsEvent(ev.Data)
		case proto.EventNoteImport:
			apply = app.noteImportEvent(ev.Data)
		}
		if apply != nil {
			app.onMain(apply)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	app.client, app.page, app.host.Name = cl, pageNotes, "this Mac"
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1100).Height(760).Background(theme.Background).TextColor(theme.Text).Children(func() {
			app.notesPage(c)
			app.noteImportDialog(c)
			app.noteExportDialog(c)
		})
	}, 1100, 760)
	settle := func(what string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			tt.Frame()
			if done() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: never; shows %q", what, tt.Texts())
			}
			time.Sleep(10 * time.Millisecond)
			for _, fn := range later {
				fn()
			}
			later = nil
		}
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("%s: %v in %q", label, err, tt.Texts())
		}
	}
	settle("the notes", func() bool { return app.noteList != nil && app.noteList.loaded })

	vault := filepath.Join(t.TempDir(), "Vault")
	os.MkdirAll(filepath.Join(vault, "Ideas"), 0o755)
	os.WriteFile(filepath.Join(vault, "Home.md"), []byte("# Home\n\nSee [[Debloater]].\n"), 0o644)
	os.WriteFile(filepath.Join(vault, "Ideas", "Debloater.md"), []byte("- [ ] scan apps\n"), 0o644)
	app.notesOptions().Drop([]string{vault}) // as the tree takes a folder dropped on it
	settle("the dialog", func() bool { return tt.HasText("Import notes") && tt.HasText("Vault") })
	tt.Advance(time.Second)
	tt.Frame()
	shot(t, tt, "notes-import-source.png")
	click("Preview")
	settle("the preview", func() bool { return tt.HasText("2 notes to import, read as a vault of Markdown.") })
	shot(t, tt, "notes-import-preview.png")
	click("Import 2 notes")
	settle("the report", func() bool { return tt.HasText("Imported 2 notes from Vault.zip") })
	shot(t, tt, "notes-import-report.png")
	click("Done")
	settle("the notes in the tree", func() bool {
		return app.noteImport == nil && tt.HasText("Debloater") && tt.HasText("Ideas") && app.notes.SelectedNote() != ""
	})

	saved := filepath.Join(t.TempDir(), "notes.zip")
	app.saveAs = func(name string) (string, error) { return saved, nil }
	click("Import and export")
	settle("the menu", func() bool { return tt.HasText("Export…") })
	click("Export…")
	settle("the export dialog", func() bool { return tt.HasText("Export notes") })
	click("Export")
	settle("the export saved", func() bool { return app.noteExport == nil })
	zr, err := zip.OpenReader(saved)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["Home.md"] || !names["Ideas/Debloater.md"] {
		t.Fatalf("the export holds %v", names)
	}
}

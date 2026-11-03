package cacheclean

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// neverCleaned is what no clean may touch, whatever it is asked.
var neverCleaned = []string{
	"vault/key", "vault-run/s1/x", "configs-run/x", "cloud/token", "accounts/a", "settings/s.json", "sessions.json",
	"agent-sessions/a", "notes/a.md", "notes/assets/n/a.png", "canvas/0123456789ab/canvas.json", "tokens/2026.jsonl",
	"goa-board.json", "usersync/x", "link-bases/x", "enowx.db", "intercept/ca-key.pem", "intercept/ca.pem",
	"teams/t1/canvas/0123456789ab/canvas.json", "teams/t1/notes/a.md", "teams/t1/sync/base/notes/a.md", "history/s1.vt",
}

func write(t *testing.T, dir, rel, text string, age time.Duration) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(p, when, when); err != nil {
		t.Fatal(err)
	}
}

// Asked for everything, a clean leaves the never-clean list whole.
func TestCleanNeverTouchesTheList(t *testing.T) {
	dir := t.TempDir()
	const old = 90 * 24 * time.Hour
	for _, rel := range neverCleaned {
		write(t, dir, rel, "kept", old)
	}
	write(t, dir, "screenshots/pasted-1.png", "a picture", old)
	write(t, dir, "reviews/x-1/shot.png", "a shot", old)
	write(t, dir, "trash/123/notes/a.md", "deleted", old)
	e := Env{Dir: dir, Now: time.Now()}
	var kinds, items []string
	for _, k := range List(e).Kinds {
		kinds = append(kinds, k.ID)
		for _, it := range k.Items {
			items = append(items, it.Path)
		}
	}
	res := Clean(e, proto.CacheCleanParams{Kinds: kinds, Items: append(items, neverCleaned...)})
	if res.Done == 0 {
		t.Fatalf("nothing cleaned: %+v", res)
	}
	for _, rel := range neverCleaned {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s is gone", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "screenshots", "pasted-1.png")); err == nil {
		t.Error("a picture asked for stayed")
	}
}

// Nothing outside the named folders may ever be removed, however it is written.
func TestAllowed(t *testing.T) {
	for _, rel := range neverCleaned {
		if Allowed(rel) {
			t.Errorf("%s is allowed", rel)
		}
	}
	for _, rel := range []string{"screenshots", "screenshots/../vault/key", "../screenshots/a", "/screenshots/a", ""} {
		if Allowed(rel) {
			t.Errorf("%q is allowed", rel)
		}
	}
	if !Allowed("screenshots/a.png") {
		t.Error("a screenshot isn't allowed")
	}
}

// What a note names stays, what is new stays, and what's in use is reported, not taken.
func TestCleanKeepsWhatIsNeeded(t *testing.T) {
	dir := t.TempDir()
	const old = 90 * 24 * time.Hour
	write(t, dir, "notes/a.md", "see ~/.enowx-app/screenshots/named.png", old)
	write(t, dir, "screenshots/named.png", "x", old)
	write(t, dir, "screenshots/new.png", "x", 0)
	write(t, dir, "screenshots/old.png", "x", old)
	write(t, dir, "canvas-chat/0123456789ab/x", "x", old)
	e := Env{Dir: dir, Now: time.Now(), InUse: func(p string) string {
		if strings.Contains(p, "canvas-chat") {
			return "s1"
		}
		return ""
	}}
	res := Clean(e, proto.CacheCleanParams{Kinds: []string{"screenshots", "helpers"}})
	for _, name := range []string{"named.png", "new.png"} {
		if _, err := os.Stat(filepath.Join(dir, "screenshots", name)); err != nil {
			t.Errorf("%s went", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "screenshots", "old.png")); err == nil {
		t.Error("old.png stayed")
	}
	if !slices.ContainsFunc(res.Failed, func(f proto.CacheFailed) bool { return strings.Contains(f.Why, "s1") }) {
		t.Errorf("what's in use wasn't reported: %+v", res)
	}
}

// A kind picked one by one isn't taken by naming the kind, and a link stays.
func TestPerItemKinds(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "app-backup/old.app/x", "x", 0)
	write(t, dir, "app-backup/other.app/x", "x", 0)
	e := Env{Dir: dir, Now: time.Now()}
	if err := os.Symlink(filepath.Join(dir, "app-backup", "old.app"), filepath.Join(dir, "app-backup", "link.app")); err != nil {
		t.Fatal(err)
	}
	Clean(e, proto.CacheCleanParams{Kinds: []string{"backups"}})
	if _, err := os.Stat(filepath.Join(dir, "app-backup", "old.app")); err != nil {
		t.Fatal("the kind alone took an item")
	}
	res := Clean(e, proto.CacheCleanParams{Items: []string{"app-backup/old.app", "app-backup/link.app"}})
	if _, err := os.Stat(filepath.Join(dir, "app-backup", "old.app")); err == nil {
		t.Error("an item picked stayed")
	}
	if _, err := os.Lstat(filepath.Join(dir, "app-backup", "link.app")); err != nil {
		t.Error("a link went")
	}
	if len(res.Failed) != 1 {
		t.Errorf("failed: %+v", res.Failed)
	}
}

// A log keeps its last part, in place.
func TestTrimLog(t *testing.T) {
	p := filepath.Join(t.TempDir(), "enowxd.log")
	data := strings.Repeat("a", logKept) + "the end"
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := trimLog(p); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if len(got) != logKept || !strings.HasSuffix(string(got), "the end") {
		t.Fatalf("%d bytes", len(got))
	}
}

// What's kept for going back isn't Clean cache's to remove.
func TestCanvasBackupIsNeverCleaned(t *testing.T) {
	if Allowed("canvas-history-backup/personal/0123456789ab/history/1.json") {
		t.Fatal("the canvases' backups can be cleaned")
	}
}

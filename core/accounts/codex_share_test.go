package accounts

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func codexFixture(t *testing.T) (*Store, string) {
	t.Helper()
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	t.Setenv("ENOWX_HOME", filepath.Join(user, "enowx"))
	t.Setenv("CODEX_HOME", filepath.Join(user, ".codex"))
	src := filepath.Join(user, ".codex")
	if err := os.MkdirAll(src, 0700); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(user, "probe")
	if err := os.Symlink(src, probe); err != nil {
		t.Fatalf("symlink capability required: %v", err)
	}
	if err := os.Remove(probe); err != nil {
		t.Fatal(err)
	}
	s, err := Open(os.Getenv("ENOWX_HOME"))
	if err != nil {
		t.Fatal(err)
	}
	return s, src
}

func writeFixture(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}

func openFixtureDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA journal_mode=WAL; CREATE TABLE IF NOT EXISTS records (value TEXT); INSERT INTO records VALUES ('synthetic')"); err != nil {
		t.Fatal(err)
	}
	return db
}

// A physical account database loses its own WAL/SHM at close. Sharing again
// must never replace those missing files with another database's journals.
func TestCodexShareSQLiteCloseReopen(t *testing.T) {
	s, src := codexFixture(t)
	mainDB := openFixtureDB(t, filepath.Join(src, "state_5.sqlite"))
	defer mainDB.Close()
	writeFixture(t, filepath.Join(src, "config.toml"), "# shared settings")
	writeFixture(t, filepath.Join(src, "skills", "fixture", "SKILL.md"), "synthetic skill")
	writeFixture(t, filepath.Join(src, "auth.json"), "default synthetic auth")
	for _, name := range []string{"state_5.sqlite-journal", "other.db", "other.db-wal", "future.sqlite3", "future.sqlite3-journal", "app-server-daemon/socket", "app-server-control/daemon.json", "future.sock", "daemon.pid", "future-runtime/state"} {
		writeFixture(t, filepath.Join(src, name), "synthetic runtime")
	}
	var homes, ids []string
	for _, name := range []string{"one", "two"} {
		acc, err := s.Add("codex", name)
		if err != nil {
			t.Fatal(err)
		}
		homes = append(homes, s.Home(acc.ID))
		ids = append(ids, acc.ID)
	}
	for _, home := range homes {
		if err := os.MkdirAll(home, 0700); err != nil {
			t.Fatal(err)
		}
		writeFixture(t, filepath.Join(home, "auth.json"), "account synthetic auth")
		db := openFixtureDB(t, filepath.Join(home, "state_5.sqlite"))
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for cycle := 0; cycle < 4; cycle++ {
		for i, home := range homes {
			for _, suffix := range []string{"-wal", "-shm"} {
				if _, err := os.Lstat(filepath.Join(home, "state_5.sqlite"+suffix)); !os.IsNotExist(err) {
					t.Fatalf("cycle %d: sidecar survived close: %v", cycle, err)
				}
			}
			reopened, err := Open(os.Getenv("ENOWX_HOME"))
			if err != nil {
				t.Fatal(err)
			}
			if env, err := reopened.Env("codex", ids[i]); err != nil || len(env) != 1 || env[0] != "CODEX_HOME="+home {
				t.Fatalf("account restart: %q %v", env, err)
			}
			for _, name := range []string{"state_5.sqlite-wal", "state_5.sqlite-shm", "state_5.sqlite-journal", "other.db", "other.db-wal", "future.sqlite3", "future.sqlite3-journal", "app-server-daemon", "app-server-control", "future.sock", "daemon.pid", "future-runtime"} {
				if _, err := os.Lstat(filepath.Join(home, name)); !os.IsNotExist(err) {
					t.Fatalf("cycle %d: runtime %s reached account: %v", cycle, name, err)
				}
			}
			for _, name := range []string{"config.toml", "skills/fixture/SKILL.md"} {
				if _, err := os.ReadFile(filepath.Join(home, name)); err != nil {
					t.Fatal(err)
				}
			}
			if got, err := os.ReadFile(filepath.Join(home, "auth.json")); err != nil || string(got) != "account synthetic auth" {
				t.Fatalf("auth changed: %q %v", got, err)
			}
			db := openFixtureDB(t, filepath.Join(home, "state_5.sqlite"))
			var integrity string
			if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
				t.Fatalf("integrity: %q %v", integrity, err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	var count int
	if err := mainDB.QueryRow("SELECT count(*) FROM records").Scan(&count); err != nil || count != 1 {
		t.Fatalf("default database changed: %d %v", count, err)
	}
	for _, home := range homes {
		db, err := sql.Open("sqlite", filepath.Join(home, "state_5.sqlite"))
		if err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow("SELECT count(*) FROM records").Scan(&count); err != nil || count != 5 {
			t.Fatalf("account writes not private: %d %v", count, err)
		}
		db.Close()
	}
}

func TestCodexShareKeepsCanonicalRolloutsAndWriterLocks(t *testing.T) {
	s, src := codexFixture(t)
	for _, dir := range []string{"sessions", "archived_sessions", "thread-writer-locks"} {
		if err := os.MkdirAll(filepath.Join(src, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture(t, filepath.Join(src, "sessions", "thread.jsonl"), "synthetic rollout")
	lockPath := filepath.Join(src, "thread-writer-locks", "thread.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := tryWriterLock(lock); err != nil {
		t.Fatal(err)
	}
	for cycle := 0; cycle < 4; cycle++ {
		for _, home := range []string{s.Home("one"), s.Home("two")} {
			if err := share(kinds["codex"], home); err != nil {
				t.Fatal(err)
			}
			for _, dir := range []string{"sessions", "archived_sessions", "thread-writer-locks"} {
				got, err := filepath.EvalSymlinks(filepath.Join(home, dir))
				want, wantErr := filepath.EvalSymlinks(filepath.Join(src, dir))
				if err != nil || wantErr != nil || got != want {
					t.Fatalf("split %s: %q %v", dir, got, err)
				}
			}
			alias := filepath.Join(home, "thread-writer-locks", "thread.lock")
			if f, err := os.OpenFile(alias, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600); !os.IsExist(err) {
				if f != nil {
					f.Close()
				}
				t.Fatalf("writer lock bypassed: %v", err)
			}
			a, err := os.Stat(alias)
			if err != nil {
				t.Fatal(err)
			}
			b, err := lock.Stat()
			if err != nil || !os.SameFile(a, b) {
				t.Fatalf("writer lock replaced: %v", err)
			}
			contender, err := os.OpenFile(alias, os.O_RDWR, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			err = tryWriterLock(contender)
			contender.Close()
			if err == nil {
				t.Fatal("active writer lock bypassed through account home")
			}
		}
	}
}

func TestCodexShareRefusesLegacyRuntimeLinks(t *testing.T) {
	for _, name := range []string{"state_5.sqlite", "state_5.sqlite-wal", "state_5.sqlite-shm", "state_5.sqlite-journal", "auth.json", "app-server-daemon", "app-server-control", "unknown-runtime"} {
		t.Run(name, func(t *testing.T) {
			s, src := codexFixture(t)
			target := filepath.Join(src, name)
			writeFixture(t, target, "preserve target")
			home := s.Home("legacy")
			if err := os.MkdirAll(home, 0700); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(home, name)
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			if err := share(kinds["codex"], home); err == nil {
				t.Fatal("legacy runtime link allowed")
			}
			if got, err := os.Readlink(link); err != nil || got != target {
				t.Fatalf("link altered: %q %v", got, err)
			}
			if got, err := os.ReadFile(target); err != nil || string(got) != "preserve target" {
				t.Fatalf("target altered: %q %v", got, err)
			}
		})
	}
}

func TestCodexShareCreatesCanonicalDirectoriesBeforeFirstLaunch(t *testing.T) {
	s, src := codexFixture(t)
	for _, home := range []string{s.Home("one"), s.Home("two")} {
		if err := share(kinds["codex"], home); err != nil {
			t.Fatal(err)
		}
		for _, dir := range []string{"sessions", "archived_sessions", "thread-writer-locks"} {
			got, err := filepath.EvalSymlinks(filepath.Join(home, dir))
			want, wantErr := filepath.EvalSymlinks(filepath.Join(src, dir))
			if err != nil || wantErr != nil || got != want {
				t.Fatalf("%s not canonical: %q %v", dir, got, err)
			}
		}
	}
}

func TestCodexExplicitHomeBypassesAccountSharing(t *testing.T) {
	s, src := codexFixture(t)
	first, last := t.TempDir(), t.TempDir()
	acc, err := s.Add("codex", "isolated")
	if err != nil {
		t.Fatal(err)
	}
	acc.Env = []string{"CODEX_HOME=" + first, "CODEX_HOME=" + last}
	if _, err := s.Save(acc); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(src, "state_5.sqlite-wal")
	writeFixture(t, target, "synthetic journal")
	link := filepath.Join(s.Home(acc.ID), "state_5.sqlite-wal")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for cycle := 0; cycle < 4; cycle++ {
		reopened, err := Open(os.Getenv("ENOWX_HOME"))
		if err != nil {
			t.Fatal(err)
		}
		env, err := reopened.Env("codex", acc.ID)
		if err != nil || len(env) != 1 || env[0] != "CODEX_HOME="+last {
			t.Fatalf("override not honored: %q %v", env, err)
		}
		if _, err := os.Lstat(filepath.Join(last, "sessions")); !os.IsNotExist(err) {
			t.Fatalf("explicit home modified: %v", err)
		}
		if got, err := os.Readlink(link); err != nil || got != target {
			t.Fatalf("unused home modified: %q %v", got, err)
		}
	}
}

func TestCodexShareRefusesSplitWriterLocks(t *testing.T) {
	for _, name := range codexCanonicalDirs {
		t.Run(name, func(t *testing.T) {
			s, src := codexFixture(t)
			home := s.Home("split")
			writeFixture(t, filepath.Join(src, name, "canonical"), "keep canonical")
			writeFixture(t, filepath.Join(home, name, "private"), "keep private")
			if err := share(kinds["codex"], home); err == nil {
				t.Fatal("split namespace accepted")
			}
			for path, want := range map[string]string{filepath.Join(src, name, "canonical"): "keep canonical", filepath.Join(home, name, "private"): "keep private"} {
				if got, err := os.ReadFile(path); err != nil || string(got) != want {
					t.Fatalf("data changed: %q %v", got, err)
				}
			}
		})
	}
}

func TestCodexShareRefusesDanglingRuntimeAndLinkedHome(t *testing.T) {
	s, src := codexFixture(t)
	home := s.Home("dangling")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "state_5.sqlite-shm")
	target := filepath.Join(src, "absent.sqlite-shm")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := share(kinds["codex"], home); err == nil {
		t.Fatal("dangling runtime link accepted")
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("dangling link changed: %q %v", got, err)
	}
	linkedHome := s.Home("linked")
	if err := os.Symlink(src, linkedHome); err != nil {
		t.Fatal(err)
	}
	if err := share(kinds["codex"], linkedHome); err == nil {
		t.Fatal("linked account home accepted")
	}
}

func TestCodexEmptyExplicitHomeIsRefused(t *testing.T) {
	s, _ := codexFixture(t)
	acc, err := s.Add("codex", "empty")
	if err != nil {
		t.Fatal(err)
	}
	acc.Env = []string{"CODEX_HOME="}
	if _, err := s.Save(acc); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Env("codex", acc.ID); err == nil {
		t.Fatal("empty home could fall back to the default sign-in")
	}
}

func TestCodexConcurrentAccountStarts(t *testing.T) {
	s, _ := codexFixture(t)
	acc, err := s.Add("codex", "simultaneous")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 8)
	for range 8 {
		go func() {
			<-start
			_, err := s.Env("codex", acc.ID)
			results <- err
		}()
	}
	close(start)
	for range 8 {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
}

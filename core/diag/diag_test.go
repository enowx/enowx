package diag

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Every secret written with a test's values; none of them may reach a log.
var planted = []string{
	"token=abcd1234efgh5678", "password: hunter2hunter2", "Authorization: Bearer abcdefghijklmnop",
	"sk-ant-api03-abcdefghijklmnopqrstuvwx", "ghp_abcdefghijklmnopqrstuvwxyz0123456789", "github_pat_11ABCDEFG0abcdefghij",
	"AKIAIOSFODNN7EXAMPLE", "postgres://user:s3cretpass@db.example/app", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0.abcdefghijkl",
	"-----BEGIN RSA PRIVATE KEY-----\nMIIEabc\n-----END RSA PRIVATE KEY-----",
	"AIzaSyA1234567890abcdefghijklmnopqrst", "xoxb-1234567890-abcdefghij",
	"QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVphYmNkZWZnaGlqa2xtbm9wcXJzdHV2d3h5eg",
}

var values = []string{"abcd1234efgh5678", "hunter2hunter2", "abcdefghijklmnop", "abcdefghijklmnopqrstuvwx", "s3cretpass", "MIIEabc", "AKIAIOSFODNN7", "eyJhbGci", "AIzaSyA", "xoxb-", "QUJDREVGR0"}

func TestLogNeverHoldsASecret(t *testing.T) {
	dir := t.TempDir()
	l := Open(dir, AppLog)
	for _, s := range planted {
		l.Event("conn.closed", "err", errors.New("read failed: "+s), "host", s)
		fmt.Fprintf(l, "level=WARN msg=%q\n", s)
	}
	// What isn't known to be plain isn't written at all.
	l.Event("x", "struct", struct{ Prompt string }{"the user's prompt"}, "list", []string{"the user's prompt"})
	l.Close()
	text := read(t, filepath.Join(dir, AppLog))
	for _, v := range values {
		if strings.Contains(text, v) {
			t.Errorf("%q reached the log:\n%s", v, text)
		}
	}
	if strings.Contains(text, "prompt") {
		t.Errorf("a value of unknown type was written:\n%s", text)
	}
	if !strings.Contains(text, "conn.closed") {
		t.Errorf("no events:\n%s", text)
	}
}

// A bundle masks what an older version wrote plainly, and holds no command's output.
func TestBundleMasksOldLogs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, DaemonLog), []byte("old "+planted[0]+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("TEMP", os.Getenv("TMPDIR"))
	t.Setenv("TMP", os.Getenv("TMPDIR"))
	if err := os.MkdirAll(TrimDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(TrimDir(), "20261104-010101-1.log"), []byte("command output"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := Collect(dir)
	if len(files) != 1 || files[0].Name != DaemonLog || strings.Contains(files[0].Text, "abcd1234efgh5678") {
		t.Fatalf("%+v", files)
	}
	var buf bytes.Buffer
	if err := WriteZip(&buf, "enowX 1.0", map[string][]proto.DiagFile{"this: Mac": files}); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range z.File {
		names[f.Name] = true
	}
	if !names["about.txt"] || !names["this- Mac/enowxd.log"] {
		t.Fatalf("%v", names)
	}
}

func TestLogRotates(t *testing.T) {
	dir := t.TempDir()
	l := Open(dir, DaemonLog)
	l.maxFile = 1000
	for i := range 4000 {
		l.Event("conn.open", "i", i)
		if i%200 == 0 {
			l.Flush()
		}
	}
	l.Close()
	cur := filepath.Join(dir, DaemonLog)
	if size, _ := fileSize(cur); size > 2000 {
		t.Fatalf("the log is %d bytes", size)
	}
	for i := 1; i <= KeepRotated; i++ {
		if _, err := os.Stat(fmt.Sprintf("%s.%d", cur, i)); err != nil {
			t.Fatalf("no rotated %d: %v", i, err)
		}
	}
	if _, err := os.Stat(fmt.Sprintf("%s.%d", cur, KeepRotated+1)); err == nil {
		t.Fatal("too many rotated")
	}
}

// A log an older version let grow is cut down when the program starts, its end kept.
func TestOversizedOldLogIsCutDownAtStart(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, DaemonLog)
	big := strings.Repeat("an old line of the log\n", (MaxFile+(1<<20))/23) + "the last line\n"
	if err := os.WriteFile(cur, []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	l := Open(dir, DaemonLog)
	l.Close()
	if size, _ := fileSize(cur); size != 0 {
		t.Fatalf("still %d bytes", size)
	}
	kept := read(t, cur+".1")
	if len(kept) > MaxFile || !strings.HasSuffix(kept, "the last line\n") || !strings.HasPrefix(kept, "an old") {
		t.Fatalf("kept %d bytes", len(kept))
	}
}

func TestStartSaysHowTheLastRunEnded(t *testing.T) {
	dir := t.TempDir()
	// Start keeps the log open for a crash, which on Windows keeps its folder from going.
	t.Cleanup(func() { _ = debug.SetCrashOutput(nil, debug.CrashOptions{}) })
	run := func(end bool) string {
		l := Open(dir, AppLog)
		l.Start("app", "1.0", "b1")
		l.Flush()
		text := read(t, filepath.Join(dir, AppLog))
		if end {
			l.End("app")
		} else {
			l.Close()
		}
		lines := strings.Split(strings.TrimSpace(text), "\n")
		return lines[len(lines)-1]
	}
	if s := run(true); !strings.Contains(s, "lastRun=first") {
		t.Fatal(s)
	}
	if s := run(false); !strings.Contains(s, "lastRun=clean") {
		t.Fatal(s)
	}
	if s := run(true); !strings.Contains(s, "lastRun=unclean") {
		t.Fatal(s)
	}
}

func TestEveryLimits(t *testing.T) {
	dir := t.TempDir()
	l := Open(dir, AppLog)
	for range 100 {
		l.Every("k", time.Hour, "rpc.slow")
	}
	if l.Doing() != "rpc.slow" {
		t.Fatal(l.Doing())
	}
	l.Close()
	if n := strings.Count(read(t, filepath.Join(dir, AppLog)), "rpc.slow"); n != 1 {
		t.Fatalf("%d lines", n)
	}
}

func touch(t *testing.T, path string, size int, age time.Duration, now time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	when := now.Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// The current logs and the most recent rotated stay, however old; what is past its age
// or the total goes; and files of other names beside them are never touched.
func TestSweepKeepsWhatMustStay(t *testing.T) {
	dir, tmp := t.TempDir(), t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TEMP", tmp)
	t.Setenv("TMP", tmp)
	now := time.Now()
	month := 40 * 24 * time.Hour
	stay := []string{DaemonLog, DaemonLog + ".1", AppLog, AppLog + ".1", "watchdog/daemon.log", "watchdog/app.log", "enowxd.log.backup", "app.log.4", "notes.log", "vault.json",
		"watchdog/app-stall-20200101-000000.txt.keep", "watchdog/other-stall-20200101-000000.txt"}
	for _, name := range stay {
		touch(t, filepath.Join(dir, filepath.FromSlash(name)), 10, month, now)
	}
	touch(t, filepath.Join(dir, DaemonLog+".2"), 10, month, now)
	touch(t, filepath.Join(dir, AppLog+".2"), 10, time.Hour, now)
	touch(t, filepath.Join(dir, "watchdog", "app-stall-20200101-000000.txt"), 10, month, now)
	touch(t, filepath.Join(dir, "watchdog", "app-lag-20261101-000000.txt"), 10, time.Hour, now) // the newest
	trim := TrimDir()
	touch(t, filepath.Join(trim, "20200101-000000-1.log"), 10, month, now)
	touch(t, filepath.Join(trim, "20261101-000000-2.log"), 10, 30*time.Minute, now)
	touch(t, filepath.Join(trim, "20261101-000000-3.log"), 10, time.Minute, now)
	touch(t, filepath.Join(trim, "someone-elses.log"), 10, month, now)
	s := Sweep(dir, now)
	if s.Files != 3 {
		t.Fatalf("removed %d", s.Files)
	}
	for _, gone := range []string{filepath.Join(dir, DaemonLog+".2"), filepath.Join(dir, "watchdog", "app-stall-20200101-000000.txt"), filepath.Join(trim, "20200101-000000-1.log")} {
		if exists(gone) {
			t.Errorf("%s stayed", gone)
		}
	}
	for _, name := range stay {
		if !exists(filepath.Join(dir, filepath.FromSlash(name))) {
			t.Errorf("%s went", name)
		}
	}
	for _, kept := range []string{filepath.Join(dir, AppLog+".2"), filepath.Join(dir, "watchdog", "app-lag-20261101-000000.txt"), filepath.Join(trim, "20261101-000000-2.log"), filepath.Join(trim, "20261101-000000-3.log"), filepath.Join(trim, "someone-elses.log")} {
		if !exists(kept) {
			t.Errorf("%s went", kept)
		}
	}
}

func TestSweepHoldsToTheTotal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	now := time.Now()
	touch(t, filepath.Join(dir, DaemonLog+".3"), 15<<20, 3*time.Hour, now)
	touch(t, filepath.Join(dir, DaemonLog+".2"), 15<<20, 2*time.Hour, now)
	touch(t, filepath.Join(dir, DaemonLog+".1"), 1<<20, time.Hour, now)
	Sweep(dir, now)
	if exists(filepath.Join(dir, DaemonLog+".3")) || !exists(filepath.Join(dir, DaemonLog+".2")) || !exists(filepath.Join(dir, DaemonLog+".1")) {
		t.Fatal("the oldest should go, until under the total")
	}
}

// A log a bundle is reading neither rotates nor goes.
func TestHoldKeepsLogsForTheBundle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	now := time.Now()
	old := filepath.Join(dir, DaemonLog+".2")
	touch(t, old, 10, 40*24*time.Hour, now)
	release := Hold(dir)
	if s := Sweep(dir, now); s.Files != 0 || !exists(old) {
		t.Fatal("swept during a hold")
	}
	if _, err := Remove(dir, now, DaemonLog+".2"); !errors.Is(err, errHeld) || !exists(old) {
		t.Fatalf("removed during a hold: %v", err)
	}
	if err := rotate(filepath.Join(dir, DaemonLog), dir); !errors.Is(err, errHeld) {
		t.Fatalf("rotated during a hold: %v", err)
	}
	release()
	if s := Sweep(dir, now); s.Files != 1 || exists(old) {
		t.Fatal("not swept after the hold")
	}
	// A hold from something that died doesn't last.
	release = Hold(dir)
	defer release()
	long := now.Add(-2 * holdFor)
	if err := os.Chtimes(filepath.Join(dir, holdFile), long, long); err != nil {
		t.Fatal(err)
	}
	if Held(dir) {
		t.Fatal("a stale hold held")
	}
}

// Clean cache removes only what it lists: not the current log, not the most recent
// rotated, nothing named from outside.
func TestRemoveOnlyWhatIsListed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	now := time.Now()
	for _, name := range []string{DaemonLog, DaemonLog + ".1", DaemonLog + ".2", "vault.json"} {
		touch(t, filepath.Join(dir, name), 10, time.Hour, now)
	}
	for _, rel := range []string{DaemonLog, DaemonLog + ".1", "vault.json", "../" + DaemonLog + ".2"} {
		if _, err := Remove(dir, now, rel); err == nil {
			t.Errorf("%s was removed", rel)
		}
	}
	if n, err := Remove(dir, now, DaemonLog+".2"); err != nil || n != 10 {
		t.Fatalf("%d, %v", n, err)
	}
	for _, name := range []string{DaemonLog, DaemonLog + ".1", "vault.json"} {
		if !exists(filepath.Join(dir, name)) {
			t.Errorf("%s went", name)
		}
	}
}

func TestRecentKeepsConnectionLines(t *testing.T) {
	files := []proto.DiagFile{
		{Name: AppLog, Text: "2026-11-04T01:00:02+07:00 conn.closed host=local\n2026-11-04T01:00:01+07:00 page.change to=canvas\n"},
		{Name: DaemonLog, Text: "2026-11-04T01:00:01+07:00 conn.open conn=ab\n"},
	}
	got := Recent(files, 10)
	if len(got) != 2 || !strings.Contains(got[0], "daemon conn.open") || !strings.Contains(got[1], "app conn.closed") {
		t.Fatalf("%q", got)
	}
}

// What goes to the daemon's output, a service's journal, is masked as its log is.
func TestScrubbedMasksAKey(t *testing.T) {
	var b strings.Builder
	line := "level=WARN msg=x ANTHROPIC_API_KEY=sk-ant-api03-abcdefghijklmnopqrstuvwx other=sk-ant-api03-abcdefghijklmnopqrstuvwx\n"
	n, err := Scrubbed(&b).Write([]byte(line))
	if err != nil || n != len(line) {
		t.Fatalf("wrote %d, %v", n, err)
	}
	if strings.Contains(b.String(), "abcdefghijklmnopqrstuvwx") || !strings.Contains(b.String(), "level=WARN") {
		t.Fatalf("%q", b.String())
	}
}

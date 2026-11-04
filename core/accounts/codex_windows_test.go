package accounts

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func tryWriterLock(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, new(windows.Overlapped))
}

func TestCodexExplicitHomeWindowsCase(t *testing.T) {
	s, _ := codexFixture(t)
	acc, err := s.Add("codex", "case")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	acc.Env = []string{"CODEX_HOME=" + t.TempDir(), "codex_home=" + home}
	if _, err := s.Save(acc); err != nil {
		t.Fatal(err)
	}
	env, err := s.Env("codex", acc.ID)
	if err != nil || len(env) != 1 || env[0] != "codex_home="+home {
		t.Fatalf("Windows home precedence: %q %v", env, err)
	}
}

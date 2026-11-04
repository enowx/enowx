package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/enowx/enowx/core/accounts"
)

// Refusing an old runtime link must survive daemon restoration: the session
// stays stopped instead of starting without its account's environment.
func TestCodexRestoreRefusesUnsafeAccountHome(t *testing.T) {
	for _, midTurn := range []bool{false, true} {
		name := "idle"
		if midTurn {
			name = "mid-turn-tab"
		}
		t.Run(name, func(t *testing.T) { codexRestoreRefusesUnsafeAccountHome(t, midTurn) })
	}
}

func codexRestoreRefusesUnsafeAccountHome(t *testing.T, midTurn bool) {
	t.Helper()
	user := t.TempDir()
	dir := filepath.Join(user, "enowx")
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	t.Setenv("ENOWX_HOME", dir)
	t.Setenv("CODEX_HOME", filepath.Join(user, ".codex"))
	store, err := accounts.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	acc, err := store.Add("codex", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(user, "synthetic.sqlite-wal")
	if err := os.WriteFile(target, []byte("preserve synthetic journal"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(store.Home(acc.ID), "state_5.sqlite-wal")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	saved := []record{{ID: "unsafe", Dir: user, Agent: "codex", Account: acc.ID,
		Relaunch: true, Command: []string{"synthetic-original"}, Restart: []string{"synthetic-must-not-start"}}}
	if midTurn {
		saved[0].MidTurn = true
		saved[0].TabAgent = []string{"synthetic-tab-must-not-start"}
		saved[0].Lost = []string{"synthetic interrupted command"}
	}
	data, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	// A regression must never find the machine's real agent executable.
	t.Setenv("PATH", t.TempDir())
	srv, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	c := connect(t, srv, nil)
	list, err := c.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].Exited || list[0].ExitCode != -1 || list[0].Account != acc.ID {
		t.Fatalf("unsafe account resumed: %+v", list)
	}
	if _, err := srv.continueSession("unsafe"); err == nil {
		t.Fatal("continuing unsafe account succeeded")
	}
	if midTurn {
		if err := srv.waitSessionReady("unsafe"); err == nil {
			t.Fatal("unsafe account became ready for the restart carry-on prompt")
		}
		plan, err := c.RestartPlan()
		if err != nil || len(plan.Lost) != 1 || plan.Lost[0].What != saved[0].Lost[0] {
			t.Fatalf("restart lost-command report changed: %+v, %v", plan.Lost, err)
		}
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "preserve synthetic journal" {
		t.Fatalf("journal changed: %q %v", got, err)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("link changed: %q %v", got, err)
	}
}

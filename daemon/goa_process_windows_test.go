package daemon

import "testing"

func TestGOAWindowsCommandIdentity(t *testing.T) {
	s := &Server{}
	for _, argv := range [][]string{
		{`C:\temp\conhost.exe`, "codex"},
		{`C:\temp\codex\conhost.exe`, "0x4"},
		{"powershell.exe", "-Command", "codex --version"},
	} {
		if s.goaOwnProcess(argv, "codex") {
			t.Fatalf("argument or directory name bypassed watcher: %q", argv)
		}
	}
	if matchesSessionCommand([]string{"pwsh.exe", "-Command", "other"}, []string{"pwsh.exe", "-Command", "codex"}) {
		t.Fatal("an unrelated shell matched the launch wrapper")
	}
}

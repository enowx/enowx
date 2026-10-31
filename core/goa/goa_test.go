package goa

import (
	"path/filepath"
	"testing"

	"github.com/enowx/enowx/proto"
)

// GOA may run enowX's tools and commands, nothing else: no other tool, no
// command chained after enowX's or hidden in its arguments.
func TestAllowedOnlyEnowxsOwn(t *testing.T) {
	bin := "/opt/enowx/enowx"
	cases := []struct {
		tool, command string
		ok            bool
	}{
		{"mcp__enowx__session_spawn", `{"prompt":"x"}`, true},
		{"mcp__enowx", "", true},
		{"ToolSearch", "", true},
		{"Bash", `"$ENOWX_BIN" goa delegate --agent claude "Fix the bug; then run the tests | tee"`, true},
		{"Bash", `$ENOWX_BIN sessions`, true},
		{"Bash", `/opt/enowx/enowx send abc 'rm -rf $(pwd)'`, true},
		{"Bash", `"$ENOWX_BIN" ls; rm -rf ~`, false},
		{"Bash", `"$ENOWX_BIN" ls && echo hi`, false},
		{"Bash", `"$ENOWX_BIN" send abc "$(cat ~/.ssh/id_rsa)"`, false},
		{"Bash", "\"$ENOWX_BIN\" send abc `whoami`", false},
		{"Bash", `"$ENOWX_BIN" ls > /tmp/x`, false},
		{"Bash", `"$ENOWX_BIN" ls
rm x`, false},
		{"Bash", `go test ./...`, false},
		{"Bash", `echo "$ENOWX_BIN"`, false},
		{"Edit", "", false},
		{"Write", "", false},
		{"Read", "", false},
		{"Task", "", false},
		{"mcp__other__x", "", false},
		{"mcp__enowxevil__x", "", false},
	}
	for _, c := range cases {
		if ok, why := Allowed(c.tool, c.command, bin); ok != c.ok {
			t.Errorf("%s %q: allowed %v (%s), want %v", c.tool, c.command, ok, why, c.ok)
		}
	}
}

// A worker's call is the user's when it publishes, can't be undone, spends
// money or touches secrets; ordinary work is GOA's and the judge's.
func TestUrgent(t *testing.T) {
	cases := map[string]string{
		"go test ./...":                    "",
		"npm run build":                    "",
		"git commit -m 'fix'":              "",
		"git push origin main":             UrgentPublish,
		"npm publish":                      UrgentPublish,
		"rm -rf build":                     UrgentUndo,
		"psql -c 'DROP TABLE orders'":      UrgentUndo,
		"git reset --hard HEAD~3":          UrgentUndo,
		"stripe charges create --amount=5": UrgentMoney,
		"cat .env":                         UrgentSecrets,
		"printenv":                         UrgentSecrets,
	}
	for cmd, want := range cases {
		if got := Urgent("Bash", cmd, nil); got != want {
			t.Errorf("%q: %q, want %q", cmd, got, want)
		}
	}
	if got := Urgent("Read", "", []string{".env.local"}); got != UrgentSecrets {
		t.Errorf("reading .env.local: %q", got)
	}
	if got := Urgent("Edit", "", []string{"src/env.go"}); got != "" {
		t.Errorf("editing env.go: %q", got)
	}
}

// Claude Code and Codex hold GOA to enowX's tools with their hooks;
// Claude Code also starts with them only; an agent with neither is watched.
func TestLevels(t *testing.T) {
	if l, _ := LevelOf("claude"); l != proto.GOAEnforced {
		t.Errorf("claude: %s", l)
	}
	if l, _ := LevelOf("codex"); l != proto.GOAEnforced {
		t.Errorf("codex: %s", l)
	}
	if l, _ := LevelOf("aider"); l != proto.GOAWatched {
		t.Errorf("aider: %s", l)
	}
}

// Tasks keep their sessions and statuses on disk; a session's unsettled
// tasks change together, settled ones don't.
func TestBoard(t *testing.T) {
	b := OpenBoard(filepath.Join(t.TempDir(), "board.json"))
	one, err := b.Add(proto.GOATask{Prompt: "Fix the totals\nand add a test", Session: "s1", Host: "vps"})
	if err != nil || one.Title != "Fix the totals" || one.Status != proto.GOATaskWorking {
		t.Fatalf("added %+v %v", one, err)
	}
	two, _ := b.Add(proto.GOATask{Prompt: "Docs", Session: "s1", Host: "vps"})
	if _, err := b.Update(one.ID[:4], func(t *proto.GOATask) { t.Status = proto.GOATaskDone }); err != nil {
		t.Fatal(err)
	}
	changed := b.Of("s1", func(t *proto.GOATask) bool { t.Status = proto.GOATaskWaiting; return true })
	if len(changed) != 1 || changed[0].ID != two.ID {
		t.Fatalf("changed %+v", changed)
	}
	again := OpenBoard(b.path)
	if list := again.List(); len(list) != 2 || list[0].Status != proto.GOATaskDone || list[1].Status != proto.GOATaskWaiting {
		t.Fatalf("read back %+v", list)
	}
	if w := again.Working(); len(w) != 1 {
		t.Fatalf("working %+v", w)
	}
}

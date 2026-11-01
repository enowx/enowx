package goa

import (
	"path/filepath"
	"testing"
	"time"

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
		{"Edit", "", true},
		{"Write", "", true},
		{"Read", "", true},
		{"apply_patch", "", true},
		{"shell", `rg -n totals src`, true},
		{"shell", `sed -n '1,40p' main.go`, true},
		{"shell", `sed -n '1e rm x' main.go`, false},
		{"shell", `sed -i s/a/b/ main.go`, false},
		{"shell", `rg --pre ./run x`, false},
		{"shell", `cat main.go | sh`, false},
		{"shell", `cat .env`, false},
		{"shell", `cat ~/.ssh/id_rsa`, false},
		{"Task", "", false},
		{"WebFetch", "", false},
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

// A task with no session named goes to the project's own: running before
// hibernated before ended, newest first; never one busy, someone else's or
// on another branch.
func TestPickSession(t *testing.T) {
	const proj = "/code/shop"
	now := time.Now()
	s := func(id string, at int, edit func(*proto.SessionInfo)) proto.SessionInfo {
		info := proto.SessionInfo{ID: id, Agent: "claude", Project: proj, State: proto.StateDone, Created: now.Add(time.Duration(at) * time.Minute)}
		if edit != nil {
			edit(&info)
		}
		return info
	}
	free := func(string) bool { return false }
	hibernated := func(i *proto.SessionInfo) { i.Exited, i.Hibernated = true, true }
	ended := func(i *proto.SessionInfo) { i.Exited = true }
	want := Want{Project: proj, Agent: "claude"}
	cases := []struct {
		name string
		list []proto.SessionInfo
		want Want
		busy func(string) bool
		id   string
	}{
		{"none", nil, want, free, ""},
		{"idle first", []proto.SessionInfo{s("hib", 9, hibernated), s("idle", 1, nil), s("old", 0, nil)}, want, free, "idle"},
		{"hibernated before ended", []proto.SessionInfo{s("end", 9, ended), s("hib", 1, hibernated)}, want, free, "hib"},
		{"ended if that is all", []proto.SessionInfo{s("end", 1, ended)}, want, free, "end"},
		{"working left alone", []proto.SessionInfo{s("w", 1, func(i *proto.SessionInfo) { i.State = proto.StateWorking })}, want, free, ""},
		{"asking left alone", []proto.SessionInfo{s("a", 1, func(i *proto.SessionInfo) { i.State = proto.StateWaiting })}, want, free, ""},
		{"an open task", []proto.SessionInfo{s("x", 1, nil)}, want, func(string) bool { return true }, ""},
		{"another project", []proto.SessionInfo{s("x", 1, func(i *proto.SessionInfo) { i.Project = "/code/blog" })}, want, free, ""},
		{"another agent", []proto.SessionInfo{s("x", 1, func(i *proto.SessionInfo) { i.Agent = "codex" })}, want, free, ""},
		{"any agent when none named", []proto.SessionInfo{s("x", 1, func(i *proto.SessionInfo) { i.Agent = "codex" })}, Want{Project: proj}, free, "x"},
		{"a worktree", []proto.SessionInfo{s("x", 1, func(i *proto.SessionInfo) { i.Worktree = true })}, want, free, ""},
		{"a tab", []proto.SessionInfo{s("x", 1, func(i *proto.SessionInfo) { i.Parent = "p" })}, want, free, ""},
		{"GOA", []proto.SessionInfo{s("x", 1, func(i *proto.SessionInfo) { i.GOA = true })}, want, free, ""},
		{"a teammate's", []proto.SessionInfo{s("x", 1, func(i *proto.SessionInfo) { i.Member = &proto.SessionMember{Name: "Rina"} })}, want, free, ""},
		{"not to a --yolo unasked", []proto.SessionInfo{s("x", 1, func(i *proto.SessionInfo) { i.SkipPermissions = true })}, want, free, ""},
		{"a shell", []proto.SessionInfo{s("x", 1, func(i *proto.SessionInfo) { i.Agent = "shell" })}, Want{Project: proj}, free, ""},
		{"hookless at rest may be working", []proto.SessionInfo{s("x", 1, func(i *proto.SessionInfo) { i.Agent, i.State = "aider", "" })}, Want{Project: proj}, free, ""},
		{"another config", []proto.SessionInfo{s("x", 1, func(i *proto.SessionInfo) { i.Config = &proto.SessionConfig{ID: "c1", Name: "GLM"} })}, Want{Project: proj, Config: "official"}, free, ""},
		{"its config", []proto.SessionInfo{s("x", 1, func(i *proto.SessionInfo) { i.Config = &proto.SessionConfig{ID: "c1", Name: "GLM"} })}, Want{Project: proj, Config: "glm"}, free, "x"},
	}
	for _, c := range cases {
		got, ok := PickSession(c.list, c.want, c.busy)
		if got.ID != c.id || ok != (c.id != "") {
			t.Errorf("%s: got %q %v, want %q", c.name, got.ID, ok, c.id)
		}
	}
}

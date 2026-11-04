package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/core/platformagent/tools"
)

// fakeTool stands for an MCP server's tool, or any other the engine only knows by these.
type fakeTool struct {
	name            string
	readOnly, edits bool
}

func (f fakeTool) Name() string                  { return f.name }
func (f fakeTool) Description() string           { return "" }
func (f fakeTool) Schema() json.RawMessage       { return json.RawMessage(`{}`) }
func (f fakeTool) ReadOnly() bool                { return f.readOnly }
func (f fakeTool) Edits() bool                   { return f.edits }
func (f fakeTool) Detail(json.RawMessage) string { return "" }
func (f fakeTool) Run(context.Context, json.RawMessage, tools.Env) tools.Result {
	return tools.Result{}
}

func builtin(t *testing.T, name string) tools.Tool {
	t.Helper()
	for _, tool := range tools.Builtin() {
		if tool.Name() == name {
			return tool
		}
	}
	t.Fatalf("no tool %s", name)
	return nil
}

func bash(command string) json.RawMessage {
	raw, _ := json.Marshal(map[string]string{"command": command})
	return raw
}

func filePath(path string) json.RawMessage {
	raw, _ := json.Marshal(map[string]string{"file_path": path})
	return raw
}

func TestModes(t *testing.T) {
	read, write, shell := builtin(t, "Read"), builtin(t, "Write"), builtin(t, "Bash")
	input := filePath("a.txt")
	cases := []struct {
		mode                       string
		read, writeOutside, doBash Verdict
	}{
		{record.ModeDefault, Allow, Ask, Ask},
		{record.ModeAcceptEdits, Allow, Ask, Ask}, // no folder set
		{record.ModePlan, Allow, Deny, Deny},
		{record.ModeBypass, Allow, Allow, Allow},
	}
	for _, c := range cases {
		e := New(c.mode, nil)
		inFolder := New(c.mode, nil)
		inFolder.SetDir(t.TempDir())
		if v, _, _ := inFolder.Check(read, input); v != c.read {
			t.Errorf("%s: Read %v", c.mode, v)
		}
		if v, _, _ := e.Check(write, input); v != c.writeOutside {
			t.Errorf("%s: Write %v", c.mode, v)
		}
		if v, _, _ := e.Check(shell, bash("ls")); v != c.doBash {
			t.Errorf("%s: Bash %v", c.mode, v)
		}
	}
	if _, _, why := New(record.ModePlan, nil).Check(write, input); why != "plan mode: reads only" {
		t.Errorf("why: %q", why)
	}
	// Plan mode holds over a kept rule.
	if v, _, _ := New(record.ModePlan, []string{"Write", "Bash(ls *)"}).Check(shell, bash("ls")); v != Deny {
		t.Errorf("a rule beat plan mode: %v", v)
	}
}

func TestSetMode(t *testing.T) {
	e := New("nonsense", nil)
	if e.Mode() != record.ModeDefault {
		t.Fatalf("mode %q", e.Mode())
	}
	if err := e.SetMode("nonsense"); err == nil || e.Mode() != record.ModeDefault {
		t.Fatalf("took an unknown mode: %v, %q", err, e.Mode())
	}
	if err := e.SetMode(record.ModePlan); err != nil || e.Mode() != record.ModePlan {
		t.Fatalf("%v, %q", err, e.Mode())
	}
}

func TestAcceptEditsStaysInTheFolder(t *testing.T) {
	base := t.TempDir()
	dir, outside := filepath.Join(base, "project"), filepath.Join(base, "other")
	for _, d := range []string{filepath.Join(dir, "sub"), outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	e := New(record.ModeAcceptEdits, nil)
	e.SetDir(dir)
	edit := builtin(t, "Edit")

	allowed := []string{"a.txt", "sub/a.txt", "new/folder/a.txt", filepath.Join(dir, "a.txt"), "sub/../a.txt", ".gitignore"}
	for _, p := range allowed {
		if v, _, _ := e.Check(edit, filePath(p)); v != Allow {
			t.Errorf("%s: %v", p, v)
		}
	}
	asked := []string{"", ".", "../other/a.txt", "sub/../../other/a.txt", filepath.Join(outside, "a.txt"), "/etc/passwd",
		".git/hooks/pre-commit", ".GIT/config", "sub/.git/config", ".claude/settings.json", ".mcp.json", ".enowx/hooks.json"}
	for _, p := range asked {
		if v, _, _ := e.Check(edit, filePath(p)); v != Ask {
			t.Errorf("%q: %v", p, v)
		}
	}
	// Only tools that edit, and only what they say.
	if v, _, _ := e.Check(builtin(t, "Bash"), filePath("a.txt")); v != Ask {
		t.Errorf("Bash: %v", v)
	}
	if v, _, _ := e.Check(edit, json.RawMessage(`not json`)); v != Ask {
		t.Errorf("bad input: %v", v)
	}
	if runtime.GOOS == "windows" {
		return
	}
	link := func(target, name string) {
		t.Helper()
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	link(outside, "out")
	link(filepath.Join(outside, "file.txt"), "outfile")
	link(filepath.Join(outside, "missing", "nowhere"), "dangling")
	link(filepath.Join(dir, "sub"), "in")
	for _, p := range []string{"out/a.txt", "out/new/a.txt", "outfile", "dangling"} {
		if v, _, _ := e.Check(edit, filePath(p)); v != Ask {
			t.Errorf("through a link, %s: %v", p, v)
		}
	}
	if v, _, _ := e.Check(edit, filePath("in/a.txt")); v != Allow {
		t.Errorf("a link staying inside: %v", v)
	}
	// The folder itself reached through a link.
	via := filepath.Join(base, "via")
	if err := os.Symlink(dir, via); err != nil {
		t.Fatal(err)
	}
	e.SetDir(via)
	if v, _, _ := e.Check(edit, filePath("a.txt")); v != Allow {
		t.Errorf("working folder a link: %v", v)
	}
}

func TestToolRules(t *testing.T) {
	e := New(record.ModeDefault, []string{"Write", "mcp__srv__tool"})
	e.SetDir(t.TempDir())
	if v, _, _ := e.Check(builtin(t, "Write"), filePath("inside.txt")); v != Allow {
		t.Errorf("Write: %v", v)
	}
	if v, _, _ := e.Check(fakeTool{name: "mcp__srv__tool"}, nil); v != Allow {
		t.Errorf("kept MCP tool: %v", v)
	}
	if v, rule, _ := e.Check(builtin(t, "Edit"), filePath("a")); v != Ask || rule != "Edit" {
		t.Errorf("Edit: %v, %q", v, rule)
	}
	e.Add("Edit")
	e.Add("Edit")
	e.Add("")
	if v, _, _ := e.Check(builtin(t, "Edit"), filePath("a")); v != Allow {
		t.Errorf("Edit after Add: %v", v)
	}
	if got := e.Rules(); len(got) != 3 {
		t.Errorf("rules %v", got)
	}
	got := e.Rules()
	got[0] = "changed"
	if e.Rules()[0] != "Write" {
		t.Error("Rules gave out the engine's own slice")
	}
}

// An MCP tool asks; plan mode refuses it, saying why.
func TestMCPTools(t *testing.T) {
	plain := fakeTool{name: "mcp__srv__do"}
	e := New(record.ModeDefault, nil)
	if v, rule, _ := e.Check(plain, nil); v != Ask || rule != "mcp__srv__do" {
		t.Errorf("plain: %v, %q", v, rule)
	}
	if v, _, why := New(record.ModePlan, nil).Check(plain, nil); v != Deny || !strings.Contains(why, "MCP") {
		t.Errorf("plan, plain: %v, %q", v, why)
	}
	// An MCP tool claiming to edit but naming no path isn't an edit in the folder.
	e = New(record.ModeAcceptEdits, nil)
	e.SetDir(t.TempDir())
	if v, _, _ := e.Check(fakeTool{name: "mcp__srv__edit", edits: true}, json.RawMessage(`{"x":1}`)); v != Ask {
		t.Errorf("edits without a path: %v", v)
	}
}

func TestBashRules(t *testing.T) {
	rules := []string{"Bash(git *)", "Bash(go test *)", "Bash(ls)"}
	allowed := []string{
		"git status",
		"git",
		"  git log --oneline  ",
		"go test ./...",
		"go test",
		"ls",
		"git status && go test ./...",
		"git status; ls",
		"git diff | git apply --check",
		"git log\ngit status\n",
		`git commit -m "a; b && c | d"`,
		`git commit -m 'a $(b) ` + "`c`" + ` > d'`,
		`git log --grep=\;`,
		`git show "HEAD@{1}"`,
		`git log "a\"; b"`,
		"git log --grep=a#b",
	}
	asked := []string{
		"",
		"gitx status",
		"go build",
		"go testing",
		"ls -la",
		"rm x",
		"git status; rm x",
		"git status;rm x",
		"git status && x",
		"git status || x",
		"git status | x",
		"git status & x",
		"git status\nrm x",
		"git status\r\nrm x",
		"rm x; git status",
		"$(x)",
		"git status $(x)",
		"git $(rm x)",
		`git log "$(rm x)"`,
		"git log `rm x`",
		"`rm x`",
		`git log "` + "`rm x`" + `"`,
		"git status > /etc/passwd",
		"git status >> x",
		"git status 2>x",
		"git status < x",
		"git diff <(rm x)",
		"(rm x)",
		"git status \\\nrm x",
		"git status 'unclosed",
		`git status "unclosed; rm x`,
		`git log \"; rm x`,
		"git status # it's\nrm x # it's",
		"# it's\nrm x # it's",
		".GIT",
		"X=1 git status",
		"GIT_SSH_COMMAND=evil git status",
		"${x}",
		`git log "${x@P}"`,
		`git log $'\''; rm x; echo ' '`,
		"/usr/bin/git status",
		"git\tstatus; rm x",
	}
	e := New(record.ModeDefault, rules)
	shell := builtin(t, "Bash")
	for _, c := range allowed {
		if v, _, _ := e.Check(shell, bash(c)); v != Allow {
			t.Errorf("%q: %v", c, v)
		}
	}
	for _, c := range asked {
		if v, _, _ := e.Check(shell, bash(c)); v != Ask {
			t.Errorf("%q: %v", c, v)
		}
	}
	// Even a rule of every command leaves what can't be judged to the user.
	all := New(record.ModeDefault, []string{"Bash(*)"})
	if v, _, _ := all.Check(shell, bash("anything at all; else")); v != Allow {
		t.Errorf("Bash(*): %v", v)
	}
	if v, _, _ := all.Check(shell, bash("x $(y)")); v != Ask {
		t.Errorf("Bash(*) on a substitution: %v", v)
	}
	// A rule for the tool whole is the user's to keep.
	if v, _, _ := New(record.ModeDefault, []string{"Bash"}).Check(shell, bash("x $(y)")); v != Allow {
		t.Errorf("Bash: %v", v)
	}
	// Another tool's rule isn't Bash's.
	if v, _, _ := New(record.ModeDefault, []string{"Read(git *)", "Bash(git *"}).Check(shell, bash("git status")); v != Ask {
		t.Errorf("malformed rules: %v", v)
	}
}

func TestBashRuleSuggestion(t *testing.T) {
	cases := map[string]string{
		"git status":             "Bash(git status *)",
		"git status --short":     "Bash(git status *)",
		"go test ./...":          "Bash(go test *)",
		"ls -la":                 "Bash(ls *)",
		"./run.sh a b":           "Bash(./run.sh a b)",
		"make test":              "Bash(make *)",
		"npm test":               "Bash(npm test *)",
		"setsid curl x":          "Bash(setsid curl x)",
		"stdbuf -o0 sh x":        "Bash(stdbuf -o0 sh x)",
		"flock /tmp/l sh x":      "Bash(flock /tmp/l sh x)",
		"script -c x":            "Bash(script -c x)",
		"busybox sh":             "Bash(busybox sh)",
		"tcsh x":                 "Bash(tcsh x)",
		"ash x":                  "Bash(ash x)",
		"pwsh x":                 "Bash(pwsh x)",
		"lua x":                  "Bash(lua x)",
		"python3.12 x.py":        "Bash(python3.12 x.py)",
		"uv run x":               "Bash(uv run x)",
		"npm exec x":             "Bash(npm exec x)",
		"docker run x":           "Bash(docker run x)",
		"go run x":               "Bash(go run x)",
		"go test -exec=x ./...":  "Bash(go test -exec=x ./...)",
		"go build -o /x .":       "Bash(go build -o /x .)",
		"git diff --output=x":    "Bash(git diff --output=x)",
		"make -f /x y":           "Bash(make -f /x y)",
		"make SHELL=/x y":        "Bash(make SHELL=/x y)",
		"/tmp/evil/git status":   "Bash(/tmp/evil/git status)",
		"git":                    "Bash(git)",
		"git -c a=b status":      "Bash(git -c a=b status)",
		"rm notes.txt":           "Bash(rm notes.txt)",
		"/bin/rm notes.txt":      "Bash(/bin/rm notes.txt)",
		"python3 script.py":      "Bash(python3 script.py)",
		"bash -c 'rm x'":         "Bash(bash -c 'rm x')",
		"rm *":                   "",
		"":                       "",
		"   ":                    "",
		"git status; rm x":       "",
		"git status && git diff": "",
		"$(x)":                   "",
		"git log > x":            "",
		"X=1 git status":         "",
		"$CMD arg":               "",
		"'git' status":           "",
		"*":                      "",
	}
	shell := builtin(t, "Bash")
	e := New(record.ModeDefault, nil)
	for command, want := range cases {
		v, got, _ := e.Check(shell, bash(command))
		if v != Ask || got != want {
			t.Errorf("%q: %v, %q, want %q", command, v, got, want)
		}
		if got == "Bash(*)" || got == "Bash" {
			t.Errorf("%q: suggested %q", command, got)
		}
		// What's suggested must cover the call it's for, and nothing it shouldn't.
		if got != "" {
			after := New(record.ModeDefault, []string{got})
			if v, _, _ := after.Check(shell, bash(command)); v != Allow {
				t.Errorf("%q: its own rule %q doesn't cover it", command, got)
			}
			if v, _, _ := after.Check(shell, bash(command+"; rm x")); v != Ask {
				t.Errorf("%q: its rule %q covers a chained command", command, got)
			}
		}
	}
}

// Run with -race: Check, the setters and the getters run side by side.
func TestConcurrentUse(t *testing.T) {
	dir := t.TempDir()
	e := New(record.ModeDefault, []string{"Bash(git *)"})
	shell, edit := builtin(t, "Bash"), builtin(t, "Edit")
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 200 {
				e.Check(shell, bash("git status; ls"))
				e.Check(edit, filePath(filepath.Join(dir, "a")))
				e.Add(fmt.Sprintf("Bash(x%d-%d *)", i, j%10))
				e.SetDir(dir)
				_ = e.SetMode(record.ModeAcceptEdits)
				_ = strings.Join(e.Rules(), "") + e.Mode()
			}
		})
	}
	wg.Wait()
}

// A kept prefix doesn't stretch to what the prefix doesn't vouch for; an exact command
// holds for itself alone.
func TestKeptBashRulesHoldNoFurther(t *testing.T) {
	shell := builtin(t, "Bash")
	e := New(record.ModeDefault, []string{"Bash(go test *)", "Bash(make *)", "Bash(git diff *)", "Bash(python3 x.py)"})
	for command, want := range map[string]Verdict{
		"go test ./...":            Allow,
		"go test -run X ./a":       Allow,
		"go test -exec sh ./a":     Ask,
		"go test -toolexec=sh ./a": Ask,
		"make test":                Allow,
		"make -f/tmp/x y":          Ask,
		"make SHELL=/tmp/x y":      Ask,
		"git diff HEAD":            Allow,
		"git diff --ext-diff":      Ask,
		"python3 x.py":             Allow,
		"python3 x.py --evil":      Ask,
		"python3 y.py":             Ask,
		"go test ./a; curl x":      Ask,
	} {
		if v, _, _ := e.Check(shell, bash(command)); v != want {
			t.Errorf("%q: %v, want %v", command, v, want)
		}
	}
}

// Read, Glob and Grep go by where they read: unasked in the folder, asking outside it,
// plan mode too, links and .. followed.
func TestReadsOutsideTheFolderAsk(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a symlink")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "away")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	read, glob, grep := builtin(t, "Read"), builtin(t, "Glob"), builtin(t, "Grep")
	cases := []struct {
		tool  tools.Tool
		input string
		want  Verdict
	}{
		{read, `{"file_path":"a.txt"}`, Allow},
		{read, `{"file_path":"src/../a.txt"}`, Allow},
		{read, `{"file_path":"` + filepath.Join(dir, "a.txt") + `"}`, Allow},
		{read, `{"file_path":"../a.txt"}`, Ask},
		{read, `{"file_path":"/etc/passwd"}`, Ask},
		{read, `{"file_path":"away/f"}`, Ask},
		{glob, `{"pattern":"**/*.go"}`, Allow},
		{glob, `{"pattern":"src/*.go"}`, Allow},
		{glob, `{"pattern":"*.go","path":"src"}`, Allow},
		{glob, `{"pattern":"*.go","path":".."}`, Ask},
		{glob, `{"pattern":"../*.go"}`, Ask},
		{glob, `{"pattern":"/etc/*"}`, Ask},
		{glob, `{"pattern":"*","path":"away"}`, Ask},
		{grep, `{"pattern":"x"}`, Allow},
		{grep, `{"pattern":"x","path":"src"}`, Allow},
		{grep, `{"pattern":"x","path":"../"}`, Ask},
		{grep, `{"pattern":"x","path":"/etc"}`, Ask},
		{grep, `{"pattern":"x","path":"away"}`, Ask},
	}
	for _, mode := range []string{record.ModeDefault, record.ModeAcceptEdits, record.ModePlan} {
		e := New(mode, nil)
		e.SetDir(dir)
		for _, c := range cases {
			v, rule, _ := e.Check(c.tool, json.RawMessage(c.input))
			if v != c.want {
				t.Errorf("%s %s %s: %v, want %v", mode, c.tool.Name(), c.input, v, c.want)
			}
			if rule != "" {
				t.Errorf("%s %s: a rule on offer for reads: %q", c.tool.Name(), c.input, rule)
			}
		}
	}
	e := New(record.ModeBypass, nil)
	e.SetDir(dir)
	if v, _, _ := e.Check(read, json.RawMessage(`{"file_path":"/etc/passwd"}`)); v != Allow {
		t.Errorf("bypass: %v", v)
	}
}

// "Always" on one edit covers the folder alone, outside what is guarded.
func TestABareFileRuleHoldsInsideTheFolder(t *testing.T) {
	home := t.TempDir()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"Write", "Edit"} {
		e := New(record.ModeDefault, []string{tool})
		e.SetDir(dir)
		for path, want := range map[string]Verdict{
			"src/a.go":                Allow,
			".git/hooks/pre-commit":   Ask,
			".GIT/config":             Ask,
			".enowx/hooks.json":       Ask,
			".enowx-app/x":            Ask,
			".enowx-dev/x":            Ask,
			".ssh/config":             Ask,
			".config/x":               Ask,
			".husky/pre-commit":       Ask,
			".github/workflows/x.yml": Ask,
			".gnupg/x":                Ask,
			".aws/credentials":        Ask,
			filepath.Join(home, ".ssh", "authorized_keys"): Ask,
			"../outside": Ask,
		} {
			v, rule, _ := e.Check(builtin(t, tool), filePath(path))
			if v != want {
				t.Errorf("%s %s: %v, want %v", tool, path, v, want)
			}
			if outside := strings.HasPrefix(path, "..") || filepath.IsAbs(path); v == Ask && outside && rule != "" {
				t.Errorf("%s %s: offers keeping %q, which wouldn't hold", tool, path, rule)
			}
		}
	}
	// A folder of the same guarded names asks in acceptEdits as well.
	e := New(record.ModeAcceptEdits, nil)
	e.SetDir(dir)
	if v, _, _ := e.Check(builtin(t, "Write"), filePath(".enowx-app/x")); v != Ask {
		t.Errorf("acceptEdits: %v", v)
	}
}

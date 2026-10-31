package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// The command line drives a real daemon: an agent spawned in a project,
// waited for until its hooks say it is done, read, typed into, listed and
// ended, as an agent would through $ENOWX_BIN.
func TestCommandLineDrivesTheDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "enowxd")
	if out, err := exec.Command("go", "build", "-o", bin, "github.com/enowx/enowx/cmd/enowxd").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	// A short home: the socket's path has a length limit.
	home, err := os.MkdirTemp("", "enx")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)
	paths := filepath.Join(tmp, "path")
	if err := os.MkdirAll(paths, 0o755); err != nil {
		t.Fatal(err)
	}
	// A "claude" that says what it was asked, reports working then done
	// through its hook, then echoes what it is sent.
	agent := "#!/bin/sh\nfor a in \"$@\"; do last=\"$a\"; done\necho \"asked: $last\"\n\"$ENOWX_BIN\" hook working\nsleep 1\n\"$ENOWX_BIN\" hook done\nwhile read -r line; do echo \"heard: $line\"; done\n"
	if err := os.WriteFile(filepath.Join(paths, "claude"), []byte(agent), 0o755); err != nil {
		t.Fatal(err)
	}
	// The daemon takes the login shell's PATH: a shell that keeps this one,
	// so the fake agent is the one found.
	shell := filepath.Join(tmp, "sh")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nfor a in \"$@\"; do last=\"$a\"; done\nexec /bin/sh -c \"$last\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "ENOWX_HOME="+home, "SHELL="+shell, "PATH="+paths+string(os.PathListSeparator)+os.Getenv("PATH"), "ENOWX_SESSION=")
	serve := exec.Command(bin, "serve")
	serve.Env = env
	if err := serve.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stop := exec.Command(bin, "stop")
		stop.Env = env
		_ = stop.Run()
		_ = serve.Wait()
	}()
	run := func(args ...string) string {
		t.Helper()
		var out []byte
		var err error
		for range 50 { // the daemon may still be starting
			cmd := exec.Command(bin, args...)
			cmd.Env = env
			if out, err = cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "isn't running") {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			t.Fatalf("enowxd %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	project := t.TempDir()
	id := run("spawn", "--agent", "claude", "--project", project, "--name", "helper", "fix the tests")
	if len(id) != 12 {
		t.Fatalf("spawn printed %q", id)
	}
	if got := run("wait", "--timeout", "20s", "helper"); got != "done" {
		t.Fatalf("wait = %q", got)
	}
	if got := run("wait", "helper", "--timeout", "20s"); got != "done" { // as the help writes it
		t.Fatalf("wait, session first = %q", got)
	}
	if screen := run("read", id[:6]); !strings.Contains(screen, "asked: fix the tests") {
		t.Fatalf("read:\n%s", screen)
	}
	run("note", id, "fixing", "the", "tests")

	// The Run menu and the Ports tool, as an agent of the session uses them.
	run("run", "--session", id, "--save", "Greet", "echo", "hello from the run menu")
	if out := run("run", "--session", id); !strings.Contains(out, "Greet") || !strings.Contains(out, "echo hello from the run menu") {
		t.Fatalf("run list:\n%s", out)
	}
	// It runs in a tab of the session, typed into its login shell (this
	// test's shell only runs -c, so what the tab shows isn't checked).
	tab := strings.Fields(run("run", "--session", id, "Greet"))[0]
	if out := run("ls"); !strings.Contains(out, tab[:12]) || !strings.Contains(out, "Greet") {
		t.Fatalf("no tab Greet:\n%s", out)
	}
	run("kill", tab)
	if out := run("ports", "--session", id); !strings.Contains(out, "No ports open") && !strings.Contains(out, "PORT") {
		t.Fatalf("ports: %s", out)
	}
	if out := run("ls"); !strings.Contains(out, "fixing the tests") {
		t.Fatalf("no note in ls:\n%s", out)
	}
	// One prompt to two agents, each on its own branch.
	fanRepo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", fanRepo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	ids := strings.Fields(run("spawn", "--agents", "claude,claude", "--branch", "try", "--project", fanRepo, "try it"))
	if len(ids) != 2 {
		t.Fatalf("fan-out printed %q", ids)
	}
	if out := run("ls"); !strings.Contains(out, "try-claude") || !strings.Contains(out, "try-claude-2") {
		t.Fatalf("fan-out's sessions in ls:\n%s", out)
	}
	for _, fid := range ids {
		run("close", fid, "--yes")
	}
	run("send", id, "and the docs")
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(run("read", id), "heard: ") {
		if time.Now().After(deadline) {
			t.Fatalf("never heard:\n%s", run("read", id))
		}
		time.Sleep(200 * time.Millisecond)
	}
	if screen := run("read", id); !strings.Contains(screen, "and the docs") {
		t.Fatalf("sent:\n%s", screen)
	}
	var list []proto.SessionInfo
	if err := json.Unmarshal([]byte(run("ls", "--json")), &list); err != nil || len(list) != 1 || list[0].Name != "helper" || list[0].Project != project {
		t.Fatalf("ls: %+v, %v", list, err)
	}
	// Hibernated by hand, its agent done: it says so, and wakes.
	if out := run("hibernate", "helper", "--dry-run"); !strings.Contains(out, "would hibernate") {
		t.Fatalf("hibernate --dry-run:\n%s", out)
	}
	if out := run("hibernate", "helper"); !strings.Contains(out, "hibernated") {
		t.Fatalf("hibernate:\n%s", out)
	}
	if out := run("ls"); !strings.Contains(out, "hibernated(by hand)") {
		t.Fatalf("ls after hibernating:\n%s", out)
	}
	run("wake", "helper")
	if out := run("status", "helper"); strings.Contains(out, "hibernated") {
		t.Fatalf("woken, still %s", out)
	}
	if !strings.Contains(run("agents"), "claude") {
		t.Fatal("agents doesn't list the claude on PATH")
	}
	noYes := exec.Command(bin, "kill", id)
	noYes.Env = env
	if out, err := noYes.CombinedOutput(); err == nil || !strings.Contains(string(out), "--yes") {
		t.Fatalf("kill without --yes: %v\n%s", err, out)
	}
	run("kill", id, "--yes")
	if out := run("ls"); strings.Contains(out, id) {
		t.Fatalf("killed, still listed:\n%s", out)
	}

	// A pipeline: each stage's agent reports done through its hook, and
	// the next one starts, until all three are.
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git: %v\n%s", err, out)
		}
	}
	aid := run("automation", "add", "--project", repo, "--name", "Nightly", "--schedule", "daily 02:00", "--disabled", "Update the dependencies")
	if out := run("automations"); !strings.Contains(out, "Nightly") || !strings.Contains(out, "daily 02:00") || !strings.Contains(out, "off") {
		t.Fatalf("automations:\n%s", out)
	}
	run("automation", "enable", "Nightly")
	if out := run("automations"); strings.Contains(out, " off ") {
		t.Fatalf("still off:\n%s", out)
	}
	run("automation", "remove", aid)

	// Hooks: the user's own are saved at once; an agent's are proposed.
	hid := run("hooks", "add", "--event", "tool.before", "--tool", "rm -rf", "--do", "refuse", "--reason", "No.", "--name", "No rm")
	if out := run("hooks"); !strings.Contains(out, hid) || !strings.Contains(out, "tool.before") || !strings.Contains(out, "block") {
		t.Fatalf("hooks:\n%s", out)
	}
	if out := run("hooks", "test", "No rm"); !strings.Contains(out, "refused") {
		t.Fatalf("hooks test: %s", out)
	}
	asAgent := exec.Command(bin, "hooks", "rm", hid)
	asAgent.Env = append(env, "ENOWX_SESSION=agent")
	if out, err := asAgent.CombinedOutput(); err != nil || !strings.Contains(string(out), "Proposed") {
		t.Fatalf("an agent's removal: %v %s", err, out)
	}
	if out := run("hooks"); !strings.Contains(out, hid) || !strings.Contains(out, "Proposed by session agent") {
		t.Fatalf("after the proposal:\n%s", out)
	}
	pid := run("pipeline", "--project", repo, "Add a search box")
	deadline = time.Now().Add(40 * time.Second)
	for {
		out := run("pipelines")
		if strings.Contains(out, pid+"  done  pipeline/add-a-search-box") {
			if strings.Count(out, ":done(") != 3 {
				t.Fatalf("pipelines:\n%s", out)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pipeline never done:\n%s", out)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

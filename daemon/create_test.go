package daemon

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

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"},
		{"branch", "fix/old-bug"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// Resolve symlinks (macOS /var -> /private/var) so paths compare equal.
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestSessionPlacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv := startServer(t)
	c := connect(t, srv, nil)
	repo := gitRepo(t)
	folder := t.TempDir()
	if _, err := c.AddProject(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddProject(folder); err != nil {
		t.Fatal(err)
	}
	projects, err := c.Projects()
	if err != nil || len(projects) != 2 || !projects[0].IsGit || projects[1].IsGit {
		t.Fatalf("projects = %+v, %v", projects, err)
	}
	detail, err := c.InspectProject(repo)
	if err != nil || detail.Branch != "main" || len(detail.Branches) != 2 {
		t.Fatalf("detail = %+v, %v", detail, err)
	}

	shell := []string{"sh", "-c", "sleep 30"}
	cases := []struct {
		name         string
		params       proto.SessionCreateParams
		wantWorktree bool
		wantDir      string
		wantBranch   string
	}{
		{"project folder", proto.SessionCreateParams{Project: repo, Command: shell}, false, repo, "main"},
		{"new branch", proto.SessionCreateParams{Project: repo, BranchMode: proto.BranchNew, Branch: "feat/search", Command: shell}, true, "", "feat/search"},
		{"existing branch", proto.SessionCreateParams{Project: repo, BranchMode: proto.BranchExisting, Branch: "fix/old-bug", Command: shell}, true, "", "fix/old-bug"},
		{"current branch", proto.SessionCreateParams{Project: repo, BranchMode: proto.BranchExisting, Branch: "main", Command: shell}, false, repo, "main"},
		{"plain folder", proto.SessionCreateParams{Project: folder, Command: shell}, false, folder, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := c.CreateSession(tc.params)
			if err != nil {
				t.Fatal(err)
			}
			if info.Worktree != tc.wantWorktree || info.Branch != tc.wantBranch || info.Project != tc.params.Project {
				t.Fatalf("info = %+v", info)
			}
			if tc.wantDir != "" && info.Dir != tc.wantDir {
				t.Fatalf("dir = %s, want %s", info.Dir, tc.wantDir)
			}
			if tc.wantWorktree {
				head, err := exec.Command("git", "-C", info.Dir, "branch", "--show-current").Output()
				if err != nil || strings.TrimSpace(string(head)) != tc.wantBranch {
					t.Fatalf("worktree branch = %q, %v", head, err)
				}
			}
		})
	}

	for _, bad := range []proto.SessionCreateParams{
		{Project: repo, BranchMode: proto.BranchNew, Branch: "fix/old-bug"}, // exists
		{Project: repo, BranchMode: proto.BranchExisting, Branch: "nope"},   // missing
		{Project: folder, BranchMode: proto.BranchNew, Branch: "feat/x"},    // not git
		{Project: repo, BranchMode: proto.BranchNew, Branch: "bad..name"},   // invalid
	} {
		if _, err := c.CreateSession(bad); err == nil {
			t.Errorf("CreateSession(%+v) succeeded", bad)
		}
	}
}

func TestPromptIsTypedIntoAgentsWithoutPromptArgument(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv := startServer(t)
	c := connect(t, srv, nil)
	// "custom" agents get the prompt typed once their screen settles.
	info, err := c.CreateSession(proto.SessionCreateParams{
		Agent: "custom", Command: []string{"sh", "-c", "echo ready; read line; echo got:$line; sleep 30"},
		Dir: os.TempDir(), Prompt: "hello agent", Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	watch(st).waitFor(t, "got:hello agent")
}

func TestFirstTypedLineBecomesThePrompt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	srv := startServer(t)
	updates := make(chan proto.SessionInfo, 64)
	c := connect(t, srv, func(ev proto.Event) {
		var info proto.SessionInfo
		if ev.Name == proto.EventSessionUpdated && json.Unmarshal(ev.Data, &info) == nil {
			updates <- info
		}
	})
	info, err := c.CreateSession(proto.SessionCreateParams{Command: []string{"cat"}, Dir: os.TempDir(), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	// An arrow key, a typo fixed with backspace, then Enter.
	if _, err := st.Write([]byte("\x1b[Afix the logn\x7fin bug\r")); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case u := <-updates:
			if u.ID == info.ID && u.Prompt != "" {
				if u.Prompt != "fix the login bug" {
					t.Fatalf("prompt = %q", u.Prompt)
				}
				return
			}
		case <-deadline:
			t.Fatal("no prompt update")
		}
	}
}

// Sign-in runs the agent's sign-in command in the project folder, never
// in a new worktree, and names the session after it.
func TestSignInRunsInTheProjectFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	fake := "#!/bin/sh\necho \"signing-in:$*\"\n"
	if err := os.WriteFile(filepath.Join(bin, "kiro-cli"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(filepath.ListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	c := connect(t, srv, nil)
	repo := gitRepo(t)
	if _, err := c.AddProject(repo); err != nil {
		t.Fatal(err)
	}
	info, err := c.CreateSession(proto.SessionCreateParams{
		Project: repo, Agent: "kiro", SignIn: true, BranchMode: proto.BranchNew, Branch: "feat/x", Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.Worktree || info.Dir != repo || info.Name != "Sign in to Kiro CLI" {
		t.Fatalf("info = %+v", info)
	}
	st, err := c.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	watch(st).waitFor(t, "signing-in:login --use-device-flow")

	if _, err := c.CreateSession(proto.SessionCreateParams{Project: repo, Agent: "custom", SignIn: true}); err == nil {
		t.Fatal("sign-in to an agent without one succeeded")
	}
}

// Switching mode cycles the agent with its key until its screen says the
// mode asked for.
func TestSwitchModeCyclesUntilTheScreenSaysIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	fake := `#!/bin/sh
i=0
show() {
	case $((i % 5)) in
	0) m="auto mode on" ;; 1) m="manual mode on" ;; 2) m="accept edits on" ;;
	3) m="plan mode on" ;; *) m="bypass permissions on" ;;
	esac
	printf '\r\nmode: %s' "$m"
}
show
stty raw -echo
while dd bs=3 count=1 >/dev/null 2>&1; do i=$((i + 1)); show; done
`
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(filepath.ListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	c := connect(t, srv, nil)
	info, err := c.CreateSession(proto.SessionCreateParams{Agent: "claude", Dir: os.TempDir(), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	s := watch(st)
	s.waitFor(t, "auto mode on")
	if err := c.SwitchMode(info.ID, "plan"); err != nil {
		t.Fatal(err)
	}
	s.waitFor(t, "plan mode on")
	time.Sleep(modeRedraw / 5)
	sess, _ := srv.sessions.Get(info.ID)
	if text := sess.Terminal().Text(); strings.Contains(text, "bypass permissions on") {
		t.Fatalf("cycled past plan:\n%s", text)
	}
	if err := c.SwitchMode(info.ID, "nonsense"); err == nil {
		t.Fatal("switched to an unknown mode")
	}
}

// An agent that takes no prompt on its command line gets it typed in once
// it shows its input, and skips permissions through its environment where
// that is how it does.
func TestAgentsTakeTheirPromptAsTheyCan(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // where the agent is given its guide
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\n[ \"$1\" = --help ] && { echo session; exit 0; }\necho \"mode=$GOOSE_MODE args=$*\"\nprintf '> '\nread -r line\necho \"got: $line\"\nsleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "goose"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	sess, err := srv.createSession(proto.SessionCreateParams{Agent: "goose", Dir: t.TempDir(), Prompt: "build the thing", SkipPermissions: true, Cols: 100, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the prompt typed in", func() bool {
		text := sess.Terminal().Text()
		return strings.Contains(text, "mode=auto args=") && strings.Contains(text, "got: ") && strings.Contains(text, "build the thing")
	})
	if text := sess.Terminal().Text(); strings.Contains(text, "args=build") {
		t.Fatalf("the prompt went on the command line too:\n%s", text)
	}
}

// A new agent that asks to trust its folder is answered yes, and its typed
// prompt waits until then.
func TestNewAgentsAreTrustedTheirFolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // where the agent is given its guide
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf 'Do you trust the files in this folder?\\n> 1. Yes, trust it\\n  2. No, exit\\n'\nread -r answer\necho trusted\nprintf '> '\nread -r line\necho \"got: $line\"\nsleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "amp"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	sess, err := srv.createSession(proto.SessionCreateParams{Agent: "amp", Dir: t.TempDir(), Prompt: "ship it", Cols: 100, Rows: 12})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if t.Failed() {
			t.Log(sess.Terminal().Text())
		}
	}()
	waitFor(t, "trusted, then the prompt", func() bool {
		text := sess.Terminal().Text()
		return strings.Contains(text, "trusted") && strings.Contains(text, "got: ") && strings.Contains(text, "ship it")
	})
}

// An agent that selects "No" first has the selection moved to yes before
// Enter.
func TestTrustMovesOffNo(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // where the agent is given its guide
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	// It redraws with the selection moved on an arrow key, and says so
	// once Enter comes on yes.
	script := "#!/bin/sh\nq='Is this a project you trust? (Like your own code)'\nprintf '%s\\n> No, exit\\n  Yes, I trust this folder\\n' \"$q\"\n" +
		"stty -icanon -echo min 3; key=$(dd bs=3 count=1 2>/dev/null); stty icanon echo\n" +
		"case \"$key\" in *'[B') ;; *) echo exited; exit 1 ;; esac\n" +
		"printf '\\033[2J\\033[H%s\\n  No, exit\\n> Yes, I trust this folder\\n' \"$q\"\nread -r answer\necho moved-to-yes\nsleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "amp"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv := startServer(t)
	sess, err := srv.createSession(proto.SessionCreateParams{Agent: "amp", Dir: t.TempDir(), Cols: 100, Rows: 12})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the selection moved to yes", func() bool { return strings.Contains(sess.Terminal().Text(), "moved-to-yes") })
}

// A new worktree gets the ignored files the project's .enowx.toml names,
// and its setup runs in a tab only when asked.
func TestNewWorktreesGetWhatTheProjectSays(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	fakeAgent(t)
	repo := gitRepo(t)
	for name, content := range map[string]string{
		".gitignore":  ".env\n",
		".env":        "TOKEN=x\n",
		".enowx.toml": "[worktree]\ncopy = [\".env\", \"../outside\"]\nsetup = \"echo ready > SETUP_RAN\"\n",
	} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	srv := startServer(t)
	c := connect(t, srv, nil)
	detail, err := c.InspectProject(repo)
	if err != nil || detail.Setup != "echo ready > SETUP_RAN" || len(detail.Copy) != 2 {
		t.Fatalf("detail %+v, %v", detail, err)
	}
	info, err := c.CreateSession(proto.SessionCreateParams{Project: repo, Agent: "claude", BranchMode: proto.BranchNew, Branch: "feat/a", RunSetup: true, Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	if env, err := os.ReadFile(filepath.Join(info.Dir, ".env")); err != nil || string(env) != "TOKEN=x\n" {
		t.Fatalf(".env in the worktree: %q, %v", env, err)
	}
	waitFor(t, "the setup to run", func() bool {
		_, err := os.Stat(filepath.Join(info.Dir, "SETUP_RAN"))
		return err == nil
	})
	sessions, _ := c.Sessions()
	tabs := 0
	for _, s := range sessions {
		if s.Parent == info.ID && s.Name == "Setup" {
			tabs++
		}
	}
	if tabs != 1 {
		t.Fatalf("%d Setup tabs: %+v", tabs, sessions)
	}
	// Not asked: the files come, the script doesn't run.
	other, err := c.CreateSession(proto.SessionCreateParams{Project: repo, Agent: "claude", BranchMode: proto.BranchNew, Branch: "feat/b", Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(other.Dir, "SETUP_RAN")); err == nil {
		t.Fatal("the setup ran unasked")
	}
	if _, err := os.Stat(filepath.Join(other.Dir, ".env")); err != nil {
		t.Fatal("no .env without setup")
	}
}

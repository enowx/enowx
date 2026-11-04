package daemon

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
)

// rulesHost is a daemon whose sessions run stand-ins for agents ids, its hooks
// reachable as an agent's are.
func rulesHost(t *testing.T, ids ...string) (string, *Server, *client.Client) {
	t.Helper()
	bin := guideAgents(t, ids...)
	t.Setenv("CODEX_HOME", t.TempDir())
	srv, _ := hookServer(t)
	return bin, srv, connect(t, srv, nil)
}

func rulesSession(t *testing.T, c *client.Client, p proto.SessionCreateParams) proto.SessionInfo {
	t.Helper()
	p.Cols, p.Rows = 80, 24
	if p.Project != "" {
		p.BranchMode = proto.BranchNone
	}
	info, err := c.CreateSession(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(session.EnvSession, info.ID)
	return info
}

func hookPrompt() string {
	_, out, _ := hookOf(`{"prompt":"next task"}`, "--event", proto.HookPromptStart)
	return out
}

func hookStart(source string) string {
	_, out, _ := hookOf(`{"source":"`+source+`"}`, "--event", proto.HookSessionStart)
	return out
}

func saveRules(t *testing.T, c *client.Client, dir, rules string) {
	t.Helper()
	if _, err := c.SaveProjectRules(proto.ProjectRules{Path: dir, Rules: rules}); err != nil {
		t.Fatal(err)
	}
}

func rulesOf(t *testing.T, srv *Server, id string) proto.SessionRules {
	t.Helper()
	sess, err := srv.sessions.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	r := sess.Info().Rules
	if r == nil {
		t.Fatal("no record of its rules")
	}
	return *r
}

// A project's rules changing reaches a running session at its next prompt, once, and
// says so in the session; rules unchanged aren't sent again.
func TestProjectRulesChangeReachesTheNextPromptOnce(t *testing.T) {
	_, srv, c := rulesHost(t, "claude")
	dir := gitRepo(t)
	saveRules(t, c, dir, "Use tabs.")
	info := rulesSession(t, c, proto.SessionCreateParams{Agent: "claude", Project: dir})
	v1 := rulesVersion("Use tabs.")
	if r := rulesOf(t, srv, info.ID); r.Version != v1 || r.Via != proto.RulesViaStart || !r.Hook {
		t.Fatalf("as it started: %+v", r)
	}
	if out := hookPrompt(); strings.Contains(out, "Use tabs.") {
		t.Fatalf("its first prompt told what it has: %s", out)
	}
	saveRules(t, c, dir, "Use spaces.")
	v2 := rulesVersion("Use spaces.")
	if r := rulesOf(t, srv, info.ID); r.Version != v1 || r.Current != v2 {
		t.Fatalf("waiting for its prompt: %+v", r)
	}
	if out := hookPrompt(); !strings.Contains(out, "Use spaces.") || !strings.Contains(out, v2) || !strings.Contains(out, "replace") {
		t.Fatalf("the change not told: %s", out)
	}
	if r := rulesOf(t, srv, info.ID); r.Version != v2 || r.Via != proto.RulesViaHook || r.At.IsZero() {
		t.Fatalf("after: %+v", r)
	}
	if out := hookPrompt(); out != "" {
		t.Fatalf("told twice: %s", out)
	}
	if _, err := c.SaveProjectRules(proto.ProjectRules{Path: dir, Clear: true}); err != nil {
		t.Fatal(err)
	}
	if out := hookPrompt(); !strings.Contains(out, "removed") {
		t.Fatalf("removal not told: %s", out)
	}
	if out := hookPrompt(); out != "" {
		t.Fatalf("removal told twice: %s", out)
	}
}

// Rules changed between a session's start and its first prompt reach it: its
// guide had the old ones. For a project's, and for GOD of Agents'.
func TestRulesChangedBeforeTheFirstPrompt(t *testing.T) {
	_, _, c := rulesHost(t, "claude")
	dir := gitRepo(t)
	saveRules(t, c, dir, "Use tabs.")
	rulesSession(t, c, proto.SessionCreateParams{Agent: "claude", Project: dir})
	saveRules(t, c, dir, "Use spaces.")
	if out := hookPrompt(); !strings.Contains(out, "Use spaces.") {
		t.Fatalf("a project's: %q", out)
	}
	if out := hookPrompt(); out != "" {
		t.Fatalf("a project's, twice: %q", out)
	}

	if err := c.Call(proto.MethodGOARulesSave, proto.GOARules{Rules: "- Old"}, nil); err != nil {
		t.Fatal(err)
	}
	rulesSession(t, c, proto.SessionCreateParams{Agent: "claude", GOA: true})
	if err := c.Call(proto.MethodGOARulesSave, proto.GOARules{Rules: "- New"}, nil); err != nil {
		t.Fatal(err)
	}
	if out := hookPrompt(); !strings.Contains(out, "- New") {
		t.Fatalf("GOA's: %q", out)
	}
	if out := hookPrompt(); out != "" {
		t.Fatalf("GOA's, twice: %q", out)
	}
}

// What a session last got outlasts the daemon: rules changed while it was down reach
// its agent coming back, which gets them once, and the record stays.
func TestRulesOutlastADaemonRestart(t *testing.T) {
	guideAgents(t, "claude", "codex")
	t.Setenv("CODEX_HOME", t.TempDir())
	home, err := os.MkdirTemp("", "enx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	t.Setenv("ENOWX_HOME", home)
	path, err := transport.SocketPath()
	if err != nil {
		t.Fatal(err)
	}
	data := t.TempDir()
	var listening net.Listener
	start := func() (*Server, *client.Client) {
		srv, err := NewServer(data)
		if err != nil {
			t.Fatal(err)
		}
		l, err := transport.Listen(path)
		if err != nil {
			t.Fatal(err)
		}
		listening = l
		go srv.Serve(l)
		t.Cleanup(func() { l.Close() })
		return srv, connect(t, srv, nil)
	}
	srv, c := start()
	dir := gitRepo(t)
	saveRules(t, c, dir, "Use tabs.")
	cl := rulesSession(t, c, proto.SessionCreateParams{Agent: "claude", Dir: dir})
	cx := rulesSession(t, c, proto.SessionCreateParams{Agent: "codex", Dir: dir})
	if out := hookPrompt(); !strings.Contains(out, "Use tabs.") {
		t.Fatalf("codex's first prompt: %q", out)
	}
	v1 := rulesVersion("Use tabs.")
	if r := rulesOf(t, srv, cx.ID); r.Version != v1 {
		t.Fatalf("%+v", r)
	}
	file, _ := srv.ownRulesPlacesOf(dir).found()
	srv.Close()
	listening.Close()

	if err := writeTeamFile(file, []byte("Use spaces.\n")); err != nil {
		t.Fatal(err)
	}
	again, _ := start()
	t.Cleanup(again.Close)
	v2 := rulesVersion("Use spaces.")
	if r := rulesOf(t, again, cx.ID); r.Version != v1 || r.Current != v2 || !r.Again {
		t.Fatalf("codex's record after the restart: %+v", r)
	}
	t.Setenv(session.EnvSession, cx.ID)
	if out := hookPrompt(); !strings.Contains(out, "Use spaces.") || !strings.Contains(out, "replace") {
		t.Fatalf("codex, what changed while down: %q", out)
	}
	if out := hookPrompt(); out != "" {
		t.Fatalf("codex, twice: %q", out)
	}
	// Claude Code's guide is written again, with what a folder of no project has.
	t.Setenv(session.EnvSession, cl.ID)
	data2, err := os.ReadFile(filepath.Join(again.agentFilesDir(cl.ID), "system-prompt.md"))
	if err != nil || !strings.Contains(string(data2), "Use spaces.") {
		t.Fatalf("claude's guide after a restart: %.200s %v", data2, err)
	}
	if r := rulesOf(t, again, cl.ID); r.Version != v2 || !r.Again {
		t.Fatalf("claude's record: %+v", r)
	}
	if out := hookStart("resume"); !strings.Contains(out, "Use spaces.") {
		t.Fatalf("claude resuming: %q", out)
	}
	if out := hookPrompt(); out != "" {
		t.Fatalf("claude, twice: %q", out)
	}
}

// The rules reach a session however its folder is spelled: another case, a link, a
// worktree, a subfolder.
func TestRulesReachASessionBySpellingAndWorktree(t *testing.T) {
	srv, _ := hookServer(t)
	main := plainFolder(t)
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", main}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	sub := filepath.Join(main, "src")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(filepath.Dir(main), "wt")
	if out, err := exec.Command("git", "-C", main, "worktree", "add", "-q", "-b", "side", wt).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	link := filepath.Join(filepath.Dir(main), "link")
	if err := os.Symlink(main, link); err != nil {
		t.Skip(err)
	}
	if _, err := srv.saveProjectRules(proto.ProjectRules{Path: main, Rules: "Use tabs."}); err != nil {
		t.Fatal(err)
	}
	lower := filepath.Join(filepath.Dir(main), "myproject")
	for _, c := range []struct {
		name, dir string
		meta      session.Meta
	}{
		{"lowercase project", main, session.Meta{Agent: "claude", Project: lower}},
		{"link", link, session.Meta{Agent: "claude", Project: link}},
		{"worktree of a project", wt, session.Meta{Agent: "claude", Project: main, Worktree: true}},
		{"worktree alone", wt, session.Meta{Agent: "claude"}},
		{"subfolder", sub, session.Meta{Agent: "claude"}},
	} {
		sess, err := srv.sessions.Create(session.Spec{Command: []string{"sh", "-c", "cat"}, Dir: c.dir, Cols: 80, Rows: 24, Meta: c.meta})
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv(session.EnvSession, sess.ID())
		if out := hookPrompt(); !strings.Contains(out, "Use tabs.") {
			t.Fatalf("%s: %q", c.name, out)
		}
		if out := hookPrompt(); out != "" {
			t.Fatalf("%s, twice: %q", c.name, out)
		}
		if g := srv.guideIn(c.meta, c.dir); !strings.Contains(g, "Use tabs.") {
			t.Fatalf("%s: not in its guide", c.name)
		}
	}
}

// Compacting drops what hooks said: the rules go again once, at the session start
// following it where the agent has one, else with the next prompt.
func TestRulesGivenAgainAfterCompaction(t *testing.T) {
	_, _, c := rulesHost(t, "claude")
	dir := gitRepo(t)
	saveRules(t, c, dir, "Use tabs.")
	rulesSession(t, c, proto.SessionCreateParams{Agent: "claude", Project: dir})
	if out := hookStart("startup"); out != "" {
		t.Fatalf("at its start, what it has: %q", out)
	}
	if out := hookPrompt(); out != "" {
		t.Fatalf("first prompt: %q", out)
	}
	compact := func() string {
		_, out, _ := hookOf(`{"trigger":"auto"}`, "--event", proto.HookCompact)
		return out
	}
	if out := compact(); out != "" {
		t.Fatalf("before compacting: %q", out)
	}
	if out := hookPrompt(); !strings.Contains(out, "Use tabs.") || strings.Contains(out, "replace") {
		t.Fatalf("the prompt after compacting: %q", out)
	}
	if out := hookPrompt(); out != "" {
		t.Fatalf("again: %q", out)
	}
	compact()
	if out := hookStart("compact"); !strings.Contains(out, "Use tabs.") {
		t.Fatalf("the session start after compacting: %q", out)
	}
	if out := hookPrompt(); out != "" {
		t.Fatalf("given twice: %q", out)
	}
	if out := hookStart("compact"); !strings.Contains(out, "Use tabs.") {
		t.Fatalf("compacted with no hook before it: %q", out)
	}
	if out := hookStart("clear"); !strings.Contains(out, "Use tabs.") {
		t.Fatalf("cleared: %q", out)
	}
	// Compacting with no rules says nothing.
	if _, err := c.SaveProjectRules(proto.ProjectRules{Path: dir, Clear: true}); err != nil {
		t.Fatal(err)
	}
	hookPrompt()
	compact()
	if out := hookPrompt(); out != "" {
		t.Fatalf("no rules: %q", out)
	}
}

// Codex is only told to read its guide, so its hooks give the rules too, once.
func TestCodexGetsRulesFromItsHookOnce(t *testing.T) {
	_, srv, c := rulesHost(t, "codex")
	dir := gitRepo(t)
	saveRules(t, c, dir, "Use tabs.")
	info := rulesSession(t, c, proto.SessionCreateParams{Agent: "codex", Project: dir})
	if r := rulesOf(t, srv, info.ID); r.Version != "" || !r.Hook {
		t.Fatalf("as it started: %+v", r)
	}
	if out := hookStart("startup"); !strings.Contains(out, "Use tabs.") || !strings.Contains(out, rulesVersion("Use tabs.")) {
		t.Fatalf("its start: %q", out)
	}
	if out := hookPrompt(); out != "" {
		t.Fatalf("twice: %q", out)
	}
}

// An agent with no hooks told to read its guide finds the current rules there with their
// version, and the session says that is all that's known. A secret never gets in.
func TestAgentWithoutHooksReadsCurrentRulesFromItsGuide(t *testing.T) {
	bin, srv, c := rulesHost(t, "grok")
	dir := gitRepo(t)
	info := rulesSession(t, c, proto.SessionCreateParams{Agent: "grok", Project: dir})
	args := startedWith(t, bin, "grok")
	file := filepath.Join(srv.agentFilesDir(info.ID), "ENOWX.md")
	if !strings.Contains(valueOf(args, "--rules"), file) {
		t.Fatalf("told to read: %s", valueOf(args, "--rules"))
	}
	saveRules(t, c, dir, "Use spaces.")
	text, err := os.ReadFile(file)
	v := rulesVersion("Use spaces.")
	if err != nil || !strings.Contains(string(text), "Use spaces.") || !strings.Contains(string(text), "Version of these rules: "+v) {
		t.Fatalf("its guide: %v %.100s", err, text)
	}
	if r := rulesOf(t, srv, info.ID); r.Version != v || r.Via != proto.RulesViaFile || r.Hook {
		t.Fatalf("%+v", r)
	}

	stored, _ := srv.ownRulesPlacesOf(dir).found()
	secret := "AKIA" + "ABCDEFGHIJKLMNOP"
	if err := writeTeamFile(stored, []byte("key "+secret+"\n")); err != nil {
		t.Fatal(err)
	}
	srv.refreshRules()
	text, _ = os.ReadFile(file)
	if strings.Contains(string(text), secret) {
		t.Fatal("a secret reached the guide's file")
	}
}

// A hook's answer holds no secret either, for rules that came by sync, or too long a text.
func TestRulesThroughAHookHoldNoSecretAndFit(t *testing.T) {
	_, srv, c := rulesHost(t, "claude")
	dir := gitRepo(t)
	saveRules(t, c, dir, "Use tabs.")
	info := rulesSession(t, c, proto.SessionCreateParams{Agent: "claude", Project: dir})
	stored, _ := srv.ownRulesPlacesOf(dir).found()
	secret := "AKIA" + "ABCDEFGHIJKLMNOP"
	if err := writeTeamFile(stored, []byte("key "+secret+"\n")); err != nil {
		t.Fatal(err)
	}
	if out := hookPrompt(); out == "" || strings.Contains(out, secret) {
		t.Fatalf("%q", out)
	}
	long := strings.Repeat("- Keep each rule on its line.\n", 400)
	saveRules(t, c, dir, long)
	out := hookPrompt()
	if len(out) > rulesInHookMax || strings.Contains(out, "Keep each rule") || !strings.Contains(out, "RULES.md") {
		t.Fatalf("%d: %.200s", len(out), out)
	}
	kept, err := os.ReadFile(filepath.Join(srv.agentFilesDir(info.ID), "RULES.md"))
	if err != nil || !strings.Contains(string(kept), "Keep each rule") {
		t.Fatalf("%v %.100s", err, kept)
	}
}

// The guide printed for an agent with no other way holds its project's rules, and
// running it counts; one with its own way gets them only when it reads.
func TestGuideCommandGivesTheRules(t *testing.T) {
	_, srv, c := rulesHost(t, "cursor-agent", "claude")
	dir := gitRepo(t)
	saveRules(t, c, dir, "Use tabs.")
	v := rulesVersion("Use tabs.")
	info := rulesSession(t, c, proto.SessionCreateParams{Agent: "cursor", Project: dir})
	if r := rulesOf(t, srv, info.ID); r.Version != "" || r.Current != v {
		t.Fatalf("not known read: %+v", r)
	}
	g, err := c.FeaturesGuide(proto.FeaturesParams{Session: info.ID})
	if err != nil || !strings.Contains(g.Guide, "Use tabs.") {
		t.Fatalf("its MCP's: %v", err)
	}
	if r := rulesOf(t, srv, info.ID); r.Version != "" {
		t.Fatalf("a program's asking counted as read: %+v", r)
	}
	g, err = c.FeaturesGuide(proto.FeaturesParams{Session: info.ID, Reading: true})
	if err != nil || !strings.Contains(g.Guide, "Use tabs.") || !strings.Contains(g.Guide, "Version of these rules: "+v) {
		t.Fatalf("run: %v", err)
	}
	if r := rulesOf(t, srv, info.ID); r.Version != v || r.Via != proto.RulesViaAsked {
		t.Fatalf("%+v", r)
	}

	loaded := rulesSession(t, c, proto.SessionCreateParams{Agent: "claude", Project: dir})
	if g, _ := c.FeaturesGuide(proto.FeaturesParams{Session: loaded.ID}); strings.Contains(g.Guide, "Use tabs.") {
		t.Fatal("given again to an agent that has them, by its MCP server")
	}
}

// A file of the agent's home serves all its sessions, so it holds no one project's rules;
// an agent that reads a file by name gets the session's own.
func TestOneProjectsRulesStayOutOfSharedFiles(t *testing.T) {
	bin, srv, c := rulesHost(t, "kiro-cli", "aider", "kilo")
	dir := gitRepo(t)
	saveRules(t, c, dir, "Use tabs.")
	rulesSession(t, c, proto.SessionCreateParams{Agent: "kiro", Project: dir})
	startedWith(t, bin, "kiro-cli")
	home, _ := os.UserHomeDir()
	text, err := os.ReadFile(filepath.Join(home, ".kiro", "steering", "enowx.md"))
	if err != nil || strings.Contains(string(text), "Use tabs.") || !strings.Contains(string(text), "guide` prints them") {
		t.Fatalf("kiro's: %v %.200s", err, text)
	}
	host, err := os.ReadFile(filepath.Join(srv.dataDir, "ENOWX.md"))
	if err != nil || strings.Contains(string(host), "Use tabs.") {
		t.Fatalf("the host's: %v", err)
	}

	info := rulesSession(t, c, proto.SessionCreateParams{Agent: "aider", Project: dir})
	read := valueOf(startedWith(t, bin, "aider"), "--read")
	if read != filepath.Join(srv.agentFilesDir(info.ID), "agent-guide.md") {
		t.Fatalf("aider reads %s", read)
	}
	saveRules(t, c, dir, "Use spaces.")
	if text, err := os.ReadFile(read); err != nil || !strings.Contains(string(text), "Use spaces.") {
		t.Fatalf("aider's: %v %.200s", err, text)
	}
	kilo := rulesSession(t, c, proto.SessionCreateParams{Agent: "kilo", Project: dir})
	var env []byte
	waitFor(t, "kilo started", func() bool {
		env, _ = os.ReadFile(filepath.Join(bin, "kilo.env"))
		return len(env) > 0
	})
	if !strings.Contains(string(env), filepath.Join(srv.agentFilesDir(kilo.ID), "agent-guide.md")) {
		t.Fatalf("kilo's config names another file")
	}
}

// A tab's agent has its session's rules: in its guide, and from its hooks.
func TestATabsAgentHasItsSessionsRules(t *testing.T) {
	bin, srv, c := rulesHost(t, "grok", "claude")
	dir := gitRepo(t)
	saveRules(t, c, dir, "Use tabs.")
	parent := rulesSession(t, c, proto.SessionCreateParams{Agent: "grok", Project: dir})
	tab := rulesSession(t, c, proto.SessionCreateParams{Agent: "claude", Parent: parent.ID})
	startedWith(t, bin, "claude")
	text, err := os.ReadFile(filepath.Join(srv.agentFilesDir(tab.ID), "system-prompt.md"))
	if err != nil || !strings.Contains(string(text), "Use tabs.") {
		t.Fatalf("its guide: %v", err)
	}
	saveRules(t, c, dir, "Use spaces.")
	if out := hookPrompt(); !strings.Contains(out, "Use spaces.") {
		t.Fatalf("its hook: %q", out)
	}
}

//go:build !windows

package daemon

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/decide"
	"github.com/enowx/enowx/internal/standinapi"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
)

// Claude Platform as it really runs: the built enowxd serving its own socket from a home
// of the test's, its sessions the program itself, against the stand-in Messages API. The
// key is only in the daemon's environment, as the vault's would be in the session's.

const e2eKey = "standin-fake-key"

// e2eBin is the program built for TestPlatformE2E, in its temporary folder.
var e2eBin string

// buildE2EProgram builds enowxd into a folder of t's.
func buildE2EProgram(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "enowxd")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/enowxd").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// e2e is one daemon of the built program, with its stand-in.
type e2e struct {
	t          *testing.T
	home, data string // HOME and ENOWX_HOME
	work       string // a project folder, not under git
	api        *standinapi.Server
	c          *client.Client
	daemon     *exec.Cmd
}

type e2eOptions struct {
	noKey   bool
	prepare func(data string) // before the daemon starts
}

func startE2E(t *testing.T, o e2eOptions) *e2e {
	t.Helper()
	root, err := os.MkdirTemp("", "cpe2e")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	e := &e2e{t: t, home: filepath.Join(root, "h"), data: filepath.Join(root, "h", ".enowx-app"), work: filepath.Join(root, "work")}
	for _, dir := range []string{e.data, e.work} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(e.work, "README.md"), []byte("hello readme\nsecond line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if o.prepare != nil {
		o.prepare(e.data)
	}
	e.api = standinapi.New()
	t.Cleanup(e.api.Close)

	// Named one by one: nothing of an outer session's (ENOWX_SESSION, its socket) gets in.
	env := []string{
		"HOME=" + e.home, "ENOWX_HOME=" + e.data, "PATH=" + os.Getenv("PATH"), "TERM=xterm-256color",
		"SHELL=/bin/sh", "TMPDIR=" + root, decide.EnvFindNone + "=1", "ANTHROPIC_BASE_URL=" + e.api.URL,
	}
	if !o.noKey {
		env = append(env, "ANTHROPIC_API_KEY="+e2eKey)
	}
	e.daemon = exec.Command(e2eBin, "serve")
	e.daemon.Env = env
	e.daemon.Dir = e.home
	if err := e.daemon.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.assertNoKeyOnDisk) // after the daemon's end, what it writes last included
	t.Cleanup(e.stopDaemon)

	sock, err := transport.SocketPathIn(e.data)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		if conn, err := transport.DialLocal(sock, nil); err == nil {
			if e.c, err = client.New(conn, nil); err != nil {
				t.Fatal(err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the daemon never listened on " + sock)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Cleanup(func() { e.c.Close() })
	return e
}

func (e *e2e) stopDaemon() {
	if e.daemon == nil || e.daemon.Process == nil {
		return
	}
	e.assertNoKeyInCommandLines()
	_ = e.daemon.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { e.daemon.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(40 * time.Second):
		e.t.Error("the daemon didn't stop within 40s of its SIGTERM")
		_ = e.daemon.Process.Kill()
		<-done
	}
	e.daemon = nil
}

// assertNoKeyOnDisk is check 12: nothing under the daemon's home holds the key.
func (e *e2e) assertNoKeyOnDisk() {
	e.t.Helper()
	_ = filepath.WalkDir(filepath.Dir(e.home), func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), e2eKey) {
				e.t.Errorf("the key is in %s", path)
			}
		}
		return nil
	})
}

// assertNoKeyInCommandLines is what `ps` shows of every process, agents' included.
func (e *e2e) assertNoKeyInCommandLines() {
	e.t.Helper()
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		if data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline")); err == nil && strings.Contains(string(data), e2eKey) {
			e.t.Errorf("the key is on the command line of process %s", entry.Name())
		}
	}
}

// run runs a command of the program's command line, as this daemon's user.
func (e *e2e) run(args ...string) (string, error) {
	cmd := exec.Command(e2eBin, args...)
	cmd.Env = []string{"HOME=" + e.home, "ENOWX_HOME=" + e.data, "PATH=" + os.Getenv("PATH")}
	cmd.Dir = e.work
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// spawn starts a Claude Platform session in the work folder.
func (e *e2e) spawn(p proto.SessionCreateParams) string {
	e.t.Helper()
	p.Agent = agent.PlatformID
	if p.Project == "" {
		p.Project = e.work
	}
	p.Cols, p.Rows = 120, 40
	info, err := e.c.CreateSession(p)
	if err != nil {
		e.t.Fatal(err)
	}
	return info.ID
}

func (e *e2e) info(id string) proto.SessionInfo {
	e.t.Helper()
	list, err := e.c.Sessions()
	if err != nil {
		e.t.Fatal(err)
	}
	for _, s := range list {
		if s.ID == id {
			return s
		}
	}
	e.t.Fatalf("no session %s in %+v", id, list)
	return proto.SessionInfo{}
}

func (e *e2e) entries(id string) []proto.TranscriptEntry {
	return e.transcript(id).Entries
}

func (e *e2e) transcript(id string) proto.Transcript {
	e.t.Helper()
	tr, err := e.c.Transcript(id, 0)
	if err != nil {
		e.t.Fatal(err)
	}
	return tr
}

func (e *e2e) screen(id string) string {
	text, _ := e.run("read", id)
	return text
}

// wait fails with the session's screen and conversation when ok never holds.
func (e *e2e) wait(id, what string, ok func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(25 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			e.t.Fatalf("never: %s\nstate %s\nscreen:\n%s\nentries: %s", what, e.info(id).State, e.screen(id), e.dumpEntries(id))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (e *e2e) dumpEntries(id string) string {
	raw, _ := json.MarshalIndent(e.entries(id), "", " ")
	return string(raw)
}

func (e *e2e) waitState(id, state string) {
	e.t.Helper()
	e.wait(id, "state "+state, func() bool { return e.info(id).State == state })
}

// waitTurns waits until the API has had n requests, and the agent is done.
func (e *e2e) waitTurns(id string, n int) {
	e.t.Helper()
	e.wait(id, fmt.Sprintf("%d requests and done", n), func() bool {
		return len(e.api.Requests()) >= n && e.info(id).State == proto.StateDone
	})
}

// say sends a message as `send` does and waits for its answer's end. The number of
// requests it takes is how many the turn is known to make.
func (e *e2e) say(id, text string, requests int) {
	e.t.Helper()
	n := len(e.api.Requests())
	e.send(id, text)
	e.waitTurns(id, n+requests)
}

func (e *e2e) send(id, text string) {
	e.t.Helper()
	if err := e.c.Input(id, text, true, true); err != nil {
		e.t.Fatal(err)
	}
}

// answer answers what the session waits on, as the chat's button (AnswerSession's keys).
func (e *e2e) answer(id, answer string) {
	e.t.Helper()
	e.waitState(id, proto.StateWaiting)
	e.wait(id, "something to answer", func() bool {
		ask, err := e.c.SessionAsk(id)
		return err == nil && len(ask.Answers) > 0
	})
	if _, err := e.c.AnswerSession(id, answer); err != nil {
		e.t.Fatal(err)
	}
}

func lastTool(entries []proto.TranscriptEntry, name string) (proto.TranscriptEntry, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Kind == proto.TranscriptTool && entries[i].Text == name {
			return entries[i], true
		}
	}
	return proto.TranscriptEntry{}, false
}

func kindCount(entries []proto.TranscriptEntry, kind string) int {
	n := 0
	for _, e := range entries {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

func hasNotice(entries []proto.TranscriptEntry, parts ...string) bool {
	return slices.ContainsFunc(entries, func(e proto.TranscriptEntry) bool {
		if e.Kind != proto.TranscriptNotice {
			return false
		}
		for _, p := range parts {
			if !strings.Contains(e.Text, p) {
				return false
			}
		}
		return true
	})
}

// historyProblem says what is wrong with a request's messages as the API would judge
// them: roles out of turn, or a tool_use with no tool_result right after it.
func historyProblem(r standinapi.Request) string {
	type block struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		ToolUseID string `json:"tool_use_id"`
	}
	type message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	var pending []string
	last := ""
	for i, raw := range r.Messages() {
		var m message
		if err := json.Unmarshal(raw, &m); err != nil {
			return err.Error()
		}
		if m.Role == last {
			return fmt.Sprintf("message %d: two of role %s in a row", i, m.Role)
		}
		last = m.Role
		var blocks []block
		_ = json.Unmarshal(m.Content, &blocks)
		if m.Role == "assistant" {
			pending = nil
			for _, b := range blocks {
				if b.Type == "tool_use" {
					pending = append(pending, b.ID)
				}
			}
			continue
		}
		for _, id := range pending {
			if !slices.ContainsFunc(blocks, func(b block) bool { return b.Type == "tool_result" && b.ToolUseID == id }) {
				return fmt.Sprintf("message %d: no tool_result for %s", i, id)
			}
		}
		pending = nil
	}
	if last != "user" {
		return "the last message isn't the user's"
	}
	return ""
}

func lastRequest(t *testing.T, api *standinapi.Server) standinapi.Request {
	t.Helper()
	list := api.Requests()
	if len(list) == 0 {
		t.Fatal("no requests")
	}
	return list[len(list)-1]
}

func toolNames(r standinapi.Request) []string {
	var tools []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(r.Raw("tools"), &tools)
	out := make([]string, len(tools))
	for i, tool := range tools {
		out[i] = tool.Name
	}
	return out
}

// TestPlatformE2E runs everything against the built program; each part has a daemon of
// its own, so what one leaves behind (a key, a kept rule) can't help another.
func TestPlatformE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the real program")
	}
	e2eBin = buildE2EProgram(t)
	t.Run("FirstPromptStreams", e2eFirstPrompt)
	t.Run("ToolsGoThroughPermission", e2eTools)
	t.Run("Bypass", e2eBypass)
	t.Run("Questions", e2eQuestions)
	t.Run("InterruptMidTool", e2eInterruptMidTool)
	t.Run("InterruptMidStream", e2eInterruptMidStream)
	t.Run("HibernateWakeAndKill", e2eResume)
	t.Run("ModeModelEffort", e2eSettings)
	t.Run("MCP", e2eMCP)
	t.Run("HookRewrite", e2eHookRewrite)
	t.Run("SystemPrompt", e2eSystemPrompt)
	t.Run("GOARefuses", e2eGOARefuses)
	t.Run("GOADelegates", e2eGOADelegates)
	t.Run("Failures", e2eFailures)
	t.Run("UsageAndBudget", e2eUsageAndBudget)
}

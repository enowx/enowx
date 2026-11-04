package cli

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// docsGOAHost is a test daemon, with a "claude" that only keeps the prompt it
// was started on, in the file it returns.
func docsGOAHost(t *testing.T) (*client.Client, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	bin := t.TempDir()
	asked := filepath.Join(bin, "asked")
	claude := "#!/bin/sh\nfor a in \"$@\"; do last=\"$a\"; done\nprintf '%s' \"$last\" > \"" + asked + ".tmp\"\nmv \"" + asked + ".tmp\" \"" + asked + "\"\ncat\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(claude), 0o755); err != nil {
		t.Fatal(err)
	}
	// The daemon takes the login shell's PATH: a shell that keeps this one, so
	// the stand-in is the agent found.
	shell := filepath.Join(bin, "sh")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nfor a in \"$@\"; do last=\"$a\"; done\nexec /bin/sh -c \"$last\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(EnvSession, "")
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a, b := net.Pipe()
	go srv.ServeConn(a)
	c, err := client.New(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c, asked
}

// docsGOARun runs a command of the given family and says what it printed.
func docsGOARun(c *client.Client, cmd func(runner, []string) error, args ...string) (string, error) {
	var out bytes.Buffer
	err := cmd(runner{c: c, out: &out}, args)
	return out.String(), err
}

// docsGOAProject is a new project of a few documents, drafts, written here
// rather than from the templates, whose words change. It returns them by key.
func docsGOAProject(t *testing.T, c *client.Client, title string) (proto.NoteInfo, map[string]string) {
	t.Helper()
	root, err := c.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: title}, Body: projectdocs.ProjectBody(projectdocs.SizeCore, projectdocs.ApprovalRequired)})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, d := range []struct{ key, body string }{
		{"prd", "## Features and priorities\n\nCheckout takes cards.\n\n## Out of scope\n\nNo refunds yet.\n"},
		{"tech-spec", "## Stack and reasons\n\nGo, as the team knows it.\n"},
		{"user-stories", "## Acceptance criteria\n\n### Story: reset\n\n- [ ] AC-1: given a signed-out user, a link is sent\n- [ ] AC-2: given an old link, it says so\n"},
		{"work-breakdown", "## Tasks\n\n### Stage 1: foundation\n\n- [ ] T-1: set up the repository (foundation, no dependencies)\n- [ ] T-2: create the first migration (foundation, depends on T-1)\n"},
	} {
		e := projectdocs.Find(d.key)
		n, err := c.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: e.Name, Parent: root.ID},
			Body: projectdocs.Meta{Kind: e.Name, Status: projectdocs.StatusDraft, Version: 1}.Line() + "\n\n" + d.body})
		if err != nil {
			t.Fatal(err)
		}
		ids[d.key] = n.ID
	}
	return root, ids
}

// docsGOAEdit saves note id with its body changed.
func docsGOAEdit(t *testing.T, c *client.Client, id string, change func(string) (string, error)) {
	t.Helper()
	n, err := c.Note(id)
	if err != nil {
		t.Fatal(err)
	}
	if n.Body, err = change(n.Body); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SaveNote(n); err != nil {
		t.Fatal(err)
	}
}

// A project's gate holds a task back until the owner has approved, then the
// task's prompt, as the worker gets it, carries what the documents say.
func TestGOADelegateFromProjectDocs(t *testing.T) {
	c, asked := docsGOAHost(t)
	_, ids := docsGOAProject(t, c, "Shop")
	folder := t.TempDir()
	stage := ids["work-breakdown"] + "#stage-1-foundation"
	delegate := []string{"delegate", "--doc", stage, "--agent", "claude", "--project", folder, "--no-route", "--json", "T-1: set up the repository"}
	board := func() proto.GOABoard {
		t.Helper()
		var b proto.GOABoard
		if err := c.Call(proto.MethodGOABoard, nil, &b); err != nil {
			t.Fatal(err)
		}
		return b
	}

	_, err := docsGOARun(c, runner.goaCmd, delegate...)
	if err == nil {
		t.Fatal("handed over with the gate closed")
	}
	for _, want := range []string{"PRD is draft, not approved", "Technical Spec is draft, not approved", "Notes page"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("no %q in: %v", want, err)
		}
	}
	docsGOAEdit(t, c, ids["prd"], projectdocs.Approve)
	if _, err = docsGOARun(c, runner.goaCmd, delegate...); err == nil || strings.Contains(err.Error(), "PRD") || !strings.Contains(err.Error(), "Technical Spec") {
		t.Fatalf("the PRD approved: %v", err)
	}
	if len(board().Tasks) != 0 {
		t.Fatalf("refused, yet on the board: %+v", board().Tasks)
	}
	docsGOAEdit(t, c, ids["tech-spec"], projectdocs.Approve)

	// Based on a section of the approved PRD.
	prd, err := c.Note(ids["prd"])
	if err != nil {
		t.Fatal(err)
	}
	hash, err := projectdocs.SectionHash(prd.Body, "Features and priorities")
	if err != nil {
		t.Fatal(err)
	}
	docsGOAEdit(t, c, ids["work-breakdown"], func(body string) (string, error) {
		return projectdocs.AddLink(body, projectdocs.Link{Doc: "PRD", Section: "Features and priorities", Hash: hash, In: "stage-1-foundation"})
	})

	// Before anything is on the board, the plan offers the first task, in a
	// command GOD of Agents is allowed to run.
	out, err := docsGOARun(c, runner.docs, "plan", ids["work-breakdown"], "--json")
	if err != nil {
		t.Fatal(err)
	}
	var plan docsPlanReport
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(plan.Tasks) != 2 || !plan.Tasks[0].Ready || plan.Tasks[1].Ready || len(plan.Blockers) != 0 {
		t.Fatalf("plan: %s", out)
	}
	command := plan.Tasks[0].Command
	if !strings.Contains(command, "--doc "+stage) || !strings.Contains(command, "--title 'T-1: set up") || !strings.Contains(command, "--project DIR") {
		t.Fatalf("command: %s", command)
	}
	if ok, why := goa.Allowed("Bash", command, ""); !ok {
		t.Fatalf("GOA may not run it: %s", why)
	}
	if ok, why := goa.Allowed("Bash", `"$ENOWX_BIN" docs verify Shop --task t1`, ""); !ok {
		t.Fatalf("GOA may not read: %s", why)
	}
	if len(board().Tasks) != 0 {
		t.Fatal("the plan handed something over")
	}

	if out, err = docsGOARun(c, runner.goaCmd, delegate...); err != nil {
		t.Fatal(err)
	}
	var task proto.GOATask
	if err := json.Unmarshal([]byte(out), &task); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if task.Title != "T-1: set up the repository" {
		t.Fatalf("title: %q", task.Title)
	}

	var prompt string
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if data, err := os.ReadFile(asked); err == nil {
			prompt = string(data)
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the worker was never asked")
		}
	}
	if !strings.HasPrefix(prompt, "Project Docs: ") {
		t.Fatalf("the prompt doesn't start with its line:\n%s", prompt)
	}
	for _, want := range []string{"T-2: create the first migration", "Checkout takes cards.", "AC-1: given a signed-out user", "The task:\n\nT-1: set up the repository"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("no %q in the worker's prompt:\n%s", want, prompt)
		}
	}
	if len(prompt) > projectdocs.ContextLimit+200 {
		t.Fatalf("%d bytes", len(prompt))
	}
	root := plan.Project
	if project, doc, section, ok := projectdocs.DocRefOf(prompt); !ok || project != root || doc != ids["work-breakdown"] || section != "stage-1-foundation" {
		t.Fatalf("read back: %q %q %q %v", project, doc, section, ok)
	}

	// The plan now says who has T-1, and offers nothing: T-2 waits on it.
	if out, err = docsGOARun(c, runner.docs, "plan", "Shop", "--json"); err != nil {
		t.Fatal(err)
	}
	plan = docsPlanReport{}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if t1 := plan.Tasks[0]; t1.Ready || len(t1.Board) != 1 || t1.Board[0].ID != task.ID {
		t.Fatalf("T-1 on the board: %s", out)
	}
	if out, err = docsGOARun(c, runner.docs, "plan", "Shop"); err != nil || !strings.Contains(out, "Nothing is ready") || !strings.Contains(out, "handed nothing over") {
		t.Fatalf("%v: %s", err, out)
	}

	if out, err = docsGOARun(c, runner.docs, "progress", "Shop", "--json"); err != nil {
		t.Fatal(err)
	}
	var progress docsProgressReport
	if err := json.Unmarshal([]byte(out), &progress); err != nil {
		t.Fatal(err)
	}
	if progress.ByStatus[projectdocs.StatusApproved] != 2 || len(progress.Blockers) != 0 || progress.TasksPlanned != 2 || progress.TasksDone != 0 ||
		len(progress.Board) != 1 || progress.Board[0].ID != task.ID || progress.CriteriaAll != 2 || progress.CriteriaDone != 0 {
		t.Fatalf("progress: %s", out)
	}

	// Verifying shows the result beside the criteria, and ticks nothing.
	if _, err = docsGOARun(c, runner.goaCmd, "task", task.ID, "--status", "done", "--result", "repository set up, make test passes"); err != nil {
		t.Fatal(err)
	}
	before, err := c.Note(ids["user-stories"])
	if err != nil {
		t.Fatal(err)
	}
	if out, err = docsGOARun(c, runner.docs, "verify", "Shop", "--task", task.ID); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"repository set up, make test passes", "- [ ] AC-1: given", "only with evidence", "notes check", "Name what remains", "ticked nothing"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in:\n%s", want, out)
		}
	}
	after, err := c.Note(ids["user-stories"])
	if err != nil {
		t.Fatal(err)
	}
	if before.Body != after.Body {
		t.Fatal("verify changed the criteria")
	}

	// A task of no project, or of another one, is said to be so.
	plain, err := docsGOARun(c, runner.goaCmd, "delegate", "--agent", "claude", "--project", folder, "--no-route", "--json", "Fix the totals")
	if err != nil {
		t.Fatal(err)
	}
	var other proto.GOATask
	if err := json.Unmarshal([]byte(plain), &other); err != nil {
		t.Fatal(err)
	}
	if out, err = docsGOARun(c, runner.docs, "verify", "Shop", "--task", other.ID); err != nil || !strings.Contains(out, "names no document") || strings.Contains(out, "AC-1") {
		t.Fatalf("a task of no project: %v\n%s", err, out)
	}
	docsGOAProject(t, c, "Blog")
	if out, err = docsGOARun(c, runner.docs, "verify", "Blog", "--task", task.ID); err != nil || !strings.Contains(out, "isn't of Blog") || strings.Contains(out, "AC-1") {
		t.Fatalf("a task of another project: %v\n%s", err, out)
	}
	if _, err = docsGOARun(c, runner.docs, "verify", "Shop", "--task", "nothing"); err == nil {
		t.Fatal("no such task")
	}
}

// A project with approval off has no gate, and a note outside any project, or
// a section that isn't there, is refused.
func TestGOADelegateDocRefs(t *testing.T) {
	c, _ := docsGOAHost(t)
	root, ids := docsGOAProject(t, c, "Shop")
	if _, err := c.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: "Groceries"}, Body: "- [ ] milk"}); err != nil {
		t.Fatal(err)
	}
	delegate := func(doc string) error {
		_, err := docsGOARun(c, runner.goaCmd, "delegate", "--doc", doc, "--agent", "claude", "--no-route", "T-1: go")
		return err
	}
	if err := delegate("Groceries"); err == nil || !strings.Contains(err.Error(), "in a project") {
		t.Fatalf("an ordinary note: %v", err)
	}
	docsGOAEdit(t, c, root.ID, func(body string) (string, error) { return projectdocs.SetApproval(body, projectdocs.ApprovalOff) })
	if err := delegate(ids["prd"] + "#nothing"); err == nil || !strings.Contains(err.Error(), "no section") {
		t.Fatalf("no such section: %v", err)
	}
	if err := delegate(root.ID); err == nil || !strings.Contains(err.Error(), "no document") {
		t.Fatalf("the project note itself: %v", err)
	}
	if err := delegate(ids["prd"]); err != nil {
		t.Fatalf("approval off: %v", err)
	}
}

// The tools are GOA's, and take what the commands take.
func TestGOADocsTools(t *testing.T) {
	var names []string
	for _, tool := range docsGOATools {
		names = append(names, tool.name)
		if agent.FeatureOfTool(tool.name) != "goa" {
			t.Fatalf("%s isn't of the feature goa", tool.name)
		}
	}
	if !slices.Equal(names, []string{"docs_plan", "docs_progress", "docs_verify"}) {
		t.Fatalf("%v", names)
	}
	served := slices.ContainsFunc(mcpGroups(), func(g mcpGroup) bool {
		return len(g.tools) > 0 && g.tools[0].name == "docs_plan"
	})
	if !served {
		t.Fatal("not served")
	}
	var delegate mcpTool
	for _, tool := range goaTools {
		if tool.name == "goa_delegate" {
			delegate = tool
		}
	}
	if args := delegate.args(map[string]any{"prompt": "go", "doc": "PRD#features"}); !slices.Contains(args, "--doc") || !slices.Contains(args, "PRD#features") {
		t.Fatalf("goa_delegate: %v", args)
	}
	if _, ok := delegate.props["doc"]; !ok {
		t.Fatal("goa_delegate has no doc")
	}
}

package cli

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// docsHarness runs commands against a real daemon of its own.
type docsHarness struct {
	t   *testing.T
	c   *client.Client
	out *bytes.Buffer
}

func newDocsHarness(t *testing.T) *docsHarness {
	t.Helper()
	// The tests may run in a session: they start outside one.
	t.Setenv(EnvSession, "")
	t.Setenv(proto.EnvNotesPropose, "")
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	a, b := net.Pipe()
	go srv.ServeConn(a)
	c, err := client.New(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return &docsHarness{t: t, c: c, out: &bytes.Buffer{}}
}

func (h *docsHarness) try(args ...string) (string, error) {
	h.out.Reset()
	r := runner{c: h.c, out: h.out}
	var err error
	if args[0] == "notes" {
		err = r.notes(args[1:])
	} else {
		err = r.docs(args)
	}
	return h.out.String(), err
}

func (h *docsHarness) run(args ...string) string {
	h.t.Helper()
	out, err := h.try(args...)
	if err != nil {
		h.t.Fatalf("%q: %v", args, err)
	}
	return out
}

func (h *docsHarness) refused(want string, args ...string) {
	h.t.Helper()
	if _, err := h.try(args...); err == nil || !strings.Contains(err.Error(), want) {
		h.t.Fatalf("%q: want refused with %q, got %v", args, want, err)
	}
}

func (h *docsHarness) body(id string) string {
	h.t.Helper()
	n, err := h.c.Note(id)
	if err != nil {
		h.t.Fatal(err)
	}
	return n.Body
}

const specBody = `## Stack and reasons

_To fill: the stack._

## Design system

Skipped: no UI of its own.

_To fill: left behind by a template._

## Architecture

Two services. [[PRD#Features and priorities]] says what they serve.

- [ ] Acceptance one

## Diagram

_To fill: a canvas._
`

// docsSample is a project of PRD, Technical Spec and Brief, whichever
// templates have arrived: made by hand, as these tests aren't about the packs.
func (h *docsHarness) docsSample() (root, prd, spec string) {
	h.t.Helper()
	save := func(title, parent, body string) string {
		h.t.Helper()
		info, err := h.c.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: title, Parent: parent}, Body: body})
		if err != nil {
			h.t.Fatal(err)
		}
		return info.ID
	}
	root = save("Shop", "", projectdocs.ProjectBody(projectdocs.SizeCore, projectdocs.ApprovalRequired))
	e := projectdocs.Find("prd")
	body, err := projectdocs.DocBody(*e)
	if err != nil {
		h.t.Fatal(err)
	}
	prd = save("PRD", root, body)
	spec = save("Technical Spec", root, projectdocs.Meta{Kind: "Technical Spec", Status: projectdocs.StatusDraft, Version: 1}.Line()+"\n\n"+specBody)
	return root, prd, spec
}

// skipWithoutTemplates skips when a pack's template hasn't arrived yet.
func skipWithoutTemplates(t *testing.T, size string) {
	t.Helper()
	for _, e := range projectdocs.Pack(size) {
		if _, err := projectdocs.Template(e.Key); err != nil {
			t.Skipf("template %s isn't here yet", e.Key)
		}
	}
}

// The core pack: its documents directly under the project, listed, attached
// to the project named, and a plain note for whatever knows nothing of them.
func TestDocsNewCore(t *testing.T) {
	skipWithoutTemplates(t, projectdocs.SizeCore)
	h := newDocsHarness(t)
	dir := filepath.Join(t.TempDir(), "web")
	os.MkdirAll(dir, 0o755)
	if _, err := h.c.AddProject(dir); err != nil {
		t.Fatal(err)
	}
	waitForCatalog(t, h.c, 1)
	id := strings.TrimSpace(h.run("new", "Shop", "--project", "web"))
	list, _ := h.c.Notes()
	pack := projectdocs.Pack(projectdocs.SizeCore)
	if len(list) != len(pack)+1 {
		t.Fatalf("%d notes for a pack of %d", len(list), len(pack))
	}
	for _, n := range list {
		if n.ID != id && n.Parent != id {
			t.Fatalf("%q isn't directly under the project", n.Title)
		}
		if len(n.Projects) != 1 || n.Projects[0].Name != "web" {
			t.Fatalf("%q not attached: %+v", n.Title, n.Projects)
		}
	}
	if got := h.run("list"); !strings.Contains(got, id+"  Shop  core, ") || !strings.Contains(got, "approval required") {
		t.Fatalf("list: %q", got)
	}
	// What's added goes where the rest is, attached as they are.
	added := strings.TrimSpace(h.run("add", "Shop", "glossary"))
	n, _ := h.c.Note(added)
	if n.Parent != id || len(n.Projects) != 1 {
		t.Fatalf("added: %+v", n.NoteInfo)
	}
	h.refused("already has", "add", "Shop", "glossary")
}

// The full pack: every document, by way of its area's page.
func TestDocsNewFull(t *testing.T) {
	skipWithoutTemplates(t, projectdocs.SizeFull)
	h := newDocsHarness(t)
	id := strings.TrimSpace(h.run("new", "Shop", "--size", "full", "--approval", "off"))
	list, _ := h.c.Notes()
	if want := len(projectdocs.Catalog) + len(projectdocs.Areas) + 1; len(list) != want {
		t.Fatalf("%d notes, want %d", len(list), want)
	}
	if got := h.run("list"); !strings.Contains(got, "full, "+itoa(len(projectdocs.Catalog))+" documents, approval off") {
		t.Fatalf("list: %q", got)
	}
	h.run("gate", id) // approval off stops nothing
	// Every template is exportable: none reads as holding a secret.
	if got := h.run("export", id, "--to", t.TempDir(), "--dry-run"); !strings.Contains(got, "business-and-product/prd.md") {
		t.Fatalf("export: %q", got)
	}
}

func itoa(n int) string {
	data, _ := json.Marshal(n)
	return string(data)
}

// A project whose making fails midway leaves nothing behind.
func TestDocsNewTakesBackOnFailure(t *testing.T) {
	h := newDocsHarness(t)
	h.refused("no template", "new", "Shop", "--with", "no-such-key")
	if list, _ := h.c.Notes(); len(list) != 0 {
		t.Fatalf("left %+v", list)
	}
	h.refused("size is", "new", "Shop", "--size", "huge")
	h.refused("no template", "new", "Shop", "--template", "Mine")
	if list, _ := h.c.Notes(); len(list) != 0 {
		t.Fatalf("left %+v", list)
	}
}

// What a session may do to Project Docs: drafts directly, approved documents
// by proposal, and approve nothing, by no way round.
func TestDocsWriteGuard(t *testing.T) {
	h := newDocsHarness(t)
	root, prd, spec := h.docsSample()
	h.run("approve", spec)
	approved := h.body(spec)
	if m, _ := projectdocs.Parse(approved); m.Status != projectdocs.StatusApproved {
		t.Fatalf("approved: %+v", m)
	}
	t.Setenv(EnvSession, "s1")

	// A draft is written directly.
	h.run("notes", "write", prd, "--append", "More.")
	if !strings.Contains(h.body(prd), "More.") {
		t.Fatal("a draft wasn't written")
	}
	// An approved document becomes a proposal, with why.
	got := h.run("notes", "write", spec, "--section", "Architecture", "--body", "Three services.\n\n- [ ] Acceptance one")
	if !strings.Contains(got, "is approved") || !strings.Contains(got, "Proposed (") {
		t.Fatalf("write: %q", got)
	}
	if h.body(spec) != approved {
		t.Fatal("an approved document changed before accepting")
	}
	props, _ := h.c.NoteProposals()
	if len(props) != 1 {
		t.Fatalf("proposals: %+v", props)
	}
	// Its agent can't accept it itself, and the owner can.
	h.refused("only the owner accepts", "notes", "accept", props[0].ID)
	t.Setenv(EnvSession, "")
	h.run("notes", "accept", props[0].ID)
	m, _ := projectdocs.Parse(h.body(spec))
	if m.Status != projectdocs.StatusApproved || m.Version != 2 || !strings.Contains(h.body(spec), "Three services.") || projectdocs.ChangedSinceApproval(h.body(spec)) {
		t.Fatalf("accepted: %+v", m)
	}
	t.Setenv(EnvSession, "s1")

	// Ticking acceptance criteria is direct.
	h.run("notes", "check", spec, "--item", "Acceptance one")
	if !strings.Contains(h.body(spec), "- [x] Acceptance one") {
		t.Fatal("not ticked")
	}
	// Approving is the owner's, whichever way it's tried.
	h.refused("only the owner approves, in the Notes page", "approve", prd)
	h.refused("only the owner approves, in the Notes page", "status", prd, "Approved")
	h.refused("only the owner", "approval", root, "off")
	h.refused("only the owner approves", "notes", "new", "Forged", "--body", projectdocs.Meta{Kind: "PRD", Status: projectdocs.StatusApproved, Version: 1}.Line())
	forged := strings.Replace(h.body(prd), "{{Draft}}", "{{Approved}}", 1)
	if got := h.run("notes", "write", prd, "--body", forged); !strings.Contains(got, "only the owner approves") {
		t.Fatalf("self-approval: %q", got)
	}
	if m, _ := projectdocs.Parse(h.body(prd)); m.Status != projectdocs.StatusDraft {
		t.Fatalf("self-approved: %+v", m)
	}
	// Turning approval off in the project's own note is too.
	off := strings.Replace(h.body(root), "Approval: required", "Approval: off", 1)
	if got := h.run("notes", "write", root, "--body", off); !strings.Contains(got, "only the owner changes") {
		t.Fatalf("approval off: %q", got)
	}
	// Bringing back an old version of an approved document is proposed too.
	versions, _ := h.c.NoteHistory(spec)
	if len(versions) == 0 {
		t.Fatal("no version to bring back")
	}
	now := h.body(spec)
	if got := h.run("notes", "restore", spec, versions[0].ID); !strings.Contains(got, "Proposed (") {
		t.Fatalf("restore: %q", got)
	}
	if h.body(spec) != now {
		t.Fatal("restored over an approved document")
	}
	// Leaving Approved is proposed, and the proposal says what was asked.
	if got := h.run("status", spec, "Outdated"); !strings.Contains(got, "Proposed (") {
		t.Fatalf("status: %q", got)
	}
	if m, _ := projectdocs.Parse(h.body(spec)); m.Status != projectdocs.StatusApproved {
		t.Fatalf("left approved directly: %+v", m)
	}
	found := false
	props, _ = h.c.NoteProposals()
	for _, p := range props {
		if m, _ := projectdocs.Parse(p.Note.Body); p.Note.ID == spec && m.Status == projectdocs.StatusOutdated {
			found = true
		}
	}
	if !found {
		t.Fatalf("no proposal of Outdated: %+v", props)
	}
	// Other statuses are the agent's.
	h.run("status", prd, "In", "review")
	if m, _ := projectdocs.Parse(h.body(prd)); m.Status != projectdocs.StatusReview {
		t.Fatalf("in review: %+v", m)
	}
	// Removing and moving what's approved, or a project.
	h.refused("is approved", "notes", "rm", spec)
	h.refused("is approved", "notes", "move", spec, "--top")
	h.refused("only the owner removes", "notes", "rm", root)
	h.refused("only the owner removes", "notes", "space", root, "--to", "personal")
	// Nor by way of what it's under.
	area, err := h.c.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: projectdocs.AreaArch, Parent: root}, Body: projectdocs.Meta{Kind: projectdocs.KindArea}.Line()})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvSession, "")
	h.run("notes", "move", spec, "--parent", area.ID)
	t.Setenv(EnvSession, "s1")
	h.refused("is approved", "notes", "rm", area.ID)
	// A draft goes.
	h.run("notes", "rm", prd)
	// Ordinary notes are as they were.
	h.run("notes", "new", "Plain", "--body", "x")
	h.run("notes", "write", "Plain", "--body", "y")
	h.run("notes", "rm", "Plain")
	// Outside a session nothing changes.
	t.Setenv(EnvSession, "")
	h.run("notes", "write", spec, "--append", "The owner's line.")
	if !strings.Contains(h.body(spec), "The owner's line.") {
		t.Fatal("the owner's write was held")
	}
}

// notes_write and notes_delete over MCP are the same commands, so the guard
// holds there too, and approving isn't offered.
func TestDocsGuardOverMCP(t *testing.T) {
	h := newDocsHarness(t)
	_, _, spec := h.docsSample()
	h.run("approve", spec)
	approved := h.body(spec)
	t.Setenv(EnvSession, "s1")
	var in bytes.Buffer
	call := func(id int, name string, args map[string]any) {
		data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		in.Write(append(data, '\n'))
	}
	in.WriteString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")
	call(2, "notes_write", map[string]any{"note": spec, "append": "An agent's line."})
	call(3, "notes_delete", map[string]any{"note": spec})
	call(4, "docs_gate", map[string]any{"project": "Shop"})
	var out bytes.Buffer
	if err := (runner{c: h.c, out: &out}).mcp(&in, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("answers: %q", out.String())
	}
	for _, name := range []string{`"docs_new"`, `"docs_export"`, `"docs_diagram"`} {
		if !strings.Contains(lines[0], name) {
			t.Fatalf("no tool %s", name)
		}
	}
	for _, name := range []string{`"docs_approve"`, `"docs_approval"`} {
		if strings.Contains(lines[0], name) {
			t.Fatalf("tool %s offered", name)
		}
	}
	if !strings.Contains(lines[1], "Proposed (") {
		t.Fatalf("notes_write: %s", lines[1])
	}
	if !strings.Contains(lines[2], "is approved") || !strings.Contains(lines[2], `"isError":true`) {
		t.Fatalf("notes_delete: %s", lines[2])
	}
	if !strings.Contains(lines[3], "Gate: blocked") || !strings.Contains(lines[3], `"isError":true`) {
		t.Fatalf("docs_gate: %s", lines[3])
	}
	if h.body(spec) != approved {
		t.Fatal("changed over MCP")
	}
}

// A link holds until its source changes, and again re-confirms it.
func TestDocsLinkThenStale(t *testing.T) {
	h := newDocsHarness(t)
	root, prd, spec := h.docsSample()
	got := h.run("link", "Technical Spec", "--to", "PRD#Features and priorities", "--in", "Architecture")
	if !strings.Contains(got, "[[PRD#Features and priorities]] @") {
		t.Fatalf("link: %q", got)
	}
	if got := h.run("summary", root); !strings.Contains(got, "0 stale") {
		t.Fatalf("summary: %q", got)
	}
	// Another section changing leaves it be.
	h.run("notes", "write", prd, "--section", "Personas and users", "--body", "Buyers.")
	if got := h.run("summary", root); !strings.Contains(got, "0 stale") {
		t.Fatalf("summary: %q", got)
	}
	h.run("notes", "write", prd, "--section", "Features and priorities", "--body", "Checkout.")
	got = h.run("summary", root)
	if !strings.Contains(got, "1 stale") || !strings.Contains(got, "changed since based on it: PRD") {
		t.Fatalf("summary: %q", got)
	}
	// Reviewed, the same link again brings it up to date, once.
	h.run("link", spec, "--to", "PRD#features-and-priorities", "--in", "Architecture")
	if got := h.run("summary", root); !strings.Contains(got, "0 stale") {
		t.Fatalf("summary: %q", got)
	}
	if strings.Count(h.body(spec), "Based on:") != 1 {
		t.Fatalf("linked twice:\n%s", h.body(spec))
	}
	// Re-confirming on an approved document changes no content: direct.
	h.run("approve", spec)
	h.run("notes", "write", prd, "--section", "Features and priorities", "--body", "Checkout and returns.")
	t.Setenv(EnvSession, "s1")
	if got := h.run("link", spec, "--to", "PRD#Features and priorities", "--in", "Architecture"); strings.Contains(got, "Proposed") {
		t.Fatalf("link: %q", got)
	}
	if got := h.run("summary", root); !strings.Contains(got, "0 stale") {
		t.Fatalf("summary: %q", got)
	}
	h.refused("no document", "link", spec, "--to", "Nothing")
	h.refused("based on itself", "link", spec, "--to", "Technical Spec")
}

func TestDocsGate(t *testing.T) {
	h := newDocsHarness(t)
	root, prd, spec := h.docsSample()
	out, err := h.try("gate", root)
	if err == nil || !strings.Contains(out, "PRD is draft") || !strings.Contains(out, "Technical Spec is draft") {
		t.Fatalf("gate: %q, %v", out, err)
	}
	h.run("approve", prd)
	h.run("approve", spec)
	if got := h.run("gate", "Shop"); !strings.Contains(got, "Gate: open") {
		t.Fatalf("gate: %q", got)
	}
	// Edited by what knows nothing of Project Docs, it blocks again.
	n, _ := h.c.Note(spec)
	n.Body += "\nAn edit elsewhere.\n"
	if _, err := h.c.SaveNote(n); err != nil {
		t.Fatal(err)
	}
	if out, err := h.try("gate", spec); err == nil || !strings.Contains(out, "changed since it was approved") {
		t.Fatalf("gate: %q, %v", out, err)
	}
	h.run("approval", root, "off")
	h.run("gate", root)
	if got := h.run("summary", root); !strings.Contains(got, "approval off") || !strings.Contains(got, "changed since approved") {
		t.Fatalf("summary: %q", got)
	}
}

// Gaps come in the topics' order; what's skipped or absent asks nothing.
func TestDocsGaps(t *testing.T) {
	h := newDocsHarness(t)
	root, _, _ := h.docsSample()
	var gaps []projectdocs.Gap
	if err := json.Unmarshal([]byte(h.run("gaps", root, "--json")), &gaps); err != nil {
		t.Fatal(err)
	}
	var names []string
	other := false
	for _, g := range gaps {
		names = append(names, g.Name)
		if g.Doc == "Technical Spec" && g.Section == "Design system" {
			t.Fatal("a skipped section asked")
		}
		other = other || g.Doc == "Technical Spec" && g.Section == "Diagram" && g.Name == ""
		if g.Area == "Idea" {
			t.Fatal("a topic of a document the project hasn't")
		}
	}
	joined := strings.Join(names, ",")
	if !strings.HasPrefix(joined, "Users,Features,Roles") || !strings.Contains(joined, "Stack") || !other {
		t.Fatalf("gaps: %+v", gaps)
	}
	if strings.Contains(joined, "Architecture") {
		t.Fatal("a filled section asked")
	}
	if got := h.run("gaps", "Shop"); !strings.Contains(got, "Which stack, and why?") {
		t.Fatalf("gaps: %q", got)
	}
}

func waitForCatalog(t *testing.T, c *client.Client, projects int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if cats, _ := c.Catalogs(); len(cats) == 1 && len(cats[0].Projects) == projects {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no catalog")
		}
	}
}

// The rest of a project's life: skip, add, diagram, summary and templates.
func TestDocsSkipDiagramTemplates(t *testing.T) {
	h := newDocsHarness(t)
	root, _, spec := h.docsSample()
	h.run("skip", "Shop", "terms", "--why", "An internal tool.")
	if !strings.Contains(h.body(root), "## Skipped documents\n\nDocuments of the catalog this project doesn't need, each with why.\n- Terms of Service and Privacy Policy: An internal tool.\n") {
		t.Fatalf("skipped:\n%s", h.body(root))
	}
	h.refused("no document", "skip", "Shop", "nothing", "--why", "x")
	h.refused("it can't be skipped", "skip", "Shop", "prd", "--why", "x")
	h.refused("--why", "skip", "Shop", "terms")

	h.refused("canvas", "diagram", spec, "--canvas", "000000000000")
	c, err := h.c.CreateCanvas(proto.CanvasCreateParams{Title: "Flow"})
	if err != nil {
		t.Fatal(err)
	}
	h.run("diagram", "Technical Spec", "--canvas", c.ID, "--from", "the PRD")
	body := h.body(spec)
	if !strings.Contains(body, "## Diagram\n\n::embed canvas:"+c.ID+"\n\nDrawn from: the PRD, checked ") || strings.Contains(body, "_To fill: a canvas._") {
		t.Fatalf("diagram:\n%s", body)
	}
	// None left to replace: added at the section's end.
	h.run("diagram", spec, "--canvas", c.ID)
	if strings.Count(h.body(spec), "::embed canvas:"+c.ID) != 2 {
		t.Fatalf("second diagram:\n%s", h.body(spec))
	}

	// By its kind, in a project named.
	if got := h.run("status", "prd", "--in-project", "Shop"); got != "Draft v1\n" {
		t.Fatalf("status: %q", got)
	}
	h.refused("--in-project", "status", "tech-spec")

	// A template, without what records state, and made into a document.
	h.run("link", spec, "--to", "PRD")
	h.run("template", "save", spec, "--name", "Our spec")
	h.run("template", "save", spec, "--name", "Our spec") // up to date, not twice
	if got := h.run("templates"); strings.Count(got, "Our spec") != 1 {
		t.Fatalf("templates: %q", got)
	}
	tpl, err := runnerTemplate(h, "Our spec")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tpl.Body, projectdocs.Marker) || strings.Contains(tpl.Body, "Based on:") {
		t.Fatalf("meta kept:\n%s", tpl.Body)
	}
	added := strings.TrimSpace(h.run("add", "Shop", "Our spec"))
	if m, ok := projectdocs.Parse(h.body(added)); !ok || m.Kind != "Our spec" || m.Status != projectdocs.StatusDraft {
		t.Fatalf("own document: %+v", m)
	}
	// Templates aren't projects.
	if got := h.run("list"); strings.Contains(got, "templates") || !strings.Contains(got, "Shop  core, 3 documents") {
		t.Fatalf("list: %q", got)
	}
}

func runnerTemplate(h *docsHarness, name string) (proto.Note, error) {
	return runner{c: h.c, out: h.out}.docsTemplate(name)
}

// Into a repository: links relative, nothing of anyone's written over, and a
// dry run writing nothing.
func TestDocsExportRepo(t *testing.T) {
	h := newDocsHarness(t)
	root, prd, spec := h.docsSample()
	dir := filepath.Join(t.TempDir(), "docs")
	got := h.run("export", root, "--to", dir, "--dry-run")
	if !strings.Contains(got, "Would write 3 files") {
		t.Fatalf("dry run: %q", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("a dry run wrote")
	}
	h.run("export", prd, "--to", dir) // from a document, its project
	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	text := read("technical-spec.md")
	want := "<!-- Exported from enowX Project Docs (note " + spec + ", v1, Draft). Notes are the source: edit there. -->\n\n# Technical Spec\n"
	if !strings.HasPrefix(text, want) {
		t.Fatalf("head:\n%s", text)
	}
	if strings.Contains(text, projectdocs.Marker) {
		t.Fatalf("meta line kept:\n%s", text)
	}
	if !strings.Contains(text, "[PRD#Features and priorities](prd.md#features-and-priorities)") {
		t.Fatalf("link:\n%s", text)
	}
	readme := read("README.md")
	if !strings.HasPrefix(readme, "<!-- Exported from enowX Project Docs (note "+root) || !strings.Contains(readme, "- [PRD](prd.md) (Draft, v1)") || strings.Contains(readme, "::pages") {
		t.Fatalf("README:\n%s", readme)
	}
	// A canvas stays as text pointing to it; a link to what isn't exported, text.
	c, err := h.c.CreateCanvas(proto.CanvasCreateParams{Title: "Flow"})
	if err != nil {
		t.Fatal(err)
	}
	h.run("diagram", spec, "--canvas", c.ID)
	h.run("notes", "write", spec, "--append", "See [[Elsewhere]] and `[[kept]]`, [[#Architecture|above]].")
	h.run("export", root, "--to", dir)
	text = read("technical-spec.md")
	if !strings.Contains(text, "::embed canvas:"+c.ID) {
		t.Fatalf("canvas:\n%s", text)
	}
	if !strings.Contains(text, "See Elsewhere and `[[kept]]`, [above](#architecture).") {
		t.Fatalf("links:\n%s", text)
	}
	// A file that isn't an export's is refused, whole: nothing else is written.
	before := read("README.md")
	h.run("notes", "write", root, "--append", "New line.")
	foreign := filepath.Join(dir, "prd.md")
	if err := os.WriteFile(foreign, []byte("# Someone's PRD\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.refused("prd.md", "export", root, "--to", dir)
	h.refused("prd.md", "export", root, "--to", dir, "--dry-run")
	if read("prd.md") != "# Someone's PRD\n" || read("README.md") != before {
		t.Fatal("written though refused")
	}
	h.run("export", root, "--to", dir, "--force")
	if !strings.Contains(read("prd.md"), "Exported from enowX") {
		t.Fatal("--force didn't write")
	}
	// Nothing is deleted: a file of ours for a document gone stays.
	h.run("notes", "rm", prd)
	h.run("export", root, "--to", dir)
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("a file was deleted")
	}
	// A secret refuses it, named by kind, its value nowhere.
	key := "AKIA" + "IOSFODNN7EXAMPLE"
	h.run("notes", "write", spec, "--append", "The key is "+key)
	_, err = h.try("export", root, "--to", dir)
	if err == nil || !strings.Contains(err.Error(), "holds a secret") || !strings.Contains(err.Error(), "technical-spec.md") || strings.Contains(err.Error(), key) {
		t.Fatalf("secret: %v", err)
	}
	if strings.Contains(read("technical-spec.md"), key) {
		t.Fatal("a secret was written")
	}
	// As a file, through the ordinary export.
	out := t.TempDir()
	h.run("notes", "write", spec, "--body", "clean")
	if got := h.run("export", root, "--format", "json", "--out", out); !strings.Contains(got, "Exported 2 notes") {
		t.Fatalf("file: %q", got)
	}
	h.refused("usage", "export", root)
	h.refused("usage", "export", root, "--to", dir, "--format", "zip")
	h.refused("usage", "export", root, "--format", "repo")
}

// A project is ordinary notes to whatever knows nothing of Project Docs:
// nothing in the header, its state a line of text.
func TestDocsAreOrdinaryNotes(t *testing.T) {
	h := newDocsHarness(t)
	root, prd, _ := h.docsSample()
	got := h.run("notes", "show", root)
	if !strings.HasPrefix(got, "# Shop\n\nProject Docs: Project · {{Core pack}} · Approval: required\n") {
		t.Fatalf("show: %q", got)
	}
	if got := h.run("notes"); !strings.Contains(got, "PRD") || !strings.Contains(got, "Shop") {
		t.Fatalf("tree: %q", got)
	}
	if got := h.run("notes", "outline", prd); !strings.Contains(got, "#features-and-priorities") {
		t.Fatalf("outline: %q", got)
	}
	h.run("notes", "check", "Technical Spec", "--item", "Acceptance one")
	h.run("notes", "rm", root)
	if list, _ := h.c.Notes(); len(list) != 0 {
		t.Fatalf("left %+v", list)
	}
}

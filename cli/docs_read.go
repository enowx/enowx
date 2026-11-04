package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

// Reading projects.

func init() {
	docsCommands["list"] = docsCommand{"list   the projects: ID, title, size, documents, approval", runner.docsList}
	docsCommands["summary"] = docsCommand{"summary PROJECT [--json]   statuses, progress, open questions, stale, unfilled, gate", runner.docsSummary}
	docsCommands["catalog"] = docsCommand{"catalog [--json]   every document there is a template for", runner.docsCatalog}
	docsCommands["topics"] = docsCommand{"topics   what to settle about a project, in the order to ask", runner.docsTopics}
	docsCommands["gaps"] = docsCommand{"gaps PROJECT [--json]   what's left to write, the topics first", runner.docsGaps}
	docsCommands["gate"] = docsCommand{"gate PROJECT   what stops it being built; fails while something does", runner.docsGate}
}

// docsList reads the top-level notes' bodies alone, so a host's other notes
// cost nothing: the tree counts documents, an area page being known by its
// place and title. How many are approved takes reading them: docs summary.
func (r runner) docsList(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: enowx-app docs list")
	}
	list, err := r.notesSeen()
	if err != nil {
		return err
	}
	count := 0
	for _, n := range list {
		if n.Parent != "" {
			continue
		}
		full, err := r.c.Note(n.ID)
		if err != nil {
			return err
		}
		m, ok := projectdocs.Parse(full.Body)
		if !ok || !m.IsProject() {
			continue
		}
		docs := 0
		for _, id := range docsDescendants(list, n.ID) {
			if i := slices.IndexFunc(list, func(c proto.NoteInfo) bool { return c.ID == id }); i < 0 ||
				!(list[i].Parent == n.ID && slices.Contains(projectdocs.Areas, list[i].Title)) {
				docs++
			}
		}
		fmt.Fprintf(r.out, "%s  %s  %s, %d documents, approval %s\n", n.ID, n.Title, m.Size, docs, m.Approval)
		count++
	}
	if count == 0 {
		fmt.Fprintln(r.out, "No projects yet: \"enowx-app docs new TITLE\" makes one.")
	}
	return nil
}

func (r runner) docsSummary(args []string) error {
	const usage = "summary PROJECT [--json]"
	fs := docsFlags("summary")
	asJSON := fs.Bool("json", false, "")
	pos, err := docsParse(fs, args, usage)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: enowx-app docs " + usage)
	}
	p, _, err := r.docsProject(pos[0])
	if err != nil {
		return err
	}
	s := p.Summarize()
	if *asJSON {
		return writeJSON(r.out, docsSummaryJSON(p, s))
	}
	fmt.Fprintf(r.out, "%s  %s  %s, approval %s\n", p.Note.ID, p.Note.Title, p.Meta.Size, p.Meta.Approval)
	var by []string
	for _, st := range projectdocs.Statuses {
		if s.ByStatus[st] > 0 {
			by = append(by, fmt.Sprintf("%d %s", s.ByStatus[st], st))
		}
	}
	fmt.Fprintf(r.out, "%d documents: %s; to-dos %d/%d; %d stale; %d sections to fill\n", s.Docs, strings.Join(by, ", "), s.Done, s.Todos, s.Stale, s.Unfilled)
	for _, d := range p.Docs {
		note := ""
		if d.Changed {
			note += "; changed since approved"
		}
		for _, st := range d.Stale {
			note += fmt.Sprintf("; %s since based on it: %s", st.Reason, st.Link.Doc)
		}
		if len(d.Unfilled) > 0 {
			note += fmt.Sprintf("; %d to fill", len(d.Unfilled))
		}
		fmt.Fprintf(r.out, "%s  %s v%d  %d/%d  %s%s\n", d.ID, docStatus(d.Meta), max(d.Meta.Version, 1), d.Done, d.Todos, d.Title, note)
	}
	if len(s.OpenQuestions) > 0 {
		fmt.Fprintln(r.out, "Open questions:")
		for _, q := range s.OpenQuestions {
			fmt.Fprintf(r.out, "  %s\n", q)
		}
	}
	printBlockers(r, s.Blockers)
	return nil
}

func docStatus(m projectdocs.Meta) string {
	if m.Status == "" {
		return projectdocs.StatusDraft
	}
	return m.Status
}

func printBlockers(r runner, blockers []projectdocs.Blocker) {
	if len(blockers) == 0 {
		fmt.Fprintln(r.out, "Gate: open")
		return
	}
	fmt.Fprintln(r.out, "Gate: blocked")
	for _, b := range blockers {
		fmt.Fprintf(r.out, "  %s %s\n", b.Doc, b.Why)
	}
}

// docsSummaryJSON is a summary as the tools read it.
func docsSummaryJSON(p *projectdocs.Project, s projectdocs.Summary) any {
	type docJSON struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Kind     string   `json:"kind"`
		Status   string   `json:"status"`
		Version  int      `json:"version"`
		Done     int      `json:"done"`
		Todos    int      `json:"todos"`
		Changed  bool     `json:"changed,omitempty"`
		Stale    []string `json:"stale,omitempty"`
		Unfilled []string `json:"unfilled,omitempty"`
	}
	type blocker struct {
		Doc string `json:"doc"`
		Why string `json:"why"`
	}
	out := struct {
		ID            string         `json:"id"`
		Title         string         `json:"title"`
		Size          string         `json:"size"`
		Approval      string         `json:"approval"`
		ByStatus      map[string]int `json:"byStatus"`
		Done          int            `json:"done"`
		Todos         int            `json:"todos"`
		OpenQuestions []string       `json:"openQuestions,omitempty"`
		Stale         int            `json:"stale"`
		Unfilled      int            `json:"unfilled"`
		Blockers      []blocker      `json:"blockers"`
		Documents     []docJSON      `json:"documents"`
	}{ID: p.Note.ID, Title: p.Note.Title, Size: p.Meta.Size, Approval: p.Meta.Approval, ByStatus: s.ByStatus, Done: s.Done, Todos: s.Todos,
		OpenQuestions: s.OpenQuestions, Stale: s.Stale, Unfilled: s.Unfilled, Blockers: []blocker{}, Documents: []docJSON{}}
	for _, b := range s.Blockers {
		out.Blockers = append(out.Blockers, blocker{b.Doc, b.Why})
	}
	for _, d := range p.Docs {
		j := docJSON{ID: d.ID, Title: d.Title, Kind: d.Meta.Kind, Status: docStatus(d.Meta), Version: max(d.Meta.Version, 1), Done: d.Done, Todos: d.Todos, Changed: d.Changed, Unfilled: d.Unfilled}
		for _, st := range d.Stale {
			j.Stale = append(j.Stale, st.Link.Ref()+" "+st.Reason)
		}
		out.Documents = append(out.Documents, j)
	}
	return out
}

func (r runner) docsCatalog(args []string) error {
	const usage = "catalog [--json]"
	fs := docsFlags("catalog")
	asJSON := fs.Bool("json", false, "")
	pos, err := docsParse(fs, args, usage)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return errors.New("usage: enowx-app docs " + usage)
	}
	if *asJSON {
		type entry struct {
			Key  string `json:"key"`
			Name string `json:"name"`
			Area string `json:"area"`
			Core bool   `json:"core"`
			When string `json:"when"`
		}
		out := make([]entry, 0, len(projectdocs.Catalog))
		for _, e := range projectdocs.Catalog {
			out = append(out, entry{e.Key, e.Name, e.Area, e.Core, e.When})
		}
		return writeJSON(r.out, out)
	}
	for _, e := range projectdocs.Catalog {
		core := "full"
		if e.Core {
			core = "core"
		}
		fmt.Fprintf(r.out, "%s\t%s\t%s\t%s\t%s\n", e.Key, e.Name, e.Area, core, e.When)
	}
	return nil
}

func (r runner) docsTopics(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: enowx-app docs topics")
	}
	for _, t := range projectdocs.Topics {
		fmt.Fprintf(r.out, "%s\t%s\t%s#%s\t%s\n", t.Area, t.Name, t.Doc, t.Heading, t.Question)
	}
	return nil
}

func (r runner) docsGaps(args []string) error {
	const usage = "gaps PROJECT [--json]"
	fs := docsFlags("gaps")
	asJSON := fs.Bool("json", false, "")
	pos, err := docsParse(fs, args, usage)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: enowx-app docs " + usage)
	}
	p, _, err := r.docsProject(pos[0])
	if err != nil {
		return err
	}
	gaps := p.Gaps()
	if *asJSON {
		if gaps == nil {
			gaps = []projectdocs.Gap{}
		}
		return writeJSON(r.out, gaps)
	}
	if len(gaps) == 0 {
		fmt.Fprintln(r.out, "No gaps: every section is filled or skipped.")
	}
	for _, g := range gaps {
		where := g.Doc
		if g.SectionID != "" {
			where += "#" + g.SectionID
		}
		if g.Name != "" {
			fmt.Fprintf(r.out, "%s\t%s\t%s\t%s\n", g.Area, g.Name, where, g.Question)
		} else {
			fmt.Fprintf(r.out, "\t\t%s\n", where)
		}
	}
	return nil
}

func (r runner) docsGate(args []string) error {
	pos, err := docsParse(docsFlags("gate"), args, "gate PROJECT")
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: enowx-app docs gate PROJECT")
	}
	p, _, err := r.docsProject(pos[0])
	if err != nil {
		return err
	}
	blockers := p.Gate()
	printBlockers(r, blockers)
	if len(blockers) > 0 {
		return fmt.Errorf("docs: %q may not be built yet: %d blocking", p.Note.Title, len(blockers))
	}
	return nil
}

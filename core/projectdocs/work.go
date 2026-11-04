package projectdocs

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/enowx/enowx/core/noteblocks"
)

// Handing a project's work to agents: the tasks its Work Breakdown plans, the
// documents' words a task's prompt carries, and the criteria it is checked
// against. The prompt's own line says which documents a task came from, so
// the board needs no field an older daemon would drop.

// ContextLimit is how many bytes of documents a task's prompt carries: enough
// for a section and what it's based on, small enough to be typed into a
// session and read by the judge routing it.
const ContextLimit = 8000

// AcceptanceHeading names the sections whose to-dos are acceptance criteria.
const AcceptanceHeading = "Acceptance criteria"

const (
	workBreakdownKey = "work-breakdown"
	userStoriesKey   = "user-stories"
)

// PlannedTask is a to-do of the Work Breakdown, written
// "T-1: verb phrase (stage, depends on T-0)".
type PlannedTask struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Stage   string   `json:"stage,omitempty"`
	Depends []string `json:"depends,omitempty"`
	Done    bool     `json:"done"`
	// Section is the #ID of the section it's written in, "" before any.
	Section string `json:"section,omitempty"`
}

// Criterion is an acceptance criterion: a to-do under an "Acceptance
// criteria" heading.
type Criterion struct {
	Doc     string `json:"doc"` // the document's note ID
	Title   string `json:"title"`
	Section string `json:"section,omitempty"`
	Text    string `json:"text"`
	Done    bool   `json:"done"`
}

var (
	taskRe   = regexp.MustCompile(`^(T-\d+):\s*(.+)$`)
	taskIDRe = regexp.MustCompile(`\bT-\d+\b`)
	// Stage and dependencies close a task's line.
	taskTailRe = regexp.MustCompile(`^(.*?)\s*\(([^()]*)\)\s*$`)
	// Two words exactly: a body's own first line has more, or one.
	refLineRe = regexp.MustCompile(`^Project Docs: ([^\s#]+) ([^\s#]+)(?:#(\S*))?$`)
)

// WorkBreakdown is the project's Work Breakdown, nil without one.
func WorkBreakdown(p *Project) *Doc { return p.Doc(workBreakdownKey) }

// PlanTasks is the tasks of the project's Work Breakdown, in its order.
func PlanTasks(p *Project) []PlannedTask {
	d := WorkBreakdown(p)
	if d == nil {
		return nil
	}
	sections := noteblocks.Outline(d.Body)
	var out []PlannedTask
	eachTodo(d.Body, func(line int, done bool, text string) {
		m := taskRe.FindStringSubmatch(text)
		if m == nil {
			return
		}
		t := PlannedTask{ID: m[1], Text: m[2], Done: done, Section: innermost(sections, line)}
		if tail := taskTailRe.FindStringSubmatch(t.Text); tail != nil {
			t.Text = tail[1]
			t.Stage, t.Depends = readTaskTail(tail[2])
		}
		out = append(out, t)
	})
	return out
}

// readTaskTail reads "stage, depends on T-1, T-2": the dependencies run from
// "depends on" to the end of the list, unless a word that is no task ends them.
func readTaskTail(tail string) (stage string, depends []string) {
	inDepends := false
	for _, part := range strings.Split(tail, ",") {
		part = strings.TrimSpace(part)
		low := strings.ToLower(part)
		ids := taskIDRe.FindAllString(part, -1)
		switch {
		case strings.HasPrefix(low, "depends on"):
			inDepends = true
			depends = append(depends, ids...)
		case strings.HasPrefix(low, "no dep"):
		case inDepends && len(ids) > 0:
			depends = append(depends, ids...)
		case stage == "":
			inDepends = false
			stage = part
		}
	}
	return stage, depends
}

// Ready is the tasks not done, whose dependencies are all done and which busy
// doesn't say someone already has. A dependency the plan doesn't hold keeps a
// task waiting: starting it could be starting too early.
func Ready(tasks []PlannedTask, busy func(id string) bool) []PlannedTask {
	done := map[string]bool{}
	for _, t := range tasks {
		done[t.ID] = t.Done
	}
	var out []PlannedTask
next:
	for _, t := range tasks {
		if t.Done || busy != nil && busy(t.ID) {
			continue
		}
		for _, dep := range t.Depends {
			if !done[dep] {
				continue next
			}
		}
		out = append(out, t)
	}
	return out
}

// TaskIDs is the planned tasks a board task's title names: its title, as its
// prompt quotes whole sections, naming the rest of a stage's tasks.
func TaskIDs(title string) []string { return taskIDRe.FindAllString(title, -1) }

// resolve is the document and section ref names: DOC or DOC#SECTION.
func (p *Project) resolve(ref string) (*Doc, *noteblocks.Section, error) {
	name, section, _ := strings.Cut(ref, "#")
	d := p.Doc(strings.TrimSpace(name))
	if d == nil {
		titles := make([]string, 0, len(p.Docs))
		for _, doc := range p.Docs {
			titles = append(titles, doc.Title)
		}
		return nil, nil, fmt.Errorf("projectdocs: %q is no document of %s: %s", name, p.Note.Title, strings.Join(titles, ", "))
	}
	if strings.TrimSpace(section) == "" {
		return d, nil, nil
	}
	s, err := noteblocks.Find(noteblocks.Outline(d.Body), section)
	if err != nil {
		return nil, nil, fmt.Errorf("projectdocs: %s: %w", d.Title, err)
	}
	return d, &s, nil
}

// Criteria is the acceptance criteria of what ref names (DOC or DOC#SECTION),
// else, as most tasks' documents have none of their own, those of User Stories
// and Acceptance Criteria.
func Criteria(p *Project, ref string) []Criterion {
	if d, s, err := p.resolve(ref); err == nil {
		if out := criteriaIn(d, s); len(out) > 0 {
			return out
		}
	}
	if d := p.Doc(userStoriesKey); d != nil {
		return criteriaIn(d, nil)
	}
	return nil
}

// criteriaIn is d's to-dos under an "Acceptance criteria" heading of any depth,
// inside scope when it has one.
func criteriaIn(d *Doc, scope *noteblocks.Section) []Criterion {
	sections := noteblocks.Outline(d.Body)
	var out []Criterion
	eachTodo(d.Body, func(line int, done bool, text string) {
		if scope != nil && (line <= scope.Line || line >= scope.End) {
			return
		}
		for _, s := range sections {
			if strings.EqualFold(s.Title, AcceptanceHeading) && s.Line < line && line < s.End {
				out = append(out, Criterion{Doc: d.ID, Title: d.Title, Section: innermost(sections, line), Text: text, Done: done})
				return
			}
		}
	})
	return out
}

// eachTodo calls see for each to-do of body outside code.
func eachTodo(body string, see func(line int, done bool, text string)) {
	fenced := false
	for i, l := range noteblocks.Lines(body) {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fenced = !fenced
			continue
		}
		if done, text, ok := noteblocks.Todo(l); ok && !fenced {
			see(i, done, text)
		}
	}
}

// innermost is the #ID of the last section holding line: sections come
// outermost first.
func innermost(sections []noteblocks.Section, line int) string {
	in := ""
	for _, s := range sections {
		if s.Line < line && line < s.End {
			in = s.ID
		}
	}
	return in
}

// RefLine is the line of a prompt saying which documents a task came from.
func RefLine(project, doc, section string) string {
	if section == "" {
		return "Project Docs: " + project + " " + doc
	}
	return "Project Docs: " + project + " " + doc + "#" + section
}

// DocRefOf reads back the line RefLine wrote, ok false for a task that didn't
// come from Project Docs.
func DocRefOf(prompt string) (project, doc, section string, ok bool) {
	for _, line := range strings.Split(prompt, "\n") {
		if m := refLineRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			return m[1], m[2], m[3], true
		}
	}
	return "", "", "", false
}

// readCommand is how a worker reads a document, or one section of it.
func readCommand(doc, section string) string {
	if section == "" {
		return `"$ENOWX_BIN" notes show ` + doc
	}
	return `"$ENOWX_BIN" notes show ` + doc + " --section " + section
}

// contextPart is one thing a prompt quotes, and how to read it when it's cut.
type contextPart struct {
	name, text, read string
}

// TaskContext is what a task's prompt carries of the project for ref, DOC or
// DOC#SECTION: that section, the PRD's and Technical Spec's sections it's based
// on, the criteria not met yet. Without such links, the document's outline and
// how to read its sections. At most limit bytes, saying what was cut.
func TaskContext(p *Project, ref string, limit int) (string, error) {
	d, s, err := p.resolve(ref)
	if err != nil {
		return "", err
	}
	section, where := "", ""
	if s != nil {
		section = s.ID
		where = ", section " + s.Title
	}
	head := RefLine(p.Note.ID, d.ID, section) + "\n" +
		fmt.Sprintf("This task comes from the project %s, document %s%s. What follows is what its documents say.\n\n", p.Note.Title, d.Title, where)

	var parts []contextPart
	if s != nil {
		parts = append(parts, contextPart{name: d.Title + " #" + s.ID, text: noteblocks.Text(d.Body, *s) + "\n", read: readCommand(d.ID, s.ID)})
	}
	linked := linkedParts(p, d, s)
	if s == nil || len(linked) == 0 {
		parts = append(parts, outlinePart(p, d))
	}
	parts = append(parts, linked...)
	if c := openCriteriaPart(p, ref); c.text != "" {
		parts = append(parts, c)
	}
	return capContext(head, parts, limit, readCommand(d.ID, "")), nil
}

// linkedParts is the sections of the gated documents, the PRD and the Technical
// Spec, that d (or section s of it) says it's based on. A link to a whole
// document isn't quoted: it would crowd out everything else, and the outline
// says how to read one.
func linkedParts(p *Project, d *Doc, s *noteblocks.Section) []contextPart {
	sections := noteblocks.Outline(d.Body)
	seen := map[string]bool{}
	var out []contextPart
	for _, l := range d.Links {
		if l.Section == "" || !linkApplies(sections, l, s) {
			continue
		}
		src := p.Doc(l.Doc)
		if src == nil || src.Entry == nil || !src.Entry.Gate {
			continue
		}
		sec, err := noteblocks.Find(noteblocks.Outline(src.Body), l.Section)
		if err != nil {
			out = append(out, contextPart{name: src.Title + " #" + l.Section,
				text: fmt.Sprintf("Based on %s, section %s, which is no longer there.\n\n", src.Title, l.Section), read: readCommand(src.ID, "")})
			continue
		}
		key := src.ID + "#" + sec.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		note := ""
		if isStale(d, l) {
			note = " It has changed since this document was based on it: go by what it says now, and say so."
		}
		out = append(out, contextPart{name: src.Title + " #" + sec.ID,
			text: fmt.Sprintf("Based on %s:%s\n\n%s\n", src.Title, note, noteblocks.Text(src.Body, sec)), read: readCommand(src.ID, sec.ID)})
	}
	return out
}

// linkApplies says link l bears on section s: written in s, in a section of s,
// in a section holding s, or for the whole document. For the whole document,
// s nil, every link does.
func linkApplies(sections []noteblocks.Section, l Link, s *noteblocks.Section) bool {
	if s == nil || l.In == "" {
		return true
	}
	in, err := noteblocks.Find(sections, l.In)
	if err != nil {
		return false
	}
	return in.Line >= s.Line && in.End <= s.End || s.Line >= in.Line && s.End <= in.End
}

func isStale(d *Doc, l Link) bool {
	for _, st := range d.Stale {
		if st.Link == l {
			return true
		}
	}
	return false
}

// outlinePart is d's outline, and how to read its sections and those of the
// documents the project is built from.
func outlinePart(p *Project, d *Doc) contextPart {
	var b strings.Builder
	fmt.Fprintf(&b, "%s has these sections:\n", d.Title)
	for _, s := range noteblocks.Outline(d.Body) {
		fmt.Fprintf(&b, "%s- #%s %s\n", strings.Repeat("  ", max(s.Level-2, 0)), s.ID, s.Title)
	}
	fmt.Fprintf(&b, "Read a section with: %s\n", readCommand(d.ID, "S"))
	for _, e := range Catalog {
		if src := p.Doc(e.Key); e.Gate && src != nil && src.ID != d.ID {
			fmt.Fprintf(&b, "%s, the same way: %s\n", src.Title, readCommand(src.ID, "S"))
		}
	}
	b.WriteString("\n")
	return contextPart{name: "the outline of " + d.Title, text: b.String(), read: `"$ENOWX_BIN" notes outline ` + d.ID}
}

// openCriteriaPart is the criteria for ref not met yet.
func openCriteriaPart(p *Project, ref string) contextPart {
	var b strings.Builder
	read := ""
	for _, c := range Criteria(p, ref) {
		if c.Done {
			continue
		}
		if read == "" {
			read = readCommand(c.Doc, "")
			fmt.Fprintf(&b, "Acceptance criteria not met yet, from %s:\n", c.Title)
		}
		fmt.Fprintf(&b, "- [ ] %s\n", c.Text)
	}
	if read == "" {
		return contextPart{}
	}
	b.WriteString("\n")
	return contextPart{name: "the acceptance criteria", text: b.String(), read: read}
}

// capContext is head and parts in at most limit bytes: parts in their order
// until they no longer fit, cut at a line, and a line for each saying what is
// missing. Room for those lines is kept first, as what's cut must be findable.
// limit 0 or less is no cap.
func capContext(head string, parts []contextPart, limit int, readAll string) string {
	var whole strings.Builder
	whole.WriteString(head)
	for _, part := range parts {
		whole.WriteString(part.text)
	}
	if limit <= 0 || whole.Len() <= limit {
		return whole.String()
	}
	room := limit - len(head)
	for _, part := range parts {
		room -= len(cutNotice(part, strings.Count(part.text, "\n")))
	}
	if room < 0 {
		return hardCap(head+"Cut to fit: its documents. Read them with: "+readAll+"\n", limit)
	}
	var kept, notices strings.Builder
	kept.WriteString(head)
	for _, part := range parts {
		if len(part.text) <= room {
			kept.WriteString(part.text)
			room -= len(part.text)
			continue
		}
		fits := strings.LastIndexByte(part.text[:room], '\n') + 1
		kept.WriteString(part.text[:fits])
		notices.WriteString(cutNotice(part, strings.Count(part.text[fits:], "\n")))
		room = 0
	}
	return kept.String() + notices.String()
}

func cutNotice(part contextPart, lines int) string {
	return fmt.Sprintf("Cut to fit: %d lines of %s. Read them with: %s\n", lines, part.name, part.read)
}

// hardCap is s in at most limit bytes, cut at a line when it can be: the first
// line, which says where the task came from, goes last.
func hardCap(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	if at := strings.LastIndexByte(s[:limit], '\n'); at >= 0 {
		return s[:at+1]
	}
	return strings.ToValidUTF8(s[:limit], "")
}

package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/core/projectdocs"
)

// Changing documents: their status, what they're based on, their diagrams.

func init() {
	docsCommands["status"] = docsCommand{"status DOC [STATUS] [--in-project P]   read it, or set Draft, In review, Outdated", runner.docsStatus}
	docsCommands["approve"] = docsCommand{"approve DOC [--in-project P]   the owner's approval; refused in a session", runner.docsApprove}
	docsCommands["approval"] = docsCommand{"approval PROJECT required|off   whether it waits for approval; refused in a session", runner.docsApproval}
	docsCommands["link"] = docsCommand{"link DOC --to SOURCE[#SECTION] [--in SECTION] [--in-project P]   say what it's based on, at the source's current hash; again re-confirms after a review", runner.docsLink}
	docsCommands["diagram"] = docsCommand{"diagram DOC --canvas ID [--section S] [--from TEXT] [--in-project P]   embed a canvas in the section (default Diagram, or Canvases)", runner.docsDiagram}
}

func (r runner) docsStatus(args []string) error {
	const usage = "status DOC [STATUS] [--in-project P]"
	fs := docsFlags("status")
	inProject := fs.String("in-project", "", "")
	pos, err := docsParse(fs, args, usage)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return errors.New("usage: enowx-app docs " + usage)
	}
	_, d, err := r.docsDoc(pos[0], *inProject)
	if err != nil {
		return err
	}
	if len(pos) == 1 {
		line := fmt.Sprintf("%s v%d", docStatus(d.Meta), max(d.Meta.Version, 1))
		if d.Changed {
			line += ", changed since approved"
		}
		fmt.Fprintln(r.out, line)
		return nil
	}
	status, err := statusNamed(strings.Join(pos[1:], " ")) // "In review" unquoted reads as two
	if err != nil {
		return err
	}
	if status == projectdocs.StatusApproved {
		if err := docsOwnerOnly("approves"); err != nil {
			return err
		}
	}
	n := d.Note
	if n.Body, err = projectdocs.SetStatus(n.Body, status); err != nil {
		return err
	}
	// Leaving Approved is a change to an approved document, and the guard's
	// proposal would keep it approved: this one says what's asked.
	if agentSession() && d.Meta.Status == projectdocs.StatusApproved && status != projectdocs.StatusApproved {
		fmt.Fprintf(r.out, "%s is approved: changes to it are proposed\n", d.Meta.Kind)
		return r.proposeNote(n)
	}
	saved, err := r.docsSave(d.Body, n)
	if saved {
		fmt.Fprintf(r.out, "%s is %s.\n", d.Title, status)
	}
	return err
}

func (r runner) docsApprove(args []string) error {
	const usage = "approve DOC [--in-project P]"
	fs := docsFlags("approve")
	inProject := fs.String("in-project", "", "")
	pos, err := docsParse(fs, args, usage)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: enowx-app docs " + usage)
	}
	if err := docsOwnerOnly("approves"); err != nil {
		return err
	}
	_, d, err := r.docsDoc(pos[0], *inProject)
	if err != nil {
		return err
	}
	n := d.Note
	if n.Body, err = projectdocs.Approve(n.Body); err != nil {
		return err
	}
	if _, err := r.c.SaveNote(n); err != nil {
		return err
	}
	m, _ := projectdocs.Parse(n.Body)
	fmt.Fprintf(r.out, "%s is approved, v%d.\n", d.Title, m.Version)
	return nil
}

func (r runner) docsApproval(args []string) error {
	const usage = "approval PROJECT required|off"
	pos, err := docsParse(docsFlags("approval"), args, usage)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return errors.New("usage: enowx-app docs " + usage)
	}
	if err := docsOwnerOnly("changes whether a project needs approval"); err != nil {
		return err
	}
	p, _, err := r.docsProject(pos[0])
	if err != nil {
		return err
	}
	n := p.Note
	if n.Body, err = projectdocs.SetApproval(n.Body, pos[1]); err != nil {
		return err
	}
	_, err = r.c.SaveNote(n)
	return err
}

func (r runner) docsLink(args []string) error {
	const usage = "link DOC --to SOURCE[#SECTION] [--in SECTION] [--in-project P]"
	fs := docsFlags("link")
	to := fs.String("to", "", "")
	in := fs.String("in", "", "")
	inProject := fs.String("in-project", "", "")
	pos, err := docsParse(fs, args, usage)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *to == "" {
		return errors.New("usage: enowx-app docs " + usage)
	}
	p, d, err := r.docsDoc(pos[0], *inProject)
	if err != nil {
		return err
	}
	source, section, _ := strings.Cut(*to, "#")
	src := p.Doc(strings.TrimSpace(source))
	if src == nil {
		return fmt.Errorf("docs: %q has no document %q", p.Note.Title, source)
	}
	if src.ID == d.ID {
		return errors.New("docs: a document can't be based on itself")
	}
	l := projectdocs.Link{Doc: src.Title, In: *in, Hash: projectdocs.ContentHash(src.Body)}
	if section = strings.TrimSpace(section); section != "" {
		s, err := noteblocks.Find(noteblocks.Outline(src.Body), section)
		if err != nil {
			return fmt.Errorf("docs: %s: %w", src.Title, err)
		}
		l.Section = s.Title
		if l.Hash, err = projectdocs.SectionHash(src.Body, s.ID); err != nil {
			return err
		}
	}
	n := d.Note
	if n.Body, err = projectdocs.AddLink(n.Body, l); err != nil {
		return err
	}
	saved, err := r.docsSave(d.Body, n)
	if saved {
		fmt.Fprintf(r.out, "%s is based on %s\n", d.Title, l.Ref())
	}
	return err
}

func (r runner) docsDiagram(args []string) error {
	const usage = "diagram DOC --canvas ID [--section S] [--from TEXT] [--in-project P]"
	fs := docsFlags("diagram")
	canvasRef := fs.String("canvas", "", "")
	section := fs.String("section", "", "")
	from := fs.String("from", "", "")
	inProject := fs.String("in-project", "", "")
	pos, err := docsParse(fs, args, usage)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *canvasRef == "" {
		return errors.New("usage: enowx-app docs " + usage)
	}
	_, d, err := r.docsDoc(pos[0], *inProject)
	if err != nil {
		return err
	}
	c, err := r.c.Canvas(*canvasRef) // only a canvas that's there is embedded
	if err != nil {
		return fmt.Errorf("docs: canvas %q: %w", *canvasRef, err)
	}
	s, err := docsDiagramSection(d.Body, *section)
	if err != nil {
		return fmt.Errorf("docs: %s: %w", d.Title, err)
	}
	add := []string{"::embed canvas:" + c.ID}
	if *from != "" {
		add = append(add, "", fmt.Sprintf("Drawn from: %s, checked %s", *from, time.Now().Format("2006-01-02")))
	}
	n := d.Note
	n.Body = docsPlaceInSection(d.Body, s, add)
	saved, err := r.docsSave(d.Body, n)
	if saved {
		fmt.Fprintf(r.out, "%s shows canvas %s in %s\n", d.Title, c.ID, s.Title)
	}
	return err
}

// docsDiagramSection is the section named, or a document's usual one.
func docsDiagramSection(body, name string) (noteblocks.Section, error) {
	sections := noteblocks.Outline(body)
	if name != "" {
		return noteblocks.Find(sections, name)
	}
	if s, err := noteblocks.Find(sections, "Diagram"); err == nil {
		return s, nil
	}
	s, err := noteblocks.Find(sections, "Canvases")
	if err != nil {
		return s, errors.New("no section Diagram or Canvases: say which with --section")
	}
	return s, nil
}

// docsPlaceInSection puts lines where the section's first To fill line is,
// or at its end when it has none left.
func docsPlaceInSection(body string, s noteblocks.Section, add []string) string {
	lines := noteblocks.Lines(body)
	end := min(s.End, len(lines))
	for i := s.Line + 1; i < end; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), projectdocs.ToFill) {
			return strings.Join(append(lines[:i:i], append(add, lines[i+1:]...)...), "\n")
		}
	}
	for end > s.Line+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	tail := append([]string{""}, add...)
	if end < len(lines) {
		tail = append(tail, "")
	}
	return strings.Join(append(lines[:end:end], append(tail, lines[end:]...)...), "\n")
}

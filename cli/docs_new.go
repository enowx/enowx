package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

// Making a project and adding to it.

func init() {
	docsCommands["new"] = docsCommand{"new TITLE [--size core|full] [--approval required|off] [--with KEY,KEY] [--template NAME] [--group G] [--team T] [--project P [--host H]]   a project and its documents; its ID printed", runner.docsNew}
	docsCommands["add"] = docsCommand{"add PROJECT KEY   a document of the catalog or a template of yours", runner.docsAdd}
	docsCommands["skip"] = docsCommand{"skip PROJECT KEY --why TEXT   record why the project doesn't need a document", runner.docsSkip}
	docsCommands["attach"] = docsCommand{"attach PROJECT --project P [--project P2] [--host H]   the project and every document, to projects of any host", runner.docsAttach}
}

func (r runner) docsNew(args []string) error {
	const usage = "new TITLE [--size core|full] [--approval required|off] [--with KEY,KEY] [--template NAME] [--group G] [--team T] [--project P [--host H]]"
	fs := docsFlags("new")
	size := fs.String("size", projectdocs.SizeCore, "")
	approval := fs.String("approval", projectdocs.ApprovalRequired, "")
	with := fs.String("with", "", "")
	group := fs.String("group", "", "")
	teamName := fs.String("team", "", "")
	host := fs.String("host", "", "")
	var templates, projects multi
	fs.Var(&templates, "template", "")
	fs.Var(&projects, "project", "")
	pos, err := docsParse(fs, args, usage)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: enowx-app docs " + usage)
	}
	if err := docsMayMake(); err != nil {
		return err
	}
	if *size != projectdocs.SizeCore && *size != projectdocs.SizeFull {
		return fmt.Errorf("docs: size is %s or %s", projectdocs.SizeCore, projectdocs.SizeFull)
	}
	if *approval != projectdocs.ApprovalRequired && *approval != projectdocs.ApprovalOff {
		return fmt.Errorf("docs: approval is %s or %s", projectdocs.ApprovalRequired, projectdocs.ApprovalOff)
	}
	team, err := r.docsTeam(*teamName)
	if err != nil {
		return err
	}
	// Whatever can be refused is, before anything is made.
	var own []proto.Note
	for _, name := range templates {
		t, err := r.docsTemplate(name)
		if err != nil {
			return err
		}
		own = append(own, t)
	}
	attach, err := r.docsAttachTargets(projects, *host)
	if err != nil {
		return err
	}
	var extra []string
	for _, key := range strings.Split(*with, ",") {
		if key = strings.TrimSpace(key); key != "" {
			extra = append(extra, key)
		}
	}
	root, made, err := projectdocs.New(projectdocs.Options{Title: pos[0], Size: *size, Approval: *approval, Group: *group, Team: team, Extra: extra, Own: own}, r.c.SaveNote)
	if err == nil && len(attach) > 0 {
		for _, id := range made {
			if _, err = r.c.AttachNote(id, attach); err != nil {
				break
			}
		}
	}
	if err != nil {
		return r.docsTakeBack(made, err)
	}
	fmt.Fprintln(r.out, root.ID)
	return nil
}

// docsAttachTargets are the projects a new project goes to: those named, or
// this session's, as a note made in a project's session belongs to it.
func (r runner) docsAttachTargets(specs []string, host string) ([]proto.NoteProject, error) {
	if len(specs) == 0 {
		if !agentSession() {
			return nil, nil
		}
		found, err := r.resolveProjects(".", "")
		if err != nil {
			return nil, nil // a session outside any project: nothing to attach to
		}
		return found, nil
	}
	var out []proto.NoteProject
	for _, spec := range specs {
		found, err := r.resolveProjects(spec, host)
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
	return out, nil
}

// docsTakeBack removes what a failed docs new made, deepest first.
func (r runner) docsTakeBack(made []string, cause error) error {
	var left []string
	for _, id := range slices.Backward(made) {
		if err := r.c.DeleteNote(id); err != nil {
			left = append(left, id)
		}
	}
	if len(left) > 0 {
		return fmt.Errorf("%w (and %s couldn't be removed)", cause, strings.Join(left, ", "))
	}
	return cause
}

func (r runner) docsAdd(args []string) error {
	const usage = "add PROJECT KEY"
	pos, err := docsParse(docsFlags("add"), args, usage)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return errors.New("usage: enowx-app docs " + usage)
	}
	if err := docsMayMake(); err != nil {
		return err
	}
	p, _, err := r.docsProject(pos[0])
	if err != nil {
		return err
	}
	n := proto.Note{NoteInfo: proto.NoteInfo{Parent: p.Note.ID}}
	if e := projectdocs.Find(pos[1]); e != nil {
		if p.Doc(e.Name) != nil {
			return fmt.Errorf("docs: %q already has %s", p.Note.Title, e.Name)
		}
		if n.Body, err = projectdocs.DocBody(*e); err != nil {
			return err
		}
		n.Title, n.Icon = e.Name, e.Icon
		if p.Meta.Size == projectdocs.SizeFull {
			if n.Parent, err = r.docsAreaPage(p, e.Area); err != nil {
				return err
			}
		}
	} else {
		t, err := r.docsTemplate(pos[1])
		if err != nil {
			return fmt.Errorf("docs: no document %q in the catalog (docs catalog), and %w", pos[1], err)
		}
		if p.Doc(t.Title) != nil {
			return fmt.Errorf("docs: %q already has %s", p.Note.Title, t.Title)
		}
		n.Title, n.Icon = t.Title, t.Icon
		n.Body = projectdocs.WithMeta(t.Body, projectdocs.Meta{Kind: t.Title, Status: projectdocs.StatusDraft, Version: 1})
	}
	info, err := r.c.SaveNote(n)
	if err != nil {
		return err
	}
	if len(p.Note.Projects) > 0 {
		if _, err := r.c.AttachNote(info.ID, p.Note.Projects); err != nil {
			return err
		}
	}
	fmt.Fprintln(r.out, info.ID)
	return nil
}

// docsAreaPage is the page of area in p, made when missing.
func (r runner) docsAreaPage(p *projectdocs.Project, area string) (string, error) {
	for _, a := range p.Areas {
		if a.Title == area {
			return a.ID, nil
		}
	}
	info, err := r.c.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: area, Icon: "lucide:folder", Parent: p.Note.ID},
		Body: projectdocs.Meta{Kind: projectdocs.KindArea}.Line() + "\n\n::pages\n"})
	if err != nil {
		return "", err
	}
	if len(p.Note.Projects) > 0 {
		if _, err := r.c.AttachNote(info.ID, p.Note.Projects); err != nil {
			return "", err
		}
	}
	return info.ID, nil
}

const docsSkippedHeading = "Skipped documents"

func (r runner) docsSkip(args []string) error {
	const usage = "skip PROJECT KEY --why TEXT"
	fs := docsFlags("skip")
	why := fs.String("why", "", "")
	pos, err := docsParse(fs, args, usage)
	if err != nil {
		return err
	}
	if len(pos) != 2 || strings.TrimSpace(*why) == "" {
		return errors.New("usage: enowx-app docs " + usage)
	}
	e := projectdocs.Find(pos[1])
	if e == nil {
		return fmt.Errorf("docs: no document %q in the catalog (docs catalog)", pos[1])
	}
	p, _, err := r.docsProject(pos[0])
	if err != nil {
		return err
	}
	if p.Doc(e.Name) != nil {
		return fmt.Errorf("docs: %q has %s: it can't be skipped while it's there", p.Note.Title, e.Name)
	}
	line := "- " + e.Name + ": " + strings.TrimSpace(*why)
	n := p.Note
	n.Body, err = sectionWritten(n.Body, docsSkippedHeading, "", line, true)
	if err != nil {
		n.Body = strings.TrimRight(p.Note.Body, "\n") + "\n\n## " + docsSkippedHeading + "\n\n" + line + "\n"
	}
	_, err = r.docsSave(p.Note.Body, n)
	return err
}

func (r runner) docsAttach(args []string) error {
	const usage = "attach PROJECT --project P [--project P2] [--host H]"
	fs := docsFlags("attach")
	host := fs.String("host", "", "")
	var projects multi
	fs.Var(&projects, "project", "")
	pos, err := docsParse(fs, args, usage)
	if err != nil {
		return err
	}
	if len(pos) != 1 || len(projects) == 0 {
		return errors.New("usage: enowx-app docs " + usage)
	}
	if err := docsMayMake(); err != nil {
		return err
	}
	p, list, err := r.docsProject(pos[0])
	if err != nil {
		return err
	}
	add, err := r.docsAttachTargets(projects, *host)
	if err != nil {
		return err
	}
	ids := append([]string{p.Note.ID}, docsDescendants(list, p.Note.ID)...)
	for _, id := range ids {
		if _, err := r.c.AttachNote(id, add); err != nil {
			return err
		}
	}
	fmt.Fprintf(r.out, "Attached %d notes.\n", len(ids))
	return nil
}

package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

// The user's own templates: ordinary notes under one top-level note.

const docsTemplatesTitle = "Project Docs templates"

func init() {
	docsCommands["templates"] = docsCommand{"templates   your own templates", runner.docsTemplates}
	docsCommands["template"] = docsCommand{"template save NOTE [--name N]   keep a note as a template, without what records its state", runner.docsTemplateCmd}
}

// docsTemplatesNote is the note holding the templates, with the notes seen;
// nil when there is none yet.
func (r runner) docsTemplatesNote() (*proto.NoteInfo, []proto.NoteInfo, error) {
	list, err := r.notesSeen()
	if err != nil {
		return nil, nil, err
	}
	for i, n := range list {
		if n.Parent != "" || !strings.EqualFold(n.Title, docsTemplatesTitle) {
			continue
		}
		full, err := r.c.Note(n.ID)
		if err != nil {
			return nil, nil, err
		}
		if m, ok := projectdocs.Parse(full.Body); ok && m.Kind == projectdocs.KindTemplates {
			return &list[i], list, nil
		}
	}
	return nil, list, nil
}

// docsTemplate is the user's template named name.
func (r runner) docsTemplate(name string) (proto.Note, error) {
	root, list, err := r.docsTemplatesNote()
	if err != nil {
		return proto.Note{}, err
	}
	if root != nil {
		for _, n := range list {
			if n.Parent == root.ID && (n.ID == name || strings.EqualFold(n.Title, name)) {
				return r.c.Note(n.ID)
			}
		}
	}
	return proto.Note{}, fmt.Errorf("no template %q of yours (docs templates)", name)
}

func (r runner) docsTemplates(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: enowx-app docs templates")
	}
	root, list, err := r.docsTemplatesNote()
	if err != nil {
		return err
	}
	count := 0
	if root != nil {
		for _, n := range list {
			if n.Parent == root.ID {
				fmt.Fprintf(r.out, "%s  %s\n", n.ID, n.Title)
				count++
			}
		}
	}
	if count == 0 {
		fmt.Fprintln(r.out, "No templates yet: \"enowx-app docs template save NOTE\" keeps one.")
	}
	return nil
}

func (r runner) docsTemplateCmd(args []string) error {
	const usage = "template save NOTE [--name N]"
	fs := docsFlags("template")
	name := fs.String("name", "", "")
	pos, err := docsParse(fs, args, usage)
	if err != nil {
		return err
	}
	if len(pos) != 2 || pos[0] != "save" {
		return errors.New("usage: enowx-app docs " + usage)
	}
	if err := docsMayMake(); err != nil {
		return err
	}
	src, err := r.findNote(pos[1])
	if err != nil {
		return err
	}
	full, err := r.c.Note(src.ID)
	if err != nil {
		return err
	}
	title := strings.TrimSpace(*name)
	if title == "" {
		title = full.Title
	}
	root, list, err := r.docsTemplatesNote()
	if err != nil {
		return err
	}
	if root == nil {
		team, _ := sessionTeam()
		info, err := r.c.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: docsTemplatesTitle, Icon: "lucide:layout-template", Team: team},
			Body: projectdocs.Meta{Kind: projectdocs.KindTemplates}.Line() + "\n\n::pages\n"})
		if err != nil {
			return err
		}
		root = &info
	}
	t := proto.Note{NoteInfo: proto.NoteInfo{Title: title, Icon: full.Icon, Parent: root.ID}, Body: projectdocs.StripMeta(full.Body)}
	// Saved again under its name, a template is brought up to date.
	for _, n := range list {
		if n.Parent == root.ID && strings.EqualFold(n.Title, title) {
			t.NoteInfo = n
			t.Icon = full.Icon
		}
	}
	info, err := r.c.SaveNote(t)
	if err != nil {
		return err
	}
	fmt.Fprintln(r.out, info.ID)
	return nil
}

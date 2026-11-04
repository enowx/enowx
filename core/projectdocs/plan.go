package projectdocs

import (
	"fmt"
	"strings"

	"github.com/enowx/enowx/proto"
)

// Options say what a new project is made of.
type Options struct {
	Title, Size, Approval string
	Icon, Group, Team     string
	// Extra adds entries of the catalog, by key, to the size's; Own are the
	// user's templates to add, with their bodies.
	Extra []string
	Own   []proto.Note
}

// Saver saves a note and says what it became.
type Saver func(proto.Note) (proto.NoteInfo, error)

// New makes a project: its note, then its documents as ordinary notes under
// it. Should a save fail, the IDs made so far come back with the error, for
// the caller to remove.
func New(o Options, save Saver) (proto.NoteInfo, []string, error) {
	if strings.TrimSpace(o.Title) == "" {
		return proto.NoteInfo{}, nil, fmt.Errorf("projectdocs: a project needs a title")
	}
	if o.Size != SizeFull {
		o.Size = SizeCore
	}
	if o.Approval != ApprovalOff {
		o.Approval = ApprovalRequired
	}
	icon := o.Icon
	if icon == "" {
		icon = "lucide:folder-kanban"
	}
	root, err := save(proto.Note{NoteInfo: proto.NoteInfo{Title: o.Title, Icon: icon, Group: o.Group, Team: o.Team},
		Body: ProjectBody(o.Size, o.Approval)})
	if err != nil {
		return proto.NoteInfo{}, nil, err
	}
	made := []string{root.ID}
	entries := Pack(o.Size)
	for _, key := range o.Extra {
		e := Find(key)
		if e == nil {
			return root, made, fmt.Errorf("projectdocs: no template %q", key)
		}
		if !hasEntry(entries, e.Key) {
			entries = append(entries, *e)
		}
	}
	areas := map[string]string{}
	for _, e := range entries {
		parent := root.ID
		if o.Size == SizeFull {
			if areas[e.Area] == "" {
				a, err := save(proto.Note{NoteInfo: proto.NoteInfo{Title: e.Area, Icon: "lucide:folder", Parent: root.ID},
					Body: Meta{Kind: KindArea}.Line() + "\n\n::pages\n"})
				if err != nil {
					return root, made, err
				}
				areas[e.Area] = a.ID
				made = append(made, a.ID)
			}
			parent = areas[e.Area]
		}
		body, err := DocBody(e)
		if err != nil {
			return root, made, err
		}
		n, err := save(proto.Note{NoteInfo: proto.NoteInfo{Title: e.Name, Icon: e.Icon, Parent: parent}, Body: body})
		if err != nil {
			return root, made, err
		}
		made = append(made, n.ID)
	}
	for _, own := range o.Own {
		kind := own.Title
		n, err := save(proto.Note{NoteInfo: proto.NoteInfo{Title: own.Title, Icon: own.Icon, Parent: root.ID},
			Body: WithMeta(own.Body, Meta{Kind: kind, Status: StatusDraft, Version: 1})})
		if err != nil {
			return root, made, err
		}
		made = append(made, n.ID)
	}
	return root, made, nil
}

func hasEntry(list []Entry, key string) bool {
	for _, e := range list {
		if e.Key == key {
			return true
		}
	}
	return false
}

// ProjectBody is a new project note's body.
func ProjectBody(size, approval string) string {
	return Meta{Kind: KindProject, Size: size, Approval: approval}.Line() + `

::progress

## Documents

::pages

## Open questions

## Skipped documents

Documents of the catalog this project doesn't need, each with why.
`
}

// DocBody is a new document's body from its template.
func DocBody(e Entry) (string, error) {
	t, err := Template(e.Key)
	if err != nil {
		return "", err
	}
	return Meta{Kind: e.Name, Status: StatusDraft, Version: 1}.Line() + "\n\n" + strings.TrimLeft(t, "\n"), nil
}

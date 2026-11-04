package projectdocsview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/widget"
)

// The Add document menu's frame, whatever it lists; it scrolls inside.
const (
	addMenuWidth  = 280
	addMenuHeight = 320
)

// addButton is Add document: a menu of the catalog not yet in the project, by
// area, and the user's templates.
func (v *View) addButton(c *ui.Context, o Options, pg Page, meta projectdocs.Meta, m *model) {
	if m == nil {
		return
	}
	b := actionButton(c, icon.Plus, "Add document")
	v.menu.OpenBelow(b, "add", 28)
	own := userTemplates(o.Notes)
	width, height := c.Size()
	v.menu.Show(c, b, "add", func(done func()) {
		ui.Scroll(c).Width(min(addMenuWidth, max(160, width-32))).Height(min(addMenuHeight, max(120, height-120))).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, a := range m.missing {
					widget.PopupLabel(c.Key("area:"+a.Area), a.Area)
					for _, e := range a.Entries {
						if widget.MenuItem(c.Key(e.Key), entryIcon(e.Icon), e.Name, false) {
							v.addEntry(o, m, e)
							done()
						}
					}
				}
				if len(own) == 0 {
					return
				}
				widget.PopupLabel(c, "Your templates")
				for _, n := range own {
					if widget.MenuItem(c.Key(n.ID), entryIcon(n.Icon), titleOf(n), false) {
						v.addOwn(o, m, n)
						done()
					}
				}
			})
		})
	})
}

// addEntry adds a catalog's document: under its area in a full pack, making
// the area if it's the first of it, as making the project does.
func (v *View) addEntry(o Options, m *model, e projectdocs.Entry) {
	body, err := projectdocs.DocBody(e)
	if err != nil {
		v.tell(err.Error(), true)
		return
	}
	root := m.project.Note.ID
	doc := proto.Note{NoteInfo: proto.NoteInfo{Title: e.Name, Icon: e.Icon, Parent: root}, Body: body}
	if m.project.Meta.Size != projectdocs.SizeFull {
		v.addNote(o, doc)
		return
	}
	for _, a := range m.project.Areas {
		if a.Title == e.Area {
			doc.Parent = a.ID
			v.addNote(o, doc)
			return
		}
	}
	o.Save(proto.Note{NoteInfo: proto.NoteInfo{Title: e.Area, Icon: "lucide:folder", Parent: root},
		Body: projectdocs.Meta{Kind: projectdocs.KindArea}.Line() + "\n\n::pages\n"}, func(a proto.NoteInfo, err error) {
		if err != nil {
			v.tell(err.Error(), true)
			return
		}
		doc.Parent = a.ID
		v.addNote(o, doc)
	})
}

// addOwn adds a document from one of the user's templates.
func (v *View) addOwn(o Options, m *model, n proto.NoteInfo) {
	o.Load(n.ID, func(full proto.Note, err error) {
		if err != nil {
			v.tell(err.Error(), true)
			return
		}
		title := titleOf(n)
		v.addNote(o, proto.Note{NoteInfo: proto.NoteInfo{Title: title, Icon: n.Icon, Parent: m.project.Note.ID},
			Body: projectdocs.WithMeta(full.Body, projectdocs.Meta{Kind: title, Status: projectdocs.StatusDraft, Version: 1})})
	})
}

func (v *View) addNote(o Options, n proto.Note) {
	o.Save(n, func(_ proto.NoteInfo, err error) {
		if err != nil {
			v.tell(err.Error(), true)
			return
		}
		v.tell("Added “"+n.Title+"”", false)
	})
}

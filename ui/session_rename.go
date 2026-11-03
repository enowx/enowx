package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/widget"
)

// Renaming a session: its menu's Rename…, or a double click on it in the
// list. The name lives on the host, so every app of it shows the same;
// an empty one goes back to the name the session takes by itself.

// renameForm is the session being renamed.
type renameForm struct {
	open bool
	id   string
	name string
	was  string // the name shown when it opened
}

// askRename opens the Rename dialog for session s.
func (a *App) askRename(s proto.SessionInfo) {
	a.renaming = &renameForm{open: true, id: s.ID, name: s.Name, was: a.sessionName(s)}
}

func (a *App) renameDialog(c *ui.Context) {
	f := a.renaming
	if f == nil {
		return
	}
	if !f.open {
		a.renaming = nil
		return
	}
	save := false
	widget.FormSheet(c, &f.open, 480, "Rename session", "", func() {
		widget.Field(c, "Name", func() {
			if widget.Input(c, &f.name, f.was).FillWidth().AutoFocus().Submitted() {
				save = true
			}
		})
	}, func() {
		if widget.PrimaryButton(c, nil, "Rename", "").Clicked() {
			save = true
		}
	})
	if !save {
		return
	}
	f.open = false
	cl, id, name := a.client, f.id, strings.Join(strings.Fields(f.name), " ") // as the host keeps it
	if cl == nil {
		return
	}
	a.renameLocally(id, name)
	go func() {
		if err := cl.RenameSession(id, name); err != nil {
			a.onMain(func() { a.status = "Couldn't rename the session: " + client.Explain(err) })
		}
	}()
}

// renameLocally shows the new name at once, the daemon's update following.
func (a *App) renameLocally(id, name string) {
	for i := range a.sessions {
		if a.sessions[i].ID == id {
			a.sessions[i].Name = name
		}
	}
}

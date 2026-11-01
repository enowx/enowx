package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Moving or copying a note (with the pages under it) or a canvas to
// another space (docs/cloud.md, Personal and team): the dialog names who
// will see it there; the host scans it for secrets first, and lets a
// team's items out only when its admins allow it.

type spaceMoveForm struct {
	open   bool
	note   *proto.NoteInfo
	canvas *proto.CanvasInfo
	title  string
	from   string // the team it is in, "" the user's own
	to     string // the space picked: a team, or spacePersonal
	copy   bool
	teams  map[string]api.Team // the teams asked about, with their members
	busy   bool
	err    string
}

// spacePersonal picks the user's own space in the dialog.
const spacePersonal = "personal"

func (a *App) openNoteSpaceMove(n proto.NoteInfo) {
	a.openSpaceMove(&spaceMoveForm{note: &n, title: noteTitle(n), from: n.Team})
}

func (a *App) openCanvasSpaceMove(cv proto.CanvasInfo) {
	a.openSpaceMove(&spaceMoveForm{canvas: &cv, title: cv.Title, from: cv.Team})
}

func (a *App) openSpaceMove(f *spaceMoveForm) {
	f.open, f.teams = true, map[string]api.Team{}
	if f.from != "" {
		f.to, f.copy = spacePersonal, true
	}
	for _, sp := range a.spacesOf().list {
		if f.to == "" && sp.Team != f.from && sp.Role != api.RoleViewer {
			f.to = sp.Team
		}
	}
	a.spaceMoving = f
	cl := a.client
	for _, sp := range a.spacesOf().list {
		team := sp.Team
		go func() {
			var t api.Team
			if cl.CloudCall("GET", teamPath(api.PathTeam, team), nil, &t) == nil {
				a.onMain(func() { f.teams[team] = t })
			}
		}()
	}
}

// spacesToMove are the spaces an item of space from may go to.
func (a *App) spacesToMove(from string) []proto.Space {
	var out []proto.Space
	if from != "" {
		out = append(out, proto.Space{Team: spacePersonal, Name: "Personal"})
	}
	for _, sp := range a.spacesOf().list {
		if sp.Team != from && sp.Role != api.RoleViewer {
			out = append(out, sp)
		}
	}
	return out
}

func (a *App) spaceMoveDialog(c *ui.Context) {
	f := a.spaceMoving
	if f == nil {
		return
	}
	if !f.open {
		a.spaceMoving = nil
		return
	}
	what := "note"
	if f.canvas != nil {
		what = "canvas"
	}
	widget.FormSheet(c, &f.open, 500, "Move or copy “"+f.title+"”", "To your own space or a team's: a team sees it, and keeps it when you leave.", func() {
		widget.Field(c, "To", func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				for _, sp := range a.spacesToMove(f.from) {
					a.spaceMoveChoice(c, f, sp)
				}
			})
		})
		out := f.from != "" // out of a team: a copy, when its admins allow it
		allowed := true
		if out {
			t, asked := f.teams[f.from]
			allowed = asked && t.CopyOut
			f.copy = true
			if asked && !allowed {
				widget.Notice(c, icon.Alert, theme.Warning, a.spaceName(f.from)+"'s admins keep its notes and canvases in the team. They can let members copy them out in the team's Options.", "", nil)
			}
		} else {
			mode := 0
			if f.copy {
				mode = 1
			}
			widget.Field(c, "As", func() {
				ui.Row(c).FillWidth().Children(func() {
					if i := widget.Segmented(c, []string{"Move", "Copy"}, mode, nil); i >= 0 {
						f.copy = i == 1
					}
				})
			})
		}
		ui.Text(c, a.spaceMoveSays(f, what)).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
	}, func() {
		where := "your space"
		if f.to != spacePersonal && f.to != "" {
			where = a.spaceName(f.to)
		}
		label := "Move to " + where
		if f.copy {
			label = "Copy to " + where
		}
		b := widget.PrimaryButton(c, nil, label, "")
		if f.busy || f.to == "" || f.from != "" && !f.teams[f.from].CopyOut {
			b.Disabled(true)
		} else if b.Clicked() {
			a.moveToSpace(f)
		}
	})
}

func (a *App) spaceMoveChoice(c *ui.Context, f *spaceMoveForm, sp proto.Space) {
	on := f.to == sp.Team
	row := ui.ButtonBase(c.Key("to:"+sp.Team)).FillWidth().Padding(8, 10).Gap(10).AlignItems(ui.Center).Justify(ui.Start).
		Radius(theme.RadiusControl).Border(1, theme.Border).Cursor(ui.CursorPointer).Label("To " + sp.Name)
	if on {
		row.Background(theme.AccentSoft).Border(1, theme.AccentLine)
	} else if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.Children(func() {
		if sp.Team == spacePersonal {
			ui.Box(c).Size(22, 22).Radius(theme.RadiusSmall).Background(theme.Hover).Center().Children(func() {
				ui.Icon(c, icon.Account).FontSize(13).TextColor(theme.TextSoft)
			})
		} else {
			widget.TeamMark(c, sp.Team, sp.Name, 22)
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			ui.Text(c, sp.Name).FontSize(theme.TextUI).SingleLine()
			sub := "Only you, on your devices"
			if sp.Team != spacePersonal {
				sub = "Team"
				if t, ok := f.teams[sp.Team]; ok && len(t.Members) > 0 {
					sub = fmt.Sprintf("Team · %d members", len(t.Members))
				}
			}
			ui.Text(c, sub).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
		})
		if on {
			ui.Icon(c, icon.Check).FontSize(13).TextColor(theme.Accent)
		}
	})
	if row.Clicked() {
		f.to = sp.Team
	}
}

// spaceMoveSays says what moving or copying will do.
func (a *App) spaceMoveSays(f *spaceMoveForm, what string) string {
	pages := ""
	if f.note != nil {
		pages = ", with the pages under it,"
	}
	if f.to == spacePersonal || f.to == "" {
		return "A copy" + pages + " goes to your own space: only you see it. The team keeps its own."
	}
	t := f.teams[f.to]
	who := "the members of " + a.spaceName(f.to)
	if n := len(t.Members); n > 0 {
		who = fmt.Sprintf("the %d members of %s", n, a.spaceName(f.to))
	}
	verb := "The " + what + pages + " moves to the team: "
	if f.copy {
		verb = "A copy" + pages + " goes to the team, yours staying yours: "
	}
	return verb + who + " see it, and agents in its projects read it. It is checked for secrets first."
}

func (a *App) moveToSpace(f *spaceMoveForm) {
	cl := a.client
	if cl == nil {
		return
	}
	to := f.to
	if to == spacePersonal {
		to = ""
	}
	f.busy, f.err = true, ""
	copy := f.copy
	go func() {
		var id string
		var err error
		if f.note != nil {
			var info proto.NoteInfo
			info, err = cl.MoveNoteToSpace(f.note.ID, to, copy)
			id = info.ID
		} else {
			var info proto.CanvasInfo
			info, err = cl.MoveCanvasToSpace(f.canvas.ID, to, copy)
			id = info.ID
		}
		a.onMain(func() {
			f.busy = false
			if err != nil {
				f.err = strings.TrimPrefix(client.Explain(err), "spaces: ")
				return
			}
			f.open = false
			a.showSpace(to) // where it went
			if f.note != nil {
				a.openNote(id, to)
			} else if hc := a.canvasList; hc != nil {
				hc.open = id
			}
		})
	}()
}

// moveSpaceOption is the Notes page's moving to a space, while there is a
// team's to move to.
func (a *App) moveSpaceOption() func(proto.NoteInfo) {
	if len(a.spacesOf().list) == 0 {
		return nil
	}
	return a.openNoteSpaceMove
}

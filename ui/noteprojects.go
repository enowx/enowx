package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/notes"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/notesview"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Notes attached to projects of any host: the projects every host
// published (their catalogs, synced), whether each is in reach, the
// dialog attaching a note, and what agents ask of projects elsewhere.

// catalogEvery is how often the catalogs are read again while shown.
const catalogEvery = 20 * time.Second

// noteCatalogs are the hosts' catalogs, the main thread's.
type noteCatalogs struct {
	list    []proto.Catalog
	read    time.Time
	reading bool
}

// loadCatalogs reads the hosts' catalogs from the host shown, when they
// are older than catalogEvery.
func (a *App) loadCatalogs() {
	cl := a.client
	if cl == nil || a.catalogs.reading || time.Since(a.catalogs.read) < catalogEvery {
		return
	}
	a.catalogs.reading = true
	go func() {
		list, err := cl.Catalogs()
		a.onMain(func() {
			a.catalogs.reading, a.catalogs.read = false, time.Now()
			if err == nil {
				a.catalogs.list = list
			}
		})
	}()
}

// projectState says how a note's project p is: on a host in reach, on
// hosts out of it, or gone from the host it was on.
func (a *App) projectState(p proto.NoteProject) string {
	reached := map[string]bool{}
	for _, h := range a.reachable() {
		reached[h.info.ID] = true
	}
	found, hostKnown := false, false
	for _, cat := range a.catalogs.list {
		hostKnown = hostKnown || cat.Host == p.Host
		for _, cp := range cat.Projects {
			if notes.IsProject(p, cat.Host, cp) {
				if reached[cat.Host] {
					return notesview.ProjectOnline
				}
				found = true
			}
		}
	}
	if !found && hostKnown {
		return notesview.ProjectBroken
	}
	return notesview.ProjectOffline
}

// newNoteFor makes a note attached to project p of the host shown, and
// opens it in the Notes page.
func (a *App) newNoteFor(p proto.ProjectInfo) {
	cl := a.client
	if cl == nil {
		return
	}
	host, hostName := a.daemonInfo.ID, a.host.Name // the name the user knows
	go func() {
		remote := ""
		if cats, err := cl.Catalogs(); err == nil {
			for _, cat := range cats {
				if cat.Host != host {
					continue
				}
				for _, cp := range cat.Projects {
					if cp.Path == p.Path {
						remote = cp.Remote
					}
				}
			}
		}
		np := proto.NoteProject{Ref: notes.ProjectRef(remote, host, p.Path), Name: p.Name, Host: host, HostName: hostName, Path: p.Path, Remote: remote}
		info, err := cl.SaveNote(proto.Note{NoteInfo: proto.NoteInfo{Title: p.Name + " notes"}})
		if err == nil {
			info, err = cl.AttachNote(info.ID, []proto.NoteProject{np})
		}
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			a.openNote(info.ID, info.Team)
		})
	}()
}

// notesOfProject are the notes attached to project path of the host
// shown.
func (a *App) notesOfProject(path string) []proto.NoteInfo {
	hn := a.noteList
	if hn == nil {
		return nil
	}
	var cp proto.CatalogProject
	cp.Path = path
	for _, cat := range a.catalogs.list {
		if cat.Host == a.daemonInfo.ID {
			for _, p := range cat.Projects {
				if p.Path == path {
					cp = p
				}
			}
		}
	}
	var out []proto.NoteInfo
	for _, n := range hn.list {
		if slices.ContainsFunc(n.Projects, func(np proto.NoteProject) bool { return notes.IsProject(np, a.daemonInfo.ID, cp) }) {
			out = append(out, n)
		}
	}
	return out
}

// showProjectNotes opens the Notes page on the notes of project path of
// the host shown.
func (a *App) showProjectNotes(path string) {
	list := a.notesOfProject(path)
	if len(list) == 0 {
		return
	}
	for _, np := range list[0].Projects {
		if np.Path == path || np.Host == a.daemonInfo.ID && np.Path == path || strings.HasPrefix(np.Ref, "git:") {
			a.showSpace(list[0].Team)
			a.notes.ShowProject(np.Ref)
			break
		}
	}
	a.page = pageNotes
}

// projectAsk does what an agent on a host asked of a project on another:
// its context, or a file read, through the host that has it.
func (a *App) projectAsk(req proto.AppRequest, reply func(any, error)) {
	var host string
	var call func(cl *client.Client) (any, error)
	switch req.Kind {
	case "project.context":
		var p proto.ProjectContextParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply(nil, err)
			return
		}
		host = p.Host
		call = func(cl *client.Client) (any, error) {
			return cl.ProjectContext(proto.ProjectContextParams{Path: p.Path, Ref: p.Ref})
		}
	default:
		var p proto.ProjectReadParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply(nil, err)
			return
		}
		host = p.Host
		call = func(cl *client.Client) (any, error) {
			return cl.ReadProjectFile(proto.ProjectReadParams{Path: p.Path, File: p.File})
		}
	}
	h, ok := a.reach(host)
	if !ok {
		reply(nil, errors.New("the project's host is out of the app's reach now"))
		return
	}
	go func() { reply(call(h.cl)) }()
}

// daemonHostName is the name the user gave the host of daemon id, when
// in reach, else name.
func (a *App) daemonHostName(id, name string) string {
	for _, h := range a.reachable() {
		if h.info.ID == id {
			return h.host.Name
		}
	}
	return name
}

// noteTitle is a note's title, or Untitled.
func noteTitle(n proto.NoteInfo) string {
	if t := strings.TrimSpace(n.Title); t != "" {
		return t
	}
	return "Untitled"
}

// projectNotesRow folds, under an open project's sessions in the session
// list, the notes attached to it into one small row: unfolded, they list
// under it, a click opening one.
func (a *App) projectNotesRow(c *ui.Context, p proto.ProjectInfo) {
	if a.client == nil {
		return
	}
	a.notesOf()
	a.loadCatalogs()
	list := a.notesOfProject(p.Path)
	if len(list) == 0 {
		return
	}
	open := a.notesOpen[p.Path]
	chevron := icon.ChevronRight
	if open {
		chevron = icon.ChevronDown
	}
	// Keyed by project: each has its own row, its hover and its click.
	head := ui.ButtonBase(c.Key("notes-head:"+p.Path)).FillWidth().Height(22).Padding(0, 8, 0, 8).Gap(5).Radius(theme.RadiusSmall).Justify(ui.Start).
		Cursor(ui.CursorPointer).Label(fmt.Sprintf("Notes of %s (%d)", p.Name, len(list))).AlignItems(ui.Center)
	if head.Hovered() {
		head.Background(theme.Hover)
	}
	head.Children(func() {
		ui.Icon(c, chevron).FontSize(10).TextColor(theme.Muted)
		ui.Icon(c, icon.Notebook).FontSize(10.5).TextColor(theme.Muted)
		ui.Text(c, textfmt.Plural(len(list), "1 note", fmt.Sprintf("%d notes", len(list)))).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
	})
	if head.Clicked() {
		if a.notesOpen == nil {
			a.notesOpen = map[string]bool{}
		}
		a.notesOpen[p.Path] = !open
	}
	if !open {
		return
	}
	const shown = 8
	for i, n := range list {
		if i == shown {
			more := ui.ButtonBase(c.Key("notes-more:"+p.Path)).FillWidth().Height(20).Padding(0, 8, 0, 34).Justify(ui.Start).Cursor(ui.CursorPointer).Label("All notes of " + p.Name)
			more.Children(func() {
				ui.Text(c, fmt.Sprintf("+%d more", len(list)-shown)).FontSize(theme.TextLabel).TextColor(theme.Muted)
			})
			if more.Clicked() {
				a.showProjectNotes(p.Path)
			}
			return
		}
		// A note can be in several projects: keyed by both.
		row := ui.ButtonBase(c.Key("note:"+p.Path+":"+n.ID)).FillWidth().Height(22).Padding(0, 8, 0, 34).Radius(theme.RadiusSmall).Justify(ui.Start).
			Cursor(ui.CursorPointer).Label("Note " + noteTitle(n)).AlignItems(ui.Center)
		if row.Hovered() {
			row.Background(theme.Hover)
		}
		row.Children(func() {
			ui.Text(c, noteTitle(n)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(1)
			if n.Team != "" {
				ui.Spacer(c)
				widget.TeamMark(c, n.Team, a.spaceName(n.Team), 12).Tooltip(a.spaceName(n.Team) + "'s note")
			}
		})
		if row.Clicked() {
			a.openNote(n.ID, n.Team)
		}
	}
}

// notesContextRow says, in the Notes page's agent panel, which projects
// the agent has as context: those note n is attached to, read-only.
func (a *App) notesContextRow(c *ui.Context, n proto.NoteInfo) {
	if len(n.Projects) == 0 {
		return
	}
	var names []string
	for _, p := range n.Projects {
		label := p.Name
		if p.HostName != "" {
			label += " · " + p.HostName
		}
		if st := a.projectState(p); st != notesview.ProjectOnline {
			label += " (" + st + ")"
		}
		names = append(names, label)
	}
	ui.Row(c).FillWidth().Shrink(0).Padding(6, 12).Gap(6).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).
		Tooltip("The agent reads these projects, read-only, to write this page").Children(func() {
		ui.Icon(c, icon.Git).FontSize(11).TextColor(theme.Muted)
		ui.Text(c, "Context: "+strings.Join(names, ", ")).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(1)
	})
}

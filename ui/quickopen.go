package ui

import (
	"path"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Quick open (⌘P) finds a file of the selected session's folder by a few
// letters of its path (file.find, on its host's daemon) and opens it in a
// tab of the side panel.

// quickOpen is the dialog's state. It belongs to the main thread.
type quickOpen struct {
	open    bool
	session string // whose folder
	query   string
	asked   string   // the query the paths answer
	paths   []string // found, best first
	pick    int
	err     string
}

// openQuickOpen opens the dialog on the selected session, or says there is
// none.
func (a *App) openQuickOpen() {
	s, ok := a.selectedSession()
	if !ok {
		a.status = "Select a session to find its files."
		return
	}
	a.quick = quickOpen{open: true, session: a.parentOf(s).ID}
	a.findFiles()
}

// findFiles asks the daemon for the files the query matches, unless it
// answered that query already.
func (a *App) findFiles() {
	q, cl := &a.quick, a.client
	if cl == nil {
		return
	}
	query, id := q.query, q.session
	q.asked = query
	go func() {
		paths, err := cl.FindFiles(id, query, 50)
		a.onMain(func() {
			if a.quick.session != id || a.quick.query != query {
				return // typed on meanwhile; that query is asked
			}
			a.quick.paths, a.quick.err = paths, ""
			if err != nil {
				a.quick.err = client.Explain(err)
			}
			a.quick.pick = 0
		})
	}()
}

func (a *App) quickOpenDialog(c *ui.Context) {
	q := &a.quick
	if !q.open {
		return
	}
	if q.query != q.asked {
		a.findFiles()
	}
	if n := len(q.paths); n > 0 {
		q.pick = (q.pick + n) % n
	}
	var chosen string
	submitted := widget.Palette(c, &q.open, &q.query, "Go to a file by a few letters of its path", func(k ui.TextKey) bool {
		switch {
		case k.Key == ui.KeyDown && k.Mods == 0, k.Key == ui.KeyN && k.Mods == ui.Ctrl:
			q.pick++
		case k.Key == ui.KeyUp && k.Mods == 0, k.Key == ui.KeyP && k.Mods == ui.Ctrl:
			q.pick--
		default:
			return false
		}
		return true
	}, func() {
		switch {
		case q.err != "":
			ui.Text(c, q.err).FontSize(theme.TextUI).TextColor(theme.Error).Padding(theme.S2, theme.S3)
		case len(q.paths) == 0 && q.asked == q.query:
			ui.Text(c, "No file matches.").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S3)
		}
		for i, p := range q.paths {
			dir, name := path.Split(p)
			row := ui.ButtonBase(c.Key(p)).Label(p)
			widget.PaletteRow(c, row, i == q.pick, icon.File, name, strings.TrimSuffix(dir, "/"), true)
			if row.Clicked() {
				chosen = p
			}
		}
	})
	if submitted && len(q.paths) > 0 {
		chosen = q.paths[q.pick]
	}
	if chosen != "" {
		q.open = false
		if s, ok := a.sessionByID(q.session); ok {
			a.selectSession(s)
			a.openFileTab(s, chosen)
		}
	}
}

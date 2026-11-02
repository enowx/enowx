package notesview

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/iconlib"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Code and tables as a page shows them, their text not selectable, so a
// click on them opens them as text.

func codeView(c *ui.Context, lang, code string) {
	ui.Column(c).FillWidth().MinWidth(0).Gap(theme.S1).Padding(theme.S3, theme.S4).Radius(theme.RadiusCard).Background(theme.CodeGround).
		InsetShadow(0, 0, 0, 0.5, theme.Border).Children(func() {
		if lang != "" {
			ui.Text(c, lang).FontSize(theme.TextLabel).TextColor(widget.CodeText).Opacity(0.6)
		}
		text := code
		if text == "" {
			text = " "
		}
		ui.Text(c, text).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(widget.CodeText).FillWidth()
	})
}

// tableState is how a table is shown: sorted by a column (by is its
// number plus one, 0 for none), and filtered.
type tableState struct {
	by     int
	desc   bool
	filter string
}

// tableView draws block i, a table, sortable by its header and filtered
// when it is long.
func (e *editor) tableView(c *ui.Context, i int, src string) {
	st := e.tables[i]
	if st == nil {
		st = &tableState{}
		e.tables[i] = st
	}
	tableView(c, src, st, e.env.openLink)
}

// drawings are the SVG drawings of pages shown, cleaned and parsed once,
// by their code; nil for code that doesn't draw. Main thread only.
var drawings = map[string]*ui.SVG{}

// drawingView is a code block of language svg as the drawing it is,
// cleaned first (core/iconlib): what runs or loads in it goes.
func drawingView(c *ui.Context, code string) {
	svg, ok := drawings[code]
	if !ok {
		if clean, err := iconlib.Sanitize([]byte(code), false); err == nil {
			svg, _ = ui.ParseSVG([]byte(clean))
		}
		if len(drawings) > 64 {
			clear(drawings)
		}
		drawings[code] = svg
	}
	if svg == nil {
		ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
			codeView(c, "svg", code)
			ui.Text(c, "This SVG doesn't draw: fix its code.").FontSize(theme.TextLabel).TextColor(theme.Error)
		})
		return
	}
	w, h := svg.Size()
	if w <= 0 || h <= 0 {
		w, h = 24, 24
	}
	// As large as it says, at most 640 and the room the page has.
	row := ui.Row(c).FillWidth().Padding(theme.S2, 0)
	most := float32(640)
	if room := row.Bounds().W; room > 0 {
		most = min(most, room)
	}
	if w > most {
		w, h = most, h*most/w
	}
	row.Children(func() {
		ui.Image(c, svg).Size(w, h).TextColor(theme.Text)
	})
}

// tableRows are a Markdown table's cells, its header first.
func tableRows(src string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.Trim(line, "|-: ") == "" {
			continue // the rule under the header
		}
		var cells []string
		for _, cell := range strings.Split(strings.Trim(line, "|"), "|") {
			cells = append(cells, strings.TrimSpace(cell))
		}
		rows = append(rows, cells)
	}
	return rows
}

// shownRows are a table's rows under its header as st shows them.
func shownRows(rows [][]string, st *tableState) [][]string {
	if len(rows) < 2 || st == nil {
		return rows
	}
	body := slices.Clone(rows[1:])
	if q := strings.ToLower(strings.TrimSpace(st.filter)); q != "" {
		body = slices.DeleteFunc(body, func(r []string) bool {
			return !strings.Contains(strings.ToLower(strings.Join(r, " ")), q)
		})
	}
	if col := st.by - 1; col >= 0 {
		cell := func(r []string) string {
			if col < len(r) {
				return r[col]
			}
			return ""
		}
		slices.SortStableFunc(body, func(a, b []string) int {
			x, y := cell(a), cell(b)
			var n int
			fx, errx := strconv.ParseFloat(strings.TrimSuffix(x, "%"), 64)
			fy, erry := strconv.ParseFloat(strings.TrimSuffix(y, "%"), 64)
			if errx == nil && erry == nil {
				n = cmp.Compare(fx, fy)
			} else {
				n = cmp.Compare(strings.ToLower(x), strings.ToLower(y))
			}
			if st.desc {
				n = -n
			}
			return n
		})
	}
	return append([][]string{rows[0]}, body...)
}

// tableView draws a table; st, when not nil, sorts it by a click on a
// column's header (again: the other way, a third time: as written) and
// filters it, with a field over it when it has more than five rows.
func tableView(c *ui.Context, src string, st *tableState, open widget.LinkOpener) {
	all := tableRows(src)
	rows := shownRows(all, st)
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	ui.Column(c).FillWidth().MinWidth(0).Gap(theme.S2).Children(func() {
		if st != nil && len(all) > 6 {
			ui.Row(c).Width(240).MaxWidth(280).Shrink(1).Children(func() {
				widget.SearchField(c, &st.filter, "Filter rows", icon.Search)
			})
		}
		ui.Column(c).FillWidth().MinWidth(0).Children(func() {
			for i, r := range rows {
				ui.Row(c.Key(i)).FillWidth().MinWidth(0).AlignItems(ui.Stretch).BorderWidth(0, 0, 0.5, 0).BorderColor(theme.Border).Children(func() {
					for j := range cols {
						cell := ""
						if j < len(r) {
							cell = r[j]
						}
						if i == 0 && st != nil && len(all) > 2 {
							headerCell(c.Key(j), cell, j, st, open)
							continue
						}
						t := widget.NoteLabel(c.Key(j), open, cell).FontSize(theme.TextUI).Grow(1).Basis(0).MinWidth(0).Padding(theme.S1, theme.S2)
						if i == 0 {
							t.FontWeight(500).TextColor(theme.TextSoft)
						}
					}
				})
			}
			if len(rows) == 1 && len(all) > 1 {
				ui.Text(c, "No row has “"+st.filter+"”").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2)
			}
		})
	})
}

// headerCell is column j's header, which sorts the table by it.
func headerCell(c *ui.Context, text string, j int, st *tableState, open widget.LinkOpener) {
	b := ui.ButtonBase(c).Grow(1).Basis(0).MinWidth(0).Padding(theme.S1, theme.S2).Gap(theme.S1).Justify(ui.Start).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("Sort by " + text)
	if b.Hovered() {
		b.Background(theme.Hover)
	}
	b.Children(func() {
		widget.NoteLabel(c, open, text).FontSize(theme.TextUI).FontWeight(500).TextColor(theme.TextSoft).Shrink(1)
		if st.by == j+1 {
			arrow := icon.ArrowUp
			if st.desc {
				arrow = icon.ArrowDown
			}
			ui.Icon(c, arrow).FontSize(12).TextColor(theme.TextSoft)
		}
	})
	if b.Clicked() {
		switch {
		case st.by != j+1:
			st.by, st.desc = j+1, false
		case !st.desc:
			st.desc = true
		default:
			st.by, st.desc = 0, false
		}
	}
}

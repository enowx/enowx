package projectdocsview

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// shownQuestions is how many open questions the summary lists.
const shownQuestions = 5

// summaryPanel is the top of a project note's page: where the project stands,
// its documents, and what can be done with them.
func (v *View) summaryPanel(c *ui.Context, o Options, pg Page, meta projectdocs.Meta) {
	card := widget.Card(c).FillWidth().Label("Project summary")
	w := card.Bounds().W
	narrow := w > 0 && w < narrowWidth
	card.Children(func() {
		v.summaryHead(c, pg, meta)
		m := v.current(pg.Note.ID)
		if m == nil {
			v.summaryWaiting(c)
		} else {
			summaryFacts(c, m, narrow)
			v.documents(c, o, m, narrow)
		}
		v.summaryActions(c, o, pg, meta, m)
		v.noteLine(c)
	})
}

func (v *View) summaryHead(c *ui.Context, pg Page, meta projectdocs.Meta) {
	pack := "Core pack"
	if meta.Size == projectdocs.SizeFull {
		pack = "Full pack"
	}
	ui.Row(c).FillWidth().Height(lineHeight).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
		ui.Icon(c, icon.Notebook).FontSize(13).TextColor(theme.TextSoft).Shrink(0)
		running(c, "Project docs · "+pack, runStyle{Size: 12.5, Weight: 600, Color: theme.Text}).Grow(1)
		on := meta.Approval != projectdocs.ApprovalOff
		label := "Approval off"
		if on {
			label = "Approval on"
		}
		ui.Row(c).Width(112).Height(lineHeight).Shrink(0).Gap(6).Justify(ui.End).AlignItems(ui.Center).Tooltip("Whether agents wait for the owner's approval before building").Children(func() {
			ui.Text(c, label).FontSize(11).TextColor(theme.TextSoft).SingleLine()
			// The switch shows the note's state: only the confirmed write changes it.
			shown := on
			if widget.Switch(c, &shown, "Require approval") {
				v.askApproval(pg, on)
			}
		})
	})
}

// askApproval confirms turning approval on or off, saying what changes.
func (v *View) askApproval(pg Page, on bool) {
	id := pg.Note.ID
	set := func(approval string) func() {
		return func() {
			v.write(id, func(body string) (string, error) { return projectdocs.SetApproval(body, approval) })
		}
	}
	if on {
		v.ask("Turn approval off?",
			"Agents may start building before the PRD and Technical Spec are approved. Documents already approved stay approved, and agents' changes to them still come as proposals.",
			"Turn off", true, set(projectdocs.ApprovalOff))
		return
	}
	v.ask("Turn approval on?",
		"Agents wait to build until you have approved the PRD and Technical Spec. Whatever isn't approved yet blocks them from now on.",
		"Turn on", false, set(projectdocs.ApprovalRequired))
}

func (v *View) summaryWaiting(c *ui.Context) {
	if v.loadErr == "" {
		line(c, icon.History, "Reading the documents…", theme.Muted)
		return
	}
	line(c, icon.Alert, "Couldn't read the documents: "+v.loadErr, theme.Error)
	if widget.GhostButton(c, "Try again").Clicked() {
		v.loadErr, v.wantLoad = "", true
	}
}

// summaryFacts is the counts, the progress, the gate and the open questions.
func summaryFacts(c *ui.Context, m *model, narrow bool) {
	columns := 4
	if narrow {
		columns = 2
	}
	ui.Grid(c).FillWidth().Columns(columns).Gap(6).Children(func() {
		for _, s := range m.counts {
			cell(c.Key(s.Status), s.N, s.Status, statusStyle(s.Status).Color)
		}
	})
	ui.Column(c).FillWidth().Gap(5).Children(func() {
		line(c, icon.Check, fmt.Sprintf("%d of %s done", m.summary.Done, plural(m.summary.Todos, "to-do", "to-dos")), theme.TextSoft)
		progress(c, m.summary.Done, m.summary.Todos)
	})
	if m.gate.Ready {
		line(c, icon.Check, m.gate.Title, theme.Done)
	} else {
		ui.Column(c).FillWidth().Label("Gate").Children(func() {
			line(c, icon.Lock, m.gate.Title, theme.Warning)
			for i, l := range m.gate.Lines {
				line(c.Key(i), nil, l, theme.TextSoft)
			}
		})
	}
	ui.Grid(c).FillWidth().Columns(3).Gap(6).Children(func() {
		cell(c, len(m.summary.OpenQuestions), "Open questions", theme.Text)
		cell(c, m.summary.Stale, "Stale", staleColor(m.summary.Stale))
		cell(c, m.summary.Unfilled, "To fill", theme.Text)
	})
	if q := m.summary.OpenQuestions; len(q) > 0 {
		ui.Column(c).FillWidth().Children(func() {
			for i, text := range q[:min(len(q), shownQuestions)] {
				line(c.Key(i), icon.Info, text, theme.TextSoft)
			}
			if len(q) > shownQuestions {
				line(c, nil, fmt.Sprintf("and %d more", len(q)-shownQuestions), theme.Muted)
			}
		})
	}
}

func staleColor(n int) ui.Color {
	if n > 0 {
		return theme.Warning
	}
	return theme.Text
}

// documents lists the documents, each opening on a click.
func (v *View) documents(c *ui.Context, o Options, m *model, narrow bool) {
	ui.Column(c).FillWidth().Gap(1).Children(func() {
		sectionTitle(c, "Documents")
		if len(m.rows) == 0 {
			line(c, nil, "No documents yet", theme.Muted)
		}
		for _, r := range m.rows {
			if r.Area {
				areaRow(c.Key(r.ID), r)
				continue
			}
			docRowView(c.Key(r.ID), o, r, narrow)
		}
	})
}

func areaRow(c *ui.Context, r docRow) {
	ui.Row(c).FillWidth().Height(lineHeight).Gap(6).AlignItems(ui.Center).MinWidth(0).Padding(6, 0, 0, 2).Children(func() {
		widget.NoteIcon(c, r.Icon, 12, theme.Muted, nil).Shrink(0)
		running(c, r.Title, runStyle{Size: 11, Color: theme.Muted}).Grow(1)
	})
}

// docRowView is a document's row. Its marks keep their own widths, however
// long the title; little room shows the status by its dot alone.
func docRowView(c *ui.Context, o Options, r docRow, narrow bool) {
	b := ui.ButtonBase(c).FillWidth().Height(rowHeight).PaddingX(6).Gap(7).Radius(6).Cursor(ui.CursorPointer).MinWidth(0).AlignItems(ui.Center).Label("Open " + r.Title)
	if b.Hovered() {
		b.Background(theme.Hover)
	}
	b.Children(func() {
		widget.NoteIcon(c, r.Icon, 14, theme.TextSoft, nil).Shrink(0)
		running(c, r.Title, runStyle{Size: 12.5, Color: theme.Text}).Grow(1)
		if r.Stale {
			ui.Icon(c, icon.Alert).FontSize(11).TextColor(theme.Warning).Shrink(0).Tooltip("Based on something that changed since")
		}
		if r.Todos > 0 && !narrow {
			ui.Row(c).Width(44).Shrink(0).Justify(ui.End).Children(func() {
				ui.Text(c, fmt.Sprintf("%d/%d", r.Done, r.Todos)).FontSize(11).TextColor(theme.Muted).SingleLine()
			})
		}
		st := statusStyle(r.Status)
		if narrow {
			ui.Box(c).Tooltip(r.Status).Shrink(0).Children(func() { widget.Dot(c, st.Color, st.Hollow, 7) })
			return
		}
		ui.Row(c).Width(78).Shrink(0).Children(func() { widget.Badge(c, st) })
	})
	if b.Clicked() {
		o.Open(r.ID)
	}
}

// summaryActions are attaching, adding and exporting; on too narrow a row they
// go on under one another.
func (v *View) summaryActions(c *ui.Context, o Options, pg Page, meta projectdocs.Meta, m *model) {
	note := pg.Note.NoteInfo
	for _, n := range o.Notes {
		if n.ID == note.ID {
			note = n
		}
	}
	ui.Row(c).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
		if o.AttachProjects != nil && actionButton(c, icon.Git, "Attach to projects…").Clicked() {
			o.AttachProjects(note)
		}
		v.applyButton(c, o, note, m)
		v.addButton(c, o, pg, meta, m)
		if o.ExportRepo != nil && m != nil && actionButton(c, icon.ArrowUp, "Export to repo…").Clicked() {
			v.openExport(o, note, m)
		}
	})
}

func actionButton(c *ui.Context, svg *ui.SVG, label string) ui.Element {
	return widget.Button(c, widget.Secondary, svg, label, "").Height(28).PaddingX(10)
}

// applyState is copying a project note's projects to its documents.
type applyState struct{ done, total int }

// applyButton copies the project note's projects to the documents without them.
func (v *View) applyButton(c *ui.Context, o Options, note proto.NoteInfo, m *model) {
	if o.Attach == nil {
		return
	}
	if v.apply != nil {
		actionButton(c, icon.Git, fmt.Sprintf("Applying %d of %d…", v.apply.done, v.apply.total)).Disabled(true)
		return
	}
	ids := missingProjects(o.Notes, m, note.Projects)
	if len(ids) == 0 {
		return
	}
	if actionButton(c, icon.Git, "Apply to "+plural(len(ids), "document", "documents")).Clicked() {
		v.apply = &applyState{total: len(ids)}
		v.applyNext(o, ids, note.Projects)
	}
}

func (v *View) applyNext(o Options, ids []string, projects []proto.NoteProject) {
	st := v.apply
	if st == nil {
		return
	}
	if st.done == len(ids) {
		v.apply = nil
		v.tell("Applied to "+plural(st.total, "document", "documents"), false)
		return
	}
	o.Attach(ids[st.done], projects, func(_ proto.NoteInfo, err error) {
		if err != nil {
			v.apply = nil
			v.tell(err.Error(), true)
			return
		}
		st.done++
		v.applyNext(o, ids, projects)
	})
}

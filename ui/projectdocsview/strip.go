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

// The status button's frame: the same for every status.
const (
	statusWidth  = 112
	statusHeight = 28
	// tinyWidth is under what width the status leaves its neighbors icons.
	tinyWidth = 260
)

// stripPanel is the top of a document's page: what it is, its status and who
// may change it, and what it's based on.
func (v *View) stripPanel(c *ui.Context, o Options, pg Page) {
	if pg.Note.Body != v.seenBody {
		v.seenBody, v.seenDoc = pg.Note.Body, docViewOf(pg.Note.Body)
	}
	d := v.seenDoc
	card := widget.Card(c).FillWidth().Gap(6).Label("Document strip")
	w := card.Bounds().W
	narrow := w > 0 && w < narrowWidth
	tiny := w > 0 && w < tinyWidth
	card.Children(func() {
		v.breadcrumb(c, o, pg.Note.ID)
		ui.Row(c).FillWidth().Height(lineHeight).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
			ui.Icon(c, icon.File).FontSize(12).TextColor(theme.TextSoft).Shrink(0)
			running(c, d.meta.Kind, runStyle{Size: 12.5, Weight: 600, Color: theme.Text}).Grow(1)
			if d.todos > 0 {
				ui.Row(c).Width(56).Shrink(0).Justify(ui.End).Children(func() {
					ui.Text(c, fmt.Sprintf("%d/%d", d.done, d.todos)).FontSize(11).TextColor(theme.Muted).SingleLine().Tooltip("To-dos done")
				})
			}
			widget.Tag(c, fmt.Sprintf("v%d", max(d.meta.Version, 1)), ui.Color{})
		})
		ui.Row(c).FillWidth().Height(statusHeight).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
			v.statusButton(c, pg, d)
			ui.Spacer(c)
			v.approvalButtons(c, pg, d, narrow, tiny)
		})
		if d.changed {
			line(c, icon.Alert, "Changed since approved: approve again, or reopen it", theme.Warning)
		}
		if pg.Proposals > 0 {
			line(c, icon.Chat, plural(pg.Proposals, "change", "changes")+" from an agent waiting, above", theme.Accent)
		}
		v.linkRows(c, o, pg, d, narrow)
		v.noteLine(c)
	})
}

// statusButton is the status, whose menu sets it.
func (v *View) statusButton(c *ui.Context, pg Page, d docView) {
	status := statusOf(d.meta)
	b := ui.ButtonBase(c).Width(statusWidth).Height(statusHeight).Shrink(0).PaddingX(9).Gap(6).Radius(theme.RadiusControl-1).
		Border(1, theme.BorderStrong).Cursor(ui.CursorPointer).AlignItems(ui.Center).Label("Status: " + status).MinWidth(0)
	if b.Hovered() {
		b.Background(theme.Hover)
	}
	st := statusStyle(status)
	b.Children(func() {
		widget.Dot(c, st.Color, st.Hollow, 7)
		running(c, status, runStyle{Size: 11.5, Color: theme.TextSoft}).Grow(1)
		ui.Icon(c, icon.ChevronDown).FontSize(10).TextColor(theme.Muted).Shrink(0)
	})
	v.menu.OpenBelow(b, "status", statusHeight)
	v.menu.Show(c, b, "status", func(done func()) {
		for _, s := range statusChoices() {
			mark := icon.Circle
			if s == status {
				mark = icon.Check
			}
			if widget.MenuItem(c.Key(s), mark, s, false) {
				v.setStatus(pg, d, s)
				done()
			}
		}
	})
}

// setStatus sets a status other than approved; taking one away from an
// approved document asks first, as it loses the approval.
func (v *View) setStatus(pg Page, d docView, status string) {
	if status == statusOf(d.meta) {
		return
	}
	id := pg.Note.ID
	do := func() {
		v.write(id, func(body string) (string, error) { return projectdocs.SetStatus(body, status) })
	}
	if statusOf(d.meta) == projectdocs.StatusApproved {
		v.ask("Drop its approval?", dropApprovalText(titleOf(pg.Note.NoteInfo), status), "Set to "+status, true, do)
		return
	}
	do()
}

// approvalButtons are Approve and Reopen, as the status has them.
func (v *View) approvalButtons(c *ui.Context, pg Page, d docView, narrow, tiny bool) {
	a := d.actions()
	id, title := pg.Note.ID, titleOf(pg.Note.NoteInfo)
	if a.Reopen {
		var b ui.Element
		if tiny || narrow && a.Approve {
			b = widget.IconButton(c, icon.Undo, "Reopen", false)
		} else {
			b = widget.Button(c, widget.Secondary, icon.Undo, "Reopen", "").Height(statusHeight).PaddingX(10)
		}
		if b.Clicked() {
			v.ask("Reopen this document?", dropApprovalText(title, projectdocs.StatusDraft), "Yes, reopen", true, func() {
				v.write(id, func(body string) (string, error) { return projectdocs.SetStatus(body, projectdocs.StatusDraft) })
			})
		}
	}
	if !a.Approve {
		return
	}
	label := a.ApproveLabel
	if narrow {
		label = "Approve"
	}
	var b ui.Element
	if tiny {
		b = widget.IconButton(c, icon.Check, label, false)
	} else {
		b = widget.Button(c, widget.Primary, icon.Check, label, "").Height(statusHeight).PaddingX(10)
	}
	if b.Clicked() {
		v.ask("Approve this document?", approveText(title), "Yes, approve", false, func() {
			v.write(id, projectdocs.Approve)
		})
	}
}

// approveText says what approving means: who may then change it, and how.
func approveText(title string) string {
	return "“" + title + "” is approved as it reads now. Agents can't change it directly from then on: what they write comes as a proposal for you to accept. You can still edit it here, which shows as changed until you approve it again."
}

// dropApprovalText says what is lost by a document leaving approved.
func dropApprovalText(title, status string) string {
	return "“" + title + "” goes to " + status + " and loses its approval. Agents can write to it directly again, and if the project requires its approval it blocks building until you approve it again."
}

// linkRows are what the document is based on: a row a link, those whose source
// changed with what can be done about it.
func (v *View) linkRows(c *ui.Context, o Options, pg Page, d docView, narrow bool) {
	if len(d.links) == 0 {
		return
	}
	m := v.current(pg.Note.ID)
	var stale []projectdocs.Stale
	if doc := m.doc(pg.Note.ID); doc != nil {
		stale = doc.Stale
	}
	ui.Column(c).FillWidth().Children(func() {
		sectionTitle(c, "Based on")
		for i, r := range linkRows(d.links, stale) {
			v.linkRow(c.Key(i), o, pg, m, r, narrow)
		}
	})
}

func (v *View) linkRow(c *ui.Context, o Options, pg Page, m *model, r linkRow, narrow bool) {
	ui.Row(c).FillWidth().Height(rowHeight).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
		svg, col, text := icon.Link, theme.TextSoft, r.Label
		if r.Why != "" {
			svg, col, text = icon.Alert, theme.Warning, r.Why
		}
		ui.Icon(c, svg).FontSize(12).TextColor(col).Shrink(0)
		running(c, text, runStyle{Size: 12, Color: col}).Grow(1)
		var src *projectdocs.Doc
		if m != nil {
			src = m.project.Doc(r.Link.Doc)
		}
		if src == nil {
			return
		}
		id := src.ID
		if rowAction(c, icon.External, "Open source", narrow) {
			o.Open(id)
		}
		if r.Why != "" && !r.Gone && rowAction(c, icon.Check, "Mark reviewed", narrow) {
			v.markReviewed(o, pg, r.Link, id)
		}
	})
}

// rowAction is a row's button: its words, or with little room its icon.
func rowAction(c *ui.Context, svg *ui.SVG, label string, narrow bool) bool {
	if narrow {
		return widget.IconButton(c, svg, label, false).Clicked()
	}
	return widget.Button(c, widget.Secondary, svg, label, "").Height(24).PaddingX(8).Clicked()
}

// markReviewed confirms link l against its source as the host has it now, not
// as the project was last derived.
func (v *View) markReviewed(o Options, pg Page, l projectdocs.Link, source string) {
	id := pg.Note.ID
	o.Load(source, func(n proto.Note, err error) {
		if err != nil {
			v.tell(err.Error(), true)
			return
		}
		v.write(id, func(body string) (string, error) { return reviewed(body, l, n.Body) })
	})
}

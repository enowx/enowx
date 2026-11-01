package notesview

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// historyDialog is a page's earlier versions: the list, the one picked
// shown, and bringing it back.
type historyDialog struct {
	open     bool
	note     proto.NoteInfo
	versions []proto.NoteVersion
	loaded   bool
	err      string
	picked   string
	shown    *proto.Note // the version picked, once read
}

// openHistory opens the history of page n.
func (v *View) openHistory(o Options, n proto.NoteInfo) {
	if o.History == nil {
		return
	}
	h := &historyDialog{open: true, note: n}
	v.history = h
	o.History(n.ID, func(list []proto.NoteVersion, err error) {
		h.versions, h.loaded = list, true
		if err != nil {
			h.err = err.Error()
		}
		if len(list) > 0 {
			v.pickVersion(o, h, list[0].ID)
		}
	})
}

func (v *View) pickVersion(o Options, h *historyDialog, id string) {
	h.picked, h.shown = id, nil
	o.Version(h.note.ID, id, func(n proto.Note, err error) {
		if h.picked != id {
			return
		}
		if err != nil {
			h.err = err.Error()
			return
		}
		h.shown = &n
	})
}

// historySheet draws the history dialog while it's open.
func (v *View) historySheet(c *ui.Context, o Options) {
	h := v.history
	if h == nil {
		return
	}
	if !h.open {
		v.history = nil
		return
	}
	w, _ := c.Size()
	widget.Sheet(c, &h.open, min(820, max(320, w-40)), func() {
		if widget.DialogHeader(c, "Version history", "Versions of “"+titleOf(h.note)+"”, kept on this host before each change from another host or an agent, and every 10 minutes while you write.", true) {
			h.open = false
		}
	}, func() {
		if h.err != "" {
			ui.Text(c, h.err).FontSize(theme.TextLabel).TextColor(theme.Error)
		}
		if h.loaded && len(h.versions) == 0 {
			ui.Text(c, "No earlier version yet.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			return
		}
		ui.Row(c).FillWidth().Gap(theme.S3).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Width(220).Shrink(0).Gap(2).Children(func() {
				for _, ver := range h.versions {
					sub := fmt.Sprintf("%s · %s", textfmt.Age(ver.Time, time.Now()), textfmt.Bytes(uint64(ver.Size)))
					if widget.ListItem(c.Key(ver.ID), ui.Color{}, ver.Time.Local().Format("2 Jan 2006, 15:04"), sub, ver.ID == h.picked, nil).Clicked() && ver.ID != h.picked {
						v.pickVersion(o, h, ver.ID)
					}
				}
			})
			widget.Card(c).Grow(1).Children(func() {
				if h.shown == nil {
					ui.Text(c, "Reading…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
					return
				}
				ui.Text(c, titleOf(h.shown.NoteInfo)).FontSize(theme.TextTitle).FontWeight(600).TextColor(theme.TextStrong)
				widget.Markdown(c, h.shown.Body)
			})
		})
	}, func() {
		if widget.GhostButton(c, "Cancel").Clicked() {
			h.open = false
		}
		b := widget.PrimaryButton(c, nil, "Restore", "")
		if h.shown == nil {
			b.Disabled(true).Opacity(0.5)
		} else if b.Clicked() {
			id, ver := h.note.ID, h.picked
			o.Restore(id, ver, func(err error) {
				if err != nil {
					h.err = err.Error()
					return
				}
				h.open = false
				if pg := v.page; pg != nil && pg.id == id {
					pg.dirty = false // the version brought back replaces the edits
					v.reload(o, pg)
				}
			})
		}
	})
}

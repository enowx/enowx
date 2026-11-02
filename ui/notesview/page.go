package notesview

import (
	"time"

	"github.com/enowx/enowx/core/noteblocks"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/iconlib"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// docLeft is the room left of a page's blocks for their drag handles: with a
// block's own padding, its text starts where the header's title does.
const docLeft = 18

// pageView is the page open: its header, its body, and beside the body the
// outline or the agent's panel.
func (v *View) pageView(c *ui.Context, o Options) {
	p := v.page
	// Once a frame, after its edits: the frame before left it current. It
	// writes the whole body out, which a long note feels at every frame.
	defer p.rememberEdit()
	v.pageHeader(c, o, p)
	if p.ed == nil {
		ui.Text(c, "Opening…").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S4, theme.S5)
		return
	}
	p.ed.env = v.env(o, p)
	agent := v.assistant && o.AgentPanel != nil
	whole := ui.Row(c).Grow(1).FillWidth().MinHeight(0)
	p.area = whole.Bounds().W // the outline's room included
	whole.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
			sc := ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).TrackScroll(&p.scroll)
			p.viewport = sc.Bounds()
			sc.Children(func() {
				// A column to read in, centered in the frame, whatever its width.
				ui.Row(c).FillWidth().Justify(ui.Center).AlignItems(ui.Start).Children(func() {
					body := ui.Column(c).FillWidth().MaxWidth(readingWidth+2*docLeft).Padding(theme.S4, docLeft, theme.S5, 0).Gap(0)
					p.editShortcuts(body)
					p.width = body.Bounds().W - 2*docLeft - theme.S3
					body.Children(func() { v.pageBody(c.Key(p.editGeneration), o, p, v.outlineShows(p, agent)) })
				})
			})
			if p.scroll.Y > 480 {
				top := widget.IconButton(c, icon.ArrowUp, "Back to top", false)
				widget.DropShadow(widget.Raise(top.Absolute().Right(theme.S5).Bottom(theme.S5).Background(theme.Popover)), 4, 14, ui.RGBA(0, 0, 0, 0.25))
				if top.Clicked() {
					p.scroll.ScrollTo(0, 0, ui.Smooth)
				}
			}
		})
		switch {
		case agent:
			ui.Column(c).Shrink(0).FillHeight().Padding(theme.S2, theme.S2, theme.S2, 0).Children(func() {
				widget.Raise(ui.Column(c.Key("notes-agent")).Width(320).Grow(1).MinHeight(0).Radius(theme.RadiusCard).Clip().Background(theme.Surface(theme.Dialog))).
					Transition(widget.PanelMotion(24)).Children(func() { o.AgentPanel(c) })
			})
		case v.outlineShows(p, false):
			v.outlinePanel(c, p)
		}
	})
	v.followJump(c, p)
	v.jumpDialog(c, p)
}

// pageHeader is the frame's top: the page's title and where it is, its state,
// then what can be done with it. It keeps its height whatever it holds.
func (v *View) pageHeader(c *ui.Context, o Options, p *page) {
	head := ui.Row(c.Key("note-header")).FillWidth().MinHeight(56).Shrink(0).Padding(theme.S2, theme.S4, theme.S2, theme.S5).Gap(theme.S2).
		BorderWidth(0, 0, 0.5, 0).BorderColor(theme.Border)
	tight := head.Bounds().W > 0 && head.Bounds().W < 560
	head.Children(func() {
		if v.narrow && widget.IconButton(c, icon.Sidebar, "Pages", v.treeOpen).Clicked() {
			v.treeOpen = !v.treeOpen
		}
		ui.Row(c).Grow(1).MinWidth(0).Gap(theme.S2).Children(func() {
			v.breadcrumb(c, o, p.id)
			if p.ed != nil && p.icon == "" {
				v.iconButton(c, o, p, 16)
			}
			v.titleField(c, p)
			switch state := pageState(v, p); {
			case v.err != "":
				ui.Row(c).MinWidth(0).Shrink(100).Children(func() {
					widget.Running(c.Key("err"), v.err, widget.RunStyle{Size: theme.TextLabel, Color: theme.Error})
				})
			case !tight && state != "":
				ui.Row(c).MinWidth(0).Shrink(100).Children(func() {
					widget.Running(c.Key("state"), state, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				})
			}
		})
		if p.ed != nil && p.mode != modeRead {
			if widget.IconButton(c, icon.Undo, "Undo edit", false).Tooltip("Undo · Ctrl+Z / ⌘Z").Disabled(len(p.edits.past) == 0).Clicked() {
				p.restoreEdit(false)
			}
			if widget.IconButton(c, icon.Redo, "Redo edit", false).Tooltip("Redo · Ctrl+Y / Ctrl+Shift+Z / ⇧⌘Z").Disabled(len(p.edits.future) == 0).Clicked() {
				p.restoreEdit(true)
			}
		}
		v.modeSwitch(c, p)
		if p.ed != nil && p.mode != modeRead && o.PickImage != nil {
			b := widget.IconButton(c, icon.Image, "Add image…", false).Disabled(p.uploading)
			if b.Clicked() {
				p.uploading = true
				o.PickImage(p.id, v.imageDone(o, p))
			}
		}
		if widget.IconButton(c, icon.ListTree, "Outline · ⌘K jumps to a section", v.outlineShows(p, v.assistant && o.AgentPanel != nil)).Clicked() {
			if p.area >= outlineWide && !v.assistant {
				v.outlineHidden = !v.outlineHidden
			} else {
				v.OpenJump()
			}
		}
		if o.CanAsk && widget.IconButton(c, icon.Sparkle, "Note agent", v.assistant).Clicked() {
			v.assistant = !v.assistant
		}
		more := widget.IconButton(c, icon.More, "More for this note", false)
		v.menu.OpenBelow(more, "page", theme.ControlHeight)
		v.menu.Show(c, more, "page", func(done func()) { v.pageItems(c, o, p.note.NoteInfo, done) })
	})
}

// outlineShows says the outline stands beside page p: not narrow, not in
// Markdown, not over the agent's panel, with sections to list.
func (v *View) outlineShows(p *page, agent bool) bool {
	return !agent && !v.outlineHidden && p.ed != nil && p.area >= outlineWide && p.mode != modeSource && len(p.ed.env.outline) >= 2
}

// pageState says what is happening to the page, or when it last changed.
func pageState(v *View, p *page) string {
	switch {
	case p.uploading:
		return "Adding image…"
	case p.saving || p.dirty:
		return "Saving…"
	case !p.note.Updated.IsZero():
		return "Edited " + editedDay(p.note.Updated, time.Now())
	}
	return ""
}

// editedDay says the day t was in words, as the header reads it.
func editedDay(t, now time.Time) string {
	t = t.Local()
	y, m, d := now.Local().Date()
	switch day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local); {
	case !day.Before(time.Date(y, m, d, 0, 0, 0, 0, time.Local)):
		return "today"
	case !day.Before(time.Date(y, m, d-1, 0, 0, 0, 0, time.Local)):
		return "yesterday"
	case t.Year() == y:
		return t.Format("2 Jan")
	}
	return t.Format("2 Jan 2006")
}

// titleField is the page's title: as wide as its words, within the room the
// header leaves, typed in unless the page is read.
func (v *View) titleField(c *ui.Context, p *page) {
	text := titleOf(proto.NoteInfo{Title: p.title})
	if p.mode == modeRead {
		ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
			widget.Running(c.Key("title"), text, widget.RunStyle{Size: theme.TextTitle, Weight: 600, Always: true})
		})
		return
	}
	natural, _ := c.MeasureText(0, ui.Span{Text: text, Size: theme.TextTitle, Weight: 600})
	title := ui.TextInputBase(c, &p.title).Width(natural + theme.S2).MinWidth(0).Shrink(1).FontSize(theme.TextTitle).FontWeight(600).
		TextColor(theme.TextStrong).Placeholder("Untitled").Label("Title")
	if p.ed == nil {
		return
	}
	title.OnTextKey(func(k ui.TextKey) bool {
		if p.titleEditKey(k) {
			return true
		}
		if k.Key != ui.KeyEnter && k.Key != ui.KeyDown || p.mode != modeEdit {
			return false
		}
		p.ed.openAt(0, 0) // the body goes on from the title
		return true
	})
	if title.Changed() {
		p.ed.changed = true
	}
	if p.titleFocus {
		focusTitle(title, p.title)
		p.titleFocus = false
	}
}

// readingWidth is how wide a page's text gets.
const readingWidth = 760

// pageBody is the page's proposals, icon, projects and body, as its mode
// shows them, then the pages inside it.
func (v *View) pageBody(c *ui.Context, o Options, p *page, outline bool) {
	ui.Column(c).FillWidth().Gap(theme.S2).Padding(0, 6, 0, docLeft).Children(func() {
		v.undoBar(c, o)
		for _, pr := range v.proposalsFor(o, p.id) {
			v.proposalCard(c.Key(pr.ID), o, pr, p.ed.markdown())
		}
		if p.icon != "" {
			v.iconButton(c, o, p, 34)
		}
	})
	if n, ok := noteOf(o, p.id); ok {
		ui.Column(c).FillWidth().Padding(0, 0, 0, docLeft).Children(func() { v.projectBadges(c, o, n) })
	}
	if done, total := noteblocks.Progress(p.ed.markdown()); total > 0 && p.mode != modeSource && !outline {
		ui.Column(c).FillWidth().Padding(0, 6, theme.S2, docLeft+6).Children(func() { progressBar(c, "To-dos", done, total) })
	}
	if p.mode == modeSource {
		ui.Column(c).FillWidth().Padding(0, 0, 0, docLeft+6).Children(func() { v.sourceView(c, o, p) })
	} else {
		p.ed.historyKey = p.editKey
		p.ed.iconOpened = func() { v.openPicker(o) }
		p.ed.pasteImage = func() bool { return v.pasteImage(o, p) }
		p.ed.picker = func(c *ui.Context, anchor ui.Element, open *bool, pick func(string)) {
			v.iconPopover(c, o, anchor, open, "", pick)
		}
		p.ed.draw(c)
	}
	if kids := p.ed.env.children; len(kids) > 0 && p.mode != modeSource {
		ui.Column(c).FillWidth().Gap(theme.S1).Padding(theme.S3, 0, theme.S5, docLeft+6).BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border).Children(func() {
			ui.Text(c, "Pages inside").FontSize(theme.TextLabel).FontWeight(500).TextColor(theme.TextSoft)
			p.ed.pagesView(c)
		})
	}
}

// proposalsFor are the changes proposed to page id, and the new pages its
// agent proposed.
func (v *View) proposalsFor(o Options, id string) []Proposal {
	var out []Proposal
	for _, p := range o.Proposals {
		if p.Note.ID == id || p.Note.ID == "" && p.For == id {
			out = append(out, p)
		}
	}
	return out
}

// iconButton shows the page's icon, which a click picks: the default, faint,
// while it has none. The size the icon asks for wins over usual, the one
// it's given.
func (v *View) iconButton(c *ui.Context, o Options, p *page, usual float32) {
	size := usual
	if r, err := iconlib.Parse(p.icon); err == nil && r.Size > 0 {
		size = float32(r.Size)
	}
	ui.Row(c).Gap(theme.S2).AlignItems(ui.Center).Shrink(0).Children(func() {
		b := ui.ButtonBase(c).Size(size+theme.S2, size+theme.S2).Radius(theme.RadiusSmall).Center().Shrink(0).Cursor(ui.CursorPointer).Label("Page icon").Tooltip("Pick an icon")
		if b.Hovered() {
			b.Background(theme.Hover)
		}
		b.Children(func() {
			ink := theme.TextStrong
			if p.icon == "" {
				ink = theme.Idle
			}
			widget.NoteIcon(c, p.icon, size, ink, nil)
		})
		if b.Clicked() {
			if p.icons = !p.icons; p.icons {
				v.openPicker(o)
			}
		}
		v.iconPopover(c, o, b, &p.icons, p.icon, func(ref string) {
			p.icon, p.ed.changed = ref, true
			if r, err := iconlib.Parse(ref); err != nil || r.Lib == "" {
				p.icons = false // a color or a size can follow a library's icon
			}
		})
		if p.note.IconReview && p.icon == p.note.Icon && p.icon != "" {
			ui.Text(c, "This icon stands in for an emoji it had: pick one").FontSize(theme.TextLabel).TextColor(theme.Warning)
		}
	})
}

// breadcrumb is the pages the page is under, each opening it; a page at the
// top has none. They give way first when the header runs out of room.
func (v *View) breadcrumb(c *ui.Context, o Options, id string) {
	byID := map[string]proto.NoteInfo{}
	for _, n := range o.Notes {
		byID[n.ID] = n
	}
	var path []proto.NoteInfo
	for n, ok := byID[id]; ok && len(path) < 16; n, ok = byID[n.Parent] {
		path = append([]proto.NoteInfo{n}, path...)
	}
	if len(path) < 2 {
		return
	}
	ui.Row(c).Gap(theme.S1).MinWidth(0).Shrink(50).Clip().Children(func() {
		for _, n := range path[:len(path)-1] {
			b := ui.ButtonBase(c.Key(n.ID)).Height(24).PaddingX(theme.S1).Radius(theme.RadiusSmall).Gap(theme.S1).Cursor(ui.CursorPointer).Label(titleOf(n)).Shrink(1).MinWidth(0).MaxWidth(160)
			if b.Hovered() {
				b.Background(theme.Hover)
			}
			b.Children(func() {
				if n.Icon != "" {
					widget.NoteIcon(c, n.Icon, 12, theme.TextSoft, nil)
				}
				ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
					widget.Running(c, titleOf(n), widget.RunStyle{Size: theme.TextUI, Color: theme.TextSoft})
				})
			})
			if b.Clicked() {
				v.Select(o, n.ID)
			}
			ui.Text(c, "/").FontSize(theme.TextUI).TextColor(theme.TextSoft).Shrink(0)
		}
	})
}

package notesview

import (
	"github.com/enowx/enowx/core/noteblocks"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/iconlib"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// pageView is the page open: where it is, its icon and title, its body.
func (v *View) pageView(c *ui.Context, o Options) {
	p := v.page
	// Once a frame, after its edits: the frame before left it current. It
	// writes the whole body out, which a long note feels at every frame.
	defer p.rememberEdit()
	head := ui.Row(c).FillWidth().Height(44).Shrink(0).PaddingX(16).Gap(6).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border)
	tight := head.Bounds().W > 0 && head.Bounds().W < 560
	head.Children(func() {
		if v.narrow && widget.IconButton(c, icon.Sidebar, "Pages", v.treeOpen).Clicked() {
			v.treeOpen = !v.treeOpen
		}
		ui.Row(c).Shrink(1).MinWidth(0).Children(func() { v.breadcrumb(c, o, p.id) })
		ui.Spacer(c)
		state := ""
		switch {
		case v.err != "":
			ui.Text(c, v.err).FontSize(11).TextColor(theme.Error).SingleLine().Shrink(1)
		case p.uploading:
			state = "Adding image…"
		case p.saving || p.dirty:
			state = "Saving…"
		case !p.note.Updated.IsZero():
			state = "Saved"
		}
		if !tight {
			ui.Text(c, state).FontSize(11).TextColor(theme.Muted).SingleLine()
		}
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
		if widget.IconButton(c, icon.ListTree, "Outline · ⌘K jumps to a section", !v.outlineHidden && p.area >= outlineWide).Clicked() {
			if p.area >= outlineWide {
				v.outlineHidden = !v.outlineHidden
			} else {
				v.OpenJump()
			}
		}
		if o.CanAsk {
			label := "Ask an agent"
			if v.assistant {
				label = "Hide the agent"
			}
			var b ui.Element
			if tight {
				b = widget.IconButton(c, icon.Chat, label, v.assistant)
			} else {
				b = widget.Button(c, widget.Secondary, icon.Chat, label, "").Height(28).PaddingX(10)
				if v.assistant {
					b.Background(theme.Selected)
				}
			}
			if b.Clicked() {
				v.assistant = !v.assistant
			}
		}
		more := widget.IconButton(c, icon.More, "More for this page", false)
		v.menu.OpenBelow(more, "page", 28)
		v.menu.Show(c, more, "page", func(done func()) { v.pageItems(c, o, p.note.NoteInfo, done) })
	})
	if p.ed == nil {
		ui.Text(c, "Opening…").FontSize(12).TextColor(theme.Muted).Padding(40)
		return
	}
	p.ed.env = v.env(o, p)
	whole := ui.Row(c).Grow(1).FillWidth().MinHeight(0)
	p.area = whole.Bounds().W // the outline's room included
	whole.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
			sc := ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).TrackScroll(&p.scroll)
			p.viewport = sc.Bounds()
			sc.Children(func() {
				body := ui.Column(c).FillWidth().MaxWidth(760).AlignSelf(ui.Center).Padding(28, 48, 0, 48).Gap(6)
				p.editShortcuts(body)
				p.width = body.Bounds().W - 96
				body.Children(func() { v.pageBody(c.Key(p.editGeneration), o, p) })
			})
			if p.scroll.Y > 480 {
				top := widget.IconButton(c, icon.ArrowUp, "Back to top", false)
				top.Absolute().Right(24).Bottom(24).Size(34, 34).Radius(17).Background(theme.Raised).Border(1, theme.Border).
					Shadow(0, 4, 14, 0, ui.RGBA(0, 0, 0, 0.25))
				if top.Clicked() {
					p.scroll.ScrollTo(0, 0, ui.Smooth)
				}
			}
		})
		if !v.outlineHidden && p.area >= outlineWide && p.mode != modeSource && len(p.ed.env.outline) >= 2 {
			v.outlinePanel(c, p)
		}
	})
	v.followJump(c, p)
	v.jumpDialog(c, p)
}

// pageBody is the page's proposals, icon, title and body, as its mode
// shows them, then the pages inside it.
func (v *View) pageBody(c *ui.Context, o Options, p *page) {
	v.undoBar(c, o)
	for _, pr := range v.proposalsFor(o, p.id) {
		v.proposalCard(c.Key(pr.ID), o, pr, p.ed.markdown())
	}
	v.docsPanel(c, o, p)
	v.iconButton(c, o, p)
	if p.mode == modeRead {
		ui.Text(c, titleOf(proto.NoteInfo{Title: p.title})).FillWidth().PaddingX(6).FontSize(32).FontWeight(750).TextColor(theme.TextStrong)
	} else {
		title := ui.TextAreaBase(c, &p.title).FillWidth().Lines(1, 4).PaddingX(6).FontSize(32).FontWeight(750).
			TextColor(theme.TextStrong).Placeholder("Untitled").Label("Title")
		title.OnTextKey(func(k ui.TextKey) bool {
			if p.titleEditKey(k) {
				return true
			}
			if k.Key != ui.KeyEnter && k.Key != ui.KeyDown || p.mode != modeEdit {
				return false
			}
			p.ed.openAt(p.ed.firstShown(), 0) // the body goes on from the title
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
	if n, ok := noteOf(o, p.id); ok {
		v.projectBadges(c, o, n)
	}
	if done, total := noteblocks.Progress(p.ed.markdown()); total > 0 && p.mode != modeSource {
		progressBar(c, "To-dos", done, total)
	}
	ui.Box(c).Height(8)
	if p.mode == modeSource {
		v.sourceView(c, o, p)
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
		ui.Column(c).FillWidth().Gap(4).Padding(8, 0, 40, 0).BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
			ui.Text(c, "Pages inside").FontSize(11.5).FontWeight(600).TextColor(theme.Muted).Padding(10, 6, 2, 6)
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

// iconButton shows the page's icon, which a click picks: the default,
// faint, while it has none.
func (v *View) iconButton(c *ui.Context, o Options, p *page) {
	size := float32(34)
	if r, err := iconlib.Parse(p.icon); err == nil && r.Size > 0 {
		size = float32(r.Size)
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		b := ui.ButtonBase(c).Size(size+14, size+14).Radius(8).Center().Cursor(ui.CursorPointer).Label("Page icon").Tooltip("Pick an icon")
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
		if p.note.IconReview && p.icon == p.note.Icon {
			ui.Text(c, "This icon stands in for an emoji it had: pick one").FontSize(11.5).TextColor(theme.Warning)
		}
	})
}

// breadcrumb is the pages the page is under, each opening it.
func (v *View) breadcrumb(c *ui.Context, o Options, id string) {
	byID := map[string]proto.NoteInfo{}
	for _, n := range o.Notes {
		byID[n.ID] = n
	}
	var path []proto.NoteInfo
	for n, ok := byID[id]; ok && len(path) < 16; n, ok = byID[n.Parent] {
		path = append([]proto.NoteInfo{n}, path...)
	}
	ui.Row(c).Gap(2).MinWidth(0).Shrink(1).AlignItems(ui.Center).Children(func() {
		for i, n := range path {
			if i > 0 {
				ui.Text(c, "/").FontSize(11.5).TextColor(theme.Muted)
			}
			b := ui.ButtonBase(c.Key(n.ID)).Height(24).PaddingX(6).Radius(5).Gap(4).Cursor(ui.CursorPointer).Label(titleOf(n)).Shrink(1).MinWidth(0)
			if b.Hovered() {
				b.Background(theme.Hover)
			}
			b.Children(func() {
				widget.NoteIcon(c, n.Icon, 13, theme.Muted, nil)
				ui.Text(c, titleOf(n)).FontSize(11.5).TextColor(theme.TextSoft).SingleLine().Shrink(1)
			})
			if b.Clicked() {
				v.Select(o, n.ID)
			}
		}
	})
}

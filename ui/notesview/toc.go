package notesview

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Getting around a long page: its outline beside it (the section read
// now lit, foldable, down to a level), Cmd+K to jump to a section, back
// to the top, and links to sections ("[[#Section]]", "[[Page#Section]]").

// outlineWide is how wide the page's area is at least for its outline to
// show beside it.
const outlineWide = 860

// jumpState is the Jump to section dialog.
type jumpState struct {
	open  bool
	query string
	pick  int
}

// OpenJump opens Jump to section, for the page open, and says whether
// there was one (the app's Cmd+K goes there on the Notes page).
func (v *View) OpenJump() bool {
	if v.page == nil || v.page.ed == nil {
		return false
	}
	v.jump = jumpState{open: true}
	return true
}

// jumpTo scrolls page p to section id, once it is laid out, first opening
// the tab or toggle it is in.
func (v *View) jumpTo(p *page, id string) { p.jumpTo = id }

// followJump scrolls to the section asked for, when the page knows where
// it is.
func (v *View) followJump(c *ui.Context, p *page) {
	if p.jumpTo == "" || p.ed == nil {
		return
	}
	s, err := noteblocks.Find(p.ed.env.outline, p.jumpTo)
	if err != nil {
		v.err, p.jumpTo = "No section “"+p.jumpTo+"” in this page", ""
		return
	}
	i := p.ed.blockAt(s.Line)
	if !p.revealed {
		p.ed.reveal(i, s.Line-p.ed.lineOf(i))
		p.revealed = true
	}
	r := p.ed.bounds[i]
	if r.H == 0 || p.viewport.H == 0 {
		c.After(fetchPoll) // not laid out yet
		return
	}
	p.scroll.ScrollTo(0, max(p.scroll.Y+r.Y-p.viewport.Y-12, 0), ui.Smooth)
	p.jumpTo, p.revealed = "", false
}

// reveal opens the tabs and toggles of block i that line (of its text)
// is in.
func (e *editor) reveal(i, line int) {
	if i < 0 || i >= len(e.blocks) || e.blocks[i].kind != kLayout {
		return
	}
	var walk func(nodes []noteblocks.Node)
	walk = func(nodes []noteblocks.Node) {
		for _, n := range nodes {
			for _, part := range n.Parts {
				if line < part.Line || line >= max(part.End, part.Line+1) {
					continue
				}
				switch n.Name {
				case "tabs":
					e.keep("tab "+keyOf(n), part.Title)
				case "toggle":
					e.keep("toggle "+keyOf(n), "open")
				}
				walk(part.Nodes)
			}
		}
	}
	walk(noteblocks.Parse(e.blocks[i].text))
}

// openLink follows a link of page p: a section of it, another page or a
// section of one; other links are the system's.
func (v *View) openLink(o Options, p *page, link string) bool {
	link = imageSource(link)
	if isImage(link) && (strings.HasPrefix(link, "assets/") || strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "http://")) {
		v.preview = &imagePreview{open: true, name: p.ed.imageName(link), ref: link}
		return true
	}
	target, ok := strings.CutPrefix(link, "note:")
	if !ok {
		return o.OpenLink != nil && o.OpenLink(link)
	}
	target, _, _ = strings.Cut(target, "|")
	name, section, _ := strings.Cut(target, "#")
	name, section = strings.TrimSpace(name), strings.TrimSpace(section)
	if name == "" {
		v.jumpTo(p, section)
		return true
	}
	for _, n := range o.Notes {
		if n.ID == name || strings.EqualFold(strings.TrimSpace(n.Title), name) {
			if n.ID == p.id {
				v.jumpTo(p, section)
				return true
			}
			v.Select(o, n.ID)
			if v.page != nil {
				v.page.jumpTo = section
			}
			return true
		}
	}
	v.err = "No page “" + name + "”"
	return true
}

// activeSection is the heading of page p read now: the last one in the
// top third of what shows, or above it.
func (p *page) activeSection() string {
	active := ""
	for _, s := range p.ed.env.outline {
		if !strings.HasPrefix(s.Kind, "h") || s.Depth > 0 {
			continue
		}
		r := p.ed.bounds[p.ed.blockAt(s.Line)]
		if r.H == 0 || r.Y > p.viewport.Y+p.viewport.H/3 {
			break
		}
		active = s.ID
	}
	return active
}

// outlinePanel is page p's outline, beside it.
func (v *View) outlinePanel(c *ui.Context, p *page) {
	out := p.ed.env.outline
	active := p.activeSection()
	level := 9
	if n, err := strconv.Atoi(p.ed.kept("outline level")); err == nil {
		level = n
	}
	// A card as tall as it needs, and no taller than the frame's: its list
	// scrolls inside it.
	ui.Column(c).Width(240+theme.S5).Shrink(0).FillHeight().Padding(theme.S4, theme.S5, theme.S4, 0).Children(func() {
		widget.Raise(ui.Column(c.Key("outline")).FillWidth().Shrink(1).MinHeight(0).Padding(theme.S3, theme.S2).Gap(theme.S1).Radius(theme.RadiusCard).Background(theme.Raised)).Children(func() {
			ui.Row(c).FillWidth().Padding(0, 0, 0, theme.S2).Gap(0).Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, "On this page", widget.RunStyle{Size: theme.TextLabel, Weight: 500, Color: theme.TextSoft})
				})
				for _, l := range []int{1, 2, 3, 9} {
					label := strconv.Itoa(l)
					if l == 9 {
						label = "All"
					}
					b := ui.ButtonBase(c.Key(l)).Height(24).MinWidth(24).PaddingX(theme.S1).Radius(theme.RadiusFull).Center().Shrink(0).Cursor(ui.CursorPointer).Label("Outline to level " + label).
						Tooltip("Headings down to this level")
					if l == level {
						b.Background(theme.Selected)
					} else if b.Hovered() {
						b.Background(theme.Hover)
					}
					b.Children(func() { ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.TextSoft) })
					if b.Clicked() {
						p.ed.keep("outline level", strconv.Itoa(l))
					}
				}
			})
			if done, total := noteblocks.Progress(p.ed.markdown()); total > 0 {
				ui.Column(c).FillWidth().Padding(theme.S1, theme.S2).Children(func() { progressBar(c, "To-dos", done, total) })
			}
			widget.Scroll(c).FillWidth().Shrink(1).MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					hidden := 99 // entries deeper than a folded one hide
					for k, s := range out {
						if s.Level > hidden {
							continue
						}
						hidden = 99
						if s.Level > level {
							continue
						}
						folded := p.ed.kept("fold "+s.ID) == "1"
						kids := k+1 < len(out) && out[k+1].Level > s.Level && out[k+1].Level <= level
						if folded && kids {
							hidden = s.Level
						}
						v.outlineEntry(c.Key(s.ID), p, s, s.ID == active, kids, folded)
					}
				})
			})
			if p.scroll.Y > 480 {
				top := ui.ButtonBase(c).FillWidth().Height(theme.ControlHeight).Shrink(0).PaddingX(theme.S2).Gap(theme.S2).Radius(theme.RadiusFull).Justify(ui.Start).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("Back to the top")
				if top.Hovered() {
					top.Background(theme.Hover)
				}
				top.Children(func() {
					ui.Icon(c, icon.ArrowUp).FontSize(12).TextColor(theme.TextSoft)
					ui.Text(c, "Back to top").FontSize(theme.TextUI).TextColor(theme.TextSoft)
				})
				if top.Clicked() {
					p.scroll.ScrollTo(0, 0, ui.Smooth)
				}
			}
		})
	})
}

// outlineEntry is section s in the outline: a click goes there, its
// chevron folds the sections under it.
func (v *View) outlineEntry(c *ui.Context, p *page, s noteblocks.Section, active, kids, folded bool) {
	row := ui.ButtonBase(c).FillWidth().Height(theme.ControlHeight).Shrink(0).Padding(0, theme.S2, 0, theme.S2+float32(max(s.Level-1, 0))*theme.S2).Gap(theme.S1).Radius(theme.RadiusFull).
		Justify(ui.Start).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("Section " + s.Title)
	switch {
	case active:
		row.Background(theme.Selected)
	case row.Hovered():
		row.Background(theme.Hover)
	}
	row.Children(func() {
		if kids {
			chev := ui.ButtonBase(c).Size(16, 16).Radius(theme.RadiusFull).Center().Shrink(0).Cursor(ui.CursorPointer).Label("Fold " + s.Title)
			chev.Children(func() {
				svg := icon.ChevronDown
				if folded {
					svg = icon.ChevronRight
				}
				ui.Icon(c, svg).FontSize(12).TextColor(theme.SoftOn(active))
			})
			if chev.Clicked() {
				v := "1"
				if folded {
					v = ""
				}
				p.ed.keep("fold "+s.ID, v)
			}
		}
		if !strings.HasPrefix(s.Kind, "h") {
			ui.Icon(c, kindIcon(s.Kind)).FontSize(12).TextColor(theme.SoftOn(active))
		}
		st := widget.RunStyle{Color: theme.TextSoft}
		if active {
			st = widget.RunStyle{Color: theme.TextStrong, Weight: 500, Always: true}
		}
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, s.Title, st) })
		if s.Todos > 0 {
			col := theme.TextSoft
			if s.Done == s.Todos {
				col = theme.Done
			}
			ui.Text(c, fmt.Sprintf("%d/%d", s.Done, s.Todos)).FontSize(theme.TextLabel).TextColor(col).Shrink(0)
		}
	})
	if row.Clicked() {
		v.jumpTo(p, s.ID)
	}
}

// kindIcon is the icon of a section that is a layout block's part.
func kindIcon(kind string) *ui.SVG {
	switch kind {
	case "tab":
		return icon.Tabs
	case "card":
		return icon.LayoutGrid
	case "callout":
		return icon.Info
	}
	return icon.ChevronRight
}

// jumpDialog is Jump to section: type to filter the page's sections,
// arrows to pick, Enter to go.
func (v *View) jumpDialog(c *ui.Context, p *page) {
	j := &v.jump
	if !j.open || p == nil || p.ed == nil {
		j.open = false
		return
	}
	// Sections titled what was typed first, then starting with it, then
	// with it inside.
	var matches []noteblocks.Section
	q := strings.ToLower(strings.TrimSpace(j.query))
	for rank := range 3 {
		for _, s := range p.ed.env.outline {
			t := strings.ToLower(s.Title)
			switch {
			case q == "":
				if rank == 0 {
					matches = append(matches, s)
				}
			case t == q:
				if rank == 0 {
					matches = append(matches, s)
				}
			case strings.HasPrefix(t, q):
				if rank == 1 {
					matches = append(matches, s)
				}
			case strings.Contains(t, q):
				if rank == 2 {
					matches = append(matches, s)
				}
			}
		}
	}
	go_ := ""
	widget.Dialog(c, &j.open, 480, theme.S2, func() {
		prev := j.query
		in := widget.Input(c, &j.query, "Jump to a section").FillWidth().AutoFocus()
		if j.query != prev {
			j.pick = 0
		}
		in.OnTextKey(func(k ui.TextKey) bool {
			switch {
			case k.Key == ui.KeyDown && k.Mods == 0:
				j.pick++
			case k.Key == ui.KeyUp && k.Mods == 0:
				j.pick--
			default:
				return false
			}
			return true
		})
		if len(matches) > 0 {
			j.pick = (j.pick + len(matches)) % len(matches)
		}
		if in.Submitted() && len(matches) > 0 {
			go_ = matches[j.pick].ID
		}
		if len(matches) == 0 {
			ui.Text(c, "No section has that in its title.").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S2, theme.S3)
		}
		widget.Scroll(c).FillWidth().MaxHeight(360).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				for k, s := range matches {
					row := ui.ButtonBase(c.Key(s.ID)).Label("Jump to " + s.Title)
					widget.ListRow(c, row, k == j.pick, "jump", func() {
						ui.Box(c).Width(float32(max(s.Level-1, 0)) * theme.S3)
						ui.Icon(c, kindIcon(s.Kind)).FontSize(12).TextColor(theme.SoftOnPicked(k == j.pick))
						ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, s.Title, widget.RunStyle{}) })
						ui.Row(c).MinWidth(0).Shrink(100).MaxWidth(160).Children(func() {
							widget.Running(c.Key("id"), "#"+s.ID, widget.RunStyle{Size: theme.TextLabel, Color: theme.SoftOnPicked(k == j.pick)})
						})
					})
					if row.Clicked() {
						go_ = s.ID
					}
				}
			})
		})
	})
	if go_ != "" {
		j.open = false
		v.jumpTo(p, go_)
	}
}

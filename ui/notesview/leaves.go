package notesview

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The directives standing alone: a table of contents, a progress bar, the
// pages inside, a spacer, an embed.

// leafView draws directive text, on line of the page's Markdown.
func (e *editor) leafView(c *ui.Context, i int, text string, line int) {
	name, args, _, ok := noteblocks.IsLeaf(text)
	if !ok {
		e.label(c, text).FontSize(theme.TextUI)
		return
	}
	switch name {
	case "toc":
		e.tocView(c)
	case "progress":
		title, done, total := e.progressAt(line)
		progressBar(c, title, done, total)
	case "spacer":
		ui.Box(c).FillWidth().Height(28)
	case "pages":
		e.pagesView(c)
	case "embed":
		e.embedView(c, args)
	}
}

// tocView is the page's headings, each a link to it.
func (e *editor) tocView(c *ui.Context) {
	widget.Card(c).FillWidth().Gap(theme.S1).Children(func() {
		ui.Text(c, "Contents").FontSize(theme.TextUI).FontWeight(500).TextColor(theme.TextSoft).Padding(0, 0, theme.S1, 0)
		shown := false
		for _, s := range e.env.outline {
			if !strings.HasPrefix(s.Kind, "h") || s.Depth > 0 {
				continue
			}
			shown = true
			link := ui.ButtonBase(c.Key(s.ID)).FillWidth().Height(theme.ControlHeight).Padding(0, theme.S3, 0, theme.S3+float32(s.Level-1)*theme.S3).Gap(theme.S2).Radius(theme.RadiusFull).
				Justify(ui.Start).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("Go to " + s.Title)
			if link.Hovered() {
				link.Background(theme.Hover)
			}
			link.Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, s.Title, widget.RunStyle{Color: theme.TextSoft})
				})
				if s.Todos > 0 {
					ui.Text(c, fmt.Sprintf("%d/%d", s.Done, s.Todos)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Shrink(0)
				}
			})
			if link.Clicked() && e.env.jump != nil {
				e.env.jump(s.ID)
			}
		}
		if !shown {
			ui.Text(c, "Headings show here.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		}
	})
}

// progressAt is the progress of the section line is in: its heading's
// title and to-dos, the page's when it is under none.
func (e *editor) progressAt(line int) (string, int, int) {
	best := -1
	for k, s := range e.env.outline {
		if strings.HasPrefix(s.Kind, "h") && s.Line < line && line < s.End && (best < 0 || s.Line > e.env.outline[best].Line) {
			best = k
		}
	}
	if best >= 0 {
		s := e.env.outline[best]
		return s.Title, s.Done, s.Todos
	}
	done, total := noteblocks.Progress(e.markdown())
	return "This page", done, total
}

// progressBar is a bar of done out of total, under its title.
func progressBar(c *ui.Context, title string, done, total int) {
	frac := float32(0)
	if total > 0 {
		frac = float32(done) / float32(total)
	}
	ui.Column(c).FillWidth().MinWidth(0).Gap(theme.S1).Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, title, widget.RunStyle{Size: theme.TextLabel, Weight: 500, Color: theme.TextSoft})
			})
			label := fmt.Sprintf("%d/%d done", done, total)
			if total == 0 {
				label = "no to-dos"
			}
			ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Shrink(0)
		})
		col := theme.Now
		if total > 0 && done == total {
			col = theme.Done
		}
		widget.Meter(c, widget.Segment{Fraction: float64(frac), Color: col})
	})
}

// pagesView is the pages under the page, each opening it.
func (e *editor) pagesView(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		if len(e.env.children) == 0 {
			ui.Text(c, "No pages inside yet: “Add a page inside” in the page's menu makes one.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		}
		for _, n := range e.env.children {
			b := ui.ButtonBase(c.Key(n.ID)).FillWidth().Height(theme.ControlHeight).Shrink(0).PaddingX(theme.S2).Gap(theme.S2).Radius(theme.RadiusFull).Justify(ui.Start).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Label("Open " + titleOf(n))
			if b.Hovered() {
				b.Background(theme.Hover)
			}
			b.Children(func() {
				widget.NoteIcon(c, n.Icon, 14, theme.TextSoft, icon.File)
				ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, titleOf(n), widget.RunStyle{}) })
			})
			if b.Clicked() && e.env.openNote != nil {
				e.env.openNote(n.ID)
			}
		}
	})
}

// embedView shows what an embed points at: an image, a canvas, or a link
// as a card with the page's title.
func (e *editor) embedView(c *ui.Context, target string) {
	target = strings.TrimSpace(target)
	switch {
	case target == "":
		ui.Text(c, "::embed needs a link: ::embed https://…, an image's, or canvas:ID").FontSize(theme.TextUI).TextColor(theme.TextSoft)
	case strings.HasPrefix(target, "canvas:"):
		id := strings.TrimPrefix(strings.TrimPrefix(target, "canvas:"), "//")
		card := ui.ButtonBase(c).FillWidth().Cursor(ui.CursorPointer).Label("Open canvas " + id)
		card.Children(func() { embedCard(c, icon.Frame, "Canvas "+id, "Opens in the Canvas page.", "") })
		if card.Clicked() && e.env.openLink != nil {
			e.env.openLink("canvas://" + id)
		}
	case isImage(target):
		r := fetch(target, true, e.env.asset)
		switch {
		case r.img != nil:
			w, h := r.size()
			most := e.env.width - 12
			if most <= 0 {
				most = 700
			}
			if w > most {
				w, h = most, h*most/w
			}
			ui.Image(c, r.img).Size(w, h).Radius(theme.RadiusCard)
		case r.err != "":
			embedCard(c, icon.Image, target, "The image didn't load: "+r.err, target)
		default:
			embedCard(c, icon.Image, target, "Loading the image…", target)
			c.After(fetchPoll)
		}
	default:
		r := fetch(target, false, e.env.asset)
		title, about := hostOf(target), target
		if r.title != "" {
			title = r.title
		}
		if !r.done {
			c.After(fetchPoll)
		}
		embedCard(c, icon.Link, title, about, target)
	}
}

// embedCard is a card with an icon, a title and a line; a click on it
// opens link.
func embedCard(c *ui.Context, svg *ui.SVG, title, about, link string) {
	card := widget.Raise(ui.ButtonBase(c).FillWidth().MinWidth(0).Padding(theme.S3, theme.S4).Gap(theme.S3).Radius(theme.RadiusCard).Background(theme.Raised).Justify(ui.Start).AlignItems(ui.Center).Label(title))
	if link != "" {
		card.Cursor(ui.CursorPointer)
		if card.Hovered() {
			card.Background(theme.Hover)
		}
	}
	card.Children(func() {
		ui.Row(c).Size(32, 32).Radius(theme.RadiusFull).Center().Background(theme.Sunken).Shrink(0).Children(func() {
			ui.Icon(c, svg).FontSize(14).TextColor(theme.TextSoft)
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, title, widget.RunStyle{Weight: 500})
			widget.Running(c.Key("about"), about, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		if link != "" {
			ui.Icon(c, icon.External).FontSize(14).TextColor(theme.TextSoft).Shrink(0)
		}
	})
	if link != "" && card.Clicked() {
		c.OpenURL(link)
	}
}

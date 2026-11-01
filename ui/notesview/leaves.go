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
	ui.Column(c).FillWidth().Gap(1).Padding(10, 12).Radius(theme.RadiusSmall).Border(1, theme.Border).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(0, 0, 4, 0).Children(func() {
			ui.Icon(c, icon.ListTree).FontSize(13).TextColor(theme.Muted)
			ui.Text(c, "Contents").FontSize(theme.TextLabel).FontWeight(600).TextColor(theme.Muted)
		})
		shown := false
		for _, s := range e.env.outline {
			if !strings.HasPrefix(s.Kind, "h") || s.Depth > 0 {
				continue
			}
			shown = true
			link := ui.ButtonBase(c.Key(s.ID)).FillWidth().Height(24).PaddingX(4).Padding(0, 0, 0, 4+float32(s.Level-1)*14).Radius(theme.RadiusSmall).
				AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("Go to " + s.Title)
			if link.Hovered() {
				link.Background(theme.Hover)
			}
			link.Children(func() {
				ui.Text(c, s.Title).FontSize(theme.TextUI).TextColor(theme.Accent).SingleLine().Grow(1).Shrink(1)
				if s.Todos > 0 {
					ui.Text(c, fmt.Sprintf("%d/%d", s.Done, s.Todos)).FontSize(theme.TextLabel).TextColor(theme.Muted)
				}
			})
			if link.Clicked() && e.env.jump != nil {
				e.env.jump(s.ID)
			}
		}
		if !shown {
			ui.Text(c, "Headings show here.").FontSize(theme.TextLabel).TextColor(theme.Muted)
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
	ui.Column(c).FillWidth().Gap(5).Padding(6, 6).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(theme.TextLabel).FontWeight(600).TextColor(theme.TextSoft).SingleLine().Shrink(1)
			ui.Spacer(c)
			label := fmt.Sprintf("%d/%d done", done, total)
			if total == 0 {
				label = "no to-dos"
			}
			ui.Text(c, label).FontSize(theme.TextLabel).TextColor(theme.Muted)
		})
		ui.Row(c).FillWidth().Height(6).Radius(theme.RadiusTail).Background(theme.Border).Clip().Children(func() {
			if frac > 0 {
				col := theme.Accent
				if done == total {
					col = theme.Done
				}
				ui.Box(c).WidthPercent(frac * 100).FillHeight().Radius(theme.RadiusTail).Background(col)
			}
		})
	})
}

// pagesView is the pages under the page, each opening it.
func (e *editor) pagesView(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		if len(e.env.children) == 0 {
			ui.Text(c, "No pages inside yet: “Add a page inside” in the page's menu makes one.").FontSize(theme.TextLabel).TextColor(theme.Muted).Padding(4, 6)
		}
		for _, n := range e.env.children {
			b := ui.ButtonBase(c.Key(n.ID)).FillWidth().Height(30).PaddingX(8).Gap(8).Radius(theme.RadiusSmall).Justify(ui.Start).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Label("Open " + titleOf(n))
			if b.Hovered() {
				b.Background(theme.Hover)
			}
			b.Children(func() {
				widget.NoteIcon(c, n.Icon, 14, theme.Muted, icon.File)
				ui.Text(c, titleOf(n)).FontSize(theme.TextUI).TextColor(theme.Text).Underline().SingleLine()
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
		ui.Text(c, "::embed needs a link: ::embed https://…, an image's, or canvas:ID").FontSize(theme.TextLabel).TextColor(theme.Muted).Padding(4, 6)
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
			ui.Image(c, r.img).Size(w, h).Radius(theme.RadiusSmall)
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
	card := ui.ButtonBase(c).FillWidth().Padding(12, 14).Gap(12).Radius(theme.RadiusSmall).Border(1, theme.Border).Background(theme.Raised).AlignItems(ui.Center).Label(title)
	if link != "" {
		card.Cursor(ui.CursorPointer)
		if card.Hovered() {
			card.Border(1, theme.AccentLine)
		}
	}
	card.Children(func() {
		ui.Row(c).Size(34, 34).Radius(theme.RadiusSmall).Center().Background(theme.AccentSoft).Shrink(0).Children(func() {
			ui.Icon(c, svg).FontSize(16).TextColor(theme.Accent)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(theme.TextUI).FontWeight(600).TextColor(theme.TextStrong).SingleLine()
			ui.Text(c, about).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
		})
		if link != "" {
			ui.Icon(c, icon.External).FontSize(13).TextColor(theme.Muted)
		}
	})
	if link != "" && card.Clicked() {
		c.OpenURL(link)
	}
}

package notesview

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// A page's layout blocks (core/noteblocks) as the page shows them: tabs,
// toggles, callouts, columns and cards, their content read-only but for
// to-dos, which tick; a click elsewhere opens the block as Markdown.

// pageEnv is what the page gives its blocks each frame.
type pageEnv struct {
	read     bool    // the page is read, not edited
	width    float32 // the page's text's, last frame
	outline  []noteblocks.Section
	children []proto.NoteInfo // the pages under it
	// openLink follows a link: "note:…" ones in the page, others false.
	openLink func(link string) bool
	jump     func(section string)             // scrolls to a section of the page
	openNote func(id string)                  // opens a page
	asset    func(ref string) ([]byte, error) // a note's file, from its host
	kept     func(key string) string          // the page's view as the user left it
	keep     func(key, value string)
}

// layoutView draws layout block i.
func (e *editor) layoutView(c *ui.Context, i int) {
	b := e.blocks[i]
	nodes := noteblocks.Parse(b.text)
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		for k, n := range nodes {
			e.nodeView(c.Key(k), i, n)
		}
	})
}

// nodeView draws node n of block i's layout.
func (e *editor) nodeView(c *ui.Context, i int, n noteblocks.Node) {
	switch {
	case n.Name == "":
		e.runView(c, i, n)
	case n.Leaf:
		e.leafView(c, i, "::"+n.Name+" "+n.Args, e.lineOf(i)+n.Start)
	case n.Name == "tabs":
		e.tabsView(c, i, n)
	case n.Name == "toggle":
		e.toggleView(c, i, n)
	case n.Name == "callout":
		e.calloutView(c, i, n)
	case n.Name == "columns":
		e.columnsView(c, i, n)
	case n.Name == "cards":
		e.cardsView(c, i, n)
	default:
		ui.Column(c).FillWidth().Gap(4).Padding(10, 12).Radius(theme.RadiusSmall).Border(1, theme.Border).Children(func() {
			ui.Text(c, n.Name+" "+n.Args).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted)
			for _, p := range n.Parts {
				e.partView(c.Key(p.Line), i, p)
			}
		})
	}
}

// partView draws a part's content.
func (e *editor) partView(c *ui.Context, i int, p noteblocks.Part) {
	ui.Column(c).FillWidth().Gap(2).MinWidth(0).Children(func() {
		for k, n := range p.Nodes {
			e.nodeView(c.Key(k), i, n)
		}
	})
}

// runView draws plain Markdown lines inside block i's layout, as the
// page shows its blocks; a to-do's box ticks it in the layout's text.
func (e *editor) runView(c *ui.Context, i int, n noteblocks.Node) {
	bs := parseBlocks(strings.Join(n.Lines, "\n"))
	if len(bs) == 1 && bs[0].kind == kText && strings.TrimSpace(bs[0].text) == "" {
		return
	}
	nums := numbersOf(bs)
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		for k, b := range bs {
			line := n.Start + b.line
			cc := c.Key(k)
			switch b.kind {
			case kCode:
				codeView(cc, b.lang, b.text)
			case kTable:
				tableView(cc, b.text, nil, e.env.openLink)
			case kDivider:
				ui.Box(cc).FillWidth().Height(1).Margin(8, 0).Background(theme.Border)
			default:
				if b.kind == kText && strings.TrimSpace(b.text) == "" {
					ui.Box(cc).Height(6)
					continue
				}
				markRow(cc, ui.Row(cc), b, nums[k], func() { e.tickLine(i, line) }, func() {
					size, weight := textStyle(b.kind)
					t := e.label(cc, b.text).FontSize(size).FontWeight(weight).FillWidth()
					switch {
					case b.kind == kTodo && b.done:
						t.TextColor(theme.Muted)
					case b.kind == kQuote:
						t.TextColor(theme.TextSoft).Italic()
					case b.kind != kText:
						t.TextColor(theme.TextStrong)
					}
				})
			}
		}
	})
}

// tickLine ticks or unticks the to-do on line of block i's text.
func (e *editor) tickLine(i, line int) {
	lines := strings.Split(e.blocks[i].text, "\n")
	if line < 0 || line >= len(lines) {
		return
	}
	done, _, ok := noteblocks.Todo(lines[line])
	if !ok {
		return
	}
	from, to := "[ ]", "[x]"
	if done {
		from, to = "[x]", "[ ]"
		if !strings.Contains(lines[line], from) {
			from = "[X]"
		}
	}
	lines[line] = strings.Replace(lines[line], from, to, 1)
	e.blocks[i].text = strings.Join(lines, "\n")
	e.changed = true
}

// keyOf names a layout's state the page keeps: its kind, its arguments
// and its parts' titles.
func keyOf(n noteblocks.Node) string {
	var titles []string
	for _, p := range n.Parts {
		titles = append(titles, p.Title)
	}
	return n.Name + ":" + n.Args + ":" + strings.Join(titles, "|")
}

func (e *editor) kept(key string) string {
	if e.env.kept == nil {
		return ""
	}
	return e.env.kept(key)
}

func (e *editor) keep(key, value string) {
	if e.env.keep != nil {
		e.env.keep(key, value)
	}
}

// tabsView draws tabs: their titles, the one picked (kept) underlined,
// and its content.
func (e *editor) tabsView(c *ui.Context, i int, n noteblocks.Node) {
	if len(n.Parts) == 0 {
		return
	}
	key := "tab " + keyOf(n)
	at := 0
	for k, p := range n.Parts {
		if p.Title == e.kept(key) {
			at = k
		}
	}
	ui.Column(c).FillWidth().Radius(theme.RadiusSmall).Border(1, theme.Border).Clip().Children(func() {
		ui.Row(c).FillWidth().Gap(2).PaddingX(6).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Background(theme.Raised).Children(func() {
			for k, p := range n.Parts {
				title := p.Title
				if title == "" {
					title = "Tab " + strconv.Itoa(k+1)
				}
				tab := ui.ButtonBase(c.Key(k)).Height(34).PaddingX(12).Center().Cursor(ui.CursorPointer).Label("Tab " + title)
				if k == at {
					tab.BorderWidth(0, 0, 2, 0).BorderColor(theme.Accent)
				} else if tab.Hovered() {
					tab.Background(theme.Hover)
				}
				tab.Children(func() {
					t := ui.Text(c, title).FontSize(theme.TextUI).SingleLine()
					if k == at {
						t.FontWeight(600).TextColor(theme.TextStrong)
					} else {
						t.TextColor(theme.Muted)
					}
				})
				if tab.Clicked() {
					e.keep(key, p.Title)
				}
			}
		})
		ui.Column(c).FillWidth().Padding(10, 12).Children(func() { e.partView(c.Key(at), i, n.Parts[at]) })
	})
}

// toggleView draws a toggle: its title, which opens and closes it (kept),
// and its content while open.
func (e *editor) toggleView(c *ui.Context, i int, n noteblocks.Node) {
	key := "toggle " + keyOf(n)
	open := e.kept(key) == "open"
	title := n.Args
	if title == "" {
		title = "Details"
	}
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		head := ui.ButtonBase(c).FillWidth().Height(30).PaddingX(6).Gap(6).Radius(theme.RadiusSmall).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("Toggle " + title)
		if head.Hovered() {
			head.Background(theme.Hover)
		}
		head.Children(func() {
			chev := icon.ChevronRight
			if open {
				chev = icon.ChevronDown
			}
			ui.Icon(c, chev).FontSize(13).TextColor(theme.Muted)
			e.label(c, title).FontSize(theme.TextUI).FontWeight(600).TextColor(theme.TextStrong).Grow(1)
		})
		if head.Clicked() {
			v := "open"
			if open {
				v = ""
			}
			e.keep(key, v)
		}
		if open && len(n.Parts) > 0 {
			ui.Column(c).FillWidth().Padding(0, 0, 4, 26).Children(func() { e.partView(c, i, n.Parts[0]) })
		}
	})
}

// calloutKinds are a callout's kinds: its icon and color.
var calloutKinds = map[string]struct {
	svg   *ui.SVG
	color func() ui.Color
}{
	"info":    {icon.Info, func() ui.Color { return theme.Accent }},
	"note":    {icon.Info, func() ui.Color { return theme.Accent }},
	"tip":     {icon.Lightbulb, func() ui.Color { return theme.Done }},
	"success": {icon.Check, func() ui.Color { return theme.Done }},
	"warning": {icon.Alert, func() ui.Color { return theme.Warning }},
	"danger":  {icon.Alert, func() ui.Color { return theme.Error }},
	"error":   {icon.Alert, func() ui.Color { return theme.Error }},
}

// calloutOf is a callout's kind and title, from its arguments.
func calloutOf(args string) (kind, title string) {
	first, rest, _ := strings.Cut(args, " ")
	if _, ok := calloutKinds[strings.ToLower(first)]; ok {
		return strings.ToLower(first), strings.TrimSpace(rest)
	}
	return "info", args
}

func (e *editor) calloutView(c *ui.Context, i int, n noteblocks.Node) {
	kind, title := calloutOf(n.Args)
	k := calloutKinds[kind]
	col := k.color()
	ui.Row(c).FillWidth().Gap(10).Padding(12, 14).Radius(theme.RadiusSmall).Background(theme.Fade(col, 0.1)).Border(1, theme.Fade(col, 0.35)).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, k.svg).FontSize(16).TextColor(col).Margin(2, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			if title != "" {
				e.label(c, title).FontSize(theme.TextUI).FontWeight(650).TextColor(theme.TextStrong)
			}
			for _, p := range n.Parts {
				e.partView(c.Key(p.Line), i, p)
			}
		})
	})
}

// narrow says the page is too narrow for things side by side.
func (e *editor) narrow() bool { return e.env.width > 0 && e.env.width < 560 }

// columnsView draws columns side by side, or one under another when the
// page is narrow.
func (e *editor) columnsView(c *ui.Context, i int, n noteblocks.Node) {
	narrow := e.narrow()
	var box ui.Element
	if narrow {
		box = ui.Column(c).FillWidth().Gap(8)
	} else {
		box = ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Start)
	}
	box.Children(func() {
		for _, p := range n.Parts {
			col := ui.Column(c.Key(p.Line)).Gap(2).MinWidth(0)
			if narrow {
				col.FillWidth()
			} else {
				col.Grow(1).Basis(0)
			}
			col.Children(func() {
				if p.Title != "" {
					e.label(c, p.Title).FontSize(theme.TextUI).FontWeight(650).TextColor(theme.TextStrong)
				}
				e.partView(c, i, p)
			})
		}
	})
}

// cardsView draws cards in a grid as wide as the page lets.
func (e *editor) cardsView(c *ui.Context, i int, n noteblocks.Node) {
	per := 3
	if e.env.width > 0 {
		per = max(1, min(4, int(e.env.width/220)))
	}
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		for row := 0; row < len(n.Parts); row += per {
			ui.Row(c.Key(row)).FillWidth().Gap(10).AlignItems(ui.Stretch).Children(func() {
				for k := row; k < row+per; k++ {
					if k >= len(n.Parts) {
						ui.Box(c.Key(k)).Grow(1).Basis(0)
						continue
					}
					p := n.Parts[k]
					ui.Column(c.Key(k)).Grow(1).Basis(0).MinWidth(0).Gap(4).Padding(12, 14).Radius(theme.RadiusSmall).
						Border(1, theme.Border).Background(theme.Raised).Children(func() {
						if p.Title != "" {
							e.label(c, p.Title).FontSize(theme.TextUI).FontWeight(650).TextColor(theme.TextStrong)
						}
						e.partView(c, i, p)
					})
				}
			})
		}
	})
}

// numbersOf are the numbers numbered items of bs show.
func numbersOf(bs []block) []int { return (&editor{blocks: bs}).numbers() }

// lineOf is the line block i starts on in the page's Markdown.
func (e *editor) lineOf(i int) int {
	line := 0
	for k := 0; k < i && k < len(e.blocks); k++ {
		line += strings.Count(e.blocks[k].markdown(1), "\n") + 1
	}
	return line
}

// blockAt is the block line of the page's Markdown is in.
func (e *editor) blockAt(line int) int {
	at := 0
	for k := range e.blocks {
		n := strings.Count(e.blocks[k].markdown(1), "\n") + 1
		if line < at+n {
			return k
		}
		at += n
	}
	return len(e.blocks) - 1
}

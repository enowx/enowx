package notesview

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
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
	ui.Column(c).FillWidth().MinWidth(0).Gap(theme.S2).Children(func() {
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
		widget.Card(c).FillWidth().Children(func() {
			ui.Text(c, n.Name+" "+n.Args).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
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
				ui.Box(cc).FillWidth().Height(0.5).Margin(theme.S2, 0).Background(theme.Border)
			default:
				if b.kind == kText && strings.TrimSpace(b.text) == "" {
					ui.Box(cc).Height(6)
					continue
				}
				markRow(cc, ui.Row(cc), b, nums[k], func() { e.tickLine(i, line) }, func() {
					size, weight := textStyle(b.kind)
					t := e.label(cc, b.text).FontSize(size).FontWeight(weight).FillWidth()
					switch {
					case e.soft && b.kind == kText:
						t.FontSize(theme.TextUI).TextColor(theme.TextSoft)
					case b.kind == kTodo && b.done:
						t.TextColor(theme.TextSoft)
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
	widget.Raise(ui.Column(c).FillWidth().MinWidth(0).Radius(theme.RadiusCard).Background(theme.Raised)).Children(func() {
		ui.Row(c).FillWidth().Padding(theme.S1).Children(func() {
			ui.Row(c).MinWidth(0).Shrink(1).Padding(theme.S1).Gap(0).Radius(theme.RadiusControl).Background(theme.Sunken).Clip().Children(func() {
				for k, p := range n.Parts {
					title := p.Title
					if title == "" {
						title = "Tab " + strconv.Itoa(k+1)
					}
					tab := ui.ButtonBase(c.Key(k)).Height(28).PaddingX(theme.S3).Radius(theme.RadiusSmall).MinWidth(0).Shrink(1).Center().Cursor(ui.CursorPointer).Label("Tab " + title)
					switch {
					case k == at:
						tab.Background(theme.Selected)
					case tab.Hovered():
						tab.Background(theme.Hover)
					}
					tab.Children(func() {
						st := widget.RunStyle{Color: theme.TextSoft}
						if k == at {
							st = widget.RunStyle{Color: theme.TextStrong, Weight: 500}
						}
						widget.Running(c, title, st)
					})
					if tab.Clicked() {
						e.keep(key, p.Title)
					}
				}
			})
		})
		ui.Column(c).FillWidth().Padding(theme.S2, theme.S4, theme.S3, theme.S4).Children(func() { e.partView(c.Key(at), i, n.Parts[at]) })
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
	ui.Column(c).FillWidth().MinWidth(0).Gap(2).Children(func() {
		head := ui.ButtonBase(c).FillWidth().MinHeight(theme.ControlHeight).Gap(theme.S2).Radius(theme.RadiusSmall).Justify(ui.Start).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("Toggle " + title)
		if head.Hovered() {
			head.Background(theme.Hover)
		}
		head.Children(func() {
			chev := icon.ChevronRight
			if open {
				chev = icon.ChevronDown
			}
			ui.Icon(c, chev).FontSize(14).TextColor(theme.TextSoft)
			e.label(c, title).FontSize(theme.TextRead).FontWeight(500).Grow(1).MinWidth(0)
		})
		if head.Clicked() {
			v := "open"
			if open {
				v = ""
			}
			e.keep(key, v)
		}
		if open && len(n.Parts) > 0 {
			ui.Column(c).FillWidth().Padding(0, 0, theme.S1, 0).Children(func() { e.partView(c, i, n.Parts[0]) })
		}
	})
}

// calloutKinds are a callout's kinds: its icon and color.
var calloutKinds = map[string]struct {
	svg   *ui.SVG
	color func() ui.Color
}{
	"info":    {icon.Info, func() ui.Color { return theme.Text }},
	"note":    {icon.Info, func() ui.Color { return theme.Text }},
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
	// Its lines keep a block's 6 of margin: the title takes the same, and the
	// gap gives it back.
	ui.Row(c).FillWidth().MinWidth(0).Gap(theme.S3-6).Padding(theme.S3, theme.S4).Radius(theme.RadiusCard).Background(theme.Sunken).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, k.svg).FontSize(16).TextColor(col).Margin(2, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(theme.S1).Children(func() {
			if title != "" {
				e.label(c, title).FontSize(theme.TextRead).FontWeight(500).TextColor(theme.TextStrong).PaddingX(theme.S2)
			}
			e.soft = true
			defer func() { e.soft = false }()
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
		box = ui.Column(c).FillWidth().MinWidth(0).Gap(theme.S2)
	} else {
		box = ui.Row(c).FillWidth().MinWidth(0).Gap(theme.S4).AlignItems(ui.Start)
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
					e.label(c, p.Title).FontSize(theme.TextRead).FontWeight(500).TextColor(theme.TextStrong).PaddingX(theme.S2)
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
	ui.Column(c).FillWidth().MinWidth(0).Gap(theme.S3).Children(func() {
		for row := 0; row < len(n.Parts); row += per {
			ui.Row(c.Key(row)).FillWidth().MinWidth(0).Gap(theme.S3).AlignItems(ui.Stretch).Children(func() {
				for k := row; k < row+per; k++ {
					if k >= len(n.Parts) {
						ui.Box(c.Key(k)).Grow(1).Basis(0)
						continue
					}
					p := n.Parts[k]
					widget.Card(c.Key(k)).Grow(1).Basis(0).Gap(theme.S1).Padding(theme.S3, theme.S4).Children(func() {
						if p.Title != "" {
							e.label(c, p.Title).FontSize(theme.TextRead).FontWeight(500).TextColor(theme.TextStrong).PaddingX(theme.S2)
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

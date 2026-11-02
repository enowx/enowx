package widget

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
)

// Markdown draws the Markdown agents answer in: headings, paragraphs,
// nested and task lists, quotes, rules, tables and fenced code with its
// language and a Copy button; inline code, bold, italics, strikethrough
// and links. Anything else shows as text.
func Markdown(c *ui.Context, src string) { MarkdownLinks(c, src, nil) }

// LinkOpener opens a link the app knows what to do with (a file of a
// session, say) and reports whether it did; the system opens the rest.
type LinkOpener func(link string) bool

// MarkdownLinks draws src as Markdown does, its links opening through
// open first.
func MarkdownLinks(c *ui.Context, src string, open LinkOpener) {
	blocks := parsed(src)
	ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
		for i, b := range blocks {
			b.draw(c.Key(i), open)
		}
	})
}

// MarkdownList draws src as Markdown in a virtual list, which builds only
// the blocks in view: for a long document. Each block is as wide as width
// at most, centered.
func MarkdownList(c *ui.Context, list *ui.ListState, src string, width float32) ui.Element {
	blocks := parsed(src)
	return ui.List(c, list, len(blocks), func(i int) {
		ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MaxWidth(width).Children(func() { blocks[i].draw(c.Key(i), nil) })
		})
	}).Gap(theme.S2)
}

// parsedCache keeps the blocks of the texts drawn lately, so a text drawn
// in frame after frame is parsed once. It belongs to the main thread, as
// drawing does; it starts over once it holds parsedKeep texts.
var parsedCache = map[string][]block{}

const parsedKeep = 1024

func parsed(src string) []block {
	if b, ok := parsedCache[src]; ok {
		return b
	}
	if len(parsedCache) >= parsedKeep {
		clear(parsedCache)
	}
	b := parseBlocks(strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n"))
	parsedCache[src] = b
	return b
}

// block is one block of a Markdown text.
type block struct {
	kind  string // "p", "h", "code", "quote", "rule", "list", "table"
	level int    // heading level
	text  string // paragraph, heading, quote; the code of a code block
	lang  string
	items []listItem
	rows  [][]string // a table's cells, its header first
}

type listItem struct {
	depth  int
	mark   string // "•", "1." or "" for a task
	task   bool
	done   bool
	text   string
	number bool
}

func parseBlocks(lines []string) []block {
	var out []block
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		t := strings.TrimSpace(line)
		switch {
		case t == "":
		case strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"):
			fence := t[:3]
			b := block{kind: "code", lang: strings.TrimSpace(t[3:])}
			var code []string
			for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), fence); i++ {
				code = append(code, lines[i])
			}
			b.text = strings.Join(code, "\n")
			out = append(out, b)
		case isRule(t):
			out = append(out, block{kind: "rule"})
		case strings.HasPrefix(t, "#"):
			level := len(t) - len(strings.TrimLeft(t, "#"))
			if level <= 6 && (len(t) == level || t[level] == ' ') {
				out = append(out, block{kind: "h", level: level, text: strings.TrimSpace(t[level:])})
				continue
			}
			out = append(out, block{kind: "p", text: t})
		case strings.HasPrefix(t, ">"):
			var quote []string
			for ; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">"); i++ {
				quote = append(quote, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), ">")))
			}
			i--
			out = append(out, block{kind: "quote", text: strings.Join(quote, " ")})
		case isTableRow(t) && i+1 < len(lines) && isTableRule(strings.TrimSpace(lines[i+1])):
			b := block{kind: "table", rows: [][]string{tableCells(t)}}
			for i += 2; i < len(lines) && isTableRow(strings.TrimSpace(lines[i])); i++ {
				b.rows = append(b.rows, tableCells(strings.TrimSpace(lines[i])))
			}
			i--
			out = append(out, b)
		case listMarker(line) != nil:
			b := block{kind: "list"}
			for ; i < len(lines); i++ {
				if it := listMarker(lines[i]); it != nil {
					b.items = append(b.items, *it)
					continue
				}
				next := strings.TrimSpace(lines[i])
				// An indented line carries on the item above it.
				if next != "" && len(b.items) > 0 && strings.HasPrefix(lines[i], "  ") {
					b.items[len(b.items)-1].text += " " + next
					continue
				}
				break
			}
			i--
			out = append(out, b)
		default:
			para := []string{t}
			for i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if next == "" || strings.HasPrefix(next, "```") || strings.HasPrefix(next, "#") || strings.HasPrefix(next, ">") ||
					listMarker(lines[i+1]) != nil || isRule(next) || isTableRow(next) {
					break
				}
				para = append(para, next)
				i++
			}
			out = append(out, block{kind: "p", text: strings.Join(para, " ")})
		}
	}
	return out
}

func isRule(t string) bool {
	if len(t) < 3 {
		return false
	}
	for _, ch := range []string{"-", "*", "_"} {
		if strings.Trim(strings.ReplaceAll(t, " ", ""), ch) == "" {
			return true
		}
	}
	return false
}

func isTableRow(t string) bool { return strings.HasPrefix(t, "|") && strings.Count(t, "|") >= 2 }

func isTableRule(t string) bool {
	return isTableRow(t) && strings.Trim(t, "|:- ") == ""
}

func tableCells(t string) []string {
	parts := strings.Split(strings.Trim(t, "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// listMarker parses a list item line, or returns nil.
func listMarker(line string) *listItem {
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	t := strings.TrimSpace(line)
	it := listItem{depth: indent / 2}
	switch {
	case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "+ "):
		it.mark, it.text = "•", t[2:]
	default:
		n := len(t) - len(strings.TrimLeft(t, "0123456789"))
		if n == 0 || n > 3 || !(strings.HasPrefix(t[n:], ". ") || strings.HasPrefix(t[n:], ") ")) {
			return nil
		}
		it.mark, it.text, it.number = t[:n]+".", t[n+2:], true
	}
	if rest, ok := strings.CutPrefix(it.text, "[ ] "); ok {
		it.task, it.text = true, rest
	} else if rest, ok := strings.CutPrefix(strings.Replace(it.text, "[X]", "[x]", 1), "[x] "); ok {
		it.task, it.done, it.text = true, true, rest
	}
	return &it
}

func (b block) draw(c *ui.Context, open LinkOpener) {
	switch b.kind {
	case "p":
		InlineLinks(c, open, b.text).FillWidth()
	case "h":
		size := map[int]float32{1: 18, 2: 16, 3: 14.5}[b.level]
		if size == 0 {
			size = 13.5
		}
		InlineLinks(c, open, b.text).FillWidth().FontSize(size).FontWeight(650).TextColor(theme.TextStrong).Margin(theme.S1, 0, 0, 0)
	case "rule":
		ui.Box(c).Height(1).FillWidth().Background(theme.Border).Margin(theme.S1, 0)
	case "quote":
		ui.Row(c).FillWidth().Gap(theme.S3).AlignItems(ui.Stretch).Children(func() {
			ui.Box(c).Width(2).Radius(theme.RadiusTail).Background(theme.AccentLine).Shrink(0)
			InlineLinks(c, open, b.text).Grow(1).Shrink(1).TextColor(theme.TextSoft)
		})
	case "list":
		ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
			for j, it := range b.items {
				ui.Row(c.Key(j)).FillWidth().Gap(theme.S2).AlignItems(ui.Start).Padding(0, 0, 0, float32(it.depth)*18).Children(func() {
					switch {
					case it.task:
						box := ui.Box(c).Size(13, 13).Radius(theme.RadiusTail).Shrink(0).Margin(theme.S1, 0, 0, 0).Center().TextColor(theme.OnAccent)
						if it.done {
							box.Background(theme.Accent).Children(func() { ui.Icon(c, icon.Check).FontSize(10) })
						} else {
							box.Border(1, theme.Idle)
						}
					case it.number:
						ui.Text(c, it.mark).TextColor(theme.Muted).Shrink(0).MinWidth(16)
					default:
						ui.Text(c, it.mark).TextColor(theme.Idle).Shrink(0)
					}
					t := InlineLinks(c, open, it.text).Grow(1).Shrink(1)
					if it.done {
						t.TextColor(theme.Muted)
					}
				})
			}
		})
	case "table":
		table(c, b.rows, open)
	case "code":
		CodeBlock(c, b.lang, b.text)
	}
}

// table draws rows as a grid, the first as its header.
func table(c *ui.Context, rows [][]string, open LinkOpener) {
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	ui.Scroll(c).FillWidth().Children(func() {
		ui.Grid(c).Columns(cols).Radius(theme.RadiusControl).Clip().Border(1, theme.Border).Children(func() {
			for i, r := range rows {
				for j := range cols {
					cell := ""
					if j < len(r) {
						cell = r[j]
					}
					box := ui.Box(c.Key(i*cols+j)).Padding(theme.S2, theme.S2).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border)
					if i == 0 {
						box.Background(theme.Raised)
					}
					box.Children(func() {
						t := InlineLinks(c, open, cell).FontSize(theme.TextLabel)
						if i == 0 {
							t.TextColor(theme.TextSoft).FontWeight(600)
						}
					})
				}
			}
		})
	})
}

// CodeBlock is fenced code: its language and a Copy button over it, the
// code below with light highlighting.
func CodeBlock(c *ui.Context, lang, code string) {
	ui.Column(c).FillWidth().Radius(theme.RadiusCard).Clip().Border(1, theme.Border).Children(func() {
		ui.Row(c).FillWidth().Height(30).PaddingX(theme.S3).Gap(theme.S2).Background(theme.Raised).
			BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
			label := lang
			if label == "" {
				label = "text"
			}
			ui.Text(c, label).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			ui.Spacer(c)
			copyButton(c, code)
		})
		ui.Scroll(c).FillWidth().Background(theme.Sunken).Children(func() {
			ui.RichText(c).Font(theme.MonoFont).FontSize(theme.TextLabel).Padding(theme.S2, theme.S3).TextColor(theme.Text).Selectable().Children(func() {
				highlight(c, lang, code)
			})
		})
	})
}

// copyButton copies text to the clipboard.
func copyButton(c *ui.Context, text string) {
	b := ui.ButtonBase(c).Height(22).PaddingX(theme.S2).Gap(theme.S1).Radius(theme.RadiusSmall).Label("Copy").Cursor(ui.CursorPointer)
	fg := theme.Muted
	if b.Hovered() {
		fg = theme.Text
	}
	b.TextColor(tint(b, "fg", fg)).Children(func() {
		ui.Icon(c, icon.Copy).FontSize(12)
		ui.Text(c, "Copy").FontSize(theme.TextLabel).SingleLine()
	})
	if Pressable(b).Clicked() {
		c.WriteClipboard(text)
	}
}

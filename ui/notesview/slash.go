package notesview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The slash menu: "/" typed in an empty text block lists the kinds of
// block, narrowed by what follows it; Enter or a click makes the block
// that kind.

type slashItem struct {
	name, hint string
	make       block
}

// slashIcon is the item putting an icon in the text, picked, rather than
// making a block.
const slashIcon = "Icon"

var slashItems = []slashItem{
	{"Text", "Plain text", block{kind: kText}},
	{"Heading 1", "#", block{kind: kH1}},
	{"Heading 2", "##", block{kind: kH2}},
	{"Heading 3", "###", block{kind: kH3}},
	{"To-do list", "[]", block{kind: kTodo}},
	{"Bulleted list", "-", block{kind: kBullet}},
	{"Numbered list", "1.", block{kind: kNumber}},
	{"Quote", ">", block{kind: kQuote}},
	{"Code", "```", block{kind: kCode}},
	{"Table", "|", block{kind: kTable, text: "| Column | Column |\n|---|---|\n|  |  |"}},
	{"Divider", "---", block{kind: kDivider}},
	{"Tabs", ":::tabs", block{kind: kLayout, text: ":::tabs\n::tab Summary\n\n::tab Details\n\n:::"}},
	{"Toggle", ":::toggle", block{kind: kLayout, text: ":::toggle Details\n\n:::"}},
	{"Columns", ":::columns", block{kind: kLayout, text: ":::columns\n::col\n\n::col\n\n:::"}},
	{"Callout", ":::callout", block{kind: kLayout, text: ":::callout info Note\n\n:::"}},
	{"Cards", ":::cards", block{kind: kLayout, text: ":::cards\n::card First\n{{Todo}}\n::card Second\n{{Todo}}\n:::"}},
	{"Table of contents", "::toc", block{kind: kLeaf, text: "::toc"}},
	{"Progress", "::progress", block{kind: kLeaf, text: "::progress"}},
	{"Pages inside", "::pages", block{kind: kLeaf, text: "::pages"}},
	{"Embed", "::embed", block{kind: kLeaf, text: "::embed https://"}},
	{"Spacer", "::spacer", block{kind: kLeaf, text: "::spacer"}},
	{slashIcon, ":icon[]", block{}},
	{"Drawing (SVG)", "```svg", block{kind: kCode, lang: "svg", text: `<svg viewBox="0 0 240 120" xmlns="http://www.w3.org/2000/svg">
  <rect x="10" y="10" width="220" height="100" rx="12" fill="none" stroke="currentColor" stroke-width="2"/>
</svg>`}},
}

// aliases are other words the slash menu finds an item by.
var aliases = map[string]string{"To-do list": "todo checkbox task", "Columns": "kolom", "Table of contents": "toc outline", "Toggle": "collapse fold", "Callout": "note warning", "Cards": "grid"}

// slashOpen says the open block shows the menu: text starting with "/",
// one word.
func (e *editor) slashOpen() bool {
	if e.open < 0 || e.open >= len(e.blocks) {
		return false
	}
	b := e.blocks[e.open]
	return b.kind == kText && strings.HasPrefix(b.text, "/") && !strings.ContainsAny(b.text, " \n")
}

// slashMatches are the items the open block's text after "/" names.
func (e *editor) slashMatches() []slashItem {
	q := strings.ToLower(strings.TrimPrefix(e.blocks[e.open].text, "/"))
	var out []slashItem
	for _, it := range slashItems {
		if strings.Contains(strings.ToLower(it.name), q) || strings.Contains(aliases[it.name], q) {
			out = append(out, it)
		}
	}
	return out
}

// slashMenu shows the menu under the open block's input.
func (e *editor) slashMenu(c *ui.Context, in ui.Element, i int) {
	items := e.slashMatches()
	if len(items) == 0 {
		e.menuIn = false
		return
	}
	e.slash = min(e.slash, len(items)-1)
	ui.Overlay(c, func() {
		h := float32(420)
		if _, wh := c.Size(); wh > 0 {
			h = min(h, wh-120)
		}
		menu := widget.Popup(ui.Column(c).AttachTo(in, ui.AnchorBottomStart, ui.AnchorTopStart).Width(264).MaxHeight(h).Margin(theme.S1, 0, 0, 0))
		e.menuIn = menu.Hovered()
		menu.Children(func() {
			widget.PopupLabel(c, "Blocks")
			ui.Scroll(c).FillWidth().Shrink(1).MinHeight(0).Children(func() {
				for j, it := range items {
					row := ui.ButtonBase(c.Key(it.name)).Justify(ui.Start).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(it.name)
					widget.PopupItem(row, j == e.slash || row.Hovered()).Children(func() {
						ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, it.name, widget.RunStyle{}) })
						ui.Text(c, it.hint).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Shrink(0)
					})
					if row.Clicked() {
						e.makeBlock(i, it)
					}
				}
			})
		})
	})
}

// slashKey takes the menu's keys: Up and Down pick, Enter makes, Escape
// leaves the text as typed. took says the menu had the key.
func (e *editor) slashKey(i int, k ui.TextKey) (done, took bool) {
	items := e.slashMatches()
	if len(items) == 0 {
		return false, false
	}
	switch k.Key {
	case ui.KeyUp:
		e.slash = (e.slash + len(items) - 1) % len(items)
	case ui.KeyDown:
		e.slash = (e.slash + 1) % len(items)
	case ui.KeyEnter:
		k.SetText("")
		e.makeBlock(i, items[min(e.slash, len(items)-1)])
	case ui.KeyEscape:
		k.SetText(strings.TrimPrefix(k.Text(), "/"))
		e.blocks[i].text = strings.TrimPrefix(e.blocks[i].text, "/")
	default:
		return false, false
	}
	return true, true
}

// makeBlock makes block i the item's kind, empty.
func (e *editor) makeBlock(i int, it slashItem) {
	if it.name == slashIcon {
		e.blocks[i].text = ""
		e.menuIn, e.seen = false, 0
		e.pickInlineIcon(i)
		return
	}
	e.blocks[i] = it.make
	e.changed, e.menuIn = true, false
	switch it.make.kind {
	case kDivider, kLayout:
		// A layout shows laid out at once; a click opens it as Markdown
		// to fill in.
		e.insert(i, block{})
		return
	case kLeaf:
		if it.make.text != "::embed https://" {
			e.insert(i, block{})
			return
		}
	}
	e.openAt(i, -1)
}

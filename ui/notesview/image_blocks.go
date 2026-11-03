package notesview

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/enowx/enowx/core/noteblocks"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

type imagePart struct {
	raw, src string
}

type imageEditor struct {
	body string
	ed   *editor
}

// imageBlockParts recognizes both older inline captions and editable columns.
func imageBlockParts(b block) ([]imagePart, bool) {
	if b.kind == kText {
		parts := imageParts(b.text)
		return parts, len(parts) > 1
	}
	if b.kind != kLayout {
		return nil, false
	}
	nodes := noteblocks.Parse(b.text)
	if len(nodes) != 1 || nodes[0].Name != "columns" || len(nodes[0].Parts) != 2 {
		return nil, false
	}
	lines := noteblocks.Lines(b.text)
	left, right := nodes[0].Parts[0], nodes[0].Parts[1]
	image := strings.TrimSpace(strings.Join(lines[left.Line+1:left.End], "\n"))
	parts := imageParts(image)
	if len(parts) != 3 || parts[0].raw != "" || parts[2].raw != "" {
		return nil, false
	}
	parts[2].raw = strings.Join(lines[right.Line+1:right.End], "\n")
	return parts, true
}

// imageParts keeps the original Markdown intact while captions edit separately.
func imageParts(text string) []imagePart {
	var parts []imagePart
	last := 0
	for _, m := range imageMatches(text) {
		parts = append(parts, imagePart{raw: text[last:m[0]]}, imagePart{
			raw: text[m[0]:m[1]], src: imageSource(text[m[4]:m[5]]),
		})
		last = m[1]
	}
	return append(parts, imagePart{raw: text[last:]})
}

func (e *editor) imageBlock(c *ui.Context, i int) {
	e.menuIn = false
	parts, _ := imageBlockParts(e.blocks[i])
	outer := ui.Column(c).FillWidth().MinWidth(0).Gap(theme.S2).Padding(theme.S1, 6)
	e.bounds[i] = outer.Bounds()
	outer.Children(func() {
		if strings.TrimSpace(parts[0].raw) != "" {
			e.imageCaption(c.Key("before"), i, parts, 0)
		}
		for j := 1; j < len(parts); j += 2 {
			part := parts[j]
			width := e.env.width
			if width <= 0 {
				width = 600
			}
			row := ui.Row(c.Key(j)).FillWidth().MinWidth(0).Gap(theme.S4).AlignItems(ui.Start)
			wide := width >= 480
			imageWidth := width - 12
			if wide {
				imageWidth = min(320, (width-28)*0.52)
			} else {
				row.Column()
			}
			if r := fetch(part.src, true, e.env.asset); r.img != nil {
				w, _ := r.size()
				imageWidth = min(imageWidth, w)
			}
			row.Children(func() {
				ui.Column(c).Width(imageWidth).Shrink(0).Gap(theme.S1).Children(func() {
					name := e.imageName(part.src)
					anchor := ui.ButtonBase(c).FillWidth().Column().Cursor(ui.CursorText).Label("Block " + strconv.Itoa(i+1))
					anchor.Children(func() {
						preview := *e
						preview.env.width = imageWidth + 12
						preview.embedView(c, part.src)
					})
					if !e.env.read {
						if anchor.Clicked() {
							e.openAt(i, -1)
						}
						e.imageMenu.Open(anchor, part.src)
						e.imageMenu.Show(c, anchor, part.src, func(done func()) {
							if widget.MenuItem(c, icon.Pencil, "Rename image…", false) {
								e.rename = &imageRename{open: true, ref: part.src, name: name}
								done()
							}
							if widget.MenuItem(c, icon.Trash, "Delete image", true) {
								e.imageRemoval = &imageRemoval{block: i, part: j}
								done()
							}
						})
					}
					ui.Row(c).FillWidth().Gap(theme.S1).AlignItems(ui.Center).Children(func() {
						ui.Row(c).Grow(1).MinWidth(0).Children(func() {
							widget.Running(c, name, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
						})
						if !e.env.read && widget.IconButton(c, icon.Link, "Mention "+name, false).Clicked() {
							e.insert(i, block{text: imageReference(name, part.src) + " "})
							e.openAt(i+1, -1)
						}
						if !e.env.read && widget.IconButton(c, icon.Trash, "Delete "+name, false).Clicked() {
							e.imageRemoval = &imageRemoval{block: i, part: j}
						}
					})
				})
				ui.Column(c).Grow(1).MinWidth(0).FillWidth().Children(func() {
					e.imageCaption(c.Key("caption"), i, parts, j+1)
				})
			})
		}
	})
}

func (e *editor) imageCaption(c *ui.Context, i int, parts []imagePart, at int) {
	if e.imageEditors == nil {
		e.imageEditors = map[string]*imageEditor{}
	}
	key := strconv.Itoa(i) + ":" + strconv.Itoa(at)
	body := parts[at].raw
	if e.blocks[i].kind == kText {
		body = strings.TrimPrefix(body, " ")
	}
	state := e.imageEditors[key]
	if state == nil || state.body != body {
		state = &imageEditor{body: body, ed: newEditor(body)}
		e.imageEditors[key] = state
	}
	child := state.ed
	child.compact, child.env = true, e.env
	child.relatedImages = e.noteImages()
	child.pasteImage, child.iconOpened, child.picker = e.pasteImage, e.iconOpened, e.picker
	child.historyKey = nil
	if e.historyKey != nil {
		child.historyKey = func(k ui.TextKey) bool {
			child.changed = true
			e.commitImageCaption(i, parts, at, state)
			return e.historyKey(k)
		}
	}
	child.leadingBackspace = nil
	if at > 0 {
		child.leadingBackspace = func() bool {
			e.imageRemoval = &imageRemoval{block: i, part: at - 1}
			return true
		}
	}
	if e.focusing && e.open == i && at == len(parts)-1 {
		child.openAt(len(child.blocks)-1, e.caret)
		e.focusing = false
	}
	area := ui.Column(c).FillWidth().MinWidth(0)
	if area.Bounds().W > 0 {
		child.env.width = area.Bounds().W
	}
	area.Children(func() { child.draw(c) })
	e.commitImageCaption(i, parts, at, state)
}

func (e *editor) commitImageCaption(i int, parts []imagePart, at int, state *imageEditor) {
	child := state.ed
	if !child.changed {
		return
	}
	child.changed = false
	body := strings.TrimSuffix(child.markdown(), "\n")
	state.body = body
	parts[at].raw = body
	if e.blocks[i].kind == kText && len(child.blocks) == 1 && child.blocks[0].kind == kText {
		parts[at].raw = " " + body
		var joined strings.Builder
		for _, part := range parts {
			joined.WriteString(part.raw)
		}
		e.blocks[i].text = joined.String()
	} else {
		var joined strings.Builder
		joined.WriteString(parts[0].raw)
		for j := 1; j < len(parts); j += 2 {
			joined.WriteString("\n:::columns\n::col\n" + parts[j].raw + "\n::col\n" + parts[j+1].raw + "\n:::\n")
		}
		parsed := parseBlocks(strings.TrimSpace(joined.String()))
		e.blocks = slices.Replace(e.blocks, i, i+1, parsed...)
	}
	e.changed = true
}

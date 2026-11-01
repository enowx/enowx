package notesview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A page is edited a block at a time where it shows (Edit), read with
// its links, tabs and toggles alone answering clicks (Read), or written
// as its Markdown (Markdown).
const (
	modeEdit = iota
	modeRead
	modeSource
)

// modeNames are the modes as the switch says them.
var modeNames = []string{"Edit", "Read", "Markdown"}

// modeSwitch is the page's modes, the one it is in lit.
func (v *View) modeSwitch(c *ui.Context, p *page) {
	if k := widget.Segmented(c, modeNames, p.mode, nil); k >= 0 {
		v.setMode(p, k)
	}
}

// setMode shows page p in mode.
func (v *View) setMode(p *page, mode int) {
	if p.ed == nil {
		return
	}
	p.rememberEdit()
	p.ed.close()
	if mode == modeSource {
		p.src = p.ed.markdown()
	}
	p.mode = mode
	p.edits.current, p.edits.group = p.editState(), ""
}

// sourceView is the page's Markdown, written as text.
func (v *View) sourceView(c *ui.Context, o Options, p *page) {
	in := ui.TextAreaBase(c, &p.src).FillWidth().Lines(16, 100000).Font(theme.MonoFont).FontSize(theme.TextUI).Padding(theme.S3, theme.S4).Label("Markdown").
		Radius(theme.RadiusCard)
	widget.Well(in, in.Focused())
	in.OnPaste(func(text string) bool { return text == "" && v.pasteImage(o, p) })
	in.OnTextKey(func(k ui.TextKey) bool {
		if _, ok := editShortcut(k.Mods, k.Key); !ok {
			return false
		}
		k.SetText(k.Text())
		p.editKey(k)
		k.SetText(p.src)
		return true
	})
	if p.sourceFocus {
		in.Focus()
		p.sourceFocus = false
	}
	if in.Changed() {
		p.ed.blocks = parseBlocks(p.src)
		p.ed.changed = true
		clear(p.ed.tables)
	}
	ui.Box(c).Height(80)
}

// env is what page p's blocks are given this frame.
func (v *View) env(o Options, p *page) pageEnv {
	var kids []proto.NoteInfo
	for _, n := range o.Notes {
		if n.Parent == p.id {
			kids = append(kids, n)
		}
	}
	return pageEnv{
		read:     p.mode == modeRead,
		width:    p.width,
		outline:  noteblocks.Outline(p.ed.markdown()),
		children: kids,
		openLink: func(link string) bool { return v.openLink(o, p, link) },
		jump:     func(id string) { v.jumpTo(p, id) },
		openNote: func(id string) { v.Select(o, id) },
		asset:    o.Asset,
		kept:     func(key string) string { return v.keptValue(o, p.id+" "+key) },
		keep: func(key, value string) {
			if v.kept == nil {
				v.kept = map[string]string{}
			}
			v.kept[p.id+" "+key] = value
			if o.Keep != nil {
				o.Keep(p.id+" "+key, value)
			}
		},
	}
}

// keptValue is how the user left key, here or as the app keeps it.
func (v *View) keptValue(o Options, key string) string {
	if val, ok := v.kept[key]; ok {
		return val
	}
	return o.Kept[key]
}

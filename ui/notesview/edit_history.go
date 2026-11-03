package notesview

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

type editState struct {
	title, icon, body string
	open, caret       int
}

type editHistory struct {
	ready        bool
	current      editState
	past, future []editState
	group        string
	last         time.Time
}

func (p *page) editState() editState {
	body := p.ed.recentMarkdown()
	if p.mode == modeSource {
		body = p.src
	}
	return editState{p.title, p.icon, body, p.ed.open, p.ed.seen}
}

func sameEdit(a, b editState) bool { return a.title == b.title && a.icon == b.icon && a.body == b.body }

func (p *page) rememberEdit() {
	if p.ed == nil {
		return
	}
	h, next := &p.edits, p.editState()
	if !h.ready {
		h.current, h.ready = next, true
		return
	}
	if sameEdit(h.current, next) {
		h.current.open, h.current.caret = next.open, next.caret
		return
	}
	group := ""
	if h.current.icon == next.icon {
		if h.current.body == next.body {
			group = "title"
		} else if h.current.title == next.title && h.current.open == next.open && (next.open >= 0 || p.mode == modeSource) && editShape(h.current.body) == editShape(next.body) {
			group = "body:" + strconv.Itoa(next.open)
		}
	}
	now := time.Now()
	if group == "" || group != h.group || now.Sub(h.last) > 600*time.Millisecond {
		h.past = append(h.past, h.current)
	}
	h.current, h.future, h.group, h.last = next, nil, group, now
	// Bound retained Markdown while keeping at least one complete undo step.
	bytes := 0
	for j := len(h.past) - 1; j >= 0; j-- {
		bytes += len(h.past[j].body) + len(h.past[j].title) + len(h.past[j].icon)
		if (len(h.past)-j > 64 || bytes > 16<<20) && j < len(h.past)-1 {
			h.past = append([]editState(nil), h.past[j+1:]...)
			break
		}
	}
}

// editShape separates typing groups from structural changes and image actions.
func editShape(body string) string {
	var shape strings.Builder
	for _, b := range parseBlocks(body) {
		shape.WriteString(strconv.Itoa(int(b.kind)) + ":" + strconv.Itoa(b.indent) + ":" + strconv.FormatBool(b.done) + ":" + b.lang + ";")
		if parts, ok := imageBlockParts(b); ok {
			for _, part := range parts {
				if part.src != "" {
					shape.WriteString(part.raw)
				} else {
					shape.WriteString(editShape(part.raw))
				}
			}
		} else if b.kind == kLayout {
			shape.WriteString(b.text)
		}
	}
	return shape.String()
}

func (p *page) restoreEdit(redo bool) bool {
	if p.ed == nil || p.mode == modeRead {
		return false
	}
	p.rememberEdit()
	h := &p.edits
	from, to := &h.past, &h.future
	if redo {
		from, to = &h.future, &h.past
	}
	if len(*from) == 0 {
		return false
	}
	*to = append(*to, h.current)
	target := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	h.current, h.group = target, ""
	p.title, p.icon, p.src = target.title, target.icon, target.body
	p.editGeneration++
	p.ed = newEditor(target.body)
	p.ed.changed, p.dirty, p.edited = true, true, time.Now()
	if p.mode == modeEdit {
		at := min(max(target.open, 0), len(p.ed.blocks)-1)
		p.ed.openAt(at, target.caret)
	} else if p.mode == modeSource {
		p.sourceFocus = true
	}
	return true
}

func editShortcut(mods ui.Modifiers, key ui.Key) (redo, ok bool) {
	switch {
	case mods == ui.Cmd && key == ui.KeyZ:
		return false, true
	case mods == ui.Cmd|ui.Shift && key == ui.KeyZ:
		return true, true
	case ui.Cmd == ui.Ctrl && mods == ui.Cmd && key == ui.KeyY:
		return true, true
	}
	return false, false
}

func (p *page) editKey(k ui.TextKey) bool {
	redo, ok := editShortcut(k.Mods, k.Key)
	if !ok || p.mode == modeRead {
		return false
	}
	p.restoreEdit(redo)
	return true // keep an empty page history from invoking an input's older undo stack
}

func (p *page) editShortcuts(area ui.Element) {
	if p.mode == modeRead {
		return
	}
	area.OnShortcut(ui.Cmd, ui.KeyZ, func() { p.restoreEdit(false) })
	area.OnShortcut(ui.Cmd|ui.Shift, ui.KeyZ, func() { p.restoreEdit(true) })
	if ui.Cmd == ui.Ctrl {
		area.OnShortcut(ui.Cmd, ui.KeyY, func() { p.restoreEdit(true) })
	}
}

func (p *page) titleEditKey(k ui.TextKey) bool {
	if _, ok := editShortcut(k.Mods, k.Key); !ok {
		return false
	}
	k.SetText(k.Text())
	p.editKey(k)
	k.SetText(p.title)
	p.ed.close()
	p.ed.focusing = false
	p.titleFocus = true
	p.sourceFocus = false
	return true
}

func focusTitle(in ui.Element, title string) {
	in.Focus()
	end := utf8.RuneCountInString(title)
	in.SetTextSelection(end, end)
}

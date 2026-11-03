package fileview

import (
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Editing a file: MyGo's text area over its rope (TextBuffer) carries the typing, selection,
// clipboard and input methods, and stays quick at ten megabytes; what's here is what makes it
// a code editor: line numbers, colors for the lines in view, keys, saving through the daemon,
// and what happens when an agent changes the file meanwhile. A programmatic edit clears the
// area's own undo, so undo is kept here, as snapshots of the rope, which share what's the same.

const (
	// editLineMost is the longest line that can be edited: the area shapes a line whole, and
	// typing in one of 100 KB takes seconds. Such a file reads in the long view.
	editLineMost = 8 << 10
	// editLex is the most that gets colors while editing.
	editLex = 256 << 10
	// editCompareMost is how long a text is compared for whether it's back as saved, on each
	// change; past it, any change counts.
	editCompareMost = 1 << 20
	editUndoMost    = 500
	editUndoGroup   = 600 * time.Millisecond
	editLexWait     = 200 * time.Millisecond
	editLineHeight  = 19
	editPad         = 8
)

// editPoint is a state of the text to go back to.
type editPoint struct {
	snap ui.TextSnapshot
	sel  int
}

// editor is a file being edited. It belongs to the main thread.
type editor struct {
	buf     *ui.TextBuffer
	crlf    bool // the file's lines end so, and will again when saved
	saved   string
	base    time.Time // the file as read: what a save writes over
	dirty   bool
	version uint64 // the text as this code last saw it
	text    string // the text, as long as it's compared
	saving  bool
	err     string

	lines  ui.TextLines
	scroll ui.ScrollState

	undo, redo []editPoint
	current    editPoint
	touched    time.Time
	selA, selB int
	setSel     *[2]int
	pending    func()

	spans   map[string][]widget.Span // lines' colors, by their text
	lexed   uint64
	lexing  bool
	changed time.Time

	// conflict is the file as an agent left it, with changes of the user's here too.
	conflict *proto.FileContent
	compare  bool
	mine     widget.LongText
	theirs   widget.LongText

	find     findBar
	gotoOpen bool
	gotoLine string
}

// editingBlocked says why the file can't be edited, "" when it can.
func editingBlocked(f *proto.FileContent) string {
	switch {
	case f.Binary || f.Image != "":
		return " " // nothing to say: it isn't text
	case f.ReadOnly:
		return "Read only"
	case f.Truncated:
		return "Read only: over " + strconv.Itoa(proto.MaxEdit>>20) + " MB, the first part shows"
	case strings.ContainsRune(f.Text, utf8.RuneError):
		// What isn't UTF-8 reaches here replaced: saving would write that over the real bytes.
		return "Read only: not UTF-8 text"
	}
	for rest := f.Text; rest != ""; {
		i := strings.IndexByte(rest, '\n')
		if i < 0 {
			i = len(rest)
		}
		if i > editLineMost {
			return "Read only: a line over " + strconv.Itoa(editLineMost>>10) + " KB"
		}
		rest = rest[min(i+1, len(rest)):]
	}
	return ""
}

// Dirty says there are changes not saved.
func (d *Doc) Dirty() bool { return d.edit != nil && d.edit.dirty }

// fromDisk takes the file as read: it starts an editor, keeps one up to date while nothing
// is changed here, and with changes here, holds what's new for the user to say.
func (d *Doc) fromDisk(f *proto.FileContent) {
	why := editingBlocked(f)
	d.editNote = strings.TrimSpace(why)
	e := d.edit
	switch {
	case why != "" && (e == nil || !e.dirty):
		d.edit = nil
	case e == nil:
		d.edit = newEditor(f)
	case f.Modified.Equal(e.base):
	case !e.dirty:
		e.load(f)
	default:
		e.conflict = f
	}
}

func newEditor(f *proto.FileContent) *editor {
	e := &editor{buf: ui.NewTextBuffer("")}
	e.load(f)
	return e
}

// load makes the file's text the editor's, starting over.
func (e *editor) load(f *proto.FileContent) {
	e.crlf = strings.Contains(f.Text, "\r\n")
	text := f.Text
	if e.crlf {
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	e.buf.Set(text)
	e.saved, e.text, e.base, e.dirty, e.conflict, e.compare = text, text, f.Modified, false, nil, false
	e.undo, e.redo = nil, nil
	e.current = editPoint{snap: e.buf.Snapshot()}
	e.version = e.buf.Version()
}

// noteChange follows what the user typed.
func (e *editor) noteChange() {
	if e.buf.Version() == e.version {
		return
	}
	now := time.Now()
	if now.Sub(e.touched) > editUndoGroup {
		e.pushUndo()
	}
	e.touched, e.changed = now, now
	e.redo = nil
	e.current = editPoint{snap: e.buf.Snapshot(), sel: e.selB}
	e.afterChange()
}

func (e *editor) pushUndo() {
	e.undo = append(e.undo, e.current)
	if len(e.undo) > editUndoMost {
		e.undo = e.undo[1:]
	}
}

// afterChange works out whether the text is as saved.
func (e *editor) afterChange() {
	e.version = e.buf.Version()
	if e.buf.ByteLen() > editCompareMost {
		e.dirty, e.text = true, ""
		return
	}
	e.text = e.buf.String()
	e.dirty = e.text != e.saved
}

// change replaces runes a to b with text, as one step to undo, the selection then from..to.
func (e *editor) change(a, b int, text string, from, to int) {
	e.pushUndo()
	e.redo = nil
	e.buf.Replace(a, b, text)
	e.current = editPoint{snap: e.buf.Snapshot(), sel: to}
	e.changed, e.touched = time.Now(), time.Time{}
	e.setSel = &[2]int{from, to}
	e.afterChange()
}

func (e *editor) stepBack() {
	if len(e.undo) == 0 {
		return
	}
	e.redo = append(e.redo, e.current)
	e.current = e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.restore()
}

func (e *editor) stepForward() {
	if len(e.redo) == 0 {
		return
	}
	e.undo = append(e.undo, e.current)
	e.current = e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.restore()
}

func (e *editor) restore() {
	e.buf.Restore(e.current.snap)
	e.setSel = &[2]int{e.current.sel, e.current.sel}
	e.changed, e.touched = time.Now(), time.Time{}
	e.afterChange()
}

// primaryMod is the key shortcuts go with: Cmd on a Mac, Ctrl elsewhere.
func primaryMod() ui.Modifiers {
	if runtime.GOOS == "darwin" {
		return ui.Super
	}
	return ui.Ctrl
}

// save writes the text through the daemon, on whatever host the session is. Force writes
// over what's on disk, the user having said so.
func (d *Doc) save(o Options, force bool) {
	e := d.edit
	cl := o.Client
	if e == nil || cl == nil || e.saving {
		return
	}
	text := e.buf.String()
	if e.crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	e.saving, e.err = true, ""
	version, plain, id, p, base := e.buf.Version(), e.buf.String(), o.Session.ID, d.path, e.base
	go savedFile(cl, id, p, text, base, force, func(done proto.FileWritten, read *proto.FileContent, err error) {
		o.Update(func() {
			e.saving = false
			switch {
			case err != nil:
				e.err = client.Explain(err)
			case done.Changed:
				if read != nil {
					e.conflict = read
				}
			default:
				e.base, e.saved, e.conflict, e.compare = done.Modified, plain, nil, false
				e.dirty = e.buf.Version() != version
			}
		})
	})
}

// savedFile saves, and reads what's there when it couldn't for the file having changed.
func savedFile(cl *client.Client, id, p, text string, base time.Time, force bool, done func(proto.FileWritten, *proto.FileContent, error)) {
	w, err := cl.WriteFile(id, p, text, base, force)
	if err != nil || !w.Changed {
		done(w, nil, err)
		return
	}
	f, err := cl.ReadFileWhole(id, p, time.Time{})
	if err != nil {
		f = proto.FileContent{Path: p, Modified: w.Modified}
	}
	done(w, &f, nil)
}

// editorView is the editor, with the bars it has open.
func (d *Doc) editorView(c *ui.Context, o Options) {
	e := d.edit
	if e.pending != nil {
		fn := e.pending
		e.pending = nil
		fn()
	}
	// With the focus in a bar of its own, the keys still work.
	if c.Shortcut(primaryMod(), ui.KeyS) {
		d.save(o, false)
	}
	if c.Shortcut(primaryMod(), ui.KeyF) {
		e.find.open = true
	}
	ui.Column(c.Key("editing")).Grow(1).FillWidth().MinHeight(0).Children(func() {
		d.conflictBar(c, o, e)
		if e.err != "" {
			ui.Row(c.Key("err")).FillWidth().Shrink(0).Padding(theme.S2, theme.S4, 0, theme.S4).Children(func() {
				widget.Notice(c, nil, theme.Del, e.err, "", nil)
			})
		}
		e.findView(c, o)
		e.gotoView(c)
		if e.conflict != nil && e.compare {
			e.compareView(c, o, d.path)
			return
		}
		e.area(c, o, d)
	})
}

// area is the line numbers and the text.
func (e *editor) area(c *ui.Context, o Options, d *Doc) {
	e.lex(c, o, d.path)
	digits := len(strconv.Itoa(e.buf.LineCount()))
	width := max(float32(digits)*8+theme.S4, 44)
	ink := ui.Hex("#6E7386")
	ui.Row(c.Key("editor")).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Box(c.Key("numbers")).Width(width).FillHeight().Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			top := r.Y + editPad - e.scroll.Y
			size := theme.TextUI
			p.Clip(r, 0, func() {
				for i := e.lines.At(e.scroll.Y); i < e.lines.Count() && top+e.lines.Top(i) < r.Y+r.H; i++ {
					word := strconv.Itoa(i + 1)
					x := r.X + width - theme.S2 - float32(len(word))*size*.6
					p.Text(x, top+e.lines.Top(i)+1, word, size, ink)
				}
			})
		})
		area := ui.TextAreaBufferBase(c.Key("text"), e.buf).Grow(1).FillHeight().MinWidth(0).NoWrap().Padding(editPad, widget.ScrollGutter, editPad, theme.S1).
			Font(theme.MonoFont).FontSize(theme.TextUI).FixedLineHeight(editLineHeight).TextColor(ui.Hex("#E4E6EE")).Label("File text").AutoFocus().
			TrackLines(&e.lines).TrackScroll(&e.scroll).
			OnTextKey(func(k ui.TextKey) bool { return d.key(c, o, k) })
		e.selA, e.selB = area.TextSelection()
		if area.Changed() {
			e.noteChange()
		}
		if e.setSel != nil {
			area.SetTextSelection(e.setSel[0], e.setSel[1])
			e.setSel = nil
		}
		if ranges := e.visibleRanges(); len(ranges) > 0 {
			area.TextRanges(ranges...)
		}
	})
}

// visibleRanges are the colors of the lines in view: each is looked up by its text, so that
// the lines keep theirs as others move around them, until they're lexed again.
func (e *editor) visibleRanges() []ui.TextRange {
	if len(e.spans) == 0 && len(e.find.hits) == 0 {
		return nil
	}
	snap := e.buf.Snapshot()
	var out []ui.TextRange
	first := e.lines.At(e.scroll.Y)
	last := e.lines.At(e.scroll.Y+2000) + 1
	for i := first; i < last && i < snap.LineCount(); i++ {
		a, _ := snap.LineRange(i)
		at := a
		for _, s := range e.spans[snap.Line(i)] {
			n := utf8.RuneCountInString(s.Text)
			out = append(out, ui.TextRange{Start: at, End: at + n, Color: s.Color()})
			at += n
		}
	}
	lo, _ := snap.LineRange(first)
	_, hi := snap.LineRange(min(last, snap.LineCount()-1))
	for _, h := range e.find.hits {
		if h[1] >= lo && h[0] <= hi {
			out = append(out, ui.TextRange{Start: h[0], End: h[1], Color: widget.CodeWarn, Weight: 700})
		}
	}
	return out
}

// lex colors the text off the main thread, a moment after it last changed.
func (e *editor) lex(c *ui.Context, o Options, p string) {
	if e.buf.ByteLen() > editLex || e.lexing || e.lexed == e.buf.Version() {
		return
	}
	if wait := editLexWait - time.Since(e.changed); wait > 0 {
		c.After(wait)
		return
	}
	version := e.buf.Version()
	snap := e.buf.Snapshot()
	e.lexing = true
	name := path.Base(p)
	go func() {
		text := snap.String()
		lines := widget.HighlightLines(name, "", text)
		spans := make(map[string][]widget.Span, len(lines))
		for _, spansOf := range lines {
			var line strings.Builder
			for _, s := range spansOf {
				line.WriteString(s.Text)
			}
			spans[line.String()] = spansOf
		}
		o.Update(func() {
			e.spans, e.lexed, e.lexing = spans, version, false
		})
	}()
}

// conflictBar says the file changed on disk with changes here too, and asks what to do.
func (d *Doc) conflictBar(c *ui.Context, o Options, e *editor) {
	f := e.conflict
	if f == nil {
		return
	}
	ui.Row(c.Key("conflict")).FillWidth().Shrink(0).Wrap().Padding(theme.S2, theme.S4).Gap(theme.S2).AlignItems(ui.Center).Background(theme.Fade(theme.Warning, .12)).Children(func() {
		ui.Text(c, "This file changed on disk, and you have changes here that aren't saved.").FontSize(theme.TextUI).TextColor(theme.Text).Grow(1).MinWidth(0)
		label := "Compare"
		if e.compare {
			label = "Back to editing"
		}
		if widget.Button(c.Key("compare"), widget.Secondary, nil, label, "").Clicked() {
			e.compare = !e.compare
		}
		if widget.Button(c.Key("mine"), widget.Secondary, nil, "Keep mine", "Stay with the text here: saving writes over what's on disk").Clicked() {
			e.base, e.conflict, e.compare = f.Modified, nil, false
		}
		if widget.Button(c.Key("theirs"), widget.Secondary, nil, "Take theirs", "Drop the changes here for what's on disk").Clicked() {
			e.load(f)
			e.setSel = &[2]int{0, 0}
		}
	})
}

// compareView shows the two side by side.
func (e *editor) compareView(c *ui.Context, o Options, p string) {
	f := e.conflict
	name := path.Base(p)
	e.mine.Load(widget.LongSource{ID: "mine" + strconv.FormatUint(e.buf.Version(), 10), Size: e.buf.ByteLen(), Name: name, Make: func() (string, string) { return e.current.snap.String(), "" }}, o.Update)
	e.theirs.Load(widget.LongSource{ID: "theirs" + f.Modified.String(), Size: len(f.Text), Name: name, Make: func() (string, string) { return f.Text, "" }}, o.Update)
	ui.Row(c.Key("compare")).Grow(1).FillWidth().MinHeight(0).Gap(theme.S2).Children(func() {
		side := func(key, title string, long *widget.LongText) {
			ui.Column(c.Key(key)).Grow(1).Basis(0).MinWidth(0).FillHeight().Children(func() {
				ui.Text(c, title).FontSize(theme.TextLabel).TextColor(ui.Hex("#8A8FA3")).Padding(theme.S2, theme.S3)
				long.Draw(c.Key("text"), widget.LongOptions{Post: o.Update, Numbers: true, Ink: ui.Hex("#E4E6EE")})
			})
		}
		side("mine", "Yours, not saved", &e.mine)
		side("theirs", "On disk", &e.theirs)
	})
}

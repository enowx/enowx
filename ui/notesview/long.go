package notesview

import (
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A long note: its Markdown and what's read from it are worked out once for each change,
// not several times a frame; only so many of its blocks are drawn at a time; and a block too
// long to lay out as text being typed in shows as it reads, and stays as it is.

const (
	// longBlock is how long a block may be to be edited, and drawn as its kind asks.
	longBlock = 64 << 10
	// sourceEditMost is how long a note may be to be edited as Markdown in one text area.
	sourceEditMost = 256 << 10
	// blocksWhole is how many blocks a note may have to draw all of them; a longer one
	// draws blocksStep at a time.
	blocksWhole = 2000
	blocksStep  = 300
	// longLook is how often what's asked each frame looks through a long note's blocks for
	// a change: that alone would be felt at every frame. Saving always looks.
	longLook = 250 * time.Millisecond
)

// workedOut is what was last worked out of an editor's blocks, and from what.
type workedOut struct {
	ready  bool
	blocks []block
	md     string
	at     time.Time // last compared with the blocks

	outlineDone bool
	outlineFor  string
	outline     []noteblocks.Section

	progressDone bool
	progressFor  string
	done, total  int
}

// markdown is the page's Markdown. It goes by the blocks themselves, whatever changed them:
// a text compares at once while it's the same one.
func (e *editor) markdown() string {
	w := &e.worked
	if !w.ready || !slices.Equal(w.blocks, e.blocks) {
		w.md, w.blocks, w.ready = renderBlocks(e.blocks), slices.Clone(e.blocks), true
	}
	w.at = time.Now()
	return w.md
}

// recentMarkdown is markdown for what's asked each frame and shows, or follows, what's
// there: on a long note, as of moments ago.
func (e *editor) recentMarkdown() string {
	w := &e.worked
	if w.ready && len(e.blocks) > blocksWhole && time.Since(w.at) < longLook {
		return w.md
	}
	return e.markdown()
}

// outline is the page's sections.
func (e *editor) outline() []noteblocks.Section {
	md, w := e.recentMarkdown(), &e.worked
	if !w.outlineDone || w.outlineFor != md {
		w.outline, w.outlineFor, w.outlineDone = noteblocks.Outline(md), md, true
	}
	return w.outline
}

// progress is how many of the page's to-dos are done, of how many.
func (e *editor) progress() (done, total int) {
	md, w := e.recentMarkdown(), &e.worked
	if !w.progressDone || w.progressFor != md {
		w.done, w.total = noteblocks.Progress(md)
		w.progressFor, w.progressDone = md, true
	}
	return w.done, w.total
}

// drawnTo is how many of the blocks are drawn, from the first.
func (e *editor) drawnTo() int {
	if len(e.blocks) <= blocksWhole {
		return len(e.blocks)
	}
	return min(max(e.shown, blocksStep), len(e.blocks))
}

// showMore follows the blocks drawn of a long note, upTo of them.
func (e *editor) showMore(c *ui.Context, upTo int) {
	left := len(e.blocks) - upTo
	next := min(left, blocksStep)
	ui.Column(c.Key("more-blocks")).FillWidth().Gap(theme.S1).Padding(theme.S3, 0, theme.S3, docLeft).AlignItems(ui.Start).Children(func() {
		ui.Text(c, strconv.Itoa(left)+" more blocks in this long page.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		if widget.Button(c.Key("show"), widget.Secondary, nil, "Show "+strconv.Itoa(next)+" more", "").Clicked() {
			e.shown = upTo + next
		}
	})
}

// sourceLong shows a Markdown too long for one text area: to read and copy, not to edit.
func (p *page) sourceLong(c *ui.Context) {
	widget.Notice(c.Key("source-long"), icon.Alert, theme.Warning, "Too long to edit as Markdown here",
		"You can read and copy it. Edit changes this page a block at a time.", nil)
	p.longSrc.Load(widget.LongSource{ID: strconv.Itoa(len(p.src)), Make: func() (string, string) { return p.src, "" }}, nil)
	ui.Column(c.Key("source-text")).FillWidth().Height(max(p.viewport.H-120, 320)).Margin(theme.S2, 0, 0, 0).Children(func() {
		p.longSrc.Draw(c.Key("text"), widget.LongOptions{})
	})
}

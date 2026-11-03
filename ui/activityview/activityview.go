// Package activityview is the side panel's Activity tool: what a
// session's agent did, turn by turn — the prompt, how long it worked, the
// tools it ran — and the files it edited, read from the transcript the
// chat view reads (docs/chat-view.md). It sums the work up where the chat
// tells it.
package activityview

import (
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

const (
	// refreshEvery is how often a shown view asks for news.
	refreshEvery = 2 * time.Second
	// tail is about how many entries of a long conversation it reads.
	tail = 400
)

// Options connect a view to the app.
type Options struct {
	Client  *client.Client
	Session proto.SessionInfo
	// Update runs a function on the main thread; Wanted, called there,
	// reports whether the view still shows, else its poller stops.
	Update func(func())
	Wanted func() bool
	// OpenFile shows a file of the session's folder, by its path in it.
	OpenFile func(path string)
	// OpenDiff shows what changed in such a file.
	OpenDiff func(path string)
}

// View is one session's activity. It belongs to the main thread; its
// poller reports through Options.Update.
type View struct {
	entries   map[int]proto.TranscriptEntry // by index
	rev       int64
	supported bool
	reason    string
	loaded    bool
	err       string
	polling   bool
	open      map[int]bool // turns unfolded, by their first entry's index

	turns []turn // worked out as entries change, newest last
	files []edited
}

// New returns a view with nothing read.
func New() *View {
	return &View{entries: map[int]proto.TranscriptEntry{}, open: map[int]bool{}}
}

func (v *View) follow(o Options) {
	if v.polling || o.Client == nil {
		return
	}
	v.polling = true
	cl, id, since := o.Client, o.Session.ID, v.rev
	go func() {
		for {
			t, err := cl.TranscriptPage(proto.TranscriptParams{ID: id, Since: since, Tail: tail})
			if err == nil {
				since = t.Rev
			}
			keep := make(chan bool, 1)
			o.Update(func() {
				if !o.Wanted() {
					v.polling = false
					keep <- false
					return
				}
				keep <- true
				v.apply(t, err, o.Session.Dir)
			})
			if !<-keep {
				return
			}
			time.Sleep(refreshEvery)
		}
	}()
}

// apply takes in what the daemon answered.
func (v *View) apply(t proto.Transcript, err error, dir string) {
	v.loaded = true
	if err != nil {
		v.err = client.Explain(err)
		return
	}
	v.err, v.supported, v.reason = "", t.Supported, t.Reason
	if t.Reset {
		v.entries = map[int]proto.TranscriptEntry{}
	}
	for _, e := range t.Entries {
		v.entries[e.Index] = e
	}
	v.rev = t.Rev
	if len(t.Entries) > 0 || t.Reset {
		v.reckon(dir)
	}
}

// turn is what came of one prompt: the prompt, and the agent's work.
type turn struct {
	start  int
	prompt string
	at     time.Time
	end    time.Time
	tools  []proto.TranscriptEntry
	failed int
	edits  int
	files  map[string]bool // what it edited
}

// edited is a file the agent changed, with the lines it added and removed.
type edited struct {
	path           string // in the session's folder when it is there
	added, removed int
	edits          int
}

// editTools are the tools that change files.
var editTools = []string{"Edit", "MultiEdit", "Write", "NotebookEdit"}

// reckon works the turns and files out from the entries.
func (v *View) reckon(dir string) {
	idx := make([]int, 0, len(v.entries))
	for i := range v.entries {
		idx = append(idx, i)
	}
	slices.Sort(idx)
	var turns []turn
	files := map[string]*edited{}
	for _, i := range idx {
		e := v.entries[i]
		if e.Kind == proto.TranscriptUser || len(turns) == 0 {
			turns = append(turns, turn{start: e.Index, at: e.Time})
		}
		t := &turns[len(turns)-1]
		if e.Time.After(t.end) {
			t.end = e.Time
		}
		switch e.Kind {
		case proto.TranscriptUser:
			t.prompt = e.Text
		case proto.TranscriptTool:
			t.tools = append(t.tools, e)
			if e.Failed {
				t.failed++
			}
			if slices.Contains(editTools, e.Text) && !e.Failed && e.Detail != "" {
				t.edits++
				p := relative(dir, e.Detail)
				if t.files == nil {
					t.files = map[string]bool{}
				}
				t.files[p] = true
				f := files[p]
				if f == nil {
					f = &edited{path: p}
					files[p] = f
				}
				f.added, f.removed, f.edits = f.added+e.Added, f.removed+e.Removed, f.edits+1
			}
		}
	}
	v.turns = turns
	v.files = v.files[:0:0]
	for _, f := range files {
		v.files = append(v.files, *f)
	}
	slices.SortFunc(v.files, func(a, b edited) int { return strings.Compare(a.path, b.path) })
}

// relative is p in dir when it is there, else p.
func relative(dir, p string) string {
	if dir != "" {
		if rel, ok := strings.CutPrefix(p, strings.TrimSuffix(dir, "/")+"/"); ok {
			return rel
		}
	}
	return p
}

// totals sums the activity up.
func (v *View) totals() (tools, failed, edits int) {
	for _, t := range v.turns {
		tools, failed, edits = tools+len(t.tools), failed+t.failed, edits+t.edits
	}
	return tools, failed, edits
}

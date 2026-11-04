package projectdocsview

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// store is notes kept in memory, standing in for a host. Reads come off the
// main thread, as the app's do; everything else runs on the test's goroutine.
type store struct {
	mu    sync.Mutex
	notes map[string]proto.Note
	order []string
	next  int
	clock time.Time
	reads int

	failAfter int // the save after which saves fail, 0 for never
	saves     int
	deleted   []string

	queue []func()

	view   View
	now    time.Time
	open   string
	opened []string
	// The page, as notesview gives it.
	unsaved  bool
	flushes  int
	rewrites []string
}

func newStore() *store {
	s := &store{notes: map[string]proto.Note{}, clock: time.Unix(1000, 0), now: time.Unix(5000, 0)}
	s.view.makeProject = fakeNew
	return s
}

func (s *store) put(n proto.Note) (proto.NoteInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.failAfter > 0 && s.saves > s.failAfter {
		return proto.NoteInfo{}, fmt.Errorf("the disk is full")
	}
	if n.ID == "" {
		s.next++
		n.ID = fmt.Sprintf("n%d", s.next)
		s.order = append(s.order, n.ID)
	}
	s.clock = s.clock.Add(time.Second)
	n.Updated = s.clock
	s.notes[n.ID] = n
	return n.NoteInfo, nil
}

func (s *store) list() []proto.NoteInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]proto.NoteInfo, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.notes[id].NoteInfo)
	}
	return out
}

func (s *store) body(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notes[id].Body
}

func (s *store) readCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

func (s *store) options() Options {
	return Options{
		Host: "this host", Notes: s.list(),
		Load: func(id string, done func(proto.Note, error)) {
			s.mu.Lock()
			n, ok := s.notes[id]
			s.mu.Unlock()
			if !ok {
				done(n, fmt.Errorf("no note %s", id))
				return
			}
			done(n, nil)
		},
		Save: func(n proto.Note, done func(proto.NoteInfo, error)) {
			info, err := s.put(n)
			done(info, err)
		},
		Delete: func(id string, done func(error)) {
			s.mu.Lock()
			delete(s.notes, id)
			for i, x := range s.order {
				if x == id {
					s.order = append(s.order[:i:i], s.order[i+1:]...)
					break
				}
			}
			s.deleted = append(s.deleted, id)
			s.mu.Unlock()
			done(nil)
		},
		Read: func(id string) (proto.Note, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.reads++
			n, ok := s.notes[id]
			if !ok {
				return n, fmt.Errorf("no note %s", id)
			}
			return n, nil
		},
		Main: func(fn func()) {
			s.mu.Lock()
			s.queue = append(s.queue, fn)
			s.mu.Unlock()
		},
		Open:           func(id string) { s.open = id; s.opened = append(s.opened, id) },
		AttachProjects: func(proto.NoteInfo) {},
		Attach: func(id string, projects []proto.NoteProject, done func(proto.NoteInfo, error)) {
			s.mu.Lock()
			n := s.notes[id]
			n.Projects = append(n.Projects, projects...)
			s.notes[id] = n
			s.mu.Unlock()
			done(n.NoteInfo, nil)
		},
		ExportRepo: func(id, folder string, force bool, done func(string, error)) {
			done(folder, nil)
		},
		Folder: func(p proto.NoteProject) string { return p.Path },
	}
}

// page is the page open, as notesview hands it to the panel.
func (s *store) page() Page {
	n := s.notes[s.open]
	return Page{
		Note:    n,
		Unsaved: s.unsaved,
		Flush:   func() { s.flushes++ },
		Rewrite: func(body string) {
			s.rewrites = append(s.rewrites, body)
			n := s.notes[s.open]
			n.Body = body
			_, _ = s.put(n)
		},
	}
}

// drain runs what waits for the main thread.
func (s *store) drain() {
	for {
		s.mu.Lock()
		q := s.queue
		s.queue = nil
		s.mu.Unlock()
		if len(q) == 0 {
			return
		}
		for _, fn := range q {
			fn()
		}
	}
}

// tester draws the panel, and the dialogs over the window, width wide.
func (s *store) tester(width, height int) *ui.Tester {
	s.view.now = func() time.Time { return s.now }
	return ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(float32(width)).Height(float32(height)).Background(theme.Background).Children(func() {
			o := s.options()
			if s.open != "" {
				s.view.Panel(c, o, s.page())
			}
			s.view.Dialogs(c, o)
		})
	}, width, height)
}

// settle goes on until cond, draining the main thread's queue and drawing.
func (s *store) settle(t *testing.T, tt *ui.Tester, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		s.drain()
		tt.Advance(20 * time.Millisecond)
		tt.Frame()
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("%s: never came: %q", what, tt.Texts())
}

func (s *store) frames(tt *ui.Tester, n int) {
	for range n {
		s.drain()
		tt.Advance(50 * time.Millisecond)
		tt.Frame()
	}
}

// fixtureBody is a document's body, whichever templates this branch has.
func fixtureBody(kind string) string {
	return projectdocs.Meta{Kind: kind, Status: projectdocs.StatusDraft, Version: 1}.Line() + "\n\n## Features and priorities\n\nText\n\n- [ ] one\n"
}

// fakeNew makes what projectdocs.New does, without needing its templates.
func fakeNew(o projectdocs.Options, save projectdocs.Saver) (proto.NoteInfo, []string, error) {
	root, err := save(proto.Note{NoteInfo: proto.NoteInfo{Title: o.Title, Icon: "lucide:folder-kanban"}, Body: projectdocs.ProjectBody(o.Size, o.Approval)})
	if err != nil {
		return proto.NoteInfo{}, nil, err
	}
	made := []string{root.ID}
	areas := map[string]string{}
	for _, e := range projectdocs.Pack(o.Size) {
		parent := root.ID
		if o.Size == projectdocs.SizeFull {
			if areas[e.Area] == "" {
				a, err := save(proto.Note{NoteInfo: proto.NoteInfo{Title: e.Area, Icon: "lucide:folder", Parent: root.ID}, Body: projectdocs.Meta{Kind: projectdocs.KindArea}.Line() + "\n"})
				if err != nil {
					return root, made, err
				}
				areas[e.Area] = a.ID
				made = append(made, a.ID)
			}
			parent = areas[e.Area]
		}
		n, err := save(proto.Note{NoteInfo: proto.NoteInfo{Title: e.Name, Icon: e.Icon, Parent: parent}, Body: fixtureBody(e.Name)})
		if err != nil {
			return root, made, err
		}
		made = append(made, n.ID)
	}
	for _, own := range o.Own {
		n, err := save(proto.Note{NoteInfo: proto.NoteInfo{Title: own.Title, Icon: own.Icon, Parent: root.ID},
			Body: projectdocs.WithMeta(own.Body, projectdocs.Meta{Kind: own.Title, Status: projectdocs.StatusDraft, Version: 1})})
		if err != nil {
			return root, made, err
		}
		made = append(made, n.ID)
	}
	return root, made, nil
}

// project makes a project in the store.
func (s *store) project(t *testing.T, title, size string) proto.NoteInfo {
	t.Helper()
	root, _, err := fakeNew(projectdocs.Options{Title: title, Size: size, Approval: projectdocs.ApprovalRequired}, s.put)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// docNamed is the ID of the note titled title.
func (s *store) docNamed(t *testing.T, title string) string {
	t.Helper()
	for _, n := range s.list() {
		if n.Title == title {
			return n.ID
		}
	}
	t.Fatalf("no note %q", title)
	return ""
}

func (s *store) setBody(id string, change func(string) string) {
	n := s.notes[id]
	n.Body = change(n.Body)
	_, _ = s.put(n)
}

func (s *store) retitle(id, title string) {
	n := s.notes[id]
	n.Title = title
	_, _ = s.put(n)
}

func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
}

var longTitle = strings.TrimSpace(strings.Repeat("A very long title that never seems to end ", 8))

var errRefused = fmt.Errorf("export: not written, these aren't files of an export: prd.md (not from Notes) (--force writes over a file)")

// proposalTester draws the panel for the page page gives.
func (s *store) proposalTester(page func() Page) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1000).Height(900).Children(func() { s.view.Panel(c, s.options(), page()) })
	}, 1000, 900)
}

// exportTester draws the panel and dialogs, exports going to export.
func (s *store) exportTester(export func(id, folder string, force bool, done func(string, error))) *ui.Tester {
	s.view.now = func() time.Time { return s.now }
	return ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1000).Height(1600).Background(theme.Background).Children(func() {
			o := s.options()
			o.ExportRepo = export
			s.view.Panel(c, o, s.page())
			s.view.Dialogs(c, o)
		})
	}, 1000, 1600)
}

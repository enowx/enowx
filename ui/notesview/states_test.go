package notesview

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
	"github.com/enowx/enowx/ui/wrapcheck"
)

const statesTable = "| Name | Size |\n|---|---|\n| Alpha | 3 |\n| Beta | 1 |\n| Gamma | 2 |\n| Delta | 5 |\n| Epsilon | 4 |\n| Zeta | 6 |\n"

const statesCards = ":::cards\n::card First\nOne.\n::card Second\nTwo.\n::card Third\nThree.\n::card Fourth\nFour.\n::card Fifth\nFive.\n:::\n"

// notesState is a state of the Notes page to draw: set shapes the page, steps
// reach it by using it, wants is what must show then, gone what must not.
type notesState struct {
	name       string
	body       string
	w          int
	options    func(o *Options)
	view       func(v *View)
	note       func(n *proto.NoteInfo)
	steps      func(t *testing.T, tt *ui.Tester, v *View)
	wants      []string
	gone       []string
	noPageOpen bool
}

func click(t *testing.T, tt *ui.Tester, labels ...string) {
	t.Helper()
	for _, l := range labels {
		if err := tt.Click(l); err != nil {
			t.Fatalf("%v; shows %q", err, tt.Texts())
		}
		frames(tt, 12)
	}
}

// Notes' states that show by what the page is given or by using it, each drawn
// (SHOTS) and said what it shows.
func TestNotesStates(t *testing.T) {
	offline := proto.NoteProject{Ref: "o", Name: "offline-one", Host: "h2", HostName: "enowxlabs", Path: "/srv/one"}
	broken := proto.NoteProject{Ref: "b", Name: "broken-one", Host: "h3", HostName: "old-vps", Path: "/srv/two"}
	on := func(ref string) proto.NoteProject {
		return proto.NoteProject{Ref: ref, Name: ref, Host: "h1", HostName: "this Mac", Path: "/work/" + ref}
	}
	prefs := proto.NoteIconPrefs{Favorites: []string{"lucide:rocket"}, Recent: []string{"lucide:bug", "tabler:calendar"}}
	withPrefs := func(o *Options) {
		o.IconPrefs = func(done func(proto.NoteIconPrefs, error)) { done(prefs, nil) }
		o.SaveIconPrefs = func(p proto.NoteIconPrefs, done func(proto.NoteIconPrefs, error)) { done(p, nil) }
		o.OpenSVG = func(done func([]byte, error)) { done(nil, errors.New("couldn't read icon.svg: it isn't a file")) }
	}
	states := []notesState{
		{name: "loading", noPageOpen: true, options: func(o *Options) { o.Loaded, o.Notes = false, nil }, wants: []string{"Reading the notes…"}},
		{name: "error", noPageOpen: true, options: func(o *Options) { o.Loaded, o.Notes, o.Err = false, nil, "the daemon didn't answer" },
			wants: []string{"The notes didn't load", "the daemon didn't answer"}},
		{name: "team-spaces", options: func(o *Options) {
			o.Space, o.Team = "Design", "0123456789abcdef"
			o.Spaces, o.PickSpace = []widget.Space{{Name: "Personal"}, {Team: "0123456789abcdef", Name: "Design"}}, func(string) {}
		}, wants: []string{"Design · on this Mac"}, gone: []string{"Personal"}},
		{name: "team-one", options: func(o *Options) { o.Space, o.Team = "Design", "0123456789abcdef" }, wants: []string{"Design · on this Mac"}},
		{name: "sync-warning", options: func(o *Options) {
			o.SyncState, o.OpenSync = func() (string, bool) { return "2 conflicts to look at", true }, func() {}
		}, wants: []string{"Roadmap"}},
		{name: "icon-review", note: func(n *proto.NoteInfo) { n.IconReview, n.Icon = true, "lucide:rocket" }, wants: []string{"Roadmap"}},
		{name: "badges", note: func(n *proto.NoteInfo) { n.Projects = []proto.NoteProject{broken, offline, on("antares"), on("vega"), on("rigel")} },
			options: func(o *Options) {
				o.AttachProjects = func(proto.NoteInfo) {}
				o.ProjectState = func(p proto.NoteProject) string {
					switch p.Ref {
					case "o":
						return ProjectOffline
					case "b":
						return ProjectBroken
					}
					return ProjectOnline
				}
			}, wants: []string{"broken-one", "offline-one", "+2"}},
		{name: "project-filter", note: func(n *proto.NoteInfo) { n.Projects = []proto.NoteProject{on("antares"), offline} }, noPageOpen: true,
			steps: func(t *testing.T, tt *ui.Tester, _ *View) { click(t, tt, "Projects: all pages") },
			wants: []string{"antares", "offline-one"}},
		{name: "tight", w: 700, body: "Some text.", steps: func(_ *testing.T, tt *ui.Tester, _ *View) { frames(tt, 4) }, gone: []string{"Edited today"}},
		{name: "header-error", view: func(v *View) { v.err = "couldn't save: the host is out of reach" }, wants: []string{"couldn't save: the host is out of reach"}},
		{name: "adding-image", steps: func(_ *testing.T, tt *ui.Tester, v *View) { v.page.uploading = true; frames(tt, 2) }, wants: []string{"Adding image…"}},
		{name: "modes", wants: []string{"Edit", "Read", "Markdown"}},
		{name: "table-sorted", body: statesTable, steps: func(t *testing.T, tt *ui.Tester, _ *View) { click(t, tt, "Name") }, wants: []string{"Alpha", "Zeta"}},
		{name: "table-no-match", body: statesTable, steps: func(t *testing.T, tt *ui.Tester, _ *View) {
			click(t, tt, "Filter rows")
			tt.Type("nothing like it")
			frames(tt, 2)
		}, wants: []string{"No row has “nothing like it”"}, gone: []string{"Alpha"}},
		{name: "cards", body: statesCards, wants: []string{"First", "Fourth", "Fifth"}},
		{name: "image-loading", body: "![A picture](assets/p1/a.png)\n", options: func(o *Options) {
			o.Asset = func(string) ([]byte, error) { select {} }
		}, wants: []string{"Loading the image…"}},
		{name: "image-not-here", body: "![A picture](assets/p1/gone.png)\n", options: func(o *Options) {
			o.Asset = func(string) ([]byte, error) { return nil, errors.New("no such file") }
		}, steps: func(_ *testing.T, tt *ui.Tester, _ *View) {
			for range 50 {
				if strings.Contains(strings.Join(tt.Texts(), "\n"), "not on this host") {
					return
				}
				time.Sleep(5 * time.Millisecond)
				frames(tt, 1)
			}
		}, wants: []string{"not on this host"}},
		{name: "proposal-stale", options: func(o *Options) {
			o.Accept, o.Reject = func(string, func(proto.NoteInfo, error)) {}, func(string, func(error)) {}
			o.Proposals = []Proposal{{Agent: "Claude Code", AgentID: "claude", NoteProposal: proto.NoteProposal{ID: "x", Before: "An older text.\n",
				Note: proto.Note{NoteInfo: proto.NoteInfo{ID: "p1", Title: "Roadmap"}, Body: "Some text.\nAnother line it adds.\n"}}}}
		}, wants: []string{"Claude Code proposes changes to this page", "The page changed since: accepting replaces it with this."}},
		{name: "proposal-new", options: func(o *Options) {
			o.Accept, o.Reject = func(string, func(proto.NoteInfo, error)) {}, func(string, func(error)) {}
			o.Proposals = []Proposal{{Agent: "Claude Code", AgentID: "claude", For: "p1", NoteProposal: proto.NoteProposal{ID: "x",
				Note: proto.Note{NoteInfo: proto.NoteInfo{Title: "Launch checklist"}, Body: "- [ ] one\n- [ ] two\n"}}}}
		}, wants: []string{"Claude Code proposes a new page: Launch checklist"}, gone: []string{"The page changed since: accepting replaces it with this."}},
		{name: "picker-empty", options: withPrefs, steps: func(t *testing.T, tt *ui.Tester, _ *View) {
			click(t, tt, "Page icon", "Search icons: rocket, bug, calendar…")
			tt.Type("nothing at all like this")
			frames(tt, 2)
		}, wants: []string{"No icon for that: try a simpler word."}, gone: []string{"Favorites", "Recent"}},
		{name: "picker-prefs", options: withPrefs, note: func(n *proto.NoteInfo) { n.Icon = "lucide:rocket" },
			steps: func(t *testing.T, tt *ui.Tester, _ *View) { click(t, tt, "Page icon") },
			wants: []string{"Favorites", "Recent", "Remove", "Size", "Show more"}},
		{name: "picker-no-icon", options: withPrefs, steps: func(t *testing.T, tt *ui.Tester, _ *View) { click(t, tt, "Page icon") },
			gone: []string{"Remove", "Size"}},
		{name: "picker-last-library", options: func(o *Options) {
			withPrefs(o)
			o.IconPrefs = func(done func(proto.NoteIconPrefs, error)) {
				done(proto.NoteIconPrefs{Libraries: []string{"lucide"}}, nil)
			}
		}, steps: func(t *testing.T, tt *ui.Tester, v *View) {
			click(t, tt, "Page icon", "Library Lucide")
			if got := v.picker.libraries(); len(got) != 1 || got[0] != "lucide" {
				t.Errorf("the last library was turned off: %q", got)
			}
		}, wants: []string{"Lucide", "Tabler", "Phosphor"}},
		{name: "picker-custom-bad", options: withPrefs, steps: func(t *testing.T, tt *ui.Tester, v *View) {
			click(t, tt, "Page icon", "Custom")
			v.picker.svg = "not a drawing"
			frames(tt, 2)
		}, wants: []string{"Use this icon"}},
		{name: "picker-custom-file", options: withPrefs, steps: func(t *testing.T, tt *ui.Tester, _ *View) { click(t, tt, "Page icon", "Custom", "Choose a file…") },
			wants: []string{"couldn't read icon.svg: it isn't a file"}},
	}
	for _, st := range states {
		t.Run(st.name, func(t *testing.T) {
			np := newNotesPage(st.body)
			if st.body == "" {
				np = newNotesPage("Some text.")
			}
			if st.note != nil {
				n := np.notes["p1"]
				st.note(&n.NoteInfo)
				np.notes["p1"] = n
			}
			w := st.w
			if w == 0 {
				w = 1280
			}
			o := func() Options {
				o := np.options()
				if st.options != nil {
					st.options(&o)
				}
				return o
			}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(theme.Window())
				ui.Column(c).Width(float32(w)).Height(800).Background(theme.Frame).TextColor(theme.Text).Children(func() { np.v.Draw(c, o()) })
			}, w, 800)
			frames(tt, 2)
			if !st.noPageOpen {
				np.v.Select(o(), "p1")
				frames(tt, 3)
			}
			if st.view != nil {
				st.view(&np.v)
				frames(tt, 2)
			}
			if st.steps != nil {
				st.steps(t, tt, &np.v)
			}
			frames(tt, 12)
			shot(t, tt, "notes-"+st.name+".png")
			shows := strings.Join(tt.Texts(), "\n")
			for _, want := range st.wants {
				if !strings.Contains(shows, want) {
					t.Errorf("no %q in %q", want, tt.Texts())
				}
			}
			for _, not := range st.gone {
				if tt.HasText(not) {
					t.Errorf("%q shows: %q", not, tt.Texts())
				}
			}
			if out := wrapcheck.OutOfParents(tt); len(out) > 0 {
				t.Errorf("out of their parents:\n%s", strings.Join(out, "\n"))
			}
		})
	}
}

// A page dragged over a row shows where it would go by the pointer's place in
// the row: a line above, a tint inside, a line below; let go there, it moves.
func TestNotesRowDrag(t *testing.T) {
	np := newNotesPage("Some text.")
	np.notes["c1"] = proto.Note{NoteInfo: proto.NoteInfo{ID: "c1", Title: "Sync design"}}
	type move struct{ id, parent, before string }
	var moves []move
	o := func() Options {
		o := np.options()
		o.Move = func(id, parent, before string, done func(error)) {
			moves = append(moves, move{id, parent, before})
			done(nil)
		}
		return o
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(1280).Height(800).Background(theme.Frame).TextColor(theme.Text).Children(func() { np.v.Draw(c, o()) })
	}, 1280, 800)
	frames(tt, 2)
	from, ok := tt.Find("Sync design")
	to, ok2 := tt.Find("Roadmap")
	if !ok || !ok2 {
		t.Fatalf("no rows: %q", tt.Texts())
	}
	row := to.Y + to.H/2 - 18 // the row is 36 tall around its title
	for _, zone := range []struct {
		name string
		y    float32
	}{{"before", 4}, {"inside", 18}, {"after", 32}} {
		tt.Press(from.X+4, from.Y+from.H/2)
		tt.Move(from.X+4, from.Y+from.H/2-8)
		frames(tt, 1)
		tt.Move(to.X+8, row+zone.y)
		frames(tt, 3)
		shot(t, tt, "notes-drag-"+zone.name+".png")
		tt.Move(from.X+200, 700)
		frames(tt, 1)
		tt.Release(from.X+200, 700) // nowhere: nothing moves
		frames(tt, 3)
	}
	if len(moves) != 0 {
		t.Fatalf("let go nowhere, it moved: %v", moves)
	}
	tt.Press(from.X+4, from.Y+from.H/2)
	tt.Move(from.X+4, from.Y+from.H/2-8)
	frames(tt, 1)
	tt.Move(to.X+8, row+18)
	frames(tt, 3)
	tt.Release(to.X+8, row+18)
	frames(tt, 3)
	if len(moves) != 1 || moves[0].id != "c1" || moves[0].parent != "p1" {
		t.Fatalf("let go inside Roadmap: %v", moves)
	}
}

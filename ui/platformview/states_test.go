package platformview

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/wrapcheck"
)

var longName = strings.Repeat("a-credential-name-", 12)[:200]

func someKeys() []Key {
	return []Key{
		{ID: "1", Name: "anthropic", Status: "Works", Color: theme.Done, Default: true, LastUsed: time.Now().Add(-2 * time.Hour)},
		{ID: "2", Name: longName, Status: "Refused", Color: theme.Error},
		{ID: "3", Name: "testing", Status: "Not checked", Color: theme.Idle, Testing: true},
	}
}

func drawn(v *View, o *Options, w, h int) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Size(float32(w), float32(h)).Background(theme.Background).TextColor(theme.Text).Children(func() { v.Sheet(c, *o) })
	}, w, h)
}

// shot saves a picture of tt where SHOTS says.
func shot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_ = png.Encode(f, tt.Image())
}

func frames(tt *ui.Tester, n int) {
	for range n {
		tt.Frame()
	}
}

func has(tt *ui.Tester, part string) bool {
	for _, x := range tt.Texts() {
		if strings.Contains(x, part) {
			return true
		}
	}
	return false
}

// Every tab in every style fits, wide and narrow, with a name of 200 characters.
func TestSheetFits(t *testing.T) {
	oldStyle, palette := theme.CurrentStyle(), theme.Current()
	defer func() {
		theme.UseStyle(oldStyle)
		theme.Apply(palette)
	}()
	for _, s := range theme.Styles {
		theme.UseStyle(s)
		theme.Apply(palette)
		for _, size := range [][2]int{{1100, 800}, {420, 500}, {300, 400}} {
			for _, st := range []struct {
				name string
				tab  int
				o    Options
			}{
				{name: "keys", o: Options{VaultOpen: true, Keys: someKeys()}},
				{name: "none", o: Options{VaultOpen: true}},
				{name: "locked", o: Options{Gate: func() {}}},
				{name: "prompt", tab: TabPrompt},
			} {
				o := st.o
				o.Host = "a-host-of-a-rather-long-name.example.com"
				o.AddKey, o.Confirm = func() {}, func(string, string, string, func()) {}
				o.OpenURL = func(string) {}
				o.LoadPrompt = func(done func(proto.PlatformPrompt, error)) {
					done(proto.PlatformPrompt{Text: strings.Repeat("Be brief. ", 400), Custom: true, Path: "/home/somebody/" + longName + "/system-prompt.md"}, nil)
				}
				v := &View{}
				v.Open(st.tab)
				tt := drawn(v, &o, size[0], size[1])
				frames(tt, 4)
				if out := wrapcheck.OutOfParents(tt); len(out) > 0 {
					t.Errorf("%s, %v, %s:\n%s", s.ID, size, st.name, strings.Join(out, "\n"))
				}
				shot(t, tt, fmt.Sprintf("sheet-%s-%d-%s.png", s.ID, size[0], st.name))
				if out := wrapcheck.ScrollbarOverlaps(tt); len(out) > 0 {
					t.Errorf("%s, %v, %s, under a bar:\n%s", s.ID, size, st.name, strings.Join(out, "\n"))
				}
			}
		}
	}
}

// Keys show by name, never by value; removing asks first, saying what stops; testing
// says how it went; the empty vault says what is missing.
func TestKeysTab(t *testing.T) {
	var removed []string
	var asked, question string
	var yes func()
	o := Options{Host: "this Mac", VaultOpen: true, Keys: someKeys(),
		RemoveKey: func(id string) { removed = append(removed, id) },
		TestKey:   func(id string, done func(string)) { done("401 from Anthropic") },
		AddKey:    func() { asked = "add" },
		Confirm: func(title, msg, action string, do func()) {
			question, yes = title+" "+msg, do
		},
	}
	v := &View{}
	v.Open(TabKeys)
	tt := drawn(v, &o, 900, 700)
	frames(tt, 3)
	if err := tt.Click("Remove anthropic"); err != nil {
		t.Fatalf("%v %q", err, tt.Texts())
	}
	frames(tt, 2)
	if len(removed) != 0 {
		t.Fatal("removed without asking")
	}
	if !strings.Contains(question, "stop") || !strings.Contains(question, "anthropic") {
		t.Fatalf("the question: %s", question)
	}
	yes()
	if len(removed) != 1 || removed[0] != "1" {
		t.Fatalf("removed %q", removed)
	}
	if err := tt.Click("Test anthropic"); err != nil {
		t.Fatal(err)
	}
	frames(tt, 3)
	if !tt.HasText("401 from Anthropic") {
		t.Fatalf("no result: %q", tt.Texts())
	}
	if err := tt.Click("Add API key"); err != nil {
		t.Fatal(err)
	}
	frames(tt, 2)
	if asked != "add" {
		t.Fatal("Add didn't open the form")
	}
	o.Keys = nil
	frames(tt, 2)
	if !has(tt, "No Anthropic API key yet") {
		t.Fatalf("empty: %q", tt.Texts())
	}
}

// The prompt loads, is saved only when changed, resets after asking, and keeps what is
// typed through closing the sheet and changing tabs.
func TestPromptTab(t *testing.T) {
	saved := proto.PlatformPrompt{Path: "/h/system-prompt.md"}
	calls := 0
	var question string
	var yes func()
	o := Options{Host: "this Mac",
		LoadPrompt: func(done func(proto.PlatformPrompt, error)) { calls++; done(saved, nil) },
		SavePrompt: func(text string, done func(proto.PlatformPrompt, error)) {
			saved.Text, saved.Custom = text, true
			done(saved, nil)
		},
		ResetPrompt: func(done func(proto.PlatformPrompt, error)) {
			saved.Text, saved.Custom = "", false
			done(saved, nil)
		},
		Confirm: func(title, msg, action string, do func()) { question, yes = title, do },
	}
	v := &View{}
	v.Open(TabPrompt)
	tt := drawn(v, &o, 900, 700)
	frames(tt, 3)
	if !has(tt, "/h/system-prompt.md") {
		t.Fatalf("no path: %q", tt.Texts())
	}
	if _, ok := tt.Find("Reset to built-in"); ok {
		t.Fatal("reset offered with nothing to reset")
	}
	v.prompt.draft = "Be brief."
	frames(tt, 2)
	if !tt.HasText("Not saved yet.") {
		t.Fatalf("no sign of the change: %q", tt.Texts())
	}
	v.open = false
	frames(tt, 2)
	v.Open(TabPrompt)
	frames(tt, 2)
	if v.prompt.draft != "Be brief." || calls != 1 {
		t.Fatalf("closing lost the draft: %q, read %d times", v.prompt.draft, calls)
	}
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	frames(tt, 3)
	if saved.Text != "Be brief." || !has(tt, "Saved.") {
		t.Fatalf("saved %q: %q", saved.Text, tt.Texts())
	}
	if err := tt.Click("Reset to built-in"); err != nil {
		t.Fatal(err)
	}
	frames(tt, 2)
	if !saved.Custom || question == "" {
		t.Fatal("reset without asking")
	}
	yes()
	frames(tt, 2)
	if saved.Custom || v.prompt.draft != "" || !has(tt, "built-in") {
		t.Fatalf("reset: %+v, draft %q", saved, v.prompt.draft)
	}
	// A failed read says so and offers to try again.
	bad := &View{}
	bad.Open(TabPrompt)
	o.LoadPrompt = func(done func(proto.PlatformPrompt, error)) {
		done(proto.PlatformPrompt{}, errors.New("unknown method"))
	}
	bt := drawn(bad, &o, 900, 700)
	frames(bt, 3)
	if !has(bt, "unknown method") || !bt.HasText("Try again") {
		t.Fatalf("failed read: %q", bt.Texts())
	}
}

// The chat's first words name the agent, its model and the cost, a key's absence with
// the way to fix it, in a narrow chat too.
func TestWelcome(t *testing.T) {
	oldStyle, palette := theme.CurrentStyle(), theme.Current()
	defer func() {
		theme.UseStyle(oldStyle)
		theme.Apply(palette)
	}()
	for _, s := range theme.Styles {
		theme.UseStyle(s)
		theme.Apply(palette)
		for _, w := range []int{900, 300} {
			opened := 0
			wl := Welcome{Name: "Claude Platform", Model: "claude-opus-5-5-with-an-enormous-model-name-that-never-fits-20261105", Effort: "High", Budget: 10, Key: KeyMissing, OpenKeys: func() { opened++ }}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(theme.Window())
				ui.Column(c).Width(float32(w)).Height(700).Background(theme.Background).TextColor(theme.Text).Children(func() { wl.Draw(c) })
			}, w, 700)
			frames(tt, 3)
			for _, want := range []string{"Claude Platform", "$10 a session", "No Anthropic API key yet", "list prices"} {
				if !has(tt, want) {
					t.Errorf("%s %d: no %q in %q", s.ID, w, want, tt.Texts())
				}
			}
			if out := wrapcheck.OutOfParents(tt); len(out) > 0 {
				t.Errorf("%s %d:\n%s", s.ID, w, strings.Join(out, "\n"))
			}
			shot(t, tt, fmt.Sprintf("welcome-%s-%d.png", s.ID, w))
			if err := tt.Click("Add an API key"); err != nil {
				t.Fatal(err)
			}
			if frames(tt, 2); opened != 1 {
				t.Fatalf("opened %d", opened)
			}
		}
	}
	for text, want := range map[string]bool{"API error 401": true, "authentication_error: x": true, "Overloaded, retrying": false, "No API key: set one": true} {
		if KeyRefused(text) != want {
			t.Errorf("KeyRefused(%q) != %v", text, want)
		}
	}
}

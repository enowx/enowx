package ui

import (
	"encoding/json"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/notify"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// sounds records what plays instead of playing it.
type sounds struct {
	mu     sync.Mutex
	played [][]byte
}

func (s *sounds) take() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.played
	s.played = nil
	return out
}

func catchSounds(t *testing.T) *sounds {
	s := &sounds{}
	old := playSound
	playSound = func(data []byte, _ float64) error {
		s.mu.Lock()
		s.played = append(s.played, data)
		s.mu.Unlock()
		return nil
	}
	t.Cleanup(func() { playSound = old })
	return s
}

// Agents finishing, asking and failing play their sounds once each, a
// burst as one, none when muted, quiet or off; an agent's own ask plays
// too; and Settings' Notifications keep the user's choices.
func TestNotificationSounds(t *testing.T) {
	snd := catchSounds(t)
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	app := New(Options{})
	var mu sync.Mutex
	var later []func()
	app.onMain = func(fn func()) { mu.Lock(); later = append(later, fn); mu.Unlock() }
	pa, pb := net.Pipe()
	go srv.ServeConn(pa)
	cl, err := client.New(pb, func(ev proto.Event) { app.handleEvent(app.gen, ev) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	app.client, app.host = cl, hosts.Host{ID: hosts.LocalID, Name: "This Mac"}
	app.page, app.settingsSection = pageSettings, sectionNotify
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(900).Height(900).Background(theme.Background).Children(func() { app.settingsBody(c) })
	}, 900, 900)
	run := func(d time.Duration) {
		deadline := time.Now().Add(d)
		for time.Now().Before(deadline) {
			mu.Lock()
			fns := later
			later = nil
			mu.Unlock()
			for _, fn := range fns {
				fn()
			}
			tt.Frame()
			time.Sleep(10 * time.Millisecond)
		}
	}
	app.loadNotify()
	run(200 * time.Millisecond)
	if !app.ntf.loaded {
		t.Fatal("settings never read")
	}

	s := proto.SessionInfo{ID: "s1", Agent: "claude", State: proto.StateWorking}
	app.sessions = []proto.SessionInfo{s}
	change := func(info proto.SessionInfo) {
		old := s
		s = info
		app.soundFor(old, info, true, "this Mac")
	}
	working := s
	done := s
	done.State = proto.StateDone
	change(done)
	run(soundGather + 300*time.Millisecond)
	if got := snd.take(); len(got) != 1 || string(got[0]) != string(notify.Builtin("chime")) {
		t.Fatalf("a turn done played %d sounds", len(got))
	}

	// A burst: done, then asking, then failing, together: one sound, the
	// failure's.
	app.ntf.last = time.Time{}
	change(working)
	waiting := s
	waiting.State = proto.StateWaiting
	change(waiting)
	failed := waiting
	failed.Exited, failed.ExitCode = true, 1
	change(failed)
	run(soundGather + 300*time.Millisecond)
	if got := snd.take(); len(got) != 1 || string(got[0]) != string(notify.Builtin("alert")) {
		t.Fatalf("a burst played %d sounds", len(got))
	}

	// Muted, then quiet hours around now: nothing.
	app.muteSession("s1", true)
	run(200 * time.Millisecond)
	s = working
	change(done)
	run(soundGather + 300*time.Millisecond)
	if got := snd.take(); len(got) != 0 {
		t.Fatalf("a muted session played %d sounds", len(got))
	}
	app.muteSession("s1", false)
	st := app.notifySettings()
	now := time.Now()
	st.Quiet, st.QuietFrom, st.QuietTo = true, now.Add(-time.Hour).Format("15:04"), now.Add(time.Hour).Format("15:04")
	app.setNotify(st)
	run(200 * time.Millisecond)
	s = working
	change(done)
	run(soundGather + 300*time.Millisecond)
	if got := snd.take(); len(got) != 0 {
		t.Fatalf("quiet hours played %d sounds", len(got))
	}
	st.Quiet = false
	app.setNotify(st)
	run(200 * time.Millisecond)

	// An agent asks for a sound, from the command line: it plays.
	app.ntf.last = time.Time{}
	if err := cl.NotifySound(proto.NotifySoundParams{Kind: proto.NotifyInput}); err != nil {
		t.Fatal(err)
	}
	run(soundGather + 500*time.Millisecond)
	if got := snd.take(); len(got) != 1 || string(got[0]) != string(notify.Builtin("ping")) {
		t.Fatalf("an agent's ask played %d sounds", len(got))
	}

	// Settings: pick another sound for finished turns; it is kept.
	if !tt.HasText("Agent finished") {
		t.Fatalf("no section; shows %q", tt.Texts())
	}
	shot(t, tt, "notify-settings.png")
	if err := tt.Click("Bell"); err != nil {
		t.Fatal(err)
	}
	run(300 * time.Millisecond)
	kept, err := cl.NotifySettings()
	if err != nil || kept.Sounds[proto.NotifyDone] != "bell" {
		t.Fatalf("kept %+v, %v", kept.Sounds, err)
	}
	snd.take()
}

// An agent on a server the app watches finishing plays its sound once on
// this device, though the event comes twice.
func TestRemoteAgentDoneSoundsOnce(t *testing.T) {
	snd := catchSounds(t)
	app := New(Options{})
	var later []func()
	app.onMain = func(fn func()) { later = append(later, fn) }
	app.ntf.loaded, app.ntf.settings = true, notify.Defaults()
	w := &hostWatch{host: hosts.Host{ID: "vps", Name: "datalix-vps"}, stop: make(chan struct{})}
	w.status.sessions = []proto.SessionInfo{{ID: "r1", Agent: "claude", State: proto.StateWorking}}
	app.watches = map[string]*hostWatch{"vps": w}
	done, _ := json.Marshal(proto.SessionInfo{ID: "r1", Agent: "claude", State: proto.StateDone})
	for range 2 {
		app.watchEvent(w, proto.Event{Name: proto.EventSessionUpdated, Data: done})
	}
	deadline := time.Now().Add(soundGather + 500*time.Millisecond)
	for time.Now().Before(deadline) {
		fns := later
		later = nil
		for _, fn := range fns {
			fn()
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := snd.take(); len(got) != 1 {
		t.Fatalf("an agent done on a server played %d sounds", len(got))
	}
}

// A notification shows what it is about: its kind, session, host and a short
// message, several stacked, each leaving on its own or kept while hovered; a
// click opens the session, in a tab in GOD of Agents' mode.
func TestNotificationsShowWhatTheyAreAbout(t *testing.T) {
	catchSounds(t)
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a1, b1 := net.Pipe()
	go srv.ServeConn(a1)
	cl, err := client.New(b1, func(proto.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	app := New(Options{})
	posted := make(chan func(), 64)
	app.onMain = func(fn func()) { posted <- fn }
	app.client, app.host = cl, hosts.Host{ID: hosts.LocalID, Name: "this Mac"}
	worker, err := cl.CreateSession(proto.SessionCreateParams{Name: "Fix the totals", Command: []string{"cat"}, Dir: t.TempDir(), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	worker.Agent = "claude"
	app.sessions = []proto.SessionInfo{worker}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Fill().Background(theme.Background).TextColor(theme.Text).Children(func() { app.chatToastsView(c) })
	}, 900, 700)

	change := func(state string, exited bool, code int) {
		old, info := worker, worker
		old.State, info.State, info.Exited, info.ExitCode = proto.StateWorking, state, exited, code
		app.soundFor(old, info, true, "this Mac")
	}
	change(proto.StateDone, false, 0)
	tt.Frame()
	for _, want := range []string{"Done", "Fix the totals", "this Mac", "It finished its turn"} {
		if !tt.HasText(want) {
			t.Fatalf("no %q: %q", want, tt.Texts())
		}
	}
	change(proto.StateWaiting, false, 0)
	change(proto.StateDone, true, 2)
	app.toastAsked(proto.NotifySoundParams{Kind: proto.NotifyInput, Session: "gone", Text: "A worker needs you: git push"}, "enowxlabs")
	tt.Frame()
	for _, want := range []string{"Needs you", "Failed", "It ended with exit code 2", "enowxlabs", "A worker needs you: git push"} {
		if !tt.HasText(want) {
			t.Fatalf("stacked, no %q: %q", want, tt.Texts())
		}
	}
	if os.Getenv("SHOTS") != "" {
		for range 6 {
			tt.Advance(100 * time.Millisecond)
			tt.Frame()
		}
		shot(t, tt, "notification-cards.png")
	}
	// A newer one of the same kind replaces it; the most are kept, the
	// oldest going first.
	change(proto.StateDone, false, 0)
	if n := len(app.sessionToasts); n != 4 {
		t.Fatalf("%d cards", n)
	}
	for i := range 3 {
		app.toastSession(proto.NotifyDone, "this Mac", "s"+string(rune('a'+i)), "another", "")
	}
	if n := len(app.sessionToasts); n != sessionToastMax {
		t.Fatalf("%d cards, at most %d", n, sessionToastMax)
	}
	// Each leaves on its own, not before its time.
	for _, card := range app.sessionToasts {
		card.shown = time.Now().Add(-sessionToastLife - time.Second)
	}
	tt.Frame()
	if len(app.sessionToasts) != 0 {
		t.Fatalf("still shown: %d", len(app.sessionToasts))
	}

	// A click opens it: the session here, in a tab in GOA's mode.
	app.toastSession(proto.NotifyDone, "this Mac", worker.ID, "Fix the totals", "")
	app.openToast(app.sessionToasts[0])
	if app.selected != worker.ID || len(app.sessionToasts) != 0 {
		t.Fatalf("opened: selected %q, cards %d", app.selected, len(app.sessionToasts))
	}
	app.state.GOA, app.goa.host, app.goa.board.Session = true, hosts.LocalID, "goa"
	app.toastSession(proto.NotifyDone, "this Mac", worker.ID, "Fix the totals", "")
	app.openToast(app.sessionToasts[0])
	if len(app.goa.tabs) != 1 || app.goa.tabs[0].id != worker.ID || app.goaShownTab() == nil || !app.goaOn() {
		t.Fatalf("in GOA: tabs %+v shown %v", app.goa.tabs, app.goaShownTab())
	}
	for app.goa.tabs[0].term == nil && app.goa.tabs[0].err == "" {
		(<-posted)()
	}
	app.closeGOATabs()
}

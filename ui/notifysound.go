package ui

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	_ "github.com/egoist/mygo/audio/mp3" // the user's MP3 sounds

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/notify"
	"github.com/enowx/enowx/proto"
)

// Sounds the app plays when an agent finishes its turn, asks for input or
// fails, on the host shown or one watched, and when an agent asks for
// one, each with a card saying what it is about (sessiontoast.go): by the
// user's settings, kept with their synced data (core/notify)
// on this device's daemon. Sounds close together play as one, the most
// urgent of them.

const (
	soundGather = 600 * time.Millisecond // sounds within it play as one
	soundGap    = 1500 * time.Millisecond
)

// notifyState is the sounds' state, the main thread's.
type notifyState struct {
	settings proto.NotifySettings
	loaded   bool
	pending  string // the kind waiting to play, "" for none
	timer    *time.Timer
	last     time.Time
	custom   map[string][]byte // the user's sounds read, by name
	sets     int               // settings written, so an older answer doesn't undo a newer one
	writing  int               // writes on their way to the daemon
	writes   chan func()       // the writes, in order
}

// playSound plays a sound file at volume (0 to 1); tests replace it.
var playSound = func(data []byte, volume float64) error {
	s, err := audio.DecodeSound(data)
	if err != nil {
		return err
	}
	s.NewPlayer().SetVolume(volume).Play()
	return nil
}

// notifySettings are the settings in use: the defaults until read.
func (a *App) notifySettings() proto.NotifySettings {
	if !a.ntf.loaded {
		return notify.Defaults()
	}
	return a.ntf.settings
}

// loadNotify reads the settings from this device's daemon, else the
// host's shown.
func (a *App) loadNotify() {
	cl := a.hubClient()
	if cl == nil {
		cl = a.client
	}
	if cl == nil {
		return
	}
	sets := a.ntf.sets
	go func() {
		st, err := cl.NotifySettings()
		if err != nil {
			return // a daemon of before: the defaults
		}
		a.onMain(func() {
			if sets == a.ntf.sets {
				a.ntf.settings, a.ntf.loaded = st, true
			}
		})
	}()
}

// setNotify keeps st as the settings, here and on this device's daemon,
// which syncs them to the other hosts.
func (a *App) setNotify(st proto.NotifySettings) {
	st = notify.Normal(st)
	a.ntf.settings, a.ntf.loaded = st, true
	a.ntf.sets++
	set := a.ntf.sets
	cl := a.hubClient()
	if cl == nil {
		cl = a.client
	}
	if cl == nil {
		return
	}
	if a.ntf.writes == nil {
		// One writer, so the daemon gets the settings in the order they
		// were made.
		a.ntf.writes = make(chan func(), 16)
		go func(q chan func()) {
			for fn := range q {
				fn()
			}
		}(a.ntf.writes)
	}
	a.ntf.writing++
	a.ntf.writes <- func() {
		out, err := cl.SetNotifySettings(st)
		a.onMain(func() {
			a.ntf.writing--
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			if set == a.ntf.sets {
				a.ntf.settings = out
			}
			a.syncSoon()
		})
	}
}

// notifyEvent takes a sound asked for, or new settings, from any host's
// connection; it runs on the client's goroutine. host names where it came
// from.
func (a *App) notifyEvent(ev proto.Event, host string) {
	switch ev.Name {
	case proto.EventNotifySettings:
		var st proto.NotifySettings
		if json.Unmarshal(ev.Data, &st) == nil {
			a.onMain(func() {
				if a.ntf.writing == 0 {
					a.ntf.settings, a.ntf.loaded = st, true
				}
			})
		}
	case proto.EventNotifySound:
		var p proto.NotifySoundParams
		if json.Unmarshal(ev.Data, &p) != nil {
			return
		}
		a.onMain(func() {
			st := a.notifySettings()
			if p.Session != "" && notify.Muted(st, "", p.Session) {
				return
			}
			a.queueSound(p.Kind)
			if p.Kind != proto.NotifyChat {
				a.toastAsked(p, host)
			}
			if p.Text != "" && !st.NoBanner {
				a.banner("enowX", host, p.Text)
			}
		})
	}
}

// soundFor plays the sound of the change from old to info, a session of
// the host shown (shown) or of one watched: its agent finished its turn,
// asks for input, or ended in failure.
func (a *App) soundFor(old, info proto.SessionInfo, shown bool, host string) {
	kind := soundKind(old, info)
	if kind == "" {
		return
	}
	st := a.notifySettings()
	if notify.Muted(st, info.Agent, info.ID) || shown && !st.Watched && a.watching(info) {
		return
	}
	a.queueSound(kind)
	a.toastChange(kind, host, info)
	a.updateBadge()
}

// soundKind is the kind of sound the change from old to info calls for,
// "" for none.
func soundKind(old, info proto.SessionInfo) string {
	switch {
	case info.Agent == "" || info.Agent == "shell":
		return ""
	case info.Exited && !old.Exited && info.ExitCode != 0:
		return proto.NotifyError
	case info.Exited || old.State == info.State:
		return ""
	case info.State == proto.StateWaiting:
		return proto.NotifyInput
	case info.State == proto.StateDone:
		return proto.NotifyDone
	}
	return ""
}

// queueSound plays a sound of kind soon, with those that come meanwhile:
// once, the most urgent's, not sooner than soundGap after the last.
func (a *App) queueSound(kind string) {
	if notify.Quiet(a.notifySettings(), time.Now()) {
		return
	}
	if urgency(kind) > urgency(a.ntf.pending) {
		a.ntf.pending = kind
	}
	if a.ntf.timer != nil {
		return
	}
	wait := soundGather
	if gap := soundGap - time.Since(a.ntf.last); gap > wait {
		wait = gap
	}
	a.ntf.timer = time.AfterFunc(wait, func() {
		a.onMain(func() {
			kind := a.ntf.pending
			a.ntf.pending, a.ntf.timer = "", nil
			if kind != "" {
				a.ntf.last = time.Now()
				a.playKind(kind)
			}
		})
	})
}

func urgency(kind string) int {
	for i, k := range notify.Kinds {
		if k == kind {
			return i + 1
		}
	}
	return 0
}

// playKind plays the sound the settings give kind.
func (a *App) playKind(kind string) {
	st := a.notifySettings()
	a.playNamed(st.Sounds[kind], st.Volume)
}

// playNamed plays sound name at volume: a built-in's at once, the user's
// once read from this device's daemon.
func (a *App) playNamed(name string, volume float64) {
	if name == "" || name == "none" || volume <= 0 {
		return
	}
	if data := notify.Builtin(name); data != nil {
		go func() { _ = playSound(data, volume) }()
		return
	}
	if !strings.HasPrefix(name, "custom:") {
		return
	}
	if data, ok := a.ntf.custom[name]; ok {
		go func() { _ = playSound(data, volume) }()
		return
	}
	cl := a.hubClient()
	if cl == nil {
		cl = a.client
	}
	if cl == nil {
		return
	}
	go func() {
		data, err := cl.NotifySoundData(name)
		if err != nil {
			return
		}
		a.onMain(func() {
			if a.ntf.custom == nil {
				a.ntf.custom = map[string][]byte{}
			}
			a.ntf.custom[name] = data
		})
		_ = playSound(data, volume)
	}()
}

// updateBadge counts the sessions waiting for the user on the app's icon,
// when the settings ask for it.
func (a *App) updateBadge() {
	if a.win == nil || !a.ntf.loaded {
		return
	}
	n := 0
	if a.ntf.settings.Badge {
		n = len(a.unread)
		for _, w := range a.watches {
			_, waiting := w.status.running()
			n += waiting
		}
		n += a.allLinkConflicts() + a.dsync.hub.Conflicts
	}
	mygo.App.SetBadgeCount(n)
}

// banner shows a desktop notification, unless notifications are off.
func (a *App) banner(title, subtitle, body string) {
	if a.state.NotifyOff || a.win == nil {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{Title: title, Subtitle: subtitle, Body: body, Silent: true})
	go func() { _ = n.Show() }()
}

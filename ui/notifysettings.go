package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/notify"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// notifySection is Settings' Notifications: the sounds the app plays when
// agents finish, ask for input or fail, wherever they run.
func (a *App) notifySection(c *ui.Context) {
	settingsColumn(c, "Notifications", "", nil, func() { a.notifyBody(c) })
}

func (a *App) notifyBody(c *ui.Context) {
	sectionIntro(c, "Notifications", "A sound when an agent finishes its turn, asks for input or fails, in a session on this device or on any host enowX reaches. These settings are the same on every host.")
	st := a.notifySettings()
	changed := false
	on := !st.Off
	if switchRow(c, &on, "Sounds", "Off silences every sound.") {
		st.Off, changed = !on, true
	}
	widget.Field(c, "Volume", func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.VolumeDown).FontSize(13).TextColor(theme.Muted)
			v := st.Volume
			s := ui.Slider(c, &v, 0, 1).Grow(1).Label("Volume")
			if s.Changed() {
				st.Volume, changed = v, true
			}
			ui.Icon(c, icon.VolumeUp).FontSize(13).TextColor(theme.Muted)
			ui.Text(c, fmt.Sprintf("%.0f%%", v*100)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Width(40)
		})
	})
	for _, k := range []string{proto.NotifyDone, proto.NotifyInput, proto.NotifyError, proto.NotifyChat} { // agents first
		label := map[string]string{proto.NotifyChat: "Team chat message", proto.NotifyDone: "Agent finished", proto.NotifyInput: "Agent needs input", proto.NotifyError: "Agent failed"}[k]
		widget.Field(c.Key(k), label, func() {
			ui.Row(c).FillWidth().Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
				for _, name := range a.soundChoices(st) {
					picked := st.Sounds[k] == name
					b := widget.Button(c.Key(name), widget.Secondary, nil, soundLabel(name), "").Height(28).PaddingX(10)
					if picked {
						b.Border(1, theme.AccentLine).Background(theme.Selected)
					}
					if b.Clicked() {
						st.Sounds[k], changed = name, true
						a.playNamed(name, st.Volume)
					}
				}
				if widget.IconButton(c, icon.Play, "Test the sound of "+strings.ToLower(label), false).Clicked() {
					a.playNamed(st.Sounds[k], max(st.Volume, 0.3))
				}
			})
		})
	}
	ui.Row(c).FillWidth().Children(func() {
		if widget.GhostButton(c, "Add a sound of your own…").Tooltip("A WAV or an MP3, kept with your settings on every host").Clicked() {
			a.addSound()
		}
	})
	quiet := st.Quiet
	if switchRow(c, &quiet, "Quiet hours", "No sounds between these times, every day.") {
		st.Quiet, changed = quiet, true
	}
	if st.Quiet {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "From").FontSize(theme.TextLabel).TextColor(theme.Muted)
			from, to := st.QuietFrom, st.QuietTo
			if widget.Input(c.Key("from"), &from, "22:00").Width(80).Changed() {
				st.QuietFrom, changed = from, true
			}
			ui.Text(c, "to").FontSize(theme.TextLabel).TextColor(theme.Muted)
			if widget.Input(c.Key("to"), &to, "07:00").Width(80).Changed() {
				st.QuietTo, changed = to, true
			}
		})
		ui.Text(c, "The system's Focus and Do Not Disturb silence enowX's banners; its sounds follow these hours.").FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
	watched := st.Watched
	if switchRow(c, &watched, "Also for the session I'm looking at", "Off: no sound for a session shown in a focused window.") {
		st.Watched, changed = watched, true
	}
	banner := !st.NoBanner
	if switchRow(c, &banner, "Desktop notifications", "A banner with the sound, which a click opens.") {
		st.NoBanner, changed = !banner, true
	}
	badge := st.Badge
	if switchRow(c, &badge, "Count on the app's icon", "The sessions waiting for you, on the Dock or taskbar icon.") {
		st.Badge, changed = badge, true
	}
	widget.Field(c, "Team chat", func() {
		modes := []string{"", "mentions", "off"}
		cur := max(0, slices.Index(modes, st.Chat))
		if i := widget.Segmented(c, []string{"Every message", "Mentions of me", "Off"}, cur, nil); i >= 0 && i != cur {
			st.Chat, changed = modes[i], true
		}
		ui.Text(c, "A message from your team, when its chat isn't open: a sound, a card in the window's corner to answer from, and a banner when the window is in the background. Mentions of you and replies to you always notify.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		for _, sp := range a.spacesOf().list {
			if sp.Team == "" {
				continue
			}
			team := sp.Team
			ui.Row(c.Key("chat:" + team)).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.spaceName(team)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Grow(1).SingleLine()
				modes := []string{"default", "", "mentions", "off"}
				cur := 0
				if m, ok := st.ChatTeams[team]; ok {
					cur = max(1, slices.Index(modes, m))
				}
				if i := widget.Segmented(c, []string{"As above", "All", "Mentions", "Off"}, cur, nil); i >= 0 && i != cur {
					teams := map[string]string{}
					for k, v := range st.ChatTeams {
						teams[k] = v
					}
					if i == 0 {
						delete(teams, team)
					} else {
						teams[team] = modes[i]
					}
					st.ChatTeams, changed = teams, true
				}
			})
		}
	})
	widget.Field(c, "Agents", func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			for _, ag := range a.agents {
				if !ag.Installed || ag.Command == "" {
					continue
				}
				on := !slices.Contains(st.MutedAgents, ag.ID)
				if switchRow(c.Key(ag.ID), &on, ag.Name, "") {
					st.MutedAgents = without(st.MutedAgents, ag.ID)
					if !on {
						st.MutedAgents = append(st.MutedAgents, ag.ID)
					}
					changed = true
				}
			}
		})
	})
	if len(st.MutedSessions) > 0 {
		widget.Field(c, "Sessions muted", func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				for _, id := range st.MutedSessions {
					ui.Row(c.Key(id)).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						name := id
						if s, ok := a.sessionByID(id); ok {
							name = a.sessionName(s)
						}
						ui.Text(c, name).FontSize(theme.TextUI).TextColor(theme.Text).Grow(1)
						if widget.GhostButton(c, "Unmute").Clicked() {
							st.MutedSessions, changed = without(st.MutedSessions, id), true
						}
					})
				}
			})
		})
	}
	if changed {
		if notify.Check(st) == nil {
			a.setNotify(st)
		} else {
			a.ntf.settings = st // a time being typed: kept once whole
		}
	}
}

// soundChoices are the sounds offered: the built-ins, the user's own the
// settings name, and none.
func (a *App) soundChoices(st proto.NotifySettings) []string {
	out := notify.Builtins()
	for _, k := range notify.Kinds {
		if n := st.Sounds[k]; strings.HasPrefix(n, "custom:") && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return append(out, "none")
}

func soundLabel(name string) string {
	if file, ok := strings.CutPrefix(name, "custom:"); ok {
		return strings.TrimSuffix(strings.TrimSuffix(file, ".wav"), ".mp3")
	}
	if name == "none" {
		return "None"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// addSound asks for a sound file of this device, keeps it with the
// settings, and makes it the sound of finished turns.
func (a *App) addSound() {
	cl := a.hubClient()
	if cl == nil {
		cl = a.client
	}
	if cl == nil || a.win == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "A sound for notifications",
			Filters: []mygo.FileFilter{{Name: "Sound", Extensions: []string{"wav", "mp3"}}}})
		if err != nil || len(paths) == 0 {
			return
		}
		data, err := os.ReadFile(paths[0])
		name := strings.ReplaceAll(filepath.Base(paths[0]), "/", "")
		if err == nil {
			err = cl.SaveNotifySound(name, data)
		}
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			st := a.notifySettings()
			st.Sounds[proto.NotifyDone] = "custom:" + name
			a.setNotify(st)
		})
	}()
}

// muteSession silences session id's sounds, or lets them play again.
func (a *App) muteSession(id string, mute bool) {
	st := a.notifySettings()
	st.MutedSessions = without(st.MutedSessions, id)
	if mute {
		st.MutedSessions = append(st.MutedSessions, id)
	}
	a.setNotify(st)
}

// without is list without s.
func without(list []string, s string) []string {
	var out []string
	for _, v := range list {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}

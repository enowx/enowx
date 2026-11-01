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
	st := a.notifySettings()
	changed := false
	settingsColumn(c, "Notifications", "The same on every host", nil, func() {
		widget.Cols(c, 3).Children(func() {
			widget.Card(c.Key("sounds")).Children(func() {
				widget.CardTitle(c, nil, "Sounds", nil)
				on := !st.Off
				if switchRow(c, &on, "A sound for notifications", "When an agent finishes its turn, asks for input or fails, on any host enowX reaches. Off silences every sound.") {
					st.Off, changed = !on, true
				}
				widget.Field(c, "Volume", func() {
					ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
						ui.Icon(c, icon.VolumeDown).FontSize(14).TextColor(theme.TextSoft)
						v := st.Volume
						if ui.Slider(c, &v, 0, 1).Grow(1).MinWidth(0).Label("Volume").Changed() {
							st.Volume, changed = v, true
						}
						ui.Icon(c, icon.VolumeUp).FontSize(14).TextColor(theme.TextSoft)
						ui.Text(c, fmt.Sprintf("%.0f%%", v*100)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
					})
				})
				watched := st.Watched
				if switchRow(c, &watched, "Also for the session I'm looking at", "Off: no sound for a session shown in a focused window.") {
					st.Watched, changed = watched, true
				}
			})
			widget.Card(c.Key("quiet")).Children(func() {
				widget.CardTitle(c, nil, "Quiet hours", nil)
				quiet := st.Quiet
				if switchRow(c, &quiet, "No sounds between these times, every day.", "") {
					st.Quiet, changed = quiet, true
				}
				if st.Quiet {
					from, to := st.QuietFrom, st.QuietTo
					widget.Cols(c, 2).Children(func() {
						widget.Field(c, "From", func() {
							if widget.Input(c.Key("from"), &from, "22:00").FillWidth().Changed() {
								st.QuietFrom, changed = from, true
							}
						})
						widget.Field(c, "To", func() {
							if widget.Input(c.Key("to"), &to, "07:00").FillWidth().Changed() {
								st.QuietTo, changed = to, true
							}
						})
					})
					widget.CardNote(c, "The system's Focus and Do Not Disturb silence enowX's banners; its sounds follow these hours.", 0)
				}
			})
			widget.Card(c.Key("device")).Children(func() {
				widget.CardTitle(c, nil, "On this device", nil)
				banner := !st.NoBanner
				if switchRow(c, &banner, "Desktop notifications", "A banner with the sound, which a click opens.") {
					st.NoBanner, changed = !banner, true
				}
				badge := st.Badge
				if switchRow(c, &badge, "Count on the app's icon", "The sessions waiting for you, on the Dock or taskbar icon.") {
					st.Badge, changed = badge, true
				}
			})
		})
		ui.Column(c.Key("kinds")).FillWidth().Gap(theme.S2).Children(func() {
			ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Heading(c, "Each kind") })
				if widget.GhostButton(c, "Add a sound of your own…").Tooltip("A WAV or an MP3, kept with your settings on every host").Clicked() {
					a.addSound()
				}
			})
			widget.Cols(c, 4).Children(func() {
				for _, k := range []string{proto.NotifyDone, proto.NotifyInput, proto.NotifyError, proto.NotifyChat} { // agents first
					label := map[string]string{proto.NotifyChat: "Team chat message", proto.NotifyDone: "Agent finished", proto.NotifyInput: "Agent needs input", proto.NotifyError: "Agent failed"}[k]
					widget.Card(c.Key(k)).Children(func() {
						widget.CardTitle(c, nil, label, nil)
						ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
							ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
								names := map[string]string{}
								choices := a.soundChoices(st)
								for _, n := range choices {
									names[n] = soundLabel(n)
								}
								picked := st.Sounds[k]
								hookSelect(c, "Sound of "+strings.ToLower(label), &picked, choices, names)
								if picked != st.Sounds[k] {
									st.Sounds[k], changed = picked, true
									a.playNamed(picked, st.Volume)
								}
							})
							if widget.IconButton(c, icon.Play, "Test the sound of "+strings.ToLower(label), false).Clicked() {
								a.playNamed(st.Sounds[k], max(st.Volume, 0.3))
							}
						})
					})
				}
			})
		})
		widget.Cols(c, 3).Children(func() {
			widget.Card(c.Key("team-chat")).Children(func() {
				widget.CardTitle(c, nil, "Team chat", nil)
				modes := []string{"", "mentions", "off"}
				cur := max(0, slices.Index(modes, st.Chat))
				if i := widget.Segmented(c, []string{"Every message", "Mentions", "Off"}, cur, nil); i >= 0 && i != cur {
					st.Chat, changed = modes[i], true
				}
				widget.CardNote(c, "A message from your team, when its chat isn't open: a sound, a card in the window's corner to answer from, and a banner when the window is in the background. Mentions of you and replies to you always notify.", 0)
				for _, sp := range a.spacesOf().list {
					if sp.Team == "" {
						continue
					}
					team := sp.Team
					ui.Row(c.Key("chat:" + team)).FillWidth().Gap(theme.S2).Children(func() {
						ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, a.spaceName(team), widget.RunStyle{}) })
						ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
							modes := []string{"default", "", "mentions", "off"}
							names := map[string]string{"default": "As above", "": "All", "mentions": "Mentions", "off": "Off"}
							cur := "default"
							if m, ok := st.ChatTeams[team]; ok {
								cur = m
							}
							picked := cur
							hookSelect(c, "Chat of "+team, &picked, modes, names)
							if picked != cur {
								teams := map[string]string{}
								for k, v := range st.ChatTeams {
									teams[k] = v
								}
								if picked == "default" {
									delete(teams, team)
								} else {
									teams[team] = picked
								}
								st.ChatTeams, changed = teams, true
							}
						})
					})
				}
			})
			widget.Card(c.Key("agents")).Children(func() {
				widget.CardTitle(c, nil, "Agents", nil)
				widget.CardNote(c, "The agents muted make no sound.", 0)
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
			widget.Card(c.Key("muted")).Children(func() {
				widget.CardTitle(c, nil, "Sessions muted", nil)
				if len(st.MutedSessions) == 0 {
					widget.CardNote(c, "No session is muted.", 0)
				}
				for _, id := range st.MutedSessions {
					ui.Row(c.Key(id)).FillWidth().Gap(theme.S2).Children(func() {
						name := id
						if s, ok := a.sessionByID(id); ok {
							name = a.sessionName(s)
						}
						ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, name, widget.RunStyle{}) })
						if widget.GhostButton(c, "Unmute").Clicked() {
							st.MutedSessions, changed = without(st.MutedSessions, id), true
						}
					})
				}
			})
		})
	})
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

package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/cli"
	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
	"github.com/enowx/enowx/version"
)

// Settings' sections beside the agents' library: what new sessions start
// with, the variables every session on a host gets, the app's own
// preferences, and what a host's daemon says about itself.

const (
	sectionLibrary     = "Agents' library"
	sectionSessions    = "New sessions"
	sectionEnvironment = "Environment"
	sectionTerminal    = "Terminal"
	sectionAutomations = "Automations"
	sectionHooks       = "Hooks"
	sectionAgents      = "Agents" // their configs: accounts and model APIs (agents.go)
	sectionApp         = "App"
	sectionFloating    = "Floating control"
	sectionDiagnostics = "Diagnostics"
	sectionSync        = "Sync"
	sectionFeatures    = "Features"
	sectionNotify      = "Notifications"
	sectionDiscord     = "Discord"
)

// hostSettings is what the sections ask the connected host, read when
// they show.
type hostSettings struct {
	host     string // the host ID it is of
	vars     []proto.SessionEnvVar
	loaded   bool
	maxAgent int
	sleep    int  // HibernateAfter, in minutes
	neverHib bool // new sessions don't hibernate on their own (every host)
	log      proto.DaemonLog
	err      string
	newName  string
	newValue string
}

// hostSettingsOf is the sections' state for the connected host, read anew
// when the host changed.
func (a *App) hostSettingsOf() *hostSettings {
	if a.hostSet == nil || a.hostSet.host != a.host.ID {
		a.hostSet = &hostSettings{host: a.host.ID}
		a.loadHostSettings()
	}
	return a.hostSet
}

func (a *App) loadHostSettings() {
	hs, cl := a.hostSet, a.client
	if cl == nil {
		return
	}
	go func() {
		vars, err := cl.SessionEnv()
		st, _ := cl.Settings()
		log, _ := cl.DaemonLog(80)
		prefs, _ := cl.HibernatePrefs()
		a.onMain(func() {
			hs.neverHib = prefs.NeverByDefault
			hs.loaded = true
			if err != nil {
				hs.err = client.Explain(err)
			}
			hs.vars, hs.maxAgent, hs.sleep, hs.log = vars, st.MaxAgents, st.HibernateAfter, log
		})
	}()
}

// settingsBody is the section shown. Each heads the frame itself; the ones
// made of a column of cards share settingsSection's frame.
func (a *App) settingsBody(c *ui.Context) {
	switch a.settingsSection {
	case "", sectionLibrary:
		a.library.Draw(c, a.libraryOptions())
	case sectionSessions:
		a.sessionDefaults(c)
	case sectionEnvironment:
		a.environmentSection(c)
	case sectionTerminal:
		a.terminalSection(c)
	case sectionAutomations:
		a.automationsSection(c)
	case sectionHooks:
		a.hooksSection(c)
	case sectionRAG:
		a.ragSection(c)
	case sectionDecide:
		a.decideSection(c)
	case sectionAgents:
		a.agentsSection(c)
	case sectionApp:
		a.appSection(c)
	case sectionFloating:
		a.floatingSection(c)
	case sectionDiagnostics:
		a.diagnosticsSection(c)
	case sectionNotify:
		a.notifySection(c)
	case sectionDiscord:
		a.discordSection(c)
	case sectionSync:
		a.syncSection(c)
	case sectionFeatures:
		a.featuresSection(c)
	case sectionCloud:
		a.cloudSection(c)
	}
}

// settingsColumn heads the frame and scrolls what's under it, at the frame's
// whole width.
func settingsColumn(c *ui.Context, title, sub string, actions, body func()) {
	widget.PageHeader(c, title, sub, actions)
	widget.PageBody(c, true, body)
}

// sessionDefaults are what New session starts with.
func (a *App) sessionDefaults(c *ui.Context) {
	settingsColumn(c, "New sessions", "What New session starts with", nil, func() {
		hs := a.hostSettingsOf()
		ui.Row(c.Key("session-defaults")).FillWidth().Gap(theme.S3).AlignItems(ui.Stretch).Children(func() {
			// As tall as the cards beside it, as last laid out: many agents scroll
			// in it rather than stretch the cards.
			cardsH := ui.Local[float32](c.Root(), "cards-height", func() float32 { return 0 })
			ui.Column(c.Key("agents")).Grow(1).Basis(0).MinWidth(0).Gap(theme.S2).Children(func() {
				widget.Heading(c, "Agent")
				list := ui.Scroll(c.Key("agent-list")).FillWidth()
				if *cardsH > 0 {
					list.MaxHeight(max(*cardsH-32, 60))
				}
				list.Children(func() {
					ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
						for _, ag := range a.agents {
							if ag.ID == "custom" || ag.Command != "" && !ag.Installed {
								continue
							}
							if widget.Pick(c, icon.Agent(ag.ID), ag.Name, "", a.defaultAgent() == ag.ID, nil).Clicked() {
								a.state.DefaultAgent = ag.ID
							}
						}
					})
				})
			})
			// Two thirds of the frame beside the agents' third, the gap between counted in.
			cards := ui.Grid(c.Key("cards")).Grow(2).Basis(theme.S3).MinWidth(0).Columns(2).Gap(theme.S3).AlignSelf(ui.Start)
			*cardsH = cards.Bounds().H
			cards.Children(func() {
				widget.Card(c).Children(func() {
					widget.CardTitle(c, nil, "In a git project", nil)
					if widget.OptionCard(c, !a.state.ProjectFolder, "A new branch and worktree", "Its own branch and folder, so sessions don't touch each other's files.") {
						a.state.ProjectFolder = false
					}
					if widget.OptionCard(c, a.state.ProjectFolder, "The project folder", "It works in the folder as it is, on the branch checked out.") {
						a.state.ProjectFolder = true
					}
				})
				widget.Card(c).Children(func() {
					widget.CardTitle(c, nil, "Agents skip permission prompts", func() {
						if widget.Switch(c, &a.skipPermissions, "Agents skip permission prompts") {
							a.saveState()
						}
					})
					widget.CardNote(c, "They run tools without asking. It also covers GOD of Agents, its workers and the chats of notes and canvases for agents that can't tell enowX they ask, like OpenCode. Claude Code and Codex can switch it in the chat; others keep theirs until they restart.", 0)
				})
				widget.Card(c).Children(func() {
					widget.CardTitle(c, nil, "Time budget for agents", nil)
					labels := []string{"None"}
					for _, m := range budgetChoices[1:] {
						labels = append(labels, workTime(int64(m)*60))
					}
					cur := slices.Index(budgetChoices, a.state.DefaultBudget)
					if i := segmentedInCard(c, labels, max(cur, 0), 250); i >= 0 {
						a.state.DefaultBudget = budgetChoices[i]
					}
				})
				widget.Card(c).Children(func() {
					widget.CardTitle(c, nil, "Agents at once on "+a.host.Name, nil)
					ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
						n := "No limit"
						if hs.maxAgent > 0 {
							n = strconv.Itoa(hs.maxAgent)
						}
						ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, n, widget.RunStyle{Size: 24, Weight: 600}) })
						if widget.GhostButton(c, "Change…").Clicked() {
							a.askAgentLimit()
						}
					})
					note := "All start at once."
					if hs.maxAgent > 0 {
						note = "The rest wait in a queue."
					}
					widget.CardNote(c, note, 0)
				})
			})
		})
		widget.Cols(c, 2).Children(func() {
			widget.Card(c).Children(func() {
				widget.CardTitle(c, nil, "Hibernate idle agents on "+a.host.Name, nil)
				widget.CardNote(c, "An agent done with its turn and left idle this long is stopped to free its memory; its session stays, and it comes back on its conversation when shown. Claude Code, Codex and other agents enowX can continue.", 0)
				labels := make([]string, len(sleepChoices))
				for i, m := range sleepChoices {
					labels[i] = "Never"
					if m > 0 {
						labels[i] = workTime(int64(m) * 60)
					}
				}
				cur := max(slices.Index(sleepChoices, hs.sleep), 0)
				if i := segmentedInCard(c, labels, cur, 250); i >= 0 && sleepChoices[i] != hs.sleep {
					a.setHibernation(sleepChoices[i])
				}
				if switchRow(c, &hs.neverHib, "New sessions don't hibernate on their own",
					"For sessions to keep running however long they're idle; the same on every host. A session's menu sets it for that session, and Hibernate stops one by hand.") {
					never, cl := hs.neverHib, a.client
					if cl != nil {
						go func() {
							if err := cl.SetAutoHibernate(proto.SessionAutoHibernateParams{Default: true, Never: never}); err != nil {
								a.onMain(func() { a.status = client.Explain(err) })
							}
						}()
					}
				}
			})
			ui.Column(c.Key("after")).MinWidth(0).Gap(theme.S3).Children(func() {
				widget.Card(c).Grow(1).Children(func() {
					widget.CardTitle(c, nil, "What New session starts with", nil)
					ui.Text(c, a.sessionDefaultsLine()).FontSize(theme.TextRead).FillWidth().MinWidth(0)
					widget.CardNote(c, "Each session is still free to differ in the New session dialog.", 0)
				})
				widget.Card(c).Children(func() {
					widget.CardTitle(c, nil, "Stalled tasks on "+a.host.Name, func() {
						// Labeled apart: the card of agents at once says Change… too.
						if widget.GhostButton(c.Key("goa-stall-limits"), "Change…").Label("Change the stall limits").Clicked() {
							a.openGOAStall()
						}
					})
					widget.CardNote(c, "How long a task of GOD of Agents may not move before it is told", 0)
				})
			})
		})
	})
}

// segmentedInCard is a Segmented whose choices share a narrow card's width, and
// keep to their own in a wide one.
func segmentedInCard(c *ui.Context, labels []string, current int, width float32) int {
	picked := -1
	ui.Row(c.Key("segmented")).FillWidth().MaxWidth(width).Shrink(0).Children(func() {
		picked = widget.SegmentedFit(c, labels, current)
	})
	return picked
}

// sessionDefaultsLine says in words what New session starts with.
func (a *App) sessionDefaultsLine() string {
	name := a.defaultAgent()
	for _, ag := range a.agents {
		if ag.ID == name {
			name = ag.Name
		}
	}
	where := "on a new branch, in a worktree"
	if a.state.ProjectFolder {
		where = "in the project folder"
	}
	tools := "asking before it runs tools"
	if a.skipPermissions {
		tools = "running tools without asking"
	}
	budget := "no time budget"
	if a.state.DefaultBudget > 0 {
		budget = "a budget of " + workTime(int64(a.state.DefaultBudget)*60)
	}
	return name + ", " + where + ", " + tools + ", with " + budget + "."
}

// quitSection says what quitting the app does to this device's agents.
func (a *App) quitSection(c *ui.Context) {
	if switchRow(c, &a.state.StopAgentsOnQuit, "Stop this device's agents when enowX quits",
		"Off, they keep working with the app closed, and automations run. Either way their sessions stay, to continue.") {
		a.saveState()
	}
}

// sleepChoices are the idle minutes hibernation is offered at; 0 never.
var sleepChoices = []int{0, 15, 30, 60, 240}

// setHibernation sets the connected host's hibernation window, keeping its
// other settings.
func (a *App) setHibernation(minutes int) {
	hs, cl := a.hostSettingsOf(), a.client
	if cl == nil {
		return
	}
	hs.sleep = minutes
	go func() {
		st, err := cl.Settings()
		if err == nil {
			st.HibernateAfter = minutes
			_, err = cl.SetSettings(st)
		}
		if err != nil {
			a.onMain(func() { a.status = client.Explain(err) })
		}
	}()
}

// environmentSection keeps the variables every session on the host gets.
func (a *App) environmentSection(c *ui.Context) {
	hs := a.hostSettingsOf()
	widget.PageHeader(c, "Environment", "Variables every session on "+a.host.Name+" gets", nil)
	widget.FigsBand(c, []widget.Fig{
		{Label: "Variables", Value: strconv.Itoa(len(hs.vars))},
		{Label: "Sessions on " + a.host.Name, Value: strconv.Itoa(len(a.sessions))},
	})
	widget.PageBody(c, true, func() {
		if hs.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, hs.err, "", nil)
		}
		widget.Cols(c, 3).Children(func() {
			for _, v := range hs.vars {
				widget.Card(c.Key(v.Name)).Children(func() {
					ui.Row(c).FillWidth().MinWidth(0).Children(func() {
						widget.Running(c, v.Name, widget.RunStyle{Mono: true, Weight: 500})
					})
					ui.Row(c).FillWidth().MinWidth(0).Children(func() { widget.Running(c.Key("value"), v.Value, widget.RunStyle{Mono: true}) })
					widget.CardFoot(c, func() {
						ui.Row(c).Grow(1).MinWidth(0).Children(func() {
							widget.Running(c, "Every session on "+a.host.Name, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
						})
						if widget.IconButton(c, icon.Trash, "Remove it", false).Clicked() {
							a.setVar(v.Name, "")
						}
					})
				})
			}
			widget.Card(c.Key("kept")).Children(func() {
				widget.CardTitle(c, nil, "Where they are kept", nil)
				widget.KV(c, "File", "~/"+transport.HomeName+"/session-env.json", true)
				widget.KV(c, "Readable by", "Its user alone", false)
				widget.KV(c, "Applies to", "Sessions started or continued after", false)
			})
		})
		widget.Card(c.Key("add-var")).Children(func() {
			widget.CardTitle(c, nil, "Add a variable", nil)
			ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.End).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					widget.Field(c, "Name", func() { widget.Input(c, &hs.newName, "ANTHROPIC_API_KEY").FillWidth().Font(theme.MonoFont) })
				})
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					widget.Field(c, "Value", func() {
						f := ui.TextInputBase(c, &hs.newValue).Password().FillWidth().Height(theme.ControlHeight).PaddingX(theme.S3).Radius(theme.RadiusFull).
							Placeholder("Its value").Label("Value")
						widget.Well(f, f.Focused())
					})
				})
				if widget.PrimaryButton(c, nil, "Add", "").Clicked() && strings.TrimSpace(hs.newName) != "" && hs.newValue != "" {
					a.setVar(strings.TrimSpace(hs.newName), hs.newValue)
					hs.newName, hs.newValue = "", ""
				}
			})
			if a.host.ID != hosts.LocalID {
				widget.CardNote(c, "Setting one sends its value to "+a.host.Name+".", 0).TextColor(theme.Warning)
			}
		})
		if len(a.sessions) > 0 {
			widget.Heading(c, "Sessions on "+a.host.Name+": they get a change when next started or continued")
			widget.Cols(c, 3).Gap(theme.S2).Children(func() {
				for _, s := range a.sessions {
					widget.FactCard(c, a.sessionName(s), filepath.Base(s.Project), s.Branch)
				}
			})
		}
	})
}

func (a *App) setVar(name, value string) {
	hs, cl := a.hostSet, a.client
	if cl == nil {
		return
	}
	go func() {
		vars, err := cl.SetSessionEnv(name, value)
		a.onMain(func() {
			if err != nil {
				hs.err = client.Explain(err)
				return
			}
			hs.vars, hs.err = vars, ""
		})
	}()
}

// floatingSection is the floating control: the radial panel over any app,
// and how it behaves. Nothing here asks the host: the panel is the app's
// own, and works the same for a session on this machine and one on a
// server.
func (a *App) floatingSection(c *ui.Context) {
	settingsColumn(c, "Floating control", "A round panel over any app", nil, func() {
		widget.Cols(c, 3).Children(func() {
			widget.Card(c.Key("floating-behaviour")).Children(func() {
				widget.CardTitle(c, nil, "Behaviour", nil)
				floating := a.state.FloatingControl
				if switchRowKey(c, "Floating control switch", &floating, "Floating control", "The panel and its shortcut. Off, "+floatingKey+" goes back to other apps.") {
					a.setFloatingControl(floating) // binds the key, and shows the panel
				}
				ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
					ui.Text(c, "Shortcut").FontSize(theme.TextUI).Grow(1)
					widget.Chip(c, floatingKey, ui.Color{})
				})
				widget.CardNote(c, "Listed with the keyboard shortcuts in App.", 0)
				fold := a.floatingFoldOnBlur()
				if switchRowKey(c, "Fold when you go elsewhere switch", &fold, "Fold when you go elsewhere", "Clicking another app, or the desktop, folds the panel away. Off, it stays until you pick an action, press Escape, or press "+floatingKey+" again.") {
					a.state.FloatingFoldOnBlur = &fold
				}
				atPointer := a.floatingAtPointer()
				if switchRowKey(c, "Show at the pointer switch", &atPointer, "Show at the pointer", "The panel blooms where the cursor is, on the display it is on. Off, it opens in the middle of that display.") {
					a.state.FloatingAtPointer = &atPointer
				}
				// What the panel lists, and how the sessions show: this is where the
				// sidebar's switch lives, beside the palette command that also toggles
				// it. It is not an App preference, so App does not carry it.
				hideSessions := a.state.HideSessions
				if switchRowKey(c, "Hide the sidebar switch", &hideSessions, "Hide the sidebar", "The sidebar folds away, giving the session the room; the command palette brings it back.") {
					a.state.HideSessions = hideSessions
				}
			})
			widget.Card(c.Key("floating-look")).Children(func() {
				widget.CardTitle(c, nil, "Appearance", nil)
				a.floatingAppearance(c)
			})
			widget.Card(c.Key("floating-preview")).Children(func() {
				widget.CardTitle(c, nil, "Preview", nil)
				a.floatingPreview(c)
				ui.Spacer(c)
				widget.CardNote(c, "A radial panel of the main actions, over any app: "+floatingKey+". While it is on, "+floatingKey+" no longer reaches other apps.", 0)
			})
		})
	})
}

// floatingFoldOnBlur and floatingAtPointer are the panel's two behaviours,
// on unless the user turned them off: the state fields are pointers so an
// app that never saved them keeps the defaults.
func (a *App) floatingFoldOnBlur() bool {
	return a.state.FloatingFoldOnBlur == nil || *a.state.FloatingFoldOnBlur
}

func (a *App) floatingAtPointer() bool {
	return a.state.FloatingAtPointer == nil || *a.state.FloatingAtPointer
}

// appSection is the app's own preferences.
func (a *App) appSection(c *ui.Context) {
	settingsColumn(c, "App", "enowX on this device", nil, func() {
		widget.Cols(c, 3).Children(func() {
			widget.Card(c.Key("editor")).Children(func() {
				widget.CardTitle(c, nil, "Editor", nil)
				names := make([]string, len(editors))
				for i, e := range editors {
					names[i] = e.name
				}
				cur := slices.IndexFunc(editors, func(e editor) bool { return e.id == a.preferredEditor().id })
				if i := segmentedInCard(c, names, cur, 200); i >= 0 {
					a.state.Editor = editors[i].id
				}
				widget.CardNote(c, "Files of a session open in it.", 0)
			})
			widget.Card(c.Key("device")).Children(func() {
				widget.CardTitle(c, nil, "On this device", nil)
				notify := !a.state.NotifyOff
				if switchRow(c, &notify, "Notifications", "When an agent needs input or finishes in a session not shown.") {
					a.state.NotifyOff = !notify
				}
				if a.tray != nil {
					keep := !a.state.CloseQuits
					where := "the notification area"
					if runtime.GOOS == "darwin" {
						where = "the menu bar"
					}
					if switchRow(c, &keep, "Keep in "+where+" when closed", "Closing the window leaves enowX's icon there, its sessions followed: click it for the sessions that need you. Quit from its menu.") {
						a.state.CloseQuits = !keep
					}
				}
			})
			widget.Card(c.Key("updates")).Children(func() {
				widget.CardTitle(c, nil, "Updates", nil)
				ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
					ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, version.String, widget.RunStyle{Size: 24, Weight: 600}) })
					if widget.GhostButton(c, "Check for updates").Clicked() {
						a.checkForUpdates(true)
					}
				})
				auto := !a.state.Updates.Manual
				if switchRow(c, &auto, "Look for updates by itself", "At start and every few hours; Check for updates looks now.") {
					a.state.Updates.Manual = !auto
				}
			})
		})
		ui.Row(c.Key("app-lower")).FillWidth().Gap(theme.S3).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(5).Basis(0).MinWidth(0).Gap(theme.S3).Children(func() {
				widget.Card(c.Key("quit")).Children(func() {
					widget.CardTitle(c, nil, "When enowX quits", nil)
					a.quitSection(c)
				})
				a.commandLineCard(c)
			})
			widget.Card(c.Key("keys")).Grow(7).Basis(0).Children(func() {
				list := shortcutList()
				widget.CardTitle(c, nil, "Keyboard shortcuts", func() {
					ui.Text(c, strconv.Itoa(len(list))).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
				})
				ui.Column(c).FillWidth().MinWidth(0).Children(func() {
					for _, s := range list {
						ui.Box(c).Height(0.5).FillWidth().Shrink(0).Background(theme.Border)
						ui.Row(c.Key(s[0])).FillWidth().Height(28).Shrink(0).Gap(theme.S3).Children(func() {
							ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, s[1], widget.RunStyle{}) })
							ui.Text(c, s[0]).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Shrink(0)
						})
					}
				})
			})
		})
	})
}

// commandLineCard offers to install the enowx command (docs/cli.md) as a
// link to this app, for shells and scripts.
func (a *App) commandLineCard(c *ui.Context) {
	exe, err := os.Executable()
	if err != nil || runtime.GOOS == "windows" {
		return
	}
	link, _ := cli.LinkPath()
	widget.Card(c.Key("command-line")).Children(func() {
		widget.CardTitle(c, nil, "Command line", nil)
		note := "enowx-app ls, spawn, send, wait… in any terminal: a link to this app at " + link + "."
		if cli.Linked(exe) {
			note = "Installed at " + link + "."
			if !cli.OnPath() {
				note += " Add ~/.local/bin to your PATH to run it by name."
			}
		}
		widget.CardNote(c, note, 0)
		widget.CodeBlockText(c, "enowx-app ls\nenowx-app spawn --agent codex \"…\"\nenowx-app wait ID")
		widget.CardFoot(c, func() {
			ui.Spacer(c)
			label := "Install"
			if cli.Linked(exe) {
				label = "Reinstall"
			}
			if widget.GhostButton(c, label).Clicked() {
				if _, err := cli.InstallLink(exe); err != nil {
					a.status = err.Error()
				}
			}
		})
	})
}

// shortcutList is the app's keyboard shortcuts, as the keys and what they
// do.
func shortcutList() [][2]string {
	return [][2]string{
		{modKey + "K", "Command palette"},
		{modKey + "P", "Go to a file of the session"},
		{modKey + "N", "New session"},
		{modKey + "O", "Open a folder"},
		{modKey + "B", "Expand or collapse the sidebar"},
		{modKey + "⇧D", "Show or hide the side panel"},
		{modKey + "⇧S", "Show or hide the Sessions panel"},
		{floatingKey, "Floating control, over any app"},
		{modKey + "⇧P", "Browser tool"},
		{modKey + "⇧L", "Sessions side by side"},
		{modKey + "⇧]  " + modKey + "⇧[", "Next / previous session"},
		{modKey + "⇧↩", "Maximize the session"},
		{modKey + "-click", "Open a path or a local page printed in a terminal"},
	}
}

// diagnosticsSection says what the connected host's daemon is, and its
// log; it restarts it.
func (a *App) diagnosticsSection(c *ui.Context) {
	hs := a.hostSettingsOf()
	info := a.daemonInfo
	widget.PageHeader(c, "Diagnostics", "What "+a.host.Name+"'s daemon says about itself", nil)
	widget.FigsBand(c, []widget.Fig{
		{Label: "Daemon", Value: info.Version},
		{Label: "Running for", Value: textfmt.Age(info.Started, time.Now())},
		{Label: "Sessions", Value: strconv.Itoa(len(a.sessions))},
	})
	widget.PageFill(c, func() {
		widget.MasterDetail(c, 380, func() {
			widget.Card(c.Key("daemon")).Children(func() {
				widget.CardTitle(c, nil, "This daemon", nil)
				widget.KV(c, "Host", a.host.Name+" · "+info.Hostname, false)
				widget.KV(c, "Daemon", "enowxd "+info.Version+" · pid "+strconv.Itoa(info.PID), false)
				widget.KV(c, "Since", info.Started.Format("2 Jan 15:04")+" · "+textfmt.Age(info.Started, time.Now()), false)
				widget.KV(c, "Sessions", strconv.Itoa(len(a.sessions)), false)
				widget.KV(c, "Home", info.Home, true)
				if info.Hash != "" {
					widget.KV(c, "Build", info.Hash[:min(len(info.Hash), 12)], true)
				}
			})
			if !info.Display && info.DisplayNote != "" {
				widget.Card(c.Key("displays")).Children(func() {
					widget.CardTitle(c, nil, "Displays", nil)
					widget.CardNote(c, info.DisplayNote, 0)
				})
			}
			widget.Card(c.Key("restart")).Children(func() {
				widget.CardTitle(c, nil, "Restart", nil)
				widget.CardNote(c, "Its sessions continue, as after an update.", 0)
				ui.Row(c).FillWidth().Children(func() {
					if widget.GhostButton(c, "Restart the daemon").Clicked() {
						a.restartDaemon()
					}
				})
			})
		}, func() {
			widget.Card(c.Key("log")).Grow(1).FillWidth().MinHeight(0).Children(func() {
				widget.CardTitle(c, nil, "End of its log", func() {
					ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
						widget.Running(c.Key("path"), hs.log.Path, widget.RunStyle{Size: theme.TextLabel, Mono: true, Color: theme.TextSoft})
					})
					if widget.Button(c, widget.Secondary, icon.Sync, "Refresh", "").Tooltip("Read the log again").Clicked() {
						a.loadHostSettings()
					}
				})
				logFill(c, hs.log.Text)
			})
		})
	})
}

// logFill is a log on the code ground, filling the height it has.
func logFill(c *ui.Context, text string) {
	ui.Scroll(c.Key("log-text")).Grow(1).FillWidth().MinHeight(0).Radius(theme.RadiusCard).Background(widget.CodeGround).Children(func() {
		ui.Column(c).FillWidth().Padding(theme.S3, theme.S4).Children(func() {
			ui.Text(c, text).Font(theme.MonoFont).FontSize(theme.TextUI).TextColor(widget.CodeText).FillWidth().MinWidth(0)
		})
	})
}

// sectionIntro heads a part of a Settings section: what it is about.
func sectionIntro(c *ui.Context, title, about string) {
	ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
		ui.Text(c, title).FontSize(theme.TextRead)
		ui.Text(c, about).FontSize(theme.TextUI).TextColor(theme.TextSoft)
	})
}

// restartDaemon restarts the connected host's daemon, and connects again.
func (a *App) restartDaemon() {
	cl, h := a.client, a.host
	if cl == nil {
		return
	}
	go func() {
		err := cl.RestartDaemon()
		time.Sleep(time.Second) // it hangs up its sessions first
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			a.notify("Restarted enowxd on " + h.Name)
			a.connect(h)
		})
	}()
}

// switchRow is a setting turned on or off: its name and what it does, the
// switch at the right. It reports a change.
func switchRow(c *ui.Context, on *bool, label, about string) bool {
	return switchRowKey(c, label, on, label, about)
}

// switchRowKey is switchRow under a key of its own, so a section whose
// intro says the same words still has one element to click by name: the
// switch, not the paragraph.
func switchRowKey(c *ui.Context, key string, on *bool, label, about string) bool {
	changed := false
	ui.Row(c.Key(key)).FillWidth().Gap(theme.S3).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, label).FontSize(theme.TextUI).FontWeight(500)
			if about != "" {
				ui.Text(c, about).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			}
		})
		changed = widget.Switch(c, on, key)
	})
	return changed
}

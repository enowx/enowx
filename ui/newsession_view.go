package ui

import (
	"github.com/egoist/mygo/ui"
	"strings"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/chatview"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

func (a *App) newSessionDialog(c *ui.Context) {
	f := &a.newSession
	if !f.open {
		return
	}
	widget.Sheet(c, &f.open, 640, func() {
		if widget.DialogHeader(c, "New session", "In "+f.project.Name+" on "+a.host.Name, true) {
			f.open = false
		}
	}, func() {
		a.sessionField(c)
		if ch := f.cont; ch != nil {
			a.continuingCard(c, *ch)
			if ch.agent != "" {
				widget.Field(c, "Message (optional)", func() {
					widget.TextBox(c, &f.prompt, "What should it do next?", 96).FillWidth()
				})
			}
			a.formError(c)
			return
		}
		ag := a.agentInfo(f.agent)
		install := needsInstall(ag)
		if f.agent != agent.Shell && !install {
			widget.Field(c, "Prompt", func() {
				widget.TextBox(c, &f.prompt, "What should the agent do?", 96).FillWidth().AutoFocus()
				a.issueButton(c)
			})
		}
		ui.Column(c.Key("agent-project")).FillWidth().Shrink(0).Children(func() {
			widget.FieldCols(c, func() { a.agentPicker(c) }, func() { a.projectPicker(c) })
		})
		if chatview.OwnChat(f.agent) {
			a.platformNewSessionNote(c)
		}
		if install {
			a.installCard(c, ag)
			a.formError(c)
			return
		}
		if f.agent == agent.Custom {
			widget.Field(c, "Command", func() {
				widget.Input(c, &f.command, "npx some-agent --flag").FillWidth().Font(theme.MonoFont)
			})
		}
		if f.project.IsGit {
			a.branchFields(c)
		} else if f.loaded {
			widget.Notice(c, icon.Folder, ui.Color{}, "This folder isn't a git repository",
				"The session runs directly in the folder. Sessions here share the same files, so two agents can overwrite each other's changes. Run git init in it to get branches and isolated worktrees.", nil)
		}
		if f.agent != agent.Shell {
			a.routeField(c)
		}
		a.accountField(c)
		if ag.SignIn != "" {
			a.signInCard(c, ag)
		}
		if ag.CanSkipPermissions && !ag.AlwaysSkips {
			a.skipPermissionsField(c)
		}
		if f.project.IsGit {
			a.alsoField(c)
			a.worktreeSetupField(c)
		}
		a.displayField(c)
		a.credsField(c)
		a.formError(c)
	}, func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, modKey+"↩ to start · Esc to cancel", widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		if widget.Button(c, widget.Secondary, nil, "Cancel", "").Clicked() {
			f.open = false
		}
		ag := a.agentInfo(f.agent)
		install := needsInstall(ag)
		label, name, svg := "Start", "Start session", icon.Play
		switch {
		case f.busy:
			label, name, svg = "Starting…", "Starting…", nil
		case f.cont != nil && f.cont.conv != nil:
			label, name, svg = "Resume", "Resume", nil
		case f.cont != nil && f.cont.session != nil && f.cont.session.Hibernated:
			label, name, svg = "Wake up", "Wake up", nil
		case f.cont != nil:
			label, name, svg = "Open session", "Open session", nil
		case install:
			label, name, svg = "Install "+ag.Name, "Install "+ag.Name, nil
		}
		b := widget.PrimaryButton(c, svg, label, "").Label(name).Disabled(f.busy)
		if install && ag.Install == "" && f.cont == nil {
			b.Disabled(true).Opacity(0.45)
		} else if b.Clicked() || c.Shortcut(ui.Cmd, ui.KeyEnter) {
			switch {
			case f.busy:
			case f.cont != nil:
				a.goOn(*f.cont)
			case install:
				a.installAgent(ag)
			default:
				a.startSession()
			}
		}
	})
}

func (a *App) formError(c *ui.Context) {
	if f := &a.newSession; f.err != "" {
		ui.Text(c, f.err).FontSize(theme.TextUI).TextColor(theme.Error)
	}
}

// formRow is a row of the form that holds a line about itself and what can
// be done: as a switch's, for what isn't one.
func formRow(c *ui.Context, title, about string, right func()) {
	ui.Row(c.Key("row:" + title)).FillWidth().MinHeight(44).Gap(theme.S3).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(theme.TextUI)
			if about != "" {
				ui.Text(c, about).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			}
		})
		ui.Row(c).Shrink(0).Children(right)
	})
}

// signInCard offers to sign the agent in, with a link and a code to open
// on any device, which is what works on a server without a browser.
func (a *App) signInCard(c *ui.Context, ag proto.AgentInfo) {
	f := &a.newSession
	formRow(c, "Not signed in yet?", ag.SignIn, func() {
		if widget.Button(c, widget.Secondary, nil, "Sign in", "").Disabled(f.busy).Clicked() && !f.busy {
			a.startSignIn(ag)
		}
	})
}

// skipPermissionsField is the switch for running the agent without
// permission prompts, remembered across sessions.
func (a *App) skipPermissionsField(c *ui.Context) {
	if widget.ToggleRow(c, "Skip permission prompts", "The agent runs without asking", &a.skipPermissions) {
		a.saveState()
	}
}

// displayField offers a virtual display for desktop apps, where the host
// can run one, and says what it needs where it can't (servers only: an
// app on this machine shows its windows here).
func (a *App) displayField(c *ui.Context) {
	f := &a.newSession
	info := a.daemonInfo
	if !info.Display && (a.host.ID == hosts.LocalID || info.DisplayNote == "") {
		return
	}
	if info.Display {
		widget.ToggleRow(c, "Run with a display", "For desktop apps: their windows open on a display of the session's own, shown in the Browser tool.", &f.display)
		return
	}
	formRow(c, "Run with a display", "For desktop apps; this host "+info.DisplayNote+".", func() {
		if info.DisplayInstall != "" && widget.Button(c, widget.Secondary, nil, "Install Xpra", "").Disabled(f.busy).Clicked() && !f.busy {
			a.installDisplay()
		}
	})
	if info.DisplayInstall != "" {
		widget.CodeBlockText(c, info.DisplayInstall)
	}
}

// worktreeSetupField says what the project's .enowx.toml gives a new
// worktree, and asks before running its setup script.
func (a *App) worktreeSetupField(c *ui.Context) {
	f := &a.newSession
	if f.branchMode == proto.BranchNone || f.project.Setup == "" && len(f.project.Copy) == 0 {
		return
	}
	if len(f.project.Copy) > 0 {
		ui.Text(c, "A new worktree gets "+strings.Join(f.project.Copy, ", ")+" from the project folder (.enowx.toml).").
			FontSize(theme.TextUI).TextColor(theme.TextSoft)
	}
	if f.project.Setup != "" {
		widget.ToggleRow(c, "Run the project's setup", "In a tab of the session, when its worktree is new: "+f.project.Setup, &f.runSetup)
	}
}

// projectPicker is the project the session starts in; another keeps what was
// typed and reads that project.
func (a *App) projectPicker(c *ui.Context) {
	f := &a.newSession
	widget.Field(c, "Project", func() {
		options, names := []string{}, map[string]string{f.project.Path: f.project.Name}
		for _, p := range a.projects {
			options = append(options, p.Path)
			names[p.Path] = p.Name
		}
		v := f.project.Path
		hookSelect(c, "Project", &v, options, names)
		if v == f.project.Path {
			return
		}
		for _, p := range a.projects {
			if p.Path == v {
				prompt := f.prompt
				a.openNewSession(p)
				f.prompt = prompt
			}
		}
	})
}

// agentPicker is a drop-down of agents with their logos.
func (a *App) agentPicker(c *ui.Context) {
	f := &a.newSession
	widget.Field(c, "Agent", func() {
		sel := ui.SelectBase(c, &f.agent)
		cur := a.agentInfo(f.agent)
		widget.Trigger(sel.Trigger, sel.Open()).Label("Agent").Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, cur.Name, widget.RunStyle{}) })
			ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
		})
		sel.Popup(func(panel ui.Element) {
			widget.Popup(panel.Margin(theme.S1, 0, 0, 0).Width(320))
			// Those on this host first; the many others after, to install.
			var here, missing []proto.AgentInfo
			for _, ag := range a.agents {
				if ag.Installed || ag.Command == "" {
					here = append(here, ag)
				} else {
					missing = append(missing, ag)
				}
			}
			widget.Scroll(c).FillWidth().MaxHeight(widget.PopupRoom(c, sel.Trigger, 420)).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					for i, ag := range here {
						if ag.ID == agent.Shell && i > 0 {
							widget.Divider(c)
						}
						a.agentItem(c, sel, ag)
					}
					if len(missing) > 0 {
						widget.Divider(c)
						widget.PopupLabel(c, "Not installed on "+a.host.Name)
						for _, ag := range missing {
							a.agentItem(c, sel, ag)
						}
					}
				})
			})
		})
	})
}

// agentItem is an agent in the picker: its logo, name and command.
func (a *App) agentItem(c *ui.Context, sel ui.SelectParts[string], ag proto.AgentInfo) {
	item := sel.Item(ag.ID)
	widget.PopupItem(item, item.Highlighted()).Children(func() {
		agentTile(c, ag.ID, 24)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, ag.Name).FontSize(theme.TextUI).SingleLine()
			sub := ag.Command
			switch ag.ID {
			case agent.Shell:
				sub = "your login shell"
			case agent.Custom:
				sub = "any program"
			}
			ui.Text(c, sub).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
		})
		if ag.ID == a.newSession.agent {
			ui.Icon(c, icon.Check).FontSize(14).TextColor(theme.Now)
		}
	})
}

func agentTile(c *ui.Context, id string, size float32) {
	widget.Avatar(c, icon.Agent(id), size, theme.Text)
}

// whereNames are the places a git project's session can run.
var whereNames = map[string]string{
	proto.BranchNew:      "New branch and worktree",
	proto.BranchExisting: "Existing branch",
	proto.BranchNone:     "The project folder",
}

// branchFields says where the session runs, and on which branch.
func (a *App) branchFields(c *ui.Context) {
	f := &a.newSession
	ui.Column(c.Key("where")).FillWidth().Shrink(0).Children(func() {
		widget.FieldCols(c, func() {
			widget.Field(c, "Where", func() {
				hookSelect(c, "Where", &f.branchMode, []string{proto.BranchNew, proto.BranchExisting, proto.BranchNone}, whereNames)
			})
		}, func() {
			widget.Field(c, "Branch", func() {
				switch f.branchMode {
				case proto.BranchNew:
					widget.Input(c, &f.branch, "feature/name").FillWidth()
				case proto.BranchExisting:
					branchSelect(c, &f.existing, f.project.Branches)
				default:
					ui.Row(c).Height(theme.ControlHeight).MinWidth(0).Children(func() {
						widget.Running(c, nonEmpty(f.project.Branch, "As it is checked out"), widget.RunStyle{Color: theme.TextSoft})
					})
				}
			})
		})
	})
	if f.branchMode == proto.BranchNew {
		widget.Field(c, "From", func() { branchSelect(c, &f.base, f.project.Branches) })
	}
	if f.branchMode == proto.BranchExisting && f.existing == f.project.Branch && f.existing != "" {
		ui.Text(c, "Checked out in the project folder, so the session runs there.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
	}
}

// branchSelect is a drop-down of branches.
func branchSelect(c *ui.Context, value *string, branches []string) {
	if len(branches) == 0 {
		ui.Row(c).Height(theme.ControlHeight).Children(func() {
			ui.Text(c, "No branches yet").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		})
		return
	}
	sel := ui.SelectBase(c, value)
	widget.Trigger(sel.Trigger, sel.Open()).Label("Branch").Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, *value, widget.RunStyle{}) })
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	sel.Popup(func(panel ui.Element) {
		widget.Popup(panel.Margin(theme.S1, 0, 0, 0).MinWidth(180).MaxWidth(360).MaxHeight(widget.PopupRoom(c, sel.Trigger, 280)))
		widget.Scroll(c).FillWidth().Children(func() {
			for _, b := range branches {
				item := sel.Item(b)
				widget.PopupItem(item, item.Highlighted()).Children(func() {
					ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, b, widget.RunStyle{}) })
					if b == *value {
						ui.Icon(c, icon.Check).FontSize(13).TextColor(theme.Now)
					}
				})
			}
		})
	})
}

package ui

import (
	"github.com/egoist/mygo/ui"
	"strings"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

func (a *App) newSessionDialog(c *ui.Context) {
	f := &a.newSession
	if !f.open {
		return
	}
	widget.Sheet(c, &f.open, 560, func() {
		ui.Column(c).Gap(5).Grow(1).MinWidth(0).Children(func() {
			widget.Breadcrumb(c, a.host.Name, f.project.Name, "New session")
			ui.Text(c, f.project.Path).Font(theme.MonoFont).FontSize(11).TextColor(theme.Muted).SingleLine()
		})
		if widget.IconButton(c, icon.Close, "Close", false).Clicked() {
			f.open = false
		}
	}, func() {
		a.sessionField(c)
		if ch := f.cont; ch != nil {
			a.continuingCard(c, *ch)
			if ch.agent != "" {
				widget.Field(c, "Message (optional)", func() {
					ui.TextArea(c, &f.prompt).Height(76).Padding(9, 11).Radius(theme.RadiusControl).
						Background(theme.Sunken).Border(1, theme.BorderStrong).Placeholder("What should it do next?")
				})
			}
			a.formError(c)
			return
		}
		a.agentPicker(c)
		if ag := a.agentInfo(f.agent); needsInstall(ag) {
			a.installCard(c, ag)
			a.formError(c)
			return
		}
		if f.agent == agent.Custom {
			widget.Field(c, "Command", func() {
				widget.Input(c, &f.command, "npx some-agent --flag").FillWidth().Font(theme.MonoFont)
			})
		}
		a.accountField(c)
		if ag := a.agentInfo(f.agent); ag.SignIn != "" {
			a.signInCard(c, ag)
		}
		if f.project.IsGit {
			a.branchFields(c)
			a.alsoField(c)
			a.worktreeSetupField(c)
		} else if f.loaded {
			widget.Notice(c, icon.Folder, ui.Color{}, "This folder isn't a git repository",
				"The session runs directly in the folder. Sessions here share the same files, so two agents can overwrite each other's changes. Run git init in it to get branches and isolated worktrees.", nil)
		}
		if a.agentInfo(f.agent).CanSkipPermissions {
			a.skipPermissionsField(c)
		}
		a.displayField(c)
		a.credsField(c)
		if f.agent != agent.Shell {
			widget.Field(c, "First prompt (optional)", func() {
				ui.TextArea(c, &f.prompt).Height(76).Padding(9, 11).Radius(theme.RadiusControl).
					Background(theme.Sunken).Border(1, theme.BorderStrong).Placeholder("What should the agent do?")
				a.issueButton(c)
				a.routeField(c)
			})
		}
		a.formError(c)
	}, func() {
		ui.Text(c, modKey+"↩ to start · Esc to cancel").FontSize(11.5).TextColor(theme.Muted)
		ui.Spacer(c)
		if widget.GhostButton(c, "Cancel").Clicked() {
			f.open = false
		}
		ag := a.agentInfo(f.agent)
		install := needsInstall(ag)
		label := "Start session"
		switch {
		case f.busy:
			label = "Starting…"
		case f.cont != nil && f.cont.conv != nil:
			label = "Resume"
		case f.cont != nil && f.cont.session != nil && f.cont.session.Hibernated:
			label = "Wake up"
		case f.cont != nil:
			label = "Open session"
		case install:
			label = "Install " + ag.Name
		}
		b := widget.PrimaryButton(c, nil, label, "").Disabled(f.busy)
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
		ui.Text(c, f.err).FontSize(12).TextColor(theme.Error)
	}
}

// signInCard offers to sign the agent in, with a link and a code to open
// on any device, which is what works on a server without a browser.
func (a *App) signInCard(c *ui.Context, ag proto.AgentInfo) {
	f := &a.newSession
	widget.Card(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, "Not signed in yet?").FontSize(12.5)
				ui.Text(c, ag.SignIn).Font(theme.MonoFont).FontSize(11).TextColor(theme.Muted).SingleLine()
			})
			if widget.GhostButton(c, "Sign in").Disabled(f.busy).Clicked() && !f.busy {
				a.startSignIn(ag)
			}
		})
	})
}

// skipPermissionsField is the switch for running the agent without
// permission prompts, remembered across sessions.
func (a *App) skipPermissionsField(c *ui.Context) {
	widget.Card(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, "Skip permission prompts").FontSize(12.5)
				ui.Text(c, "The agent runs commands and edits files without asking, and keeps doing so when enowX continues the session after a restart.").
					FontSize(11.5).TextColor(theme.Muted)
			})
			if widget.Switch(c, &a.skipPermissions, "Skip permission prompts") {
				a.saveState()
			}
		})
	})
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
	widget.Card(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, "Run with a display").FontSize(12.5)
				note := "For desktop apps: their windows open on a display of the session's own, shown in the Browser tool."
				if !info.Display {
					note = "For desktop apps; this host " + info.DisplayNote + "."
				}
				ui.Text(c, note).FontSize(11.5).TextColor(theme.Muted)
			})
			if info.Display {
				widget.Switch(c, &f.display, "Run with a display")
			} else if info.DisplayInstall != "" && widget.GhostButton(c, "Install Xpra").Disabled(f.busy).Clicked() && !f.busy {
				a.installDisplay()
			}
		})
		if !info.Display && info.DisplayInstall != "" {
			ui.Text(c, info.DisplayInstall).Font(theme.MonoFont).FontSize(11).Padding(8, 10).Radius(7).Background(theme.Sunken).FillWidth().Margin(10, 0, 0, 0)
		}
	})
}

// worktreeSetupField says what the project's .enowx.toml gives a new
// worktree, and asks before running its setup script.
func (a *App) worktreeSetupField(c *ui.Context) {
	f := &a.newSession
	if f.branchMode == proto.BranchNone || f.project.Setup == "" && len(f.project.Copy) == 0 {
		return
	}
	widget.Card(c).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			if len(f.project.Copy) > 0 {
				ui.Text(c, "A new worktree gets "+strings.Join(f.project.Copy, ", ")+" from the project folder (.enowx.toml).").
					FontSize(11.5).TextColor(theme.Muted)
			}
			if f.project.Setup != "" {
				switchRow(c, &f.runSetup, "Run the project's setup", "In a tab of the session, when its worktree is new: "+f.project.Setup)
			}
		})
	})
}

// agentPicker is a drop-down of agents with their logos.
func (a *App) agentPicker(c *ui.Context) {
	f := &a.newSession
	widget.Field(c, "Agent", func() {
		sel := ui.SelectBase(c, &f.agent)
		cur := a.agentInfo(f.agent)
		trig := widget.Trigger(sel.Trigger, sel.Open()).Height(42).Label("Agent")
		trig.Children(func() {
			agentTile(c, cur.ID, 26)
			ui.Text(c, cur.Name).SingleLine()
			if cur.Command != "" {
				ui.Text(c, cur.Command).Font(theme.MonoFont).FontSize(11).TextColor(theme.Muted)
			}
			ui.Spacer(c)
			ui.Icon(c, icon.ChevronDown).FontSize(13).TextColor(theme.Muted)
		})
		sel.Popup(func(panel ui.Element) {
			widget.Popup(panel.Margin(6, 0, 0, 0).Width(518))
			// Those on this host first; the many others after, to install.
			var here, missing []proto.AgentInfo
			for _, ag := range a.agents {
				if ag.Installed || ag.Command == "" {
					here = append(here, ag)
				} else {
					missing = append(missing, ag)
				}
			}
			ui.Scroll(c).FillWidth().MaxHeight(420).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					for i, ag := range here {
						if ag.ID == agent.Shell && i > 0 {
							widget.Divider(c)
						}
						a.agentItem(c, sel, ag)
					}
					if len(missing) > 0 {
						widget.Divider(c)
						widget.PopupLabel(c, "NOT INSTALLED ON "+strings.ToUpper(a.host.Name))
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
		agentTile(c, ag.ID, 26)
		ui.Column(c).Gap(1).Grow(1).Children(func() {
			ui.Text(c, ag.Name)
			sub := ag.Command
			switch ag.ID {
			case agent.Shell:
				sub = "your login shell"
			case agent.Custom:
				sub = "any program"
			}
			ui.Text(c, sub).Font(theme.MonoFont).FontSize(11).TextColor(theme.Muted)
		})
		if ag.ID == a.newSession.agent {
			ui.Icon(c, icon.Check).FontSize(14).TextColor(theme.Accent)
		}
	})
}

func agentTile(c *ui.Context, id string, size float32) {
	widget.Avatar(c, icon.Agent(id), size, theme.Text)
}

func (a *App) branchFields(c *ui.Context) {
	f := &a.newSession
	widget.Field(c, "Branch", func() {
		widget.FitGrid(c, 220, 2).Gap(8).Children(func() {
			for _, m := range []struct{ mode, title, body string }{
				{proto.BranchNew, "New branch", "In its own worktree, so it never touches your other sessions"},
				{proto.BranchExisting, "Existing branch", "Continue work on a branch that already exists"},
			} {
				if widget.OptionCard(c.Key(m.mode), f.branchMode == m.mode, m.title, m.body) {
					f.branchMode = m.mode
				}
			}
		})
		switch f.branchMode {
		case proto.BranchNew:
			ui.Grid(c).FillWidth().ColumnTracks(ui.Fr(1), ui.Fixed(180)).Gap(8).Margin(4, 0, 0, 0).Children(func() {
				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, "Branch name").FontSize(11.5).TextColor(theme.Muted)
					widget.Input(c, &f.branch, "feat/user-search").FillWidth().Font(theme.MonoFont).AutoFocus()
				})
				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, "From").FontSize(11.5).TextColor(theme.Muted)
					branchSelect(c, &f.base, f.project.Branches)
				})
			})
		case proto.BranchExisting:
			ui.Column(c).FillWidth().Gap(6).Margin(4, 0, 0, 0).Children(func() {
				ui.Text(c, "Branch").FontSize(11.5).TextColor(theme.Muted)
				branchSelect(c, &f.existing, f.project.Branches)
				if f.existing == f.project.Branch && f.existing != "" {
					ui.Text(c, "Checked out in the project folder, so the session runs there.").FontSize(11.5).TextColor(theme.Muted)
				}
			})
		}
	})
}

// branchSelect is a drop-down of branches.
func branchSelect(c *ui.Context, value *string, branches []string) {
	if len(branches) == 0 {
		ui.Text(c, "No branches yet").FontSize(12).TextColor(theme.Muted)
		return
	}
	sel := ui.SelectBase(c, value)
	widget.Trigger(sel.Trigger, sel.Open()).Label("Branch").Children(func() {
		ui.Text(c, *value).Font(theme.MonoFont).FontSize(12).SingleLine().Shrink(1)
		ui.Spacer(c)
		ui.Icon(c, icon.ChevronDown).FontSize(13).TextColor(theme.Muted)
	})
	sel.Popup(func(panel ui.Element) {
		widget.Popup(panel.Margin(6, 0, 0, 0).MinWidth(180).MaxHeight(280))
		ui.Scroll(c).FillWidth().Children(func() {
			for _, b := range branches {
				item := sel.Item(b)
				widget.PopupItem(item, item.Highlighted()).Children(func() {
					ui.Text(c, b).Font(theme.MonoFont).FontSize(12).Grow(1).SingleLine()
					if b == *value {
						ui.Icon(c, icon.Check).FontSize(13).TextColor(theme.Accent)
					}
				})
			}
		})
	})
}

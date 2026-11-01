package ui

import (
	"github.com/enowx/enowx/ui/theme"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/widget"
)

// The Run button of a session's tab strip lists its project's commands
// (core/commands): saved, run lately, then found in its files. Picking one
// runs it in a new tab, typed into a shell, where it can be stopped and
// run again; the daemon remembers it among the recent ones.

// commandList is what the app knows of session id's project's commands,
// asked for again each time the menu opens.
type commandList struct {
	list    []proto.RunCommand
	loading bool
}

// loadCommands asks the daemon for session s's project's commands.
func (a *App) loadCommands(s proto.SessionInfo) {
	cl := a.client
	if cl == nil {
		return
	}
	if a.runCommands == nil {
		a.runCommands = map[string]*commandList{}
	}
	cmds := a.runCommands[s.ID]
	if cmds == nil {
		cmds = &commandList{}
		a.runCommands[s.ID] = cmds
	}
	cmds.loading = true
	go func() {
		list, err := cl.ProjectCommands(s.ID)
		a.onMain(func() {
			cmds.loading = false
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			cmds.list = list
		})
	}()
}

// runButton is the tab strip's Run button and its menu of commands.
func (a *App) runButton(c *ui.Context, s proto.SessionInfo) {
	b := widget.IconButton(c, icon.Play, "Run a command of this project", false)
	if b.Clicked() {
		a.loadCommands(s)
	}
	a.menu.OpenBelow(b, "run:"+s.ID, theme.ControlHeight)
	a.menu.Show(c, b, "run:"+s.ID, func(done func()) {
		cmds := a.runCommands[s.ID]
		switch {
		case cmds == nil || cmds.loading && len(cmds.list) == 0:
			widget.MenuItem(c, icon.Sync, "Reading the project…", false)
		case len(cmds.list) == 0:
			widget.MenuItem(c, icon.Code, "No commands found yet", false)
		}
		if cmds != nil {
			group := ""
			for i, cmd := range cmds.list {
				if g := commandGroup(cmd); i > 0 && g != group {
					widget.MenuSeparator(c)
				}
				group = commandGroup(cmd)
				if widget.MenuItem(c.Key(cmd.Command), icon.Terminal, commandLabel(cmd), false) {
					a.runCommand(s, cmd)
					done()
				}
			}
		}
		widget.MenuSeparator(c)
		if widget.MenuItem(c, icon.Plus, "Save a command…", false) {
			a.askCommand(s)
			done()
		}
	})
}

// commandGroup sorts a command into the menu's groups: saved, recent,
// and found in the files.
func commandGroup(c proto.RunCommand) string {
	if c.Source == proto.CommandSaved || c.Source == proto.CommandRecent {
		return c.Source
	}
	return "found"
}

// commandLabel is a command in the menu: its name, then the command when
// the name doesn't say it.
func commandLabel(c proto.RunCommand) string {
	if c.Name == "" || c.Name == c.Command || strings.HasSuffix(c.Command, " "+c.Name) {
		return c.Command
	}
	return c.Name + " — " + c.Command
}

// runCommand runs cmd in a new tab of session s, brought to the front.
func (a *App) runCommand(s proto.SessionInfo, cmd proto.RunCommand) {
	cl := a.client
	if cl == nil {
		return
	}
	top := a.parentOf(s)
	go func() {
		info, err := cl.RunCommand(top.ID, cmd)
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			a.upsert(info)
			a.selectSession(info)
		})
	}()
}

// commandForm is the Save a command dialog.
type commandForm struct {
	open          bool
	session       proto.SessionInfo
	name, command string
}

// askCommand opens the Save a command dialog for session s's project.
func (a *App) askCommand(s proto.SessionInfo) {
	a.commandForm = &commandForm{open: true, session: s}
}

// commandDialog saves a command for a project, and may run it at once;
// it lists the saved and recent ones, to forget them.
func (a *App) commandDialog(c *ui.Context) {
	f := a.commandForm
	if f == nil {
		return
	}
	if !f.open {
		a.commandForm = nil
		return
	}
	save, run := false, false
	widget.Dialog(c, &f.open, 480, 14, func() {
		widget.DialogHeader(c, "Save a command", "It shows first under Run in every session of this project, and runs in a new tab.", false)
		widget.Field(c, "Command", func() {
			if widget.Input(c, &f.command, "npm run dev").FillWidth().Submitted() {
				save = true
			}
		})
		widget.Field(c, "Name", func() {
			if widget.Input(c, &f.name, "optional, as the tab shows it").FillWidth().Submitted() {
				save = true
			}
		})
		if cmds := a.runCommands[f.session.ID]; cmds != nil {
			for _, cmd := range cmds.list {
				if cmd.Source != proto.CommandSaved && cmd.Source != proto.CommandRecent {
					continue
				}
				ui.Row(c.Key(cmd.Command)).FillWidth().Gap(8).Children(func() {
					ui.Text(c, commandLabel(cmd)).FontSize(theme.TextLabel).SingleLine().Grow(1).Shrink(1)
					ui.Text(c, cmd.Source).FontSize(theme.TextLabel).SingleLine()
					if widget.IconButton(c, icon.Close, "Forget it", false).Clicked() {
						a.saveCommand(f.session, cmd, true)
					}
				})
			}
		}
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.open = false
			}
			if widget.Button(c, widget.Secondary, icon.Check, "Save", "").Clicked() {
				save = true
			}
			if widget.PrimaryButton(c, icon.Play, "Save and run", "").Clicked() {
				save, run = true, true
			}
		})
	})
	if !save || strings.TrimSpace(f.command) == "" {
		return
	}
	cmd := proto.RunCommand{Name: strings.TrimSpace(f.name), Command: strings.TrimSpace(f.command)}
	a.saveCommand(f.session, cmd, false)
	if run {
		a.runCommand(f.session, cmd)
	}
	f.open = false
}

// saveCommand saves cmd for session s's project, or forgets it.
func (a *App) saveCommand(s proto.SessionInfo, cmd proto.RunCommand, remove bool) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		list, err := cl.SaveCommand(s.ID, cmd, remove)
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			if a.runCommands == nil {
				a.runCommands = map[string]*commandList{}
			}
			a.runCommands[s.ID] = &commandList{list: list}
		})
	}()
}

// runPaletteCommands offer the selected session's project's commands in
// the command palette, and saving one.
func (a *App) runPaletteCommands() []command {
	s, ok := a.selectedSession()
	if !ok {
		return nil
	}
	var out []command
	if cmds := a.runCommands[s.ID]; cmds != nil {
		for _, cmd := range cmds.list {
			out = append(out, command{"run:" + cmd.Command, "Run " + commandLabel(cmd), "in a new tab · " + cmd.Source, icon.Play, func() { a.runCommand(s, cmd) }})
		}
	}
	return append(out, command{"save-command", "Save a command…", "for " + a.sessionName(s) + "'s project", icon.Plus, func() {
		a.loadCommands(s)
		a.askCommand(s)
	}})
}

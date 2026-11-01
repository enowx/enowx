package libraryview

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// kindPlugins is the library's Plugins: Claude Code's, read from its
// files, changed through its CLI on the host (env.plugins.run).
const kindPlugins = "plugins"

// pluginState is the Plugins panel's.
type pluginState struct {
	list    proto.ClaudePlugins
	loaded  bool
	running string // the action under way, as said to the user
	output  string // what the CLI said last
	failed  bool
	install string // a plugin to install: "name@marketplace"
	markets bool   // the marketplaces show, not the plugins
	market  string // a marketplace to add: "owner/repo", a URL
}

func (v *View) loadPlugins(o Options) {
	cl := o.Client
	if cl == nil {
		return
	}
	go func() {
		list, err := cl.Plugins()
		o.Update(func() {
			v.plugins.loaded = true
			if err != nil {
				v.plugins.output, v.plugins.failed = client.Explain(err), true
				return
			}
			v.plugins.list = list
		})
	}()
}

// runPlugin has the CLI do action to arg, then reads the plugins again.
func (v *View) runPlugin(o Options, action, arg, doing string) {
	cl := o.Client
	if cl == nil || v.plugins.running != "" {
		return
	}
	v.plugins.running, v.plugins.output = doing, ""
	go func() {
		res, err := cl.PluginRun(action, arg)
		o.Update(func() {
			v.plugins.running = ""
			switch {
			case err != nil:
				v.plugins.output, v.plugins.failed = client.Explain(err), true
			default:
				v.plugins.output, v.plugins.failed = res.Output, !res.OK
			}
			v.loadPlugins(o)
		})
	}()
}

// pluginsPanel shows the plugins, or the marketplaces, in the list's and
// the detail's room.
func (v *View) pluginsPanel(c *ui.Context, o Options) {
	ps := &v.plugins
	if !ps.loaded {
		ps.loaded = true // asked once; a reply sets it again
		v.loadPlugins(o)
	}
	ui.Column(c).FillWidth().Padding(14, 20, 10, 20).Gap(10).BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			tabs := []string{fmt.Sprintf("Installed · %d", len(ps.list.Plugins)), fmt.Sprintf("Marketplaces · %d", len(ps.list.Marketplaces))}
			cur := 0
			if ps.markets {
				cur = 1
			}
			if i := widget.Segmented(c.Key("tabs"), tabs, cur, nil); i >= 0 {
				ps.markets = i == 1
			}
			ui.Spacer(c)
			if widget.IconButton(c, icon.Sync, "Read again", false).Clicked() {
				v.loadPlugins(o)
			}
		})
		// What to add goes on top, where it is found.
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			if ps.markets {
				add := widget.Input(c, &ps.market, "owner/repo, or a URL").FillWidth().Font(theme.MonoFont).Submitted()
				if (widget.Button(c, widget.Secondary, icon.Plus, "Add marketplace", "").Clicked() || add) && strings.TrimSpace(ps.market) != "" {
					v.runPlugin(o, proto.PluginMarketplaceAdd, ps.market, "Adding "+ps.market)
					ps.market = ""
				}
				return
			}
			add := widget.Input(c, &ps.install, "plugin@marketplace").FillWidth().Font(theme.MonoFont).Submitted()
			if (widget.Button(c, widget.Secondary, icon.Plus, "Install", "").Clicked() || add) && strings.TrimSpace(ps.install) != "" {
				v.runPlugin(o, proto.PluginInstall, ps.install, "Installing "+ps.install)
				ps.install = ""
			}
		})
		switch {
		case ps.running != "":
			ui.Row(c).Gap(8).Children(func() {
				widget.Spinner(c, 14)
				ui.Text(c, ps.running+"…").FontSize(theme.TextLabel).TextColor(theme.Muted)
			})
		case ps.output != "":
			col := theme.Muted
			if ps.failed {
				col = theme.Error
			}
			ui.Text(c, ps.output).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(col).MaxLines(3)
		case !ps.list.CLI && len(ps.list.Plugins) > 0:
			ui.Text(c, "Claude Code isn't on "+o.Host+"'s PATH: plugins show, but can't be changed.").FontSize(theme.TextLabel).TextColor(theme.Warning)
		}
	})
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(8, 12).Gap(2).Children(func() {
			if ps.markets {
				for _, m := range ps.list.Marketplaces {
					widget.Row(c.Key("m:"+m.Name), m.Name, m.Source, "", func() {
						widget.MoreMenu(c, &v.menu, "market:"+m.Name, func(done func()) {
							if widget.MenuItem(c, icon.Sync, "Update", false) {
								v.runPlugin(o, proto.PluginMarketplaceUpdate, m.Name, "Updating "+m.Name)
								done()
							}
							if widget.MenuItem(c, icon.Trash, "Remove", true) {
								v.runPlugin(o, proto.PluginMarketplaceRemove, m.Name, "Removing "+m.Name)
								done()
							}
						})
					})
				}
				return
			}
			if len(ps.list.Plugins) == 0 {
				ui.Text(c, "None yet: install one from a marketplace.").FontSize(theme.TextLabel).TextColor(theme.Muted).Padding(8)
			}
			for _, p := range ps.list.Plugins {
				v.pluginRow(c.Key(p.ID), o, p)
			}
		})
	})
}

func (v *View) pluginRow(c *ui.Context, o Options, p proto.ClaudePlugin) {
	widget.Row(c, p.Name, p.Marketplace+" · "+p.Version, p.Description, func() {
		on := p.Enabled
		if widget.Switch(c, &on, "Enabled") {
			action, doing := proto.PluginDisable, "Disabling "+p.Name
			if on {
				action, doing = proto.PluginEnable, "Enabling "+p.Name
			}
			v.runPlugin(o, action, p.ID, doing)
		}
		widget.MoreMenu(c, &v.menu, "plugin:"+p.ID, func(done func()) {
			if widget.MenuItem(c, icon.Sync, "Update", false) {
				v.runPlugin(o, proto.PluginUpdate, p.ID, "Updating "+p.Name)
				done()
			}
			if widget.MenuItem(c, icon.Trash, "Uninstall", true) {
				v.runPlugin(o, proto.PluginUninstall, p.ID, "Uninstalling "+p.Name)
				done()
			}
		})
	})
}

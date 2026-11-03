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
	read    bool   // list is what the host said
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
			v.plugins.list, v.plugins.read = list, true
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

// pluginsPanel shows the plugins, or the marketplaces, and what the CLI said
// last, which share the height.
func (v *View) pluginsPanel(c *ui.Context, o Options) {
	ps := &v.plugins
	ui.Row(c.Key("tools")).FillWidth().Shrink(0).Gap(theme.S3).Children(func() {
		tabs := []string{fmt.Sprintf("Plugins (%d)", len(ps.list.Plugins)), fmt.Sprintf("Marketplaces (%d)", len(ps.list.Marketplaces))}
		cur := 0
		if ps.markets {
			cur = 1
		}
		if i := widget.Segmented(c.Key("tabs"), tabs, cur, nil); i >= 0 {
			ps.markets = i == 1
		}
		ui.Spacer(c)
		if widget.Button(c, widget.Secondary, icon.Sync, "Refresh", "").Clicked() {
			v.loadPlugins(o)
		}
	})
	// What to add goes on top, where it is found.
	ui.Row(c.Key("add")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		if ps.markets {
			add := widget.Input(c, &ps.market, "owner/repo or a URL").Grow(1).Basis(0).MinWidth(0).Font(theme.MonoFont).Submitted()
			if (widget.Button(c, widget.Secondary, icon.Plus, "Add", "").Clicked() || add) && strings.TrimSpace(ps.market) != "" {
				v.runPlugin(o, proto.PluginMarketplaceAdd, ps.market, "Adding "+ps.market)
				ps.market = ""
			}
			return
		}
		add := widget.Input(c, &ps.install, "name@marketplace").Grow(1).Basis(0).MinWidth(0).Font(theme.MonoFont).Submitted()
		if (widget.Button(c, widget.Secondary, icon.Plus, "Install", "").Clicked() || add) && strings.TrimSpace(ps.install) != "" {
			v.runPlugin(o, proto.PluginInstall, ps.install, "Installing "+ps.install)
			ps.install = ""
		}
	})
	switch {
	case ps.running != "":
		ui.Row(c.Key("running")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
			widget.Spinner(c, 14)
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Running(c, ps.running+"…", widget.RunStyle{Color: theme.TextSoft})
			})
		})
	case !ps.list.CLI && len(ps.list.Plugins) > 0:
		widget.Banner(c, theme.Warning, "Claude Code isn't on "+o.Host+"'s PATH: plugins show, but can't be changed.", nil)
	}
	widget.Scroll(c.Key("plugin-cards")).Grow(1).Basis(0).FillWidth().MinHeight(0).Children(func() {
		widget.Cols(c, 2).Children(func() {
			if ps.markets {
				for _, m := range ps.list.Marketplaces {
					v.marketCard(c.Key("m:"+m.Name), o, m)
				}
				return
			}
			for _, p := range ps.list.Plugins {
				v.pluginCard(c.Key(p.ID), o, p)
			}
		})
		if ps.markets && ps.read && len(ps.list.Marketplaces) == 0 {
			ui.Text(c.Key("none"), "None yet: add one.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		}
		if !ps.markets && ps.read && len(ps.list.Plugins) == 0 {
			ui.Text(c.Key("none"), "None yet: install one from a marketplace.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		}
	})
	if ps.output != "" {
		col := widget.CodeText
		if ps.failed {
			col = theme.Error
		}
		codeFill(c, "plugin-output", func() { codeLine(c, ps.output, col) })
	}
}

// pluginCardBody is a card of the grid: its head, a line about it, and its
// menu at the bottom.
func pluginCardBody(c *ui.Context, head func(), about string, menu func()) {
	widget.Card(c).Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).Children(head)
		if about != "" {
			ui.Text(c, about).FontSize(theme.TextUI).TextColor(theme.TextSoft).FillWidth().MinWidth(0).MaxLines(2)
		}
		ui.Spacer(c)
		ui.Row(c).FillWidth().Shrink(0).Justify(ui.End).Children(menu)
	})
}

func (v *View) marketCard(c *ui.Context, o Options, m proto.ClaudeMarketplace) {
	pluginCardBody(c, func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			widget.Running(c, m.Name, widget.RunStyle{Size: theme.TextRead, Weight: 600})
		})
	}, m.Source, func() {
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

func (v *View) pluginCard(c *ui.Context, o Options, p proto.ClaudePlugin) {
	pluginCardBody(c, func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(theme.S2).Children(func() {
			ui.Row(c).MinWidth(0).Shrink(1).Children(func() {
				widget.Running(c, p.Name, widget.RunStyle{Size: theme.TextRead, Weight: 600})
			})
			ui.Row(c).MinWidth(0).Shrink(100).Children(func() {
				widget.Running(c.Key("from"), p.Version+" · "+p.Marketplace, widget.RunStyle{Color: theme.TextSoft})
			})
		})
		on := p.Enabled
		if widget.Switch(c, &on, "Enabled") {
			action, doing := proto.PluginDisable, "Disabling "+p.Name
			if on {
				action, doing = proto.PluginEnable, "Enabling "+p.Name
			}
			v.runPlugin(o, action, p.ID, doing)
		}
	}, p.Description, func() {
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

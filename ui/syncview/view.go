package syncview

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// View draws the sync page's body under the app's header. Its Close
// button, while nothing runs, closes it for good (see Closed).
func (d *Dialog) View(c *ui.Context) {
	if d.closed {
		return
	}
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(16).Gap(14).Children(func() {
		ui.Text(c, "Skills, subagents, commands, memory files, settings and MCP servers of your agents, and your git identity.").
			FontSize(12).TextColor(theme.Muted)
		d.direction(c)
		switch {
		case d.loading:
			ui.Text(c, "Reading both sides…").TextColor(theme.Muted).Padding(24, 0)
		case d.err != "":
			ui.Text(c, d.err).FontSize(theme.TextUI).TextColor(theme.Error)
		default:
			d.list(c)
		}
		d.footer(c)
	})
}

// Running reports whether a sync is under way, so the page stays open.
func (d *Dialog) Running() bool { return d.running }

func (d *Dialog) direction(c *ui.Context) {
	local, remote := d.opts.Local.Name, d.opts.Host.Name
	widget.FitGrid(c, 260, 2).Gap(8).Children(func() {
		for _, toRemote := range []bool{true, false} {
			from, to := local, remote
			if !toRemote {
				from, to = remote, local
			}
			if widget.OptionCard(c.Key(toRemote), d.toRemote == toRemote, from+" to "+to, "Copies what "+from+" has over to "+to) &&
				!d.running && d.toRemote != toRemote {
				d.toRemote, d.finished = toRemote, false
				if d.scans != nil {
					d.buildRows()
				}
			}
		}
	})
}

func (d *Dialog) list(c *ui.Context) {
	if len(d.rows) == 0 {
		ui.Text(c, "Nothing to sync: "+d.sourceName()+" has no skills, commands, memory files or MCP servers yet.").
			TextColor(theme.Muted).Padding(16, 0)
		return
	}
	same := 0
	for _, r := range d.rows {
		if r.status == statusSame {
			same++
		}
	}
	ui.Row(c).FillWidth().Gap(12).Children(func() {
		ui.Box(c).Grow(1).Children(func() {
			widget.SearchField(c, &d.query, "Search skills, MCP servers, commands…", icon.Search)
		})
		if same > 0 {
			widget.Switch(c, &d.showSame, "Show unchanged")
			ui.Text(c, fmt.Sprintf("Unchanged (%d)", same)).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
	})
	ui.Scroll(c).FillWidth().Grow(1).MinHeight(120).Children(func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			shown := 0
			for _, kind := range kinds {
				idx := d.visibleRows(kind)
				if len(idx) == 0 {
					continue
				}
				shown++
				d.section(c.Key(kind), kind, idx)
			}
			if shown == 0 {
				msg := "Everything is the same on both sides."
				if d.query != "" {
					msg = "Nothing matches."
				}
				ui.Text(c, msg).TextColor(theme.Muted).Padding(16, 4)
			}
		})
	})
}

func (d *Dialog) section(c *ui.Context, kind string, idx []int) {
	checked, eligible := 0, 0
	counts := map[string]int{}
	for _, i := range idx {
		r := d.rows[i]
		counts[r.status]++
		if r.checked {
			checked++
		}
		if selectable(r) {
			eligible++
		}
	}
	folded := d.collapsed[kind] && strings.TrimSpace(d.query) == ""
	widget.Card(c).FillWidth().Padding(0).Gap(0).Children(func() {
		head := ui.Row(c).FillWidth().Gap(10).Padding(10, 12)
		head.Children(func() {
			box := ui.ButtonBase(c).Label("Select all "+kindPlural(kind)).Size(16, 16).Radius(theme.RadiusTail).Center().
				Shrink(0).Cursor(ui.CursorPointer).Border(1.5, theme.BorderStrong)
			switch {
			case checked > 0 && checked >= eligible:
				box.Background(theme.Accent).BorderColor(theme.Accent).TextColor(theme.OnAccent).Children(func() { ui.Icon(c, icon.Check).FontSize(11) })
			case checked > 0:
				box.BorderColor(theme.Accent).Children(func() { ui.Box(c).Size(8, 2).Radius(theme.RadiusTail).Background(theme.Accent) })
			}
			if box.Clicked() && !d.running {
				all := checked > 0 && checked >= eligible
				for _, i := range idx {
					if all {
						d.rows[i].checked = false
					} else if selectable(d.rows[i]) {
						d.rows[i].checked = true
					}
				}
			}
			toggle := ui.ButtonBase(c).Grow(1).Gap(8).Cursor(ui.CursorPointer).Label(kindPlural(kind))
			toggle.Children(func() {
				chev := icon.ChevronDown
				if folded {
					chev = icon.ChevronRight
				}
				ui.Icon(c, chev).FontSize(12).TextColor(theme.Muted)
				ui.Text(c, kindPlural(kind)).FontSize(theme.TextUI)
				ui.Text(c, fmt.Sprint(len(idx))).FontSize(theme.TextLabel).TextColor(theme.Muted)
				ui.Box(c).Grow(1)
				if n := counts[statusNew]; n > 0 {
					widget.Tag(c, fmt.Sprintf("%d new", n), theme.Accent)
				}
				if n := counts[statusChanged]; n > 0 {
					widget.Tag(c, fmt.Sprintf("%d changed", n), theme.Warning)
				}
				ui.Text(c, fmt.Sprintf("%d selected", checked)).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			})
			if toggle.Clicked() {
				d.collapsed[kind] = !folded
			}
		})
		if folded {
			return
		}
		ui.Column(c).FillWidth().Gap(1).Padding(0, 4, 4, 4).Children(func() {
			for _, i := range idx {
				d.rowView(c.Key(d.rows[i].item.ID), &d.rows[i])
			}
		})
	})
}

func (d *Dialog) rowView(c *ui.Context, r *row) {
	line := ui.Row(c).FillWidth().Gap(10).Padding(8, 10).Radius(theme.RadiusControl).AlignItems(ui.Start)
	if r.checked {
		line.Background(theme.AccentSoft)
	}
	line.Children(func() {
		box := ui.CheckboxBase(c, &r.checked).Label(itemTitle(r.item)).Size(16, 16).Radius(theme.RadiusTail).Center().
			Margin(2, 0, 0, 0).Shrink(0).Cursor(ui.CursorPointer).Border(1.5, theme.BorderStrong)
		if r.checked {
			box.Background(theme.Accent).BorderColor(theme.Accent).TextColor(theme.OnAccent).Children(func() {
				ui.Icon(c, icon.Check).FontSize(11)
			})
		} else if box.Hovered() {
			box.BorderColor(theme.TextSoft)
		}
		if d.running {
			box.Disabled(true)
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Icon(c, agentIcon(r.item.Agent)).FontSize(12).TextColor(theme.TextSoft).Label(d.agentName(r.item.Agent))
				ui.Text(c, itemTitle(r.item)).SingleLine().Shrink(1)
				ui.Text(c, d.agentName(r.item.Agent)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
				ui.Box(c).Grow(1)
				switch r.status {
				case statusNew:
					widget.Tag(c, statusNew, theme.Accent)
				case statusChanged:
					widget.Tag(c, statusChanged, theme.Warning)
				default:
					widget.Tag(c, statusSame, theme.Muted)
				}
			})
			if r.item.Detail != "" {
				ui.Text(c, r.item.Detail).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			}
			if len(r.item.Secrets) > 0 {
				what := strings.Join(r.item.Secrets, ", ")
				msg, col := "Has secrets ("+what+"); tick it to send them to "+d.destName()+".", theme.Muted
				if r.checked {
					msg, col = "Sends "+what+" to "+d.destName()+".", theme.Warning
				}
				ui.Row(c).Gap(6).Children(func() {
					ui.Icon(c, icon.Lock).FontSize(11).TextColor(col)
					ui.Text(c, msg).FontSize(theme.TextLabel).TextColor(col)
				})
			}
			for _, w := range r.item.Warnings {
				ui.Text(c, w).FontSize(theme.TextLabel).TextColor(theme.Error)
			}
		})
	})
}

func (d *Dialog) footer(c *ui.Context) {
	switch {
	case d.running:
		ui.Text(c, fmt.Sprintf("Syncing %d of %d…", min(d.done+1, d.total), d.total)).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	case d.finished:
		summary := fmt.Sprintf("Synced %d of %d to %s.", d.total-len(d.failures), d.total, d.destName())
		ui.Text(c, summary+" Restart running agents there to pick up the changes.").FontSize(theme.TextLabel).TextColor(theme.Done)
		ui.Column(c).FillWidth().Gap(3).Children(func() {
			for _, f := range d.failures {
				ui.Text(c, f).FontSize(theme.TextLabel).TextColor(theme.Error)
			}
			for _, w := range d.warnings {
				ui.Text(c, w).FontSize(theme.TextLabel).TextColor(theme.Warning)
			}
		})
	default:
		ui.Text(c, "Agent logins aren't synced: sign in to each agent once on "+d.destName()+". Paths in your home folder move to theirs, and changed config files keep a .enowx.bak copy.").
			FontSize(11).TextColor(theme.Muted)
	}
	ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
		if widget.GhostButton(c, "Close").Clicked() && !d.running {
			d.Close()
		}
		n := len(d.checkedRows())
		label := fmt.Sprintf("Sync %d to %s", n, d.destName())
		if n == 1 {
			label = "Sync 1 item to " + d.destName()
		}
		b := widget.PrimaryButton(c, icon.Sync, label, "")
		if n == 0 || d.running || d.loading {
			b.Disabled(true).Opacity(0.5)
		} else if b.Clicked() {
			d.run()
		}
	})
}

func (d *Dialog) agentName(id string) string {
	switch id {
	case "shared":
		return "Shared"
	case "claude":
		return "Claude Code"
	case "codex":
		return "Codex"
	case "opencode":
		return "OpenCode"
	case "gemini":
		return "Gemini CLI"
	}
	return d.opts.AgentName(id)
}

func agentIcon(id string) *ui.SVG {
	if id == "shared" {
		return icon.Folder
	}
	return icon.Agent(id)
}

// itemTitle names an item for people: commands and subagents without
// their .md, the git identity as such.
func itemTitle(it proto.EnvItem) string {
	switch it.Kind {
	case proto.EnvGit:
		return "Git identity"
	case proto.EnvCommand:
		return "/" + strings.TrimSuffix(strings.TrimSuffix(it.Name, ".md"), ".toml")
	case proto.EnvSubagent:
		return strings.TrimSuffix(it.Name, ".md")
	}
	return it.Name
}

func kindPlural(kind string) string {
	switch kind {
	case proto.EnvSkill:
		return "Skills"
	case proto.EnvSubagent:
		return "Subagents"
	case proto.EnvCommand:
		return "Slash commands"
	case proto.EnvMemory:
		return "Memory files"
	case proto.EnvSettings:
		return "Settings"
	case proto.EnvMCP:
		return "MCP servers"
	case proto.EnvGit:
		return "Git"
	}
	return kind
}

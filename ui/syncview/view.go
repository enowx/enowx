package syncview

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// View draws the whole page in the app's frame. Back to hosts, while nothing
// runs, closes it for good (see Closed).
func (d *Dialog) View(c *ui.Context) {
	if d.closed {
		return
	}
	widget.PageHeader(c, "Sync environment", "Skills, subagents, commands, memory files, settings and MCP servers", func() { d.actions(c) })
	if d.loading || d.err != "" {
		ui.Column(c.Key("state")).Grow(1).FillWidth().Center().Gap(theme.S2).Padding(theme.S5).Children(func() {
			if d.err != "" {
				ui.Text(c, d.err).FontSize(theme.TextUI).TextColor(theme.Error).TextAlign(ui.Center)
				return
			}
			widget.Spinner(c, 18)
			ui.Text(c, "Reading both sides…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		})
		return
	}
	d.figures(c)
	d.tools(c)
	d.list(c)
	d.footer(c)
}

// Running reports whether a sync is under way, so the page stays open.
func (d *Dialog) Running() bool { return d.running }

func (d *Dialog) actions(c *ui.Context) {
	if widget.GhostButton(c, "Back to hosts").Clicked() {
		if d.running {
			d.opts.Back()
		} else {
			d.Close()
		}
	}
	d.direction(c)
	n := len(d.checkedRows())
	label := fmt.Sprintf("Sync %d to %s", n, d.destName())
	if n == 1 {
		label = "Sync 1 item to " + d.destName()
	}
	b := widget.PrimaryButton(c, nil, label, "").MaxWidth(280)
	if n == 0 || d.running || d.loading {
		b.Disabled(true)
	} else if b.Clicked() {
		d.run()
	}
}

// direction picks which way the copies go; it can't change while they do.
func (d *Dialog) direction(c *ui.Context) {
	local, remote := d.opts.Local.Name, d.opts.Host.Name
	way := func(toRemote bool) string {
		if toRemote {
			return local + " to " + remote
		}
		return remote + " to " + local
	}
	picked := d.toRemote
	sel := ui.SelectBase(c, &picked)
	widget.Trigger(sel.Trigger, sel.Open()).Width(200).Label("Direction").Disabled(d.running).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, way(d.toRemote), widget.RunStyle{}) })
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	sel.Popup(func(panel ui.Element) {
		widget.Popup(panel.Margin(theme.S1, 0, 0, 0).Width(260))
		for _, toRemote := range []bool{true, false} {
			item := sel.Item(toRemote)
			widget.PopupItem(item, item.Highlighted()).Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, way(toRemote), widget.RunStyle{}) })
			})
		}
	})
	if picked != d.toRemote && !d.running {
		d.toRemote, d.finished = picked, false
		if d.scans != nil {
			d.buildRows()
		}
	}
}

func (d *Dialog) figures(c *ui.Context) {
	counts := map[string]int{}
	for _, r := range d.rows {
		counts[r.status]++
	}
	widget.FigsBand(c, []widget.Fig{
		{Label: "New", Value: strconv.Itoa(counts[statusNew])},
		{Label: "Changed", Value: strconv.Itoa(counts[statusChanged])},
		{Label: "Unchanged", Value: strconv.Itoa(counts[statusSame])},
		{Label: "Selected", Value: strconv.Itoa(len(d.checkedRows()))},
	})
}

// tools are the search and whether what is the same shows.
func (d *Dialog) tools(c *ui.Context) {
	if len(d.rows) == 0 {
		return
	}
	same := 0
	for _, r := range d.rows {
		if r.status == statusSame {
			same++
		}
	}
	ui.Row(c.Key("tools")).FillWidth().Shrink(0).Padding(theme.S4, theme.S5, 0, theme.S5).Gap(theme.S3).Children(func() {
		ui.Box(c).Width(320).Shrink(1).MinWidth(0).Children(func() {
			widget.SearchField(c, &d.query, "Search skills, MCP servers, commands…", icon.Search)
		})
		ui.Spacer(c)
		if same > 0 {
			ui.Text(c, fmt.Sprintf("Show unchanged (%d)", same)).FontSize(theme.TextUI).SingleLine().Shrink(0)
			widget.Switch(c, &d.showSame, "Show unchanged")
		}
	})
}

// list is a card a kind, those with something to show.
func (d *Dialog) list(c *ui.Context) {
	if len(d.rows) == 0 {
		ui.Column(c.Key("nothing")).Grow(1).FillWidth().Center().Padding(theme.S5).Children(func() {
			ui.Text(c, "Nothing to sync: "+d.sourceName()+" has no skills, commands, memory files or MCP servers yet.").FontSize(theme.TextUI).TextColor(theme.TextSoft).TextAlign(ui.Center)
		})
		return
	}
	var shown []string
	idx := map[string][]int{}
	for _, kind := range kinds {
		if rows := d.visibleRows(kind); len(rows) > 0 {
			shown = append(shown, kind)
			idx[kind] = rows
		}
	}
	if len(shown) == 0 {
		msg := "Everything is the same on both sides."
		if d.query != "" {
			msg = "Nothing matches."
		}
		ui.Column(c.Key("nothing")).Grow(1).FillWidth().Center().Padding(theme.S5).Children(func() {
			ui.Text(c, msg).FontSize(theme.TextUI).TextColor(theme.TextSoft)
		})
		return
	}
	widget.FillGrid(c, 3, len(shown), sectionMinHeight, func() {
		for _, kind := range shown {
			d.section(c.Key(kind), kind, idx[kind])
		}
	})
}

// sectionMinHeight keeps a few rows of a kind in sight; the others scroll
// inside its card.
const sectionMinHeight = 200

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
	widget.GridCard(c).Children(func() {
		ui.Row(c.Key("head")).FillWidth().Shrink(0).MinHeight(24).Gap(theme.S2).Children(func() {
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
			toggle := ui.ButtonBase(c).Grow(1).MinWidth(0).Gap(theme.S2).Justify(ui.Start).Cursor(ui.CursorPointer).Label(kindPlural(kind))
			toggle.Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, kindPlural(kind), widget.RunStyle{Size: theme.TextRead, Weight: 600})
				})
				if n := counts[statusNew]; n > 0 {
					widget.Tag(c, fmt.Sprintf("%d new", n), theme.Working)
				}
				if n := counts[statusChanged]; n > 0 {
					widget.Tag(c, fmt.Sprintf("%d changed", n), theme.Warning)
				}
				chev := icon.ChevronDown
				if folded {
					chev = icon.ChevronRight
				}
				ui.Icon(c, chev).FontSize(12).TextColor(theme.TextSoft)
			})
			if toggle.Clicked() {
				d.collapsed[kind] = !folded
			}
		})
		if folded {
			widget.CardNote(c.Key("folded"), fmt.Sprintf("%d items, %d selected", len(idx), checked), 1)
			return
		}
		widget.Scroll(c.Key("rows")).Grow(1).FillWidth().MinHeight(0).Children(func() {
			widget.CardLines(c, func() {
				for n, i := range idx {
					d.rowView(c.Key(d.rows[i].item.ID), &d.rows[i], n == 0)
				}
			})
		})
	})
}

// rowView is an item: its name, its agent and what it holds, how it compares,
// and its switch. An item the same on both sides shows its switch only under
// the pointer or while ticked: it's rarely sent.
func (d *Dialog) rowView(c *ui.Context, r *row, first bool) {
	line := ui.Column(c).FillWidth().Shrink(0).MinHeight(32).Padding(theme.S1, 0).Gap(theme.S1)
	if !first {
		line.BorderWidth(theme.Rule, 0, 0, 0).BorderColor(theme.Border)
	}
	hovered := line.Hovered()
	line.Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, itemTitle(r.item), widget.RunStyle{Size: theme.TextLabel})
				sub := d.agentName(r.item.Agent)
				if r.item.Detail != "" {
					sub += " · " + r.item.Detail
				}
				col := theme.TextSoft
				if len(r.item.Secrets) > 0 {
					what := strings.Join(r.item.Secrets, ", ")
					sub += " · Has secrets (" + what + ")"
					if r.checked {
						sub, col = d.agentName(r.item.Agent)+" · Sends "+what+" to "+d.destName(), theme.Warning
					}
				}
				widget.Running(c.Key("sub"), sub, widget.RunStyle{Size: theme.TextLabel, Color: col})
			})
			switch r.status {
			case statusNew:
				widget.StatusLine(c, theme.Working, false, statusNew)
			case statusChanged:
				widget.StatusLine(c, theme.Warning, false, statusChanged)
			default:
				widget.StatusLine(c, theme.Done, false, statusSame)
			}
			if d.running {
				ui.Box(c).Opacity(0.5).Shrink(0).Children(func() {
					on := r.checked
					widget.Switch(c, &on, "Sync "+itemTitle(r.item))
				})
			} else if r.status != statusSame || r.checked || hovered {
				widget.Switch(c, &r.checked, "Sync "+itemTitle(r.item))
			}
		})
		for _, w := range r.item.Warnings {
			ui.Text(c, w).FontSize(theme.TextLabel).TextColor(theme.Error).FillWidth().MinWidth(0)
		}
	})
}

// footer says how the sync goes, or what doesn't go with it.
func (d *Dialog) footer(c *ui.Context) {
	ui.Column(c.Key("foot")).FillWidth().Shrink(0).MaxHeight(120).Padding(0, theme.S5, theme.S4, theme.S5).Gap(theme.S1).Children(func() {
		switch {
		case d.running:
			ui.Text(c, fmt.Sprintf("Syncing %d of %d…", min(d.done+1, d.total), d.total)).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		case d.finished:
			summary := fmt.Sprintf("Synced %d of %d to %s.", d.total-len(d.failures), d.total, d.destName())
			ui.Text(c, summary+" Restart running agents there to pick up the changes.").FontSize(theme.TextLabel).TextColor(theme.Done).Shrink(0)
			widget.Scroll(c.Key("what-went")).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
					for _, f := range d.failures {
						ui.Text(c, f).FontSize(theme.TextLabel).TextColor(theme.Error)
					}
					for _, w := range d.warnings {
						ui.Text(c, w).FontSize(theme.TextLabel).TextColor(theme.Warning)
					}
				})
			})
		default:
			ui.Text(c, "Agent logins aren't synced: sign in to each agent once on "+d.destName()+". Paths in your home folder move to theirs, and changed config files keep a .enowx.bak copy.").
				FontSize(theme.TextLabel).TextColor(theme.TextSoft).FillWidth().MinWidth(0)
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
		return "Git identity"
	}
	return kind
}

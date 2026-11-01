package ui

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A team's kit (docs/cloud.md): the skills, commands and subagents it
// shares, taken from the connected host's agents, in the team's dialog.

// hostTeamKit is the connected host's teams' kits, as last listed, and
// its agents' items to add from. It belongs to the main thread.
type hostTeamKit struct {
	host                       string
	list                       []proto.TeamKitItem
	items                      []proto.EnvItem // the host's agents' skills, commands, subagents
	adding                     bool            // the host's items show, to add
	filter                     string          // narrows the host's items
	templateName, templateRepo string          // a template to add (teamkitdefs.go)
	err                        string
}

func (a *App) teamKitOf() *hostTeamKit {
	if a.teamKitList == nil || a.teamKitList.host != a.host.ID {
		a.teamKitList = &hostTeamKit{host: a.host.ID}
		a.loadTeamKit()
	}
	return a.teamKitList
}

func (a *App) loadTeamKit() {
	tk, cl := a.teamKitList, a.client
	if tk == nil || cl == nil {
		return
	}
	go func() {
		list, err := cl.TeamKit()
		if err != nil {
			return
		}
		a.onMain(func() { tk.list = list })
	}()
}

// teamKitEvent takes the kits as they changed.
func (a *App) teamKitEvent(data json.RawMessage) func() {
	var list []proto.TeamKitItem
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	return func() {
		if tk := a.teamKitList; tk != nil && tk.host == a.host.ID {
			tk.list = list
		}
	}
}

// cloudTeamKit is the team dialog's Kit tab.
func (a *App) cloudTeamKit(c *ui.Context, cs *cloudSettings) {
	tk := a.teamKitOf()
	team := cs.teams.team
	can := team.Role != "viewer"
	if tk.err != "" {
		widget.Notice(c, icon.Alert, theme.Error, tk.err, "", nil)
	}
	var mine []proto.TeamKitItem
	for _, it := range tk.list {
		if it.Team == team.ID && (it.Kind == proto.EnvSkill || it.Kind == proto.EnvCommand || it.Kind == proto.EnvSubagent) {
			mine = append(mine, it)
		}
	}
	ui.Text(c, "Skills, commands and subagents the team shares: every session in its projects is told of them, whatever its agent.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	if len(mine) == 0 {
		ui.Text(c, "Nothing shared yet.").FontSize(theme.TextLabel).TextColor(theme.Muted)
	}
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		for _, it := range mine {
			ui.Row(c.Key("kit:"+it.Kind+":"+it.Name)).FillWidth().Gap(10).AlignItems(ui.Center).Padding(8, 10).Radius(theme.RadiusControl).Background(theme.Sunken).Children(func() {
				widget.Tag(c, it.Kind, theme.TextSoft)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, it.Name).FontSize(theme.TextUI).SingleLine()
					if it.About != "" {
						ui.Text(c, it.About).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
					}
				})
				if can && widget.IconButton(c, icon.Close, "Take "+it.Name+" out of the kit", false).Clicked() {
					item := it
					a.confirm("Take "+item.Name+" out of the team's kit?", "Sessions in the team's projects stop being told of it. Your agents' own copy stays.", "Take it out", func() {
						cl := a.client
						go func() {
							if err := cl.RemoveFromTeamKit(item.Team, item.Kind, item.Name); err != nil {
								a.onMain(func() { tk.err = client.Explain(err) })
							}
						}()
					})
				}
			})
		}
	})
	a.kitDefinitions(c, cs)
	if !can {
		return
	}
	if !tk.adding {
		if widget.Button(c, widget.Secondary, icon.Plus, "Share one of "+a.host.Name+"'s agents' skills…", "").Clicked() {
			tk.adding = true
			cl := a.client
			go func() {
				scan, err := cl.EnvScan()
				a.onMain(func() {
					if err != nil {
						tk.err = client.Explain(err)
						return
					}
					tk.items = nil
					for _, it := range scan.Items {
						if it.Kind == proto.EnvSkill || it.Kind == proto.EnvCommand || it.Kind == proto.EnvSubagent {
							tk.items = append(tk.items, it)
						}
					}
				})
			}()
		}
		return
	}
	widget.Field(c, a.host.Name+"'s agents", func() {
		if len(tk.items) == 0 {
			ui.Text(c, "No skills, commands or subagents here.").FontSize(theme.TextLabel).TextColor(theme.Muted)
			return
		}
		widget.SearchField(c, &tk.filter, "Filter skills, commands, subagents", icon.Search)
		q := strings.ToLower(strings.TrimSpace(tk.filter))
		shown := 0
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			for _, it := range tk.items {
				if q != "" && !strings.Contains(strings.ToLower(it.Name+" "+it.Kind+" "+a.agentName(it.Agent)), q) {
					continue
				}
				shown++
				shared := slices.ContainsFunc(mine, func(k proto.TeamKitItem) bool { return k.Kind == it.Kind && k.Name == it.Name })
				ui.Row(c.Key("host:"+it.ID)).FillWidth().Gap(8).AlignItems(ui.Center).Padding(4, 6).Children(func() {
					widget.Tag(c, it.Kind, theme.TextSoft)
					ui.Text(c, it.Name).FontSize(theme.TextUI).Grow(1).Shrink(1).MinWidth(0).SingleLine()
					ui.Text(c, a.agentName(it.Agent)).FontSize(theme.TextLabel).TextColor(theme.Muted)
					switch {
					case shared:
						widget.Tag(c, "shared", theme.Done)
					case len(it.Secrets) > 0:
						widget.Tag(c, "holds secrets", theme.Warning)
					default:
						if widget.Button(c, widget.Secondary, nil, "Share", "").Clicked() {
							cl, team, id := a.client, team.ID, it.ID
							go func() {
								_, err := cl.AddToTeamKit(team, id)
								a.onMain(func() {
									tk.err = ""
									if err != nil {
										tk.err = client.Explain(err)
									}
								})
							}()
						}
					}
				})
			}
		})
		if shown == 0 {
			ui.Text(c, "Nothing matches “"+tk.filter+"”.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
	})
}

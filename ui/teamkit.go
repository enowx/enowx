package ui

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
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
	pageFilter                 string          // narrows the Team page's cards
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
	widget.CardNote(c, "Skills, commands and subagents the team shares: every session in its projects is told of them, whatever its agent.", 0)
	if len(mine) == 0 {
		widget.CardNote(c.Key("none"), "Nothing shared yet.", 0)
	}
	ui.Column(c.Key("mine")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		for _, it := range mine {
			ui.Row(c.Key("kit:"+it.Kind+":"+it.Name)).FillWidth().Shrink(0).Gap(theme.S2).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).Background(theme.Raised).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					widget.Running(c, it.Name, widget.RunStyle{Weight: 500})
					sub := roleName(it.Kind)
					if it.About != "" {
						sub += " · " + it.About
					}
					widget.Running(c.Key("sub"), sub, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				})
				if can && widget.Button(c, widget.Secondary, nil, "Take it out", "").Tooltip("Take "+it.Name+" out of the kit").Clicked() {
					a.confirmTakeKitOut(it)
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
			a.readKitItems()
		}
		return
	}
	widget.Field(c, a.host.Name+"'s agents", func() { a.kitShareList(c, team) })
}

// confirmTakeKitOut asks before an item leaves the team's kit.
func (a *App) confirmTakeKitOut(it proto.TeamKitItem) {
	tk := a.teamKitOf()
	about := "Sessions in the team's projects stop being told of it. Your agents' own copy stays."
	if it.Kind != proto.EnvSkill && it.Kind != proto.EnvCommand && it.Kind != proto.EnvSubagent {
		about = "Members stop seeing it. What they set up from it stays theirs."
	}
	a.confirm("Take "+it.Name+" out of the team's kit?", about, "Take it out", func() {
		cl := a.client
		go func() {
			if err := cl.RemoveFromTeamKit(it.Team, it.Kind, it.Name); err != nil {
				a.onMain(func() { tk.err = client.Explain(err) })
			}
		}()
	})
}

// readKitItems reads what the host's agents have to share.
func (a *App) readKitItems() {
	tk, cl := a.teamKitOf(), a.client
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

// kitShareList is the host's agents' items, to share with team.
func (a *App) kitShareList(c *ui.Context, team api.Team) {
	tk := a.teamKitOf()
	if len(tk.items) == 0 {
		widget.CardNote(c, "No skills, commands or subagents here.", 0)
		return
	}
	shared := func(it proto.EnvItem) bool {
		return slices.ContainsFunc(tk.list, func(k proto.TeamKitItem) bool { return k.Team == team.ID && k.Kind == it.Kind && k.Name == it.Name })
	}
	widget.SearchField(c, &tk.filter, "Filter skills, commands, subagents", icon.Search)
	q := strings.ToLower(strings.TrimSpace(tk.filter))
	shown := 0
	ui.Column(c.Key("items")).FillWidth().Shrink(0).Children(func() {
		for _, it := range tk.items {
			if q != "" && !strings.Contains(strings.ToLower(it.Name+" "+it.Kind+" "+a.agentName(it.Agent)), q) {
				continue
			}
			shown++
			ui.Row(c.Key("host:"+it.ID)).FillWidth().Shrink(0).MinHeight(40).Gap(theme.S2).BorderWidth(theme.Rule, 0, 0, 0).BorderColor(theme.Border).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					widget.Running(c, it.Name, widget.RunStyle{})
					widget.Running(c.Key("sub"), roleName(it.Kind)+" · "+a.agentName(it.Agent), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
				})
				switch {
				case shared(it):
					widget.Chip(c, "shared", theme.Done)
				case len(it.Secrets) > 0:
					widget.Chip(c, "holds secrets", theme.Warning)
				default:
					if widget.Button(c, widget.Secondary, nil, "Share", "").Clicked() {
						cl, id, item := a.client, team.ID, it.ID
						go func() {
							_, err := cl.AddToTeamKit(id, item)
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
		widget.CardNote(c.Key("no-match"), "Nothing matches “"+tk.filter+"”.", 0)
	}
}

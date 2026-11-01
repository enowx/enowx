package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Hosts shared with a team (docs/cloud.md, Hosts and sessions): the
// connected host's share, picked in the team's Hosts tab, and the hosts
// the team's members share, in the rail. Opening one connects to it over
// Tailscale as a visitor, with a ticket the cloud gives this member for
// it, and shows its shared projects and their sessions.

// pageTeamHost is a team's shared host, as a visitor sees it.
const pageTeamHost = "teamhost"

// hostTeamHosts is the connected host's share and its teams' hosts, as
// last read. It belongs to the main thread.
type hostTeamHosts struct {
	host  string
	share proto.HostShare
	hosts []proto.TeamHost
	err   string
}

func (a *App) teamHostsOf() *hostTeamHosts {
	if a.teamHostList == nil || a.teamHostList.host != a.host.ID {
		a.teamHostList = &hostTeamHosts{host: a.host.ID}
		a.loadTeamHosts()
	}
	return a.teamHostList
}

func (a *App) loadTeamHosts() {
	th, cl := a.teamHostList, a.client
	if th == nil || cl == nil {
		return
	}
	go func() {
		hosts, err := cl.TeamHosts()
		share, serr := cl.HostShare()
		a.onMain(func() {
			if err == nil {
				th.hosts = hosts
			}
			if serr == nil {
				th.share = share
			}
		})
	}()
}

// teamHostsEvent takes the teams' hosts as a sync changed them.
func (a *App) teamHostsEvent(data json.RawMessage) func() {
	var list []proto.TeamHost
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	return func() {
		if th := a.teamHostList; th != nil && th.host == a.host.ID {
			th.hosts = list
		}
	}
}

// cloudTeamHosts is the team dialog's Hosts tab: what the connected host
// shares with the team, and the hosts its members share.
func (a *App) cloudTeamHosts(c *ui.Context, cs *cloudSettings) {
	th := a.teamHostsOf()
	team := cs.teams.team
	if th.err != "" {
		widget.Notice(c, icon.Alert, theme.Error, th.err, "", nil)
	}
	widget.Field(c, "Shared by "+a.host.Name, func() {
		if team.Role == api.RoleViewer {
			ui.Text(c, "Viewers don't share hosts.").FontSize(theme.TextLabel).TextColor(theme.Muted)
			return
		}
		var shared []string
		var allow *bool
		for _, t := range th.share.Teams {
			if t.Team == team.ID {
				shared, allow = t.Projects, t.AllowSessions
			}
		}
		ui.Text(c, "The projects picked show to the team: their sessions and files. Members reach "+a.host.Name+" over Tailscale: share its node with their Tailscale accounts.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		switch {
		case th.share.Err != "" && len(shared) > 0:
			widget.Notice(c, icon.Alert, theme.Warning, th.share.Err, "", nil)
		case th.share.Addr != "" && len(shared) > 0:
			ui.Text(c, "Members reach it at "+th.share.Addr).FontSize(theme.TextLabel).TextColor(theme.Done)
		}
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			if len(a.projects) == 0 {
				ui.Text(c, a.host.Name+" has no projects yet.").FontSize(theme.TextLabel).TextColor(theme.Muted)
			}
			for _, p := range a.projects {
				on := slices.Contains(shared, p.Path)
				ui.Row(c.Key("share:"+p.Path)).FillWidth().Gap(8).AlignItems(ui.Center).Padding(3, 0).Children(func() {
					if widget.Checkbox(c, &on, p.Name) {
						next := slices.DeleteFunc(slices.Clone(shared), func(x string) bool { return x == p.Path })
						if on {
							next = append(next, p.Path)
						}
						a.shareHost(team.ID, team.Name, next)
					}
					ui.Text(c, textfmt.TildePath(p.Path, a.daemonInfo.Home)).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
				})
			}
		})
		if len(shared) > 0 && allow != nil {
			// A daemon that knows the setting says it; an older one doesn't.
			a.teamSessionsSwitch(c, team.ID, team.Name, shared, *allow)
		}
	})
	widget.Field(c, "Shared by members", func() {
		var list []proto.TeamHost
		for _, h := range th.hosts {
			if h.Team == team.ID {
				list = append(list, h)
			}
		}
		if len(list) == 0 {
			ui.Text(c, "No member shares a host yet.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		for _, h := range list {
			ui.Row(c.Key("th:"+h.Device)).FillWidth().Gap(10).AlignItems(ui.Center).Padding(8, 10).Radius(theme.RadiusControl).Background(theme.Sunken).Children(func() {
				ui.Icon(c, icon.Server).FontSize(14).TextColor(theme.TextSoft)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, h.Name).FontSize(theme.TextUI).SingleLine()
					ui.Text(c, fmt.Sprintf("%s's · %s", h.Owner, textfmt.Plural(len(h.Projects), "1 project", fmt.Sprintf("%d projects", len(h.Projects))))).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
				})
				if widget.Button(c, widget.Secondary, nil, "Open", "").Clicked() {
					cs.teams.open = ""
					a.openTeamHost(h)
				}
			})
		}
	})
}

// shareHost shares projects of the connected host with team, after a
// confirmation the first time.
func (a *App) shareHost(team, name string, projects []string) {
	th := a.teamHostsOf()
	do := func() { a.setTeamShare(proto.HostShareTeam{Team: team, Projects: projects}) }
	sharing := slices.ContainsFunc(th.share.Teams, func(t proto.HostShareTeam) bool { return t.Team == team })
	if sharing || len(projects) == 0 {
		do()
		return
	}
	a.confirm("Share "+a.host.Name+" with "+name+"?",
		"The team's members see the projects you pick here, their sessions and files, reaching "+a.host.Name+" over Tailscale. Each signs in with a ticket the cloud gives them; take a project back here any time.",
		"Share", do)
}

// setTeamShare sets what the connected host shares with a team.
func (a *App) setTeamShare(p proto.HostShareTeam) {
	th, cl := a.teamHostsOf(), a.client
	go func() {
		share, err := cl.SetHostShareTeam(p)
		a.onMain(func() {
			th.err = ""
			if err != nil {
				th.err = strings.TrimPrefix(client.Explain(err), "teams: ")
				return
			}
			th.share = share
		})
	}()
}

// teamSessionsWarning is what letting members start sessions means.
const teamSessionsWarning = "Members' sessions run as you on this host: an agent there can do what you can. Turn on only for people you trust with this machine."

// teamSessionsSwitch lets the team's members start sessions of their own
// in the shared projects, or not; turning it on asks first.
func (a *App) teamSessionsSwitch(c *ui.Context, team, name string, shared []string, allow bool) {
	ui.Column(c.Key("team-sessions")).FillWidth().Gap(4).Padding(6, 0, 0, 0).Children(func() {
		on := allow
		if widget.Switch(c, &on, "Members start sessions here") {
			set := func() { a.setTeamShare(proto.HostShareTeam{Team: team, Projects: shared, AllowSessions: &on}) }
			if !on {
				set()
			} else {
				a.confirm("Let "+name+"'s members start sessions on "+a.host.Name+"?", teamSessionsWarning, "Let them", set)
			}
		}
		ui.Text(c, teamSessionsWarning).FontSize(theme.TextLabel).TextColor(theme.Warning)
	})
}

// teamVisit is a shared host the app visits: its connection, who the host
// let in, and what it shows them. It belongs to the main thread.
type teamVisit struct {
	host     proto.TeamHost
	cl       *client.Client
	me       proto.Visitor
	projects []proto.ProjectInfo
	sessions []proto.SessionInfo
	busy     string
	err      string
	read     time.Time
	watch    *watchPane // the session watched, if any (takeover.go)
}

// openTeamHost connects to a shared host as a visitor, and shows it.
func (a *App) openTeamHost(h proto.TeamHost) {
	a.closeVisit()
	v := &teamVisit{host: h, busy: "Connecting to " + h.Name + "…"}
	a.visit, a.page = v, pageTeamHost
	cl := a.client
	go func() {
		vc, me, err := visitHost(cl, h, func(ev proto.Event) { a.visitEvent(v, ev) })
		a.onMain(func() {
			if a.visit != v {
				if vc != nil {
					vc.Close() // left meanwhile
				}
				return
			}
			v.busy = ""
			if err != nil {
				v.err = err.Error()
				return
			}
			v.cl, v.me = vc, me
			a.refreshVisit(v)
		})
	}()
}

// visitHost asks the cloud, through the connected host, for a ticket into
// h, connects to it and hands it over.
func visitHost(cl *client.Client, h proto.TeamHost, onEvent func(proto.Event)) (*client.Client, proto.Visitor, error) {
	var tk api.HostTicket
	if err := cl.CloudCall("POST", teamPath(api.PathHostTickets, h.Team), api.HostTicketParams{Host: h.Device}, &tk); err != nil {
		return nil, proto.Visitor{}, fmt.Errorf("a ticket into %s: %s", h.Name, client.Explain(err))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	nc, err := (&net.Dialer{}).DialContext(ctx, "tcp", h.Addr)
	if err != nil {
		return nil, proto.Visitor{}, fmt.Errorf("can't reach %s at %s: is its node shared with your Tailscale account, and Tailscale on here? (%v)", h.Name, h.Addr, err)
	}
	vc, err := client.New(nc, onEvent)
	if err != nil {
		nc.Close()
		return nil, proto.Visitor{}, err
	}
	me, err := vc.TeamAuth(tk.Ticket)
	if err != nil {
		vc.Close()
		return nil, proto.Visitor{}, errors.New(h.Name + " didn't let you in: " + client.Explain(err))
	}
	return vc, me, nil
}

// refreshVisit reads the shared host's projects and sessions again.
func (a *App) refreshVisit(v *teamVisit) {
	if v.cl == nil {
		return
	}
	v.read = time.Now()
	go func() {
		projects, perr := v.cl.Projects()
		sessions, serr := v.cl.Sessions()
		a.onMain(func() {
			if perr != nil || serr != nil {
				v.err = client.Explain(errors.Join(perr, serr))
				return
			}
			v.err, v.projects, v.sessions = "", projects, sessions
		})
	}()
}

// closeVisit leaves the shared host visited, if any.
func (a *App) closeVisit() {
	a.stopWatching(a.visit)
	if a.visit != nil && a.visit.cl != nil {
		a.visit.cl.Close()
	}
	a.visit = nil
}

// teamHostPage shows a shared host's projects and their sessions.
func (a *App) teamHostPage(c *ui.Context) {
	v := a.visit
	if v == nil {
		a.page = ""
		return
	}
	// A visitor gets no events: the page reads again now and then.
	if v.cl != nil && time.Since(v.read) > 3*time.Second {
		a.refreshVisit(v)
	}
	c.After(3 * time.Second)
	a.viewHeader(c, func() {
		widget.Breadcrumb(c, a.spaceName(v.host.Team), v.host.Name)
		ui.Spacer(c)
		if v.me.Role != "" {
			widget.Tag(c, "as "+v.me.Role, theme.TextSoft)
		}
		if widget.GhostButton(c, "Leave").Clicked() {
			a.closeVisit()
			a.page = ""
		}
	})
	if v.watch != nil {
		a.watchView(c, v)
		return
	}
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(20, 24).Gap(16).Children(func() {
			ui.Text(c, v.host.Owner+" shares "+v.host.Name+" with "+a.spaceName(v.host.Team)+": these projects and their sessions.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			if v.busy != "" {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					widget.Spinner(c, 12)
					ui.Text(c, v.busy).FontSize(theme.TextLabel).TextColor(theme.Muted)
				})
			}
			if v.err != "" {
				widget.Notice(c, icon.Alert, theme.Error, v.err, "", nil)
			}
			for _, p := range v.projects {
				if v.me.Role != "viewer" {
					ui.Row(c.Key("vpn:" + p.Path)).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Spacer(c)
						for _, ag := range []struct{ id, name string }{{"claude", "Claude Code"}, {"codex", "Codex"}} {
							if widget.Button(c.Key(p.Path+":"+ag.id), widget.Secondary, icon.Plus, ag.name+" session", "").
								Tooltip("A session of yours in " + p.Name + ", as your own " + ag.name + " account on " + v.host.Name).Clicked() {
								a.startVisitSession(v, p, ag.id)
							}
						}
					})
				}
				widget.Field(c.Key("vp:"+p.Path), p.Name, func() {
					var in []proto.SessionInfo
					for _, s := range v.sessions {
						if s.Project == p.Path || strings.HasPrefix(s.Dir, p.Path) {
							in = append(in, s)
						}
					}
					if len(in) == 0 {
						ui.Text(c, "No session now.").FontSize(theme.TextLabel).TextColor(theme.Muted)
					}
					for _, s := range in {
						ui.Row(c.Key("vs:"+s.ID)).FillWidth().Gap(10).AlignItems(ui.Center).Padding(8, 10).Radius(theme.RadiusControl).Background(theme.Sunken).Children(func() {
							widget.Badge(c, sessionBadge(s))
							ui.Text(c, nonEmptyName(s)).FontSize(theme.TextUI).Grow(1).Shrink(1).MinWidth(0).SingleLine()
							ui.Text(c, a.agentName(s.Agent)).FontSize(theme.TextLabel).TextColor(theme.Muted)
							if s.Branch != "" {
								widget.Tag(c, s.Branch, theme.TextSoft)
							}
							if !s.Exited && widget.Button(c, widget.Secondary, icon.Eye, "Watch", "").Clicked() {
								a.watchSession(v, s)
							}
						})
					}
				})
			}
		})
	})
}

func nonEmptyName(s proto.SessionInfo) string {
	if s.Name != "" {
		return s.Name
	}
	return s.ID
}

// sessionBadge is a session's state as a badge.
func sessionBadge(s proto.SessionInfo) widget.Status {
	switch {
	case s.Exited:
		return widget.Status{Label: "ended", Color: theme.Muted, Hollow: true}
	case s.State == proto.StateWaiting:
		return widget.Status{Label: "waiting for input", Color: theme.Warning}
	case s.State == proto.StateWorking:
		return widget.Status{Label: "working", Color: theme.Accent}
	}
	return widget.Status{Label: "idle", Color: theme.Done, Hollow: true}
}

// startVisitSession starts a session of the visitor's own in a shared
// project, and watches it: theirs, they type into it.
func (a *App) startVisitSession(v *teamVisit, p proto.ProjectInfo, agentID string) {
	cl := v.cl
	v.busy = "Starting a session in " + p.Name + "…"
	go func() {
		info, err := cl.CreateSession(proto.SessionCreateParams{Agent: agentID, Project: p.Path, Cols: 120, Rows: 40})
		a.onMain(func() {
			v.busy = ""
			if err != nil {
				v.err = client.Explain(err)
				return
			}
			v.sessions = append(v.sessions, info)
			a.watchSession(v, info)
			if v.watch != nil {
				v.watch.controls = true // their own
			}
		})
	}()
}

// sharedProject says the connected host shares project with a team.
func (a *App) sharedProject(project string) bool {
	// As last read: a menu starts no reads.
	th := a.teamHostList
	if project == "" || th == nil || th.host != a.host.ID {
		return false
	}
	for _, t := range th.share.Teams {
		if slices.Contains(t.Projects, project) {
			return true
		}
	}
	return false
}

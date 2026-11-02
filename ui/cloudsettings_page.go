package ui

import (
	"fmt"
	"slices"
	"sort"
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

// Settings › Cloud's page: the host's sign-in, then its teams or its devices.
// The window is at least 1200 wide, so the frame always has room for three cards.

// cloudRecord is a line of a team's log, for Latest in your teams.
type cloudRecord struct {
	team  string
	entry api.AuditEntry
}

// cloudLatestShown is how many of the teams' logs' lines the page shows.
const cloudLatestShown = 4

// cloudTeamsInFull reads each team's members, and the logs the user may read
// (those of the teams they're an admin of), newest first. A team that can't be
// read just shows less: its card has what the list said.
func cloudTeamsInFull(cl *client.Client, teams []api.Team) (map[string]api.Team, []cloudRecord) {
	full := make(map[string]api.Team, len(teams))
	var latest []cloudRecord
	for _, t := range teams {
		var one api.Team
		if cl.CloudCall("GET", teamPath(api.PathTeam, t.ID), nil, &one) == nil {
			full[t.ID] = one
		}
		if !canAdmin(t.Role) {
			continue
		}
		var log []api.AuditEntry
		if cl.CloudCall("GET", teamPath(api.PathAudit, t.ID), nil, &log) == nil {
			for _, e := range log {
				latest = append(latest, cloudRecord{team: t.Name, entry: e})
			}
		}
	}
	sort.SliceStable(latest, func(i, j int) bool { return latest[i].entry.At.After(latest[j].entry.At) })
	return full, latest[:min(len(latest), cloudLatestShown)]
}

func (a *App) cloudSection(c *ui.Context) {
	cs := a.cloudOf()
	var tabs func()
	if cs.loaded && cs.st.SignedIn {
		tabs = func() {
			labels := []string{fmt.Sprintf("%s (%d)", cloudTabs[0], len(cs.teams.list)), fmt.Sprintf("%s (%d)", cloudTabs[1], cloudDeviceCount(cs.devices))}
			if i := widget.Segmented(c, labels, cs.tab, nil); i >= 0 {
				cs.tab = i
			}
		}
	}
	widget.PageHeader(c, "Cloud", a.host.Name+"'s sign-in", tabs)
	switch {
	case !cs.loaded:
		widget.PageBody(c, true, func() {
			cloudErrAndBusy(c, cs)
			ui.Text(c, "Asking "+a.host.Name+"…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		})
	case !cs.st.SignedIn:
		a.cloudSignedOut(c, cs)
	default:
		widget.FigsBand(c, a.cloudFigures(cs))
		if cs.tab == 1 {
			a.cloudDevicesTab(c, cs)
		} else {
			a.cloudTeamsTab(c, cs)
		}
		a.recoverySheets(c, cs)
		a.cloudTeamDialogs(c, cs)
	}
}

// cloudErrAndBusy says what went wrong and what is under way.
func cloudErrAndBusy(c *ui.Context, cs *cloudSettings) {
	if cs.err != "" {
		widget.Notice(c, icon.Alert, theme.Error, cs.err, "", nil)
	}
	if cs.busy != "" {
		ui.Row(c.Key("busy")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
			widget.Spinner(c, 14)
			ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, cs.busy, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft}) })
		})
	}
}

// cloudSignedOut is the page before signing in, or while a sign-in waits.
func (a *App) cloudSignedOut(c *ui.Context, cs *cloudSettings) {
	widget.PageBodyGrid(c, 2, 2, 180, func() { cloudErrAndBusy(c, cs) }, func() {
		widget.GridCard(c.Key("sign-in")).Children(func() {
			if p := cs.st.Pending; p != nil {
				// The sign-in dialog shows the code; here, a line of it.
				widget.CardTitle(c, nil, "Signing in", nil)
				ui.Row(c.Key("waiting")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
					widget.Spinner(c, 12)
					ui.Row(c).Grow(1).MinWidth(0).Children(func() {
						widget.Running(c, "Signing in: approve "+p.UserCode+" in the browser", widget.RunStyle{})
					})
				})
				widget.CardFoot(c, func() {
					if widget.GhostButton(c, "Show").Clicked() {
						a.cloudSigningIn = true
					}
					if widget.GhostButton(c, "Cancel").Clicked() {
						a.cloudCancel()
					}
				})
				return
			}
			widget.CardTitle(c, nil, "Not signed in", nil)
			widget.CardNote(c, "Not signed in. Sign in with GitHub: the browser opens on a code to approve.", 0)
			widget.CardFoot(c, func() {
				if widget.PrimaryButton(c, nil, "Sign in", "").Clicked() {
					a.openCloudSignIn()
				}
			})
		})
		widget.GridCard(c.Key("adds")).Children(func() {
			widget.CardTitle(c, nil, "What it adds", nil)
			widget.CardNote(c, "Your teams in enowX: their members, the notes and canvases they share, their hosts. Off until you sign in; nothing reaches the cloud before. This is "+a.host.Name+"'s sign-in: each host signs in as a device of your account.", 0)
		})
	}, nil)
}

// cloudFigures are the account's figures, of what the page has read.
func (a *App) cloudFigures(cs *cloudSettings) []widget.Fig {
	people := map[string]bool{}
	teams := map[string]bool{}
	for _, t := range cs.full {
		teams[t.ID] = true
		for _, m := range t.Members {
			people[m.Account.ID] = true
		}
	}
	shared := 0
	for _, p := range a.teamProjectsOf().list {
		if teams[p.Team] {
			shared++
		}
	}
	recovery := "None"
	if cloudRecoverySet(cs.devices) {
		recovery = "Set"
	}
	return []widget.Fig{
		{Label: "Teams", Value: fmt.Sprint(len(cs.teams.list))},
		{Label: "People in them", Value: fmt.Sprint(len(people))},
		{Label: "Projects shared", Value: fmt.Sprint(shared)},
		{Label: "Devices", Value: fmt.Sprint(cloudDeviceCount(cs.devices))},
		{Label: "Recovery key", Value: recovery},
	}
}

// cloudDeviceCount counts the devices that sign in: the recovery key is kept
// among them, and shown by its own card.
func cloudDeviceCount(devices []api.Device) int {
	n := 0
	for _, d := range devices {
		if !d.Recovery {
			n++
		}
	}
	return n
}

// cloudAccountCard is who the host is signed in as.
func (a *App) cloudAccountCard(c *ui.Context, cs *cloudSettings) {
	widget.Card(c.Key("account")).FillWidth().Shrink(0).Padding(theme.S3, theme.S4).Children(func() {
		ui.Row(c).FillWidth().Gap(theme.S3).Children(func() {
			widget.InitialAvatar(c, cs.st.Account, 32)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				who := cs.st.Account
				if cs.st.GitHub != "" {
					who += " @" + cs.st.GitHub
				}
				widget.Running(c, who, widget.RunStyle{Weight: 600})
				widget.Running(c.Key("device"), a.cloudDeviceLine(cs.st), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
			if widget.GhostButton(c, "Sign out").Clicked() {
				a.cloudSignOut()
			}
		})
	})
}

// cloudDeviceLine says what this host is to the account.
func (a *App) cloudDeviceLine(st proto.CloudStatus) string {
	kind := "a computer"
	if st.DeviceKind == proto.CloudDeviceHost {
		kind = "a server"
	}
	line := a.host.Name + " · " + kind
	if st.DeviceName != a.host.Name {
		line = a.host.Name + " is " + st.DeviceName + " · " + kind
	}
	if st.URL != "" {
		line += " · " + st.URL
	}
	return line
}

func deviceKind(kind string) string {
	if kind == proto.CloudDeviceHost {
		return "a host"
	}
	return "the app"
}

// cloudTeamsTab is the Teams tab: the account, its teams and adding one, the
// latest of their logs.
func (a *App) cloudTeamsTab(c *ui.Context, cs *cloudSettings) {
	ts := &cs.teams
	cols, n := 3, len(ts.list)+1
	if len(ts.list) == 0 {
		cols, n = 2, 2
	}
	widget.PageBodyGrid(c, cols, n, 220, func() {
		cloudErrAndBusy(c, cs)
		a.cloudAccountCard(c, cs)
	}, func() {
		if len(ts.list) == 0 {
			widget.GridCard(c.Key("no-teams")).Children(func() {
				widget.EmptyState(c, icon.Account, "No teams yet", "Make one and invite the people you work with, or join one with the link someone sent you.", nil)
			})
		}
		for _, t := range ts.list {
			a.cloudTeamCard(c.Key("team:"+t.ID), cs, t)
		}
		widget.GridCard(c.Key("add-one")).Children(func() {
			widget.CardTitle(c, nil, "Add one", nil)
			widget.CardNote(c, "Each has its own space of notes and canvases, picked in the rail.", 0)
			ui.Spacer(c)
			widget.CardFoot(c, func() {
				if widget.GhostButton(c, "Join with an invite").Clicked() {
					ts.joining, ts.join = true, ""
				}
				if widget.PrimaryButton(c, icon.Plus, "New team", "").Clicked() {
					ts.making, ts.newName = true, ""
				}
			})
		})
	}, func() {
		if len(cs.latest) == 0 {
			return
		}
		widget.Heading(c, "Latest in your teams")
		ui.Column(c.Key("latest")).FillWidth().Shrink(0).Children(func() {
			now := time.Now()
			for _, r := range cs.latest {
				cloudRecordRow(c.Key(fmt.Sprint(r.team, r.entry.ID)), r, now)
			}
		})
	})
}

// cloudRecordRow is a line of a team's log, its time and what happened.
func cloudRecordRow(c *ui.Context, r cloudRecord, now time.Time) {
	e := r.entry
	who := e.Account.Name
	if who == "" {
		who = "The cloud"
	}
	what := auditActions[e.Action]
	if what == "" {
		what = e.Action
	}
	line := who + " " + what
	if e.Detail != "" {
		line += ": " + e.Detail
	}
	line += " · " + r.team
	if day := ragDay(e.At, now); day != "Today" {
		line = day + ": " + line
	}
	ui.Row(c).FillWidth().Shrink(0).MinHeight(36).Gap(theme.S3).BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Text(c, e.At.Local().Format("15:04")).FontSize(theme.TextUI).TextColor(theme.TextSoft).SingleLine().Shrink(0)
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, line, widget.RunStyle{}) })
	})
}

// cloudTeamCard is one team; a click opens its dialog.
func (a *App) cloudTeamCard(c *ui.Context, cs *cloudSettings, t api.Team) {
	card := ui.ButtonBase(c).MinWidth(0).MinHeight(0).Clip().Padding(theme.S4).AlignItems(ui.Stretch).Justify(ui.Start).
		Radius(theme.RadiusCard).Cursor(ui.CursorPointer).Label("Team " + t.Name)
	widget.Raise(card).Background(theme.Raised)
	if card.Hovered() {
		card.InsetShadow(0, 0, 0, 1, theme.BorderStrong)
	}
	card.Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).FillHeight().Gap(theme.S3).Children(func() {
			widget.CardTitle(c, nil, t.Name, nil)
			full, read := cs.full[t.ID]
			ui.Row(c.Key("chips")).FillWidth().Shrink(0).Wrap().Gap(theme.S1).Children(func() {
				widget.Chip(c, roleName(t.Role), theme.TextStrong)
				if read {
					n := len(full.Members)
					widget.Chip(c, textfmt.Plural(n, "1 member", fmt.Sprintf("%d members", n)), ui.Color{})
				}
				n := 0
				for _, p := range a.teamProjectsOf().list {
					if p.Team == t.ID {
						n++
					}
				}
				widget.Chip(c, textfmt.Plural(n, "1 project", fmt.Sprintf("%d projects", n)), ui.Color{})
			})
			ui.Column(c.Key("members")).FillWidth().Grow(1).Basis(0).MinHeight(0).Clip().Children(func() {
				widget.CardLines(c, func() {
					for i, m := range full.Members {
						widget.CardLine(c.Key(m.Account.ID), i == 0, m.Account.Name, "", func() {
							ui.Text(c, roleName(m.Role)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
						})
					}
				})
			})
			ui.Row(c.Key("foot")).FillWidth().Shrink(0).Children(func() {
				ui.Spacer(c)
				if widget.GhostButton(c, "Open").Label("Open " + t.Name).Clicked() {
					a.openCloudTeam(cs, t.ID)
				}
			})
		})
	})
	if card.Clicked() {
		a.openCloudTeam(cs, t.ID)
	}
}

// roleName is how a role reads alone.
func roleName(role string) string {
	if role == "" {
		return role
	}
	return strings.ToUpper(role[:1]) + role[1:]
}

// cloudDevicesTab is the account's devices, its recovery key and a code to
// approve for another device.
func (a *App) cloudDevicesTab(c *ui.Context, cs *cloudSettings) {
	widget.PageBodyGrid(c, 1, 1, 136, func() {
		cloudErrAndBusy(c, cs)
		widget.CardNote(c, "The app and the hosts signed in to your account. Sign out one you no longer use.", 0).Shrink(0)
		list := slices.DeleteFunc(slices.Clone(cs.devices), func(d api.Device) bool { return d.Recovery })
		sort.SliceStable(list, func(i, j int) bool { return list[i].Current && !list[j].Current })
		widget.Cols(c, 3).Children(func() {
			for _, d := range list {
				a.cloudDeviceCard(c.Key("dev:"+d.ID), d)
			}
			a.recoveryCard(c, cs)
		})
	}, func() {
		widget.GridCard(c.Key("approve")).Children(func() {
			widget.CardTitle(c, nil, "Approve a device", nil)
			widget.CardNote(c, "The code another device shows, to approve it from here.", 0)
			ui.Row(c.Key("code")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					widget.Input(c, &cs.approve, "XXXX-XXXX").FillWidth().Font(theme.MonoFont)
				})
				if widget.GhostButton(c, "Approve").Clicked() && strings.TrimSpace(cs.approve) != "" {
					a.approveDevice(cs, strings.TrimSpace(cs.approve))
				}
			})
		})
	}, nil)
}

// cloudDeviceCard is a device of the account; this one can't sign itself out here.
func (a *App) cloudDeviceCard(c *ui.Context, d api.Device) {
	widget.Card(c).Children(func() {
		widget.CardTitle(c, nil, d.Name, nil)
		widget.StatusLine(c, theme.Done, false, "Signed in")
		kind := "Computer"
		if d.Kind == api.DeviceHost {
			kind = "Server"
		}
		about := kind + " · seen " + textfmt.Age(d.LastSeen, time.Now()) + " ago"
		if d.Current {
			about = kind + " · this device"
		}
		widget.CardNote(c, about, 0)
		widget.CardFoot(c, func() {
			ui.Spacer(c)
			if d.Current {
				widget.Chip(c, "This device", ui.Color{})
				return
			}
			if widget.DangerButton(c, "Sign out").Label("Sign " + d.Name + " out").Clicked() {
				id, name := d.ID, d.Name
				a.confirm("Sign "+name+" out?", "Its teams' spaces leave Notes and Canvas until it signs in again, and it stops taking the teams' changes. Your own notes stay.", "Sign out", func() {
					a.cloudDo("Signing "+name+" out…", func(cl *client.Client) error {
						return cl.CloudCall("DELETE", strings.Replace(api.PathDevice, "{id}", id, 1), nil, nil)
					}, a.loadCloud)
				})
			}
		})
	})
}

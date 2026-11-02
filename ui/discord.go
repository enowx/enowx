package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/discordrpc"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Discord Rich Presence: under the user's name in Discord, what they work
// on in enowX, the selected session's project, branch and agent, how long
// it has run, and a link to get enowX. It goes to the Discord app on this
// device only, while it runs; Settings › Discord turns it off, or keeps
// the project's and branch's names out of it.

// discordAppID is the Discord application the presence shows as (its
// Application ID, in Discord's Developer Portal), with images uploaded
// to its Rich Presence assets as "enowx" and the agents' IDs
// (discordAgents); ENOWX_DISCORD_APP_ID replaces it. Empty, there's no
// presence.
var discordAppID = "1517644526510932009"

// discordAgents are the agents with an image of their own in the Discord
// application.
var discordAgents = map[string]bool{"claude": true, "codex": true, "gemini": true, "opencode": true, "cursor": true, "amp": true}

const (
	discordMinGap = 15 * time.Second // Discord takes five updates in 20 s
	discordRetry  = 30 * time.Second // looking again for a Discord not running
)

// discordPrefs are the user's choices, kept in the app's state.
type discordPrefs struct {
	Off         bool `json:"off,omitempty"`
	HideProject bool `json:"hideProject,omitempty"`
}

// discordState is the presence as the main thread computed it, and what
// the presence's goroutine said of Discord. The goroutine owns the
// connection; the main thread hands it the latest activity on want.
type discordState struct {
	want    chan *discordrpc.Activity // the latest, a slot of one
	last    *discordrpc.Activity
	started time.Time
	status  string // "Connected", "Discord isn't running", an error
}

func (a *App) discordID() string {
	if id := os.Getenv("ENOWX_DISCORD_APP_ID"); id != "" {
		return id
	}
	return discordAppID
}

// discordPresence computes what Discord should show, and hands it to the
// presence's goroutine when it changed. It runs as the window is drawn.
func (a *App) discordPresence() {
	if a.discordID() == "" || a.onMain == nil {
		return
	}
	d := &a.discord
	if d.want == nil {
		d.want, d.started = make(chan *discordrpc.Activity, 1), time.Now()
		go a.discordLoop(d.want)
	}
	var act *discordrpc.Activity
	if !a.state.Discord.Off {
		act = a.discordActivity()
	}
	if discordrpc.Equal(act, d.last) {
		return
	}
	d.last = act
	select {
	case <-d.want: // an older one not sent yet
	default:
	}
	d.want <- act
}

// discordActivity is the selected session, else the app itself.
func (a *App) discordActivity() *discordrpc.Activity {
	act := &discordrpc.Activity{
		Assets:  &discordrpc.Assets{LargeImage: "enowx", LargeText: "enowX: coding agents, side by side"},
		Buttons: []discordrpc.Button{{Label: "Get enowX", URL: "https://enowx.ai"}},
	}
	running := 0
	for _, s := range a.sessions {
		if !s.Exited {
			running++
		}
	}
	s, ok := a.selectedSession()
	if !ok || s.Exited {
		act.Details, act.Timestamps = "In enowX", &discordrpc.Times{Start: a.discord.started.Unix()}
		if running > 0 {
			act.State = plural(running, "session") + " open"
		}
		return act
	}
	name := a.agentName(s.Agent)
	switch {
	case a.state.Discord.HideProject:
		act.Details = "Working with " + name
	default:
		project := filepath.Base(s.Project)
		if p, ok := a.projectAt(s.Project); ok && p.Name != "" {
			project = p.Name
		}
		act.Details = "Working on " + project
		if s.Branch != "" {
			act.Details += " · " + s.Branch
		}
	}
	act.State = name + " · " + plural(running, "session")
	if base := agent.Base(s.Agent); discordAgents[base] {
		act.Assets.SmallImage, act.Assets.SmallText = base, name
	}
	act.Timestamps = &discordrpc.Times{Start: s.Created.Unix()}
	return act
}

func plural(n int, one string) string {
	return textfmt.Plural(n, "1 "+one, fmt.Sprintf("%d %ss", n, one))
}

// discordLoop owns the connection to Discord: it shows what want hands
// it, no more often than Discord allows, and looks for Discord again
// while it isn't running.
func (a *App) discordLoop(want <-chan *discordrpc.Activity) {
	var (
		client  *discordrpc.Client
		pending *discordrpc.Activity
		have    bool // pending is to show
		sent    time.Time
		tried   time.Time
	)
	say := func(status string) { a.onMain(func() { a.discord.status = status }) }
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case act := <-want:
			pending, have = act, true
		case <-tick.C:
		}
		if !have || time.Since(sent) < discordMinGap {
			continue
		}
		if client == nil {
			if pending == nil {
				have = false // nothing shown, nothing to clear
				continue
			}
			if time.Since(tried) < discordRetry {
				continue
			}
			tried = time.Now()
			c, err := discordrpc.Dial(a.discordID())
			if err != nil {
				say("Discord isn't running on this device")
				continue
			}
			client = c
		}
		if err := client.SetActivity(pending); err != nil {
			client.Close()
			client = nil
			say("Lost Discord: " + err.Error())
			continue
		}
		sent, have = time.Now(), false
		if pending == nil {
			say("Off")
		} else {
			say("Showing in Discord")
		}
	}
}

// discordSection is Settings › Discord.
func (a *App) discordSection(c *ui.Context) {
	widget.PageHeader(c, "Discord", "Rich Presence, through the Discord app on this device", nil)
	if a.discordID() == "" {
		widget.PageBody(c, true, func() {
			widget.CardNote(c, "This build of enowX has no Discord application set.", 0)
		})
		return
	}
	widget.PageFill(c, func() {
		widget.Cols(c, 2).Shrink(0).Children(func() {
			widget.Card(c.Key("device")).Children(func() {
				widget.CardTitle(c, nil, "On this device", nil)
				on := !a.state.Discord.Off
				if switchRow(c, &on, "Show what you work on", "Discord shows it under your name while it runs here.") {
					a.state.Discord.Off = !on
				}
				names := !a.state.Discord.HideProject
				if switchRow(c, &names, "Show the project and branch", "Off, Discord shows the agent only: “Working with Claude Code”.") {
					a.state.Discord.HideProject = !names
				}
			})
			widget.Card(c.Key("status")).Children(func() {
				widget.CardTitle(c, nil, "Discord on this device", nil)
				status := a.discord.status
				if status == "" {
					status = "Looking for Discord…"
				}
				col := theme.Warning
				if status == "Showing in Discord" {
					col = theme.Done
				}
				widget.Banner(c, col, status, nil)
				widget.CardNote(c, "It reaches the Discord app over its local socket (a named pipe on Windows). Nothing goes through a server of enowX's.", 0)
			})
		})
		widget.Heading(c, "What Discord shows")
		ui.Row(c.Key("shows")).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Gap(theme.S3).Children(func() {
			details := "Working on a project · its branch"
			if a.state.Discord.HideProject {
				details = "Working with Claude Code"
			}
			discordSample(c, "With a session selected", details, "Claude Code · "+plural(max(len(a.sessions), 1), "session"))
			discordSample(c, "With none selected", "In enowX", plural(len(a.sessions), "session")+" open")
		})
	})
}

// discordSample is a card with what Discord's profile card shows.
func discordSample(c *ui.Context, title, details, state string) {
	widget.Card(c.Key(title)).Grow(1).Basis(0).Children(func() {
		widget.CardTitle(c, nil, title, nil)
		ui.Row(c).Grow(1).FillWidth().MinHeight(0).Padding(theme.S5).Gap(theme.S4).Radius(theme.RadiusCard).Background(theme.Sunken).Justify(ui.Center).Children(func() {
			ui.Box(c).Size(72, 72).Shrink(0).Radius(theme.RadiusCard).Background(theme.Hover).Center().Children(func() {
				ui.Icon(c, icon.Agent("claude")).FontSize(32)
			})
			ui.Column(c).MinWidth(0).Shrink(1).Children(func() {
				ui.Text(c, "Playing").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
				ui.Text(c, "enowX").FontSize(theme.TextUI).FontWeight(500)
				widget.CardNote(c, details, 2)
				widget.CardNote(c, state, 1)
			})
		})
	})
}

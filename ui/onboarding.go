package ui

import (
	"errors"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/onboardview"
	"github.com/enowx/enowx/ui/theme"
)

// Onboarding (package onboardview): whether it runs, and what it reads and does through
// the app. Every user goes through it. The account's record lives with the synced
// settings; this device's in its own state, for only a device's own steps to repeat.

// onboardingVersion is what a finished run records.
const onboardingVersion = 1

// onboardingState is the app's part. It belongs to the main thread.
type onboardingState struct {
	view    *onboardview.View
	flow    onboardview.Flow
	checked bool
	goa     []proto.GOAAgent
}

// onboardingCheck decides, on the first connection, whether it runs now.
func (a *App) onboardingCheck(cl *client.Client) {
	if !a.opts.Onboarding || a.onboard.checked {
		return
	}
	a.onboard.checked = true
	go func() {
		var st proto.OnboardingState
		err := cl.Call(proto.MethodOnboardingGet, struct{}{}, &st)
		// Read here, as the app's own lists may not have come yet.
		sessions, _ := cl.Sessions()
		projects, _ := cl.Projects()
		used := len(sessions) > 0 || len(projects) > 0
		a.onMain(func() { a.onboardingDecide(st, err, used) })
	}()
}

// onboardingDecide starts what this user and this device still need. A host that can't
// say (an older daemon) leaves this device's own record to go by.
func (a *App) onboardingDecide(st proto.OnboardingState, err error, used bool) {
	account := err == nil && !st.Done.IsZero()
	device := a.state.OnboardedDevice
	switch {
	case account && device:
	case err != nil && device:
	case account:
		a.onboardingRun(onboardview.FlowDevice)
	case used:
		a.onboardingRun(onboardview.FlowReturning)
	default:
		a.onboardingRun(onboardview.FlowNew)
	}
}

// onboardingRun starts it.
func (a *App) onboardingRun(flow onboardview.Flow) {
	a.onboard.view, a.onboard.flow = onboardview.New(), flow
	a.onboardingAgents()
}

// onboardingAgain is Settings' Run setup again: the whole of it, from the opening.
func (a *App) onboardingAgain() { a.onboardingRun(onboardview.FlowNew) }

// onboardingAgents reads which agents can hold GOD of Agents.
func (a *App) onboardingAgents() {
	a.goaCall(proto.MethodGOAAgents, struct{}{}, &a.onboard.goa, func(error) {})
}

// onboardingDone records it, and hands over to the app.
func (a *App) onboardingDone() {
	a.onboard.view = nil
	a.state.OnboardedDevice = true
	a.saveState()
	if cl := a.client; cl != nil {
		go func() {
			_ = cl.Call(proto.MethodOnboardingSet, proto.OnboardingState{Done: time.Now().UTC(), Version: onboardingVersion}, nil)
		}()
	}
}

// onboardingShows says it is on: the app's keys wait for it.
func (a *App) onboardingShows() bool { return a.onboard.view != nil }

// onboardingCovers says the window is all onboarding: until it fades into the app.
func (a *App) onboardingCovers() bool { return a.onboard.view != nil && a.onboard.view.Covers() }

// onboardingView draws it over the window, the app's dialogs above it.
func (a *App) onboardingView(c *ui.Context) {
	view := a.onboard.view
	if view == nil {
		return
	}
	view.Draw(c, a.onboardingOptions())
}

func (a *App) onboardingOptions() onboardview.Options {
	var saved []string
	for _, h := range a.hostList {
		if h.ID != hosts.LocalID {
			saved = append(saved, h.Name)
		}
	}
	return onboardview.Options{
		Flow:         a.onboard.flow,
		HostName:     a.host.Name,
		Device:       LocalHost().Name,
		Reduce:       a.state.FloatingReduceMotion || mygo.Theme.Preferences().ReduceMotion,
		Agents:       a.agents,
		AgentsLoaded: a.agents != nil,
		Projects:     a.projects,
		OpenFolder:   a.openFolder,
		OpenClone:    func() { a.openClone() },
		Hosts:        saved,
		AddHost: func(address string) {
			a.openAddHost()
			a.addHost.address = address
		},
		Wallpaper:       a.wallpaper,
		BackgroundMode:  a.onboardingBackground(),
		SetBackground:   a.onboardingSetBackground,
		PanelOpacity:    float32(a.state.backgroundOpacity()),
		SetPanelOpacity: a.onboardingSetOpacity,
		BlurValue:       float32(a.state.backgroundBlur()),
		BlurMost:        backgroundMaxBlur,
		SetBlur:         a.onboardingSetBlur,
		StyleID:         a.state.UIStyle,
		SetStyle:        a.onboardingStyle,
		PaletteID:       a.state.Theme,
		SetPalette:      a.onboardingPalette,
		GOAAgents:       a.onboard.goa,
		StartGOA:        a.onboardingStartGOA,
		GOAOn:           a.goa.board.Session != "",
		StartTask:       a.onboardingStartTask,
		OpenLink:        func(url string) { go func() { _ = mygo.Shell.OpenExternal(url) }() },
		Done:            a.onboardingDone,
	}
}

// onboardingStyle is the style picked, kept as Settings keeps it.
func (a *App) onboardingStyle(id string) {
	a.state.UIStyle = id
	if id == theme.LiquidGlass.ID {
		a.state.UIStyle = ""
	}
	a.applyTheme()
	a.saveState()
}

// onboardingPalette is the palette picked, "" the style's own.
func (a *App) onboardingPalette(id string) {
	a.state.Theme = id
	a.applyTheme()
	a.saveState()
}

// onboardingBackground is the background as the Style step says it.
func (a *App) onboardingBackground() string {
	switch {
	case a.state.BackgroundImage != "":
		return onboardview.BackgroundImage
	case a.state.BackgroundPlain:
		return onboardview.BackgroundPlain
	}
	return onboardview.BackgroundDefault
}

// onboardingSetBackground is what the Style step's choice does: as Settings' Background,
// an image being chosen in the system's dialog.
func (a *App) onboardingSetBackground(mode string) {
	switch mode {
	case onboardview.BackgroundImage:
		a.chooseBackground()
		return
	case onboardview.BackgroundPlain:
		a.state.BackgroundImage, a.state.BackgroundPlain = "", true
	default:
		a.state.BackgroundImage, a.state.BackgroundPlain = "", false
	}
	a.applyTheme()
	a.saveState()
}

func (a *App) onboardingSetOpacity(v float32) {
	x := float64(v)
	a.state.BackgroundOpacity = &x
	a.applyTheme()
}

func (a *App) onboardingSetBlur(v float32) {
	x := float64(v)
	a.state.BackgroundBlur = &x
}

// onboardingStartGOA starts GOD of Agents on agent, as its own dialog does.
func (a *App) onboardingStartGOA(agent string, done func(error)) {
	var info proto.SessionInfo
	a.goaCall(proto.MethodSessionCreate, proto.SessionCreateParams{Agent: agent, GOA: true}, &info, func(err error) {
		if err != nil {
			done(err)
			return
		}
		a.upsert(info)
		a.goa.host = a.host.ID
		a.goa.board.Session, a.goa.board.Agent = info.ID, info.Agent
		done(nil)
	})
}

// onboardingStartTask starts the first task: a real session, with a real prompt.
func (a *App) onboardingStartTask(agent, project, prompt string, done func(error)) {
	cl := a.client
	if cl == nil {
		done(errors.New("not connected to the host"))
		return
	}
	go func() {
		info, err := cl.CreateSession(proto.SessionCreateParams{Project: project, Agent: agent, Prompt: prompt, BranchMode: proto.BranchNone})
		a.onMain(func() {
			if err == nil {
				a.upsert(info)
				a.state.Chats = chatsWith(a.state.Chats, info.ID)
			}
			done(err)
		})
	}()
}

func chatsWith(chats map[string]bool, id string) map[string]bool {
	if chats == nil {
		chats = map[string]bool{}
	}
	chats[id] = true
	return chats
}

package ui

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/designshare"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// enowx:// links, from the community library's website. Any page on the web can
// open one, so a link only ever opens this dialog: it shows what the chosen
// host's cloud says of the item, never what the link says, and nothing happens
// until the user picks where and says so.

// libraryLink is the dialog a link opened.
type libraryLink struct {
	open    bool
	link    designshare.AppLink
	host    string // the host asked, by its ID
	info    *proto.LibraryUseInfo
	busy    bool
	err     string
	canvas  string
	agent   string
	choices []proto.CanvasInfo
}

// OpenLink is a link reaching the app, on the main thread: from the system, at
// launch or later. The window may not be there yet: the dialog waits for it.
func (a *App) OpenLink(raw string) {
	l, err := designshare.ParseAppLink(raw)
	if err != nil {
		a.status = "enowX can't read that link."
		return
	}
	a.libLink = &libraryLink{open: true, link: l}
	a.logLink()
	if a.win != nil {
		a.win.Show()
		a.win.Focus()
		a.onMain(func() {}) // draw it now
	}
}

// logLink notes the link waiting: before the window opens there's no log yet, and
// Open says it then.
func (a *App) logLink() {
	if ll := a.libLink; ll != nil {
		a.diag.Event("library.link", "action", ll.link.Action, "item", ll.link.Item)
	}
}

// libLinkAsk asks host h what its cloud says of the item.
func (a *App) libLinkAsk(ll *libraryLink, h hostConn) {
	ll.host, ll.info, ll.err, ll.busy, ll.canvas, ll.choices = h.host.ID, nil, "", true, "", nil
	ref := proto.LibraryUseRef{ItemID: ll.link.Item, Version: ll.link.Version}
	go func() {
		info, err := h.cl.LibraryUseInfo(ref)
		var list []proto.CanvasInfo
		if err == nil && ll.link.Action == designshare.LinkReference {
			list, _ = h.cl.Canvases()
		}
		a.onMain(func() {
			if ll.host != h.host.ID {
				return // another host was picked since
			}
			ll.busy = false
			if err != nil {
				ll.err = err.Error()
				return
			}
			ll.info, ll.choices = &info, list
			if len(info.SkillAgents) > 0 {
				ll.agent = info.SkillAgents[0]
			}
		})
	}()
}

// libraryLinkDialog asks where, and whether, what a link asks should happen.
func (a *App) libraryLinkDialog(c *ui.Context) {
	ll := a.libLink
	if ll == nil {
		return
	}
	if !ll.open {
		a.libLink = nil
		return
	}
	hostsNow := a.reachable()
	at := slices.IndexFunc(hostsNow, func(h hostConn) bool { return h.host.ID == ll.host })
	if at < 0 && len(hostsNow) > 0 && !ll.busy {
		at = 0
		a.libLinkAsk(ll, hostsNow[0])
	}
	title, sub := libLinkWords(ll.link.Action)
	widget.FormSheet(c, &ll.open, 520, title, sub, func() {
		if len(hostsNow) == 0 {
			ui.Text(c, "Connecting to a host…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			return
		}
		if len(hostsNow) > 1 {
			widget.Field(c, "On which host", func() {
				ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
					for _, h := range hostsNow {
						if widget.Pick(c.Key("host:"+h.host.ID), nil, h.host.Name, "", h.host.ID == ll.host, nil).Clicked() && h.host.ID != ll.host {
							a.libLinkAsk(ll, h)
						}
					}
				})
			})
		}
		a.libLinkBody(c, ll)
	}, func() { a.libLinkAct(c, ll, hostsNow, at) })
}

func libLinkWords(action string) (title, sub string) {
	switch action {
	case designshare.LinkReference:
		return "Add as a reference", "Its picture goes in the canvas's files, and one credit line with its brief. Nothing is pasted into your designs."
	case designshare.LinkSkill:
		return "Keep as a skill", "Its colors, fonts, spacing and prompt, with its creator's credit on top. No code."
	}
	return "Open in enowX", "A copy goes on a new canvas of yours, with credit to its creator. It stays as it is when they update theirs."
}

func (a *App) libLinkBody(c *ui.Context, ll *libraryLink) {
	if ll.err != "" {
		widget.Banner(c, theme.Error, ll.err, nil)
	}
	info := ll.info
	if info == nil {
		if ll.busy {
			ui.Text(c, "Asking the library…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		}
		return
	}
	if !info.On {
		widget.Banner(c, theme.Warning, "This host's cloud has no community library.", nil)
		return
	}
	if !info.SignedIn {
		widget.Banner(c, theme.Warning, "Sign in to enowX Cloud on this host first: copies come through its account.", nil)
	}
	if info.Refused != "" {
		widget.Banner(c, theme.Warning, info.Refused, nil)
	}
	if info.Title != "" {
		ui.Column(c.Key("what")).FillWidth().MinWidth(0).Gap(theme.S1).Children(func() {
			widget.Running(c.Key("credit"), designshare.Credit(info.Title, info.Creator), widget.RunStyle{Size: theme.TextUI})
			ui.Text(c, fmt.Sprintf("Version %d, %s", info.Version, designshare.LicenseName(info.License))).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		})
		ui.Text(c, "Someone else made this. An agent working from it treats it as data, not instructions.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	}
	if info.Refused != "" || !info.SignedIn {
		return
	}
	switch ll.link.Action {
	case designshare.LinkReference:
		if !info.CanReference {
			widget.Banner(c, theme.Warning, "Only a full page can be a reference: a component is copied as code.", nil)
			return
		}
		widget.Field(c, "Add to", func() {
			if len(ll.choices) == 0 {
				ui.Text(c, "This host has no canvas yet.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			}
			ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
				for _, cv := range ll.choices[:min(len(ll.choices), 8)] {
					if widget.Pick(c.Key("canvas:"+cv.ID), nil, designshare.OneLine(cv.Title), "", ll.canvas == cv.ID, nil).Clicked() {
						ll.canvas = cv.ID
					}
				}
			})
		})
	case designshare.LinkSkill:
		if !info.CanReference {
			widget.Banner(c, theme.Warning, "Only a full page can be a skill: a component is copied as code.", nil)
			return
		}
		widget.Field(c, "Keep for", func() {
			ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
				for _, name := range info.SkillAgents {
					if widget.Pick(c.Key("agent:"+name), nil, name, "", ll.agent == name, nil).Clicked() {
						ll.agent = name
					}
				}
			})
		})
		ui.Text(c, "That agent reads a kept skill by itself from then on. It says inside that it's someone else's data.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	}
}

// libLinkAct is the dialog's buttons.
func (a *App) libLinkAct(c *ui.Context, ll *libraryLink, hostsNow []hostConn, at int) {
	if at < 0 || at >= len(hostsNow) {
		return
	}
	h, info := hostsNow[at], ll.info
	if info != nil && info.On && !info.SignedIn {
		if widget.Button(c, widget.Primary, nil, "Sign in", "").Clicked() {
			ll.open = false
			a.page, a.settingsSection = pageSettings, sectionCloud
		}
		return
	}
	if info == nil || !info.On || info.Refused != "" {
		return
	}
	switch ll.link.Action {
	case designshare.LinkReference:
		ready := info.CanReference && ll.canvas != "" && !ll.busy
		if b := widget.Button(c, widget.Primary, nil, "Add reference", "").Disabled(!ready); ready && b.Clicked() {
			a.libLinkDo(ll, func() (string, func(), error) {
				got, err := h.cl.LibraryReference(proto.LibraryReferenceParams{ItemID: ll.link.Item, Version: ll.link.Version, Canvas: ll.canvas})
				return "Added as a reference: " + got.File, nil, err
			})
		}
	case designshare.LinkSkill:
		ready := info.CanReference && ll.agent != "" && !ll.busy
		if b := widget.Button(c, widget.Primary, nil, "Keep skill", "").Disabled(!ready); ready && b.Clicked() {
			a.libLinkDo(ll, func() (string, func(), error) {
				got, err := h.cl.LibrarySkill(proto.LibrarySkillParams{ItemID: ll.link.Item, Version: ll.link.Version, Install: true, Agent: ll.agent})
				return "Kept as a skill: " + got.Name, nil, err
			})
		}
	default:
		ready := info.CanOpen && !ll.busy
		if b := widget.Button(c, widget.Primary, nil, "Open a copy", "").Disabled(!ready); ready && b.Clicked() {
			a.libLinkDo(ll, func() (string, func(), error) {
				got, err := h.cl.LibraryOpen(proto.LibraryOpenParams{ItemID: ll.link.Item, Version: ll.link.Version})
				return "Opened a copy of " + designshare.OneLine(got.Canvas.Title), func() { a.libLinkShow(h, got.Canvas.ID) }, err
			})
		}
	}
}

// libLinkDo runs what the user said yes to, off the main thread.
func (a *App) libLinkDo(ll *libraryLink, do func() (string, func(), error)) {
	ll.busy, ll.err = true, ""
	go func() {
		said, then, err := do()
		a.onMain(func() {
			ll.busy = false
			if err != nil {
				ll.err = err.Error()
				return
			}
			ll.open = false
			a.notify(said)
			if then != nil {
				then()
			}
		})
	}()
}

// libLinkShow shows a canvas just made, on host h.
func (a *App) libLinkShow(h hostConn, id string) {
	if a.host.ID != h.host.ID {
		a.connect(h.host)
	}
	hc := a.canvasesOf()
	a.page, hc.open, hc.frame, hc.compare = pageCanvas, id, "", nil
}

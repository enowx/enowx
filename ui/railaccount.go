package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/widget"
)

// The account the connected host is signed in to the cloud as. Signed out
// it signs in, one click (the dialog, cloudsignin.go, opens the browser on
// the code).

// accountAction is the account's button at the top right, a menu: who it is,
// the space Notes and Canvas show, and what belongs to the account.
func (a *App) accountAction(c *ui.Context) {
	var st proto.CloudStatus
	loaded := false
	if a.client != nil {
		if cs := a.cloudOf(); cs.loaded {
			st, loaded = cs.st, true
		}
	}
	label, tip := "Account", "Account"
	if st.Account != "" {
		label = "Account: " + st.Account
		where := "Personal"
		if team := a.space(); team != "" {
			where = a.spaceName(team)
		}
		tip = st.Account + " · enowX Cloud · " + where
	}
	b := actionButton(c, "account", label, icon.Account, true).Tooltip(tip)
	a.menu.OpenBelow(b, "account", titleBarHeight)
	a.menu.Show(c, b, "account", func(done func()) {
		switch {
		case !loaded:
		case st.Pending != nil:
			if widget.MenuItemSub(c.Key("approve"), icon.Account, "Approve "+st.Pending.UserCode, "In the browser", false) {
				a.cloudSigningIn = true
				done()
			}
			widget.MenuSeparator(c)
		case !st.SignedIn:
			if widget.MenuItemSub(c.Key("sign-in"), icon.Account, "Sign in", "enowX Cloud with GitHub: your teams, their notes and hosts", false) {
				a.openCloudSignIn()
				done()
			}
			widget.MenuSeparator(c)
		default:
			sub := "Signed in to enowX Cloud"
			if st.GitHub != "" {
				sub = "@" + st.GitHub + " · " + a.host.Name + " as " + deviceKind(st.DeviceKind)
			}
			if widget.MenuItemSub(c.Key("who"), nil, st.Account, sub, false) {
				a.page, a.settingsSection = pageSettings, sectionCloud
				done()
			}
			widget.MenuSeparator(c)
			if spaces := a.spacesOf().list; len(spaces) > 0 {
				widget.PopupLabel(c, "Space for Notes and Canvas")
				space := func(team, name string, svg *ui.SVG) {
					if team == a.space() {
						svg = icon.Check
					}
					if widget.MenuItem(c.Key("space:"+team), svg, name, false) {
						a.showSpace(team)
						done()
					}
				}
				space("", "Personal", icon.Account)
				for _, sp := range spaces {
					space(sp.Team, sp.Name, icon.Users)
				}
				widget.MenuSeparator(c)
			}
		}
		if a.client != nil && widget.MenuItem(c.Key("teams"), icon.Users, "Team", false) {
			a.openTeamPage()
			done()
		}
		if a.client != nil && widget.MenuItem(c.Key("credits"), icon.Credits, "Credits", false) {
			a.page = pageCredits
			done()
		}
		if widget.MenuItem(c.Key("appearance"), icon.Palette, "Appearance", false) {
			a.accentInput = a.state.Accent
			a.page = pageAppearance
			done()
		}
		if loaded && st.SignedIn {
			widget.MenuSeparator(c)
			if widget.MenuItem(c.Key("sign-out"), icon.Power, "Sign out…", false) {
				a.cloudSignOut()
				done()
			}
		}
	})
}

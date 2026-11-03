package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// cloudMemberRow is a member of the open team. For the user, the owner, or when
// the user is no admin, it only says the role.
func (a *App) cloudMemberRow(c *ui.Context, cs *cloudSettings, m api.Member) {
	ts := &cs.teams
	ui.Row(c.Key("member:"+m.Account.ID)).FillWidth().Shrink(0).Gap(theme.S3).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).Background(theme.Raised).Children(func() {
		widget.InitialAvatar(c, m.Account.Name, 32)
		me := m.Account.ID == cs.st.AccountID
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			name := m.Account.Name
			if me {
				name += " (you)"
			}
			widget.Running(c, name, widget.RunStyle{Weight: 500})
			if m.Account.GitHub != "" {
				widget.Running(c.Key("github"), "@"+m.Account.GitHub, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			}
		})
		if me || m.Role == api.RoleOwner || !canAdmin(ts.team.Role) {
			widget.Chip(c, roleName(m.Role), ui.Color{})
			return
		}
		a.memberRoleMenu(c, cs, m)
		a.memberMoreMenu(c, cs, m)
	})
}

// memberRoleMenu is a member's role, which an admin changes from it.
func (a *App) memberRoleMenu(c *ui.Context, cs *cloudSettings, m api.Member) {
	b := widget.MenuButton(c, widget.Secondary, roleName(m.Role)).Label("Role of " + m.Account.Name).Tooltip("Change " + m.Account.Name + "'s role")
	key := "role:" + m.Account.ID
	a.menu.OpenBelow(b, key, theme.ControlHeight)
	a.menu.Show(c, b, key, func(done func()) {
		for _, r := range []string{api.RoleAdmin, api.RoleMember, api.RoleViewer} {
			label := roleName(r)
			if r == m.Role {
				label += " (now)"
			}
			svg := map[string]*ui.SVG{api.RoleAdmin: icon.Shield, api.RoleMember: icon.Account, api.RoleViewer: icon.Eye}[r]
			if widget.MenuItemTip(c.Key(r), svg, label, roleAbout(r), false) {
				done()
				if r != m.Role {
					a.setMemberRole(cs, m.Account.ID, r)
				}
			}
		}
	})
}

// memberMoreMenu is what else an admin can do about a member: remove them, and
// for the owner, hand the team over.
func (a *App) memberMoreMenu(c *ui.Context, cs *cloudSettings, m api.Member) {
	ts := &cs.teams
	acc, name := m.Account.ID, m.Account.Name
	b := widget.IconButton(c, icon.More, "More about "+name, false)
	key := "member-more:" + acc
	a.menu.OpenBelow(b, key, theme.ControlHeight)
	a.menu.Show(c, b, key, func(done func()) {
		if ts.team.Role == api.RoleOwner {
			if widget.MenuItem(c.Key("owner"), icon.Shield, "Make owner…", false) {
				done()
				a.confirm("Make "+name+" the owner of "+ts.team.Name+"?", name+" becomes its owner, who alone can delete it or hand it over again; you become an admin.", "Make owner", func() {
					a.setMemberRole(cs, acc, api.RoleOwner)
				})
			}
		}
		if widget.MenuItem(c.Key("remove"), icon.Trash, "Remove…", true) {
			done()
			a.confirm("Remove "+name+" from "+ts.team.Name+"?", name+" loses the team's notes, canvases and hosts; new items use a key they never had. What they could read before, they may have kept.", "Remove", func() {
				a.removeMember(cs, acc)
			})
		}
	})
}

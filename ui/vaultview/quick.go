package vaultview

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The quick ways to fill a new credential, before typing it: a login the
// connected host's tools already keep, a token copied from the platform's
// page (opened in the browser, the copy noticed), signing in in the
// browser, and last, the platform's own command line in a session.

// tokenPage is a token being made on the platform's page: the clipboard
// as it was when the page opened, and a token copied since, offered.
type tokenPage struct {
	before string
	offer  string
	polled time.Time
}

// quickWays draws the ways for platform p, in a new credential's form.
func (v *View) quickWays(c *ui.Context, o Options, f *form, p vault.Platform) {
	found := foundFor(o.Found, p.ID)
	if o.Actions.Scan != nil && !f.scanned {
		f.scanned = true
		o.Actions.Scan() // what the host's tools keep, for every platform
	}
	widget.Field(c, "Already on "+o.Current.Name, func() {
		switch {
		case o.Scanning && len(found) == 0:
			ui.Text(c, "Looking at what "+o.Current.Name+"'s tools keep…").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		case len(found) == 0:
			ui.Text(c, "Nothing of "+p.Name+" kept by "+o.Current.Name+"'s tools.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
		for i, fd := range found {
			ui.Row(c.Key(i)).FillWidth().Gap(theme.S2).Padding(theme.S1, theme.S2, theme.S1, theme.S3).Radius(theme.RadiusCard).Background(theme.Sunken).Children(func() {
				Logo(c, p, 22)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
					who := fd.Name
					if fd.Account != "" {
						who += " · " + fd.Account
					}
					ui.Text(c, who).FontSize(theme.TextUI).TextColor(theme.Text).SingleLine()
					ui.Text(c, "from "+fd.Source).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine().Tooltip(fd.Source)
				})
				if widget.Button(c, widget.Secondary, icon.Check, "Use", "").Label("Use " + fd.Name).Clicked() {
					f.useFound(fd)
				}
			})
		}
	})
	if p.Link != "" && o.Actions.OpenURL != nil {
		v.tokenPageRow(c, o, f, p)
	}
	if p.Device != nil && o.Actions.DeviceLogin != nil {
		v.deviceSection(c, o, f, p)
	}
	if p.Login != "" && o.Actions.Login != nil {
		cmd := strings.Fields(p.Login)[0]
		ui.Row(c).FillWidth().Children(func() {
			b := widget.Button(c, widget.Secondary, icon.Terminal, "Log in with "+cmd+" instead", "").
				Tooltip("Runs " + p.Login + " in a session on " + o.Current.Name + "; import it afterwards")
			if b.Clicked() {
				o.Actions.Login(p.ID)
				f.open = false
			}
		})
	}
}

// tokenPageRow opens the page where p's token is made, and offers a token
// copied there since.
func (v *View) tokenPageRow(c *ui.Context, o Options, f *form, p vault.Platform) {
	field := vault.TokenField(p)
	widget.Field(c, "Make one on "+p.Name, func() {
		ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
			if widget.Button(c, widget.Secondary, icon.Globe, "Open the token page", "").
				Tooltip(p.Link).Clicked() {
				f.page = &tokenPage{before: c.ReadClipboard()}
				o.Actions.OpenURL(p.Link)
			}
			hint := "Copy the token there: it is offered here."
			if f.page != nil && f.page.offer == "" {
				hint = "Waiting for a token you copy on " + p.Name + "…"
			}
			ui.Text(c, hint).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Grow(1).Shrink(1).MinWidth(0)
		})
		tp := f.page
		if tp == nil || field == "" {
			return
		}
		if tp.offer == "" && time.Since(tp.polled) > 600*time.Millisecond {
			tp.polled = time.Now()
			if s := strings.TrimSpace(c.ReadClipboard()); s != tp.before && s != f.values[field] && vault.TokenLike(p.ID, s) {
				tp.offer = s
			}
		}
		if tp.offer == "" {
			c.After(700 * time.Millisecond)
			return
		}
		ui.Row(c.Key("offer")).FillWidth().Gap(theme.S2).Padding(theme.S1, theme.S2, theme.S1, theme.S3).Radius(theme.RadiusCard).
			Background(theme.Fade(theme.Accent, 0.14)).Children(func() {
			ui.Icon(c, icon.Key).FontSize(13).TextColor(theme.Accent)
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, "You copied a "+p.Name+" token ("+vault.Mask(tp.offer)+")", widget.RunStyle{Size: theme.TextLabel})
			})
			if widget.Button(c, widget.Primary, nil, "Use it", "").Clicked() {
				f.values[field] = tp.offer
				f.page = nil
			}
			if widget.IconButton(c, icon.Close, "Not this one", false).Clicked() {
				tp.before, tp.offer = tp.offer, ""
			}
		})
	})
}

// foundFor are the logins found on the host for platform id.
func foundFor(all []proto.VaultFound, id string) []proto.VaultFound {
	var out []proto.VaultFound
	for _, fd := range all {
		if fd.Platform == id {
			out = append(out, fd)
		}
	}
	return out
}

// useFound fills the form with login fd.
func (f *form) useFound(fd proto.VaultFound) {
	if f.platform != nil && f.name == f.platform.ID && fd.Name != "" {
		f.name = fd.Name
	}
	if fd.Account != "" {
		f.account = fd.Account
	}
	for k, val := range fd.Fields {
		f.values[k] = val
	}
}

// freeName is name, or name with a number after it when a credential of
// list already has it.
func freeName(name string, list []vault.Credential) string {
	taken := func(n string) bool {
		for _, c := range list {
			if strings.EqualFold(c.Name, n) {
				return true
			}
		}
		return false
	}
	if name == "" || !taken(name) {
		return name
	}
	for i := 2; ; i++ {
		if n := fmt.Sprintf("%s-%d", name, i); !taken(n) {
			return n
		}
	}
}

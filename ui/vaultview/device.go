package vaultview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// deviceSection signs in to p in the browser, filling the token in: the
// user's OAuth app's client ID, then the code to type on p's page.
func (v *View) deviceSection(c *ui.Context, o Options, f *form, p vault.Platform) {
	d := f.device
	if d == nil {
		d = &deviceLogin{clientID: o.ClientIDs[p.ID]}
		f.device = d
	}
	widget.Field(c, "Or sign in in the browser", func() {
		switch {
		case d.code != "":
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, d.code).FontSize(theme.TextTitle).FontWeight(700).Font(theme.MonoFont).TextColor(theme.TextStrong).Selectable()
				if widget.Button(c, widget.Secondary, icon.Copy, "Copy", "").Clicked() {
					c.WriteClipboard(d.code)
				}
			})
			ui.Text(c, "Type it at "+d.url+" (opened in your browser), signed in to "+p.Name+"; the token fills in here.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		case d.waiting:
			ui.Text(c, "Asking "+p.Name+" for a code…").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		default:
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				widget.Input(c, &d.clientID, "Your OAuth app's client ID").Grow(1).Shrink(1).MinWidth(0)
				if widget.Button(c, widget.Secondary, icon.Globe, "Sign in", "").Clicked() {
					d.waiting, d.err = true, ""
					o.Actions.DeviceLogin(p.ID, d.clientID, func(code, url string) {
						d.code, d.url = code, url
					}, func(token string, err error) {
						d.waiting, d.code = false, ""
						if err != nil {
							d.err = err.Error()
							return
						}
						f.values[p.Device.Field], d.done = token, true
					})
				}
			})
			ui.Text(c, "No token to make: sign in on "+p.Name+"'s page. It takes an OAuth app of yours: "+p.Device.Help+".").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
		if d.err != "" {
			ui.Text(c, d.err).FontSize(theme.TextLabel).TextColor(theme.Error)
		}
		if d.done && d.code == "" && !d.waiting && d.err == "" {
			ui.Text(c, "Signed in: the token is filled in. Add it to keep it.").FontSize(theme.TextLabel).TextColor(theme.Done)
		}
	})
}

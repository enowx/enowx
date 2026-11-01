package ui

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/vaultview"
	"github.com/enowx/enowx/ui/widget"
)

// vaultAPIURLs are the AI platforms' own base URLs, for an agent pointed
// at one with a key from the vault.
var vaultAPIURLs = map[string]string{
	"anthropic":  "https://api.anthropic.com",
	"openai":     "https://api.openai.com/v1",
	"gemini":     "https://generativelanguage.googleapis.com/v1beta",
	"openrouter": "https://openrouter.ai/api/v1",
	"neuralwatt": "https://api.neuralwatt.com/v1",
}

// vaultAPIKeys offers the vault's AI API keys for an agent's model API:
// picking one fills the key, and the URL when empty.
func (a *App) vaultAPIKeys(c *ui.Context, f *agentAPIForm) {
	v := a.vaultOf()
	if v.data == nil {
		return
	}
	var keys []vault.Credential
	for _, cr := range v.data.Credentials {
		if vault.Lookup(cr.Platform).Group == "AI APIs" && cr.Fields["api_key"] != "" && !cr.Revoked {
			keys = append(keys, cr)
		}
	}
	if len(keys) == 0 {
		return
	}
	ui.Row(c).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "From the vault:").FontSize(theme.TextLabel).TextColor(theme.Muted)
		for _, cr := range keys {
			b := ui.ButtonBase(c.Key(cr.ID)).Height(26).PaddingX(8).Gap(6).Radius(theme.RadiusSmall).Border(1, theme.Border).Cursor(ui.CursorPointer).Label("Use " + cr.Name)
			if b.Hovered() {
				b.Background(theme.Hover)
			}
			b.Children(func() {
				vaultview.Logo(c, vault.Lookup(cr.Platform), 16)
				ui.Text(c, cr.Name).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
			})
			if widget.Pressable(b).Clicked() {
				f.key = cr.Fields["api_key"]
				if f.url == "" {
					f.url = vaultAPIURLs[cr.Platform]
				}
				v.used = time.Now()
			}
		}
	})
}

// credsField is New session's credentials to start the session with,
// lent by the vault (the host asked about first, as for any use).
func (a *App) credsField(c *ui.Context) {
	v := a.vaultOf()
	f := &a.newSession
	switch {
	case v.file == nil:
		return
	case v.data == nil:
		ui.Text(c, "Unlock the vault (Credentials) to start the session with logins from it.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		return
	case len(v.data.Credentials) == 0:
		return
	}
	widget.Field(c, "Credentials (optional)", func() {
		ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
			for _, cr := range v.data.Credentials {
				if cr.Revoked || cr.Type == vault.TypeSSHKey {
					continue
				}
				on := slices.Contains(f.creds, cr.Name) || cr.Auto
				b := ui.ButtonBase(c.Key(cr.ID)).Height(28).PaddingX(9).Gap(6).Radius(theme.RadiusCard).Cursor(ui.CursorPointer).Label(cr.Name)
				if on {
					b.Background(theme.AccentSoft).Border(1, theme.AccentLine)
				} else {
					b.Border(1, theme.Border)
					if b.Hovered() {
						b.Background(theme.Hover)
					}
				}
				b.Children(func() {
					vaultview.Logo(c, vault.Lookup(cr.Platform), 16)
					ui.Text(c, cr.Name).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
				})
				if widget.Pressable(b).Clicked() && !cr.Auto {
					if on {
						f.creds = slices.DeleteFunc(f.creds, func(n string) bool { return n == cr.Name })
					} else {
						f.creds = append(f.creds, cr.Name)
					}
				}
			}
		})
		ui.Text(c, "Their variables are in the session's environment; git, docker and ssh get the vault's logins anyway.").FontSize(theme.TextLabel).TextColor(theme.Muted)
	})
}

package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/vps"
	"github.com/enowx/enowx/internal/quietcmd"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Create a server makes a server at a cloud provider with the user's own
// API token (core/vps), with their SSH key for root, then adds it to the
// hosts and connects, which installs enowX there. The token is kept in the
// system's credential store, by provider.

// vpsForm is the Create a server dialog. It belongs to the main thread;
// what it asks the provider answers through the window's Update.
type vpsForm struct {
	open     bool
	provider string
	token    string
	remember bool
	loaded   string // the provider whose regions show
	regions  []vps.Option
	region   string
	sizes    []vps.Option
	size     string
	name     string
	key      string // the public key's file
	// existing: connect to a server the user has (servers, pick) rather
	// than create one, for providers that list them (vps.Lister).
	existing bool
	servers  []vps.Server
	listed   bool
	pick     string
	// fromVault: the token is the vault's credential for the provider,
	// which connecting reads again, not to keep in the keychain.
	fromVault bool
	busy      string // what is under way, "" for nothing
	err       string
}

func tokenKey(provider string) string { return "vps-token:" + provider }

// Where providers and servers are reached: replaced in tests.
var (
	vpsAPI  = ""   // a provider's API address, "" for its own
	sshPort = "22" // the port a new server's SSH answers on
)

// openCreateServer opens the dialog on the first provider, with its token
// from the credential store when there is one.
func (a *App) openCreateServer() {
	f := &vpsForm{open: true, provider: vps.Providers[0].ID, remember: true, name: "enowx-" + time.Now().Format("0102-1504")}
	f.key = publicKeyFile()
	a.creating = f
	a.loadToken(f)
}

func (a *App) loadToken(f *vpsForm) {
	provider := f.provider
	if tok := a.vaultToken(provider); tok != "" {
		f.token, f.remember, f.fromVault = tok, false, true
		a.loadRegions(f)
		return
	}
	go func() {
		tok, err := mygo.Secrets.Get(tokenKey(provider))
		a.onMain(func() {
			if err == nil && f.provider == provider && f.token == "" {
				f.token = string(tok)
				a.loadRegions(f)
			}
		})
	}()
}

// publicKeyFile is the user's SSH public key, "" when they have none.
func publicKeyFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, name := range []string{"id_ed25519.pub", "id_ecdsa.pub", "id_rsa.pub"} {
		p := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func (a *App) vpsClient(f *vpsForm) (vps.Client, error) { return vps.New(f.provider, f.token, vpsAPI) }

func (a *App) loadRegions(f *vpsForm) {
	c, err := a.vpsClient(f)
	if err != nil {
		f.err = err.Error()
		return
	}
	f.busy, f.err = "Reading regions…", ""
	provider := f.provider
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		regions, err := c.Regions(ctx)
		a.onMain(func() {
			if f.provider != provider {
				return
			}
			f.busy = ""
			if err != nil {
				f.err = err.Error()
				return
			}
			f.regions, f.loaded, f.sizes, f.size = regions, provider, nil, ""
			if len(regions) > 0 {
				f.region = regions[0].ID
				a.loadSizes(f)
			}
			if l, ok := c.(vps.Lister); ok {
				a.loadServers(f, l)
			}
		})
	}()
}

func (a *App) loadSizes(f *vpsForm) {
	c, err := a.vpsClient(f)
	if err != nil {
		f.err = err.Error()
		return
	}
	region := f.region
	f.busy = "Reading sizes…"
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		sizes, err := c.Sizes(ctx, region)
		a.onMain(func() {
			if f.region != region {
				return
			}
			f.busy = ""
			if err != nil {
				f.err = err.Error()
				return
			}
			f.sizes, f.size = sizes, ""
			if len(sizes) > 0 {
				f.size = sizes[0].ID
			}
		})
	}()
}

func (a *App) createServerDialog(c *ui.Context) {
	f := a.creating
	if f == nil {
		return
	}
	if !f.open {
		a.creating = nil
		return
	}
	creating := strings.HasPrefix(f.busy, "Creating") || strings.HasPrefix(f.busy, "Waiting") || strings.HasPrefix(f.busy, "Adding")
	widget.Dialog(c, &f.open, 560, 14, func() {
		about := "At a cloud provider, with your own account and API token: an Ubuntu 24.04 server with your SSH key, which enowX then sets up and connects to. The provider bills you for it."
		if vps.NeedsCertificate(f.provider) {
			about = "A Debian 13 VM at InstaBox, with your API key, reached through InstaBox's SSH gateway with a certificate it signs for your SSH key each time enowX connects. InstaBox bills it by the second while it runs, and pauses it when idle."
		}
		widget.DialogHeader(c, "Create a server", about, false)
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
		widget.Field(c, "Provider", func() {
			names := make([]string, len(vps.Providers))
			cur := 0
			for i, p := range vps.Providers {
				names[i] = p.Name
				if p.ID == f.provider {
					cur = i
				}
			}
			if i := widget.Segmented(c, names, cur, nil); i >= 0 && vps.Providers[i].ID != f.provider {
				*f = vpsForm{open: true, provider: vps.Providers[i].ID, remember: f.remember, name: f.name, key: f.key}
				a.loadToken(f)
			}
		})
		p, _ := vps.Find(f.provider)
		widget.Field(c, "API token", func() {
			ui.Column(c).FillWidth().Gap(5).Children(func() {
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					widget.Input(c, &f.token, "Paste the token").Password().Grow(1)
					if widget.GhostButton(c, "Use it").Disabled(strings.TrimSpace(f.token) == "" || f.busy != "").Clicked() {
						a.loadRegions(f)
					}
				})
				ui.Text(c, p.TokenHelp+".").FontSize(theme.TextLabel).TextColor(theme.Muted)
				if f.fromVault {
					ui.Text(c, "From the vault: its "+p.Name+" credential.").FontSize(theme.TextLabel).TextColor(theme.Done)
				} else {
					switchRow(c, &f.remember, "Keep it in the system's keychain", "To create more servers, and to delete this one from enowX.")
				}
			})
		})
		if f.loaded == f.provider && f.listed {
			widget.Field(c, "Machine", func() {
				i := 0
				if f.existing {
					i = 1
				}
				if j := widget.Segmented(c, []string{"New machine", fmt.Sprintf("Use one you have (%d)", len(f.servers))}, i, nil); j >= 0 {
					f.existing = j == 1
				}
			})
		}
		if f.loaded == f.provider && f.existing {
			a.existingServers(c, f)
		}
		if f.loaded == f.provider && len(f.regions) > 0 && !f.existing {
			if len(f.regions) > 1 || !vps.NeedsCertificate(f.provider) { // InstaBox has no regions to pick
				widget.Field(c, "Region", func() {
					prev := f.region
					optionSelect(c, &f.region, f.regions, "Region")
					if f.region != prev {
						a.loadSizes(f)
					}
				})
			}
			widget.Field(c, "Size", func() {
				if len(f.sizes) == 0 {
					ui.Text(c, "No sizes offered there.").FontSize(theme.TextLabel).TextColor(theme.Muted)
					return
				}
				optionSelect(c, &f.size, f.sizes, "Size")
			})
			widget.Field(c, "Name", func() { widget.Input(c, &f.name, "enowx-1").FillWidth().Font(theme.MonoFont) })
			widget.Field(c, "SSH key", func() {
				if f.key != "" {
					whose := ", for root"
					if vps.NeedsCertificate(f.provider) {
						whose = ", which " + p.Name + " signs a certificate for each time enowX connects"
					}
					ui.Text(c, f.key+whose).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
					return
				}
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					ui.Text(c, "You have no SSH key yet.").FontSize(theme.TextLabel).TextColor(theme.Muted).Grow(1)
					if widget.GhostButton(c, "Make one").Clicked() {
						a.makeKey(f)
					}
				})
			})
		}
		widget.DialogFooter(c, f.busy, func() {
			if widget.GhostButton(c, "Cancel").Disabled(creating).Clicked() && !creating {
				f.open = false
			}
			if f.existing {
				ok := f.pick != "" && f.key != "" && f.busy == ""
				if widget.PrimaryButton(c, icon.Server, "Connect", "").Disabled(!ok).Clicked() && ok {
					a.useServer(f)
				}
				return
			}
			ok := f.size != "" && f.key != "" && strings.TrimSpace(f.name) != "" && f.busy == ""
			if widget.PrimaryButton(c, icon.Server, "Create", "").Disabled(!ok).Clicked() && ok {
				a.createServer(f)
			}
		})
	})
	if creating {
		f.open = true // it finishes, or fails, before the dialog goes
	}
}

// optionSelect is a drop-down of provider options, with prices.
func optionSelect(c *ui.Context, value *string, opts []vps.Option, label string) {
	sel := ui.SelectBase(c, value)
	cur := vps.Option{Name: *value}
	for _, o := range opts {
		if o.ID == *value {
			cur = o
		}
	}
	widget.Trigger(sel.Trigger, sel.Open()).FillWidth().Label(label).Children(func() {
		ui.Text(c, cur.Name).FontSize(theme.TextUI).SingleLine()
		ui.Text(c, optionNote(cur)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
		ui.Spacer(c)
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.Muted)
	})
	sel.Popup(func(panel ui.Element) {
		widget.Popup(panel.Margin(6, 0, 0, 0).Width(480))
		ui.Scroll(c).FillWidth().MaxHeight(320).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, o := range opts {
					item := sel.Item(o.ID)
					widget.PopupItem(item, item.Highlighted()).Children(func() {
						ui.Text(c, o.Name).FontSize(theme.TextUI).SingleLine()
						ui.Text(c, optionNote(o)).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine().Shrink(1)
					})
				}
			})
		})
	})
}

func optionNote(o vps.Option) string {
	note := o.Detail
	if o.Monthly > 0 {
		note += fmt.Sprintf(" · %.2f a month", o.Monthly)
	}
	return note
}

// makeKey makes the user an ed25519 SSH key with ssh-keygen, no passphrase,
// where they have none.
func (a *App) makeKey(f *vpsForm) {
	home, err := os.UserHomeDir()
	if err != nil {
		f.err = err.Error()
		return
	}
	key := filepath.Join(home, ".ssh", "id_ed25519")
	f.busy = "Making an SSH key…"
	go func() {
		_ = os.MkdirAll(filepath.Dir(key), 0o700)
		out, err := quietcmd.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-C", "enowX", "-f", key).CombinedOutput()
		a.onMain(func() {
			f.busy = ""
			if err != nil {
				f.err = "ssh-keygen: " + strings.TrimSpace(string(out))
				return
			}
			f.key = key + ".pub"
		})
	}()
}

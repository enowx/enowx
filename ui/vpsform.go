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
	about, detail := "At Hetzner, DigitalOcean or Vultr, with your own API token.", "An Ubuntu 24.04 server with your SSH key, which enowX then sets up and connects to. Paid to the provider."
	if vps.NeedsCertificate(f.provider) {
		about, detail = "At InstaBox, with your API key.", "A Debian 13 VM, reached through InstaBox's SSH gateway with a certificate it signs for your SSH key each time enowX connects. InstaBox bills it by the second while it runs, and pauses it when idle."
	}
	p, _ := vps.Find(f.provider)
	picksRegion := f.loaded == f.provider && len(f.regions) > 0 && !f.existing
	widget.Sheet(c, &f.open, 560, func() {
		widget.SheetHead(c, &f.open, "Create a server", about)
	}, func() {
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
		widget.FieldCols(c, func() {
			widget.Field(c, "Provider", func() { a.providerSelect(c, f) })
		}, func() {
			if !picksRegion || (len(f.regions) <= 1 && vps.NeedsCertificate(f.provider)) { // InstaBox has no regions to pick
				return
			}
			widget.Field(c, "Region", func() {
				prev := f.region
				optionSelect(c, &f.region, f.regions, "Region")
				if f.region != prev {
					a.loadSizes(f)
				}
			})
		})
		widget.Field(c, "API token", func() {
			ui.Row(c.Key("token")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					widget.Input(c, &f.token, "Paste the token").Password().FillWidth()
				})
				if widget.GhostButton(c, "Use it").Disabled(strings.TrimSpace(f.token) == "" || f.busy != "").Clicked() {
					a.loadRegions(f)
				}
			})
			widget.CardNote(c.Key("help"), p.TokenHelp+".", 0)
			if f.fromVault {
				widget.StatusLine(c.Key("vault"), theme.Done, false, "From the vault: its "+p.Name+" credential.")
			} else {
				switchRow(c, &f.remember, "Keep it in the system's keychain", "To create more servers, and to delete this one from enowX.")
			}
		})
		if f.loaded == f.provider && f.listed {
			widget.Field(c, "Machine", func() {
				i := 0
				if f.existing {
					i = 1
				}
				ui.Row(c.Key("machine")).FillWidth().Shrink(0).Children(func() {
					if j := widget.Segmented(c, []string{"New machine", fmt.Sprintf("Use one you have (%d)", len(f.servers))}, i, nil); j >= 0 {
						f.existing = j == 1
					}
				})
			})
		}
		if f.loaded == f.provider && f.existing {
			a.existingServers(c, f)
		}
		if picksRegion {
			// Under a parent of their own: the provider's row above is also FieldCols.
			ui.Column(c.Key("size-name")).FillWidth().Shrink(0).Children(func() {
				widget.FieldCols(c, func() {
					widget.Field(c, "Size", func() {
						if len(f.sizes) == 0 {
							widget.CardNote(c, "No sizes offered there.", 0)
							return
						}
						optionSelect(c, &f.size, f.sizes, "Size")
					})
				}, func() {
					widget.Field(c, "Name", func() { widget.Input(c, &f.name, "enowx-1").FillWidth() })
				})
			})
			a.vpsKeyField(c, f, p.Name)
		}
		widget.CardNote(c.Key("detail"), detail, 0)
	}, func() {
		if f.busy != "" {
			ui.Row(c.Key("busy")).Grow(1).Basis(0).MinWidth(0).Gap(theme.S2).Children(func() {
				widget.Spinner(c, 12)
				ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, f.busy, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft}) })
			})
		}
		if widget.GhostButton(c, "Cancel").Disabled(creating).Clicked() && !creating {
			f.open = false
		}
		if f.existing {
			ok := f.pick != "" && f.key != "" && f.busy == ""
			if widget.PrimaryButton(c, nil, "Connect", "").Disabled(!ok).Clicked() && ok {
				a.useServer(f)
			}
			return
		}
		ok := f.size != "" && f.key != "" && strings.TrimSpace(f.name) != "" && f.busy == ""
		if widget.PrimaryButton(c, nil, "Create", "").Disabled(!ok).Clicked() && ok {
			a.createServer(f)
		}
	})
	if creating {
		f.open = true // it finishes, or fails, before the dialog goes
	}
}

// providerSelect picks the provider, which starts the form over on its token.
func (a *App) providerSelect(c *ui.Context, f *vpsForm) {
	opts := make([]vps.Option, len(vps.Providers))
	for i, p := range vps.Providers {
		opts[i] = vps.Option{ID: p.ID, Name: p.Name}
	}
	id := f.provider
	optionSelect(c, &id, opts, "Provider")
	if id != f.provider {
		*f = vpsForm{open: true, provider: id, remember: f.remember, name: f.name, key: f.key}
		a.loadToken(f)
	}
}

// vpsKeyField says which of the user's SSH keys a new server takes, or makes one.
func (a *App) vpsKeyField(c *ui.Context, f *vpsForm, provider string) {
	widget.Field(c, "SSH key", func() {
		if f.key == "" {
			ui.Row(c.Key("no-key")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.CardNote(c, "You have no SSH key yet.", 0) })
				if widget.GhostButton(c, "Make one").Clicked() {
					a.makeKey(f)
				}
			})
			return
		}
		whose := ", for root"
		if vps.NeedsCertificate(f.provider) {
			whose = ", which " + provider + " signs a certificate for each time enowX connects"
		}
		widget.CardNote(c.Key("key"), f.key+whose, 0)
	})
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
		optionFace(c, cur)
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	sel.Popup(func(panel ui.Element) {
		widget.Popup(panel.Margin(theme.S1, 0, 0, 0).Width(max(sel.Trigger.Bounds().W, 240)))
		ui.Scroll(c).FillWidth().MaxHeight(320).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, o := range opts {
					item := sel.Item(o.ID)
					widget.PopupItem(item, item.Highlighted()).Children(func() {
						optionFace(c, o)
					})
				}
			})
		})
	})
}

// optionFace is an option's name and, in what's left, its note, each running
// when it can't fit.
func optionFace(c *ui.Context, o vps.Option) {
	ui.Row(c.Key("name")).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, o.Name, widget.RunStyle{}) })
	if note := optionNote(o); note != "" {
		ui.Row(c.Key("note")).Grow(2).Basis(0).MinWidth(0).Children(func() {
			widget.Running(c, note, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
	}
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

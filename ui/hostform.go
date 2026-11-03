package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/vps"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// addHostForm is the state of the Add VPS dialog.
type addHostForm struct {
	open     bool
	manual   bool // show the address form when the tailnet has devices
	name     string
	address  string
	identity string
	jump     string
	device   bool // one of the user's computers, not a server
	tailnet  bool // through enowX's own tailnet device (hosts.Host.Tailnet)
	err      string
	// editing is the saved host being edited; its ID is "" when adding.
	editing        hosts.Host
	forgetPassword bool
	// Whom sessions run as when SSH signs in as root: root itself, or a
	// regular user ("" lets enowX pick its own, made when missing).
	asRoot bool
	runAs  string
}

func (a *App) addHostDialog(c *ui.Context) {
	f := &a.addHost
	if !f.open {
		return
	}
	editing := f.editing.ID != ""
	hasTailnet := len(a.peers) > 0 && !editing
	byTailnet := hasTailnet && !f.manual
	title, about := "Add VPS", "Any Linux server you can reach over SSH, by IP, domain or Tailscale name."
	if editing {
		title = "Edit " + f.editing.Name
	}
	submit := false
	widget.FormSheet(c, &f.open, 640, title, about, func() {
		if hasTailnet {
			cur := 0
			if f.manual {
				cur = 1
			}
			ui.Row(c.Key("how")).FillWidth().Shrink(0).Children(func() {
				if i := widget.Segmented(c, []string{"On your tailnet", "By address"}, cur, nil); i >= 0 {
					f.manual, f.err = i == 1, ""
				}
			})
		} else if a.peersErr != "" && !editing {
			widget.CardNote(c.Key("peers-err"), a.peersErr, 0)
		}
		if byTailnet {
			a.tailnetSection(c)
			ui.Row(c.Key("by-address")).FillWidth().Shrink(0).Children(func() {
				ui.Spacer(c)
				if widget.GhostButton(c, "Add a server by address instead").Clicked() {
					f.manual = true
				}
			})
			return
		}
		if !editing {
			a.sshConfigSection(c)
		}
		widget.Field(c, "SSH address", func() {
			in := widget.Input(c, &f.address, "203.0.113.10 (signs in as root) or user@my-vps:2222").FillWidth().Font(theme.MonoFont).AutoFocus()
			submit = in.Submitted()
		})
		widget.FieldCols(c, func() {
			widget.Field(c, "Name (optional)", func() {
				submit = widget.Input(c, &f.name, "vps-sg-1").FillWidth().Submitted() || submit
			})
		}, func() {
			widget.Field(c, "Listed under", func() {
				kind := ""
				if f.device {
					kind = hosts.KindDevice
				}
				optionSelect(c, &kind, []vps.Option{{ID: "", Name: "Servers"}, {ID: hosts.KindDevice, Name: "Devices"}}, "Listed under")
				f.device = kind == hosts.KindDevice
			})
		})
		widget.Field(c, "Private key (optional)", func() {
			submit = widget.Input(c, &f.identity, "~/.ssh/id_ed25519 · also tries ~/.ssh/config and the SSH agent").FillWidth().Font(theme.MonoFont).Submitted() || submit
		})
		widget.Field(c, "Jump host (optional)", func() {
			submit = widget.Input(c, &f.jump, "user@bastion · a server in front of it; ~/.ssh/config's ProxyJump otherwise").FillWidth().Font(theme.MonoFont).Submitted() || submit
		})
		a.runAsField(c, f)
		switchRow(c, &f.tailnet, "Through enowX's own tailnet", "enowX joins your tailnet as a device of its own, so the server is reached by its tailnet name without Tailscale on this machine. It asks you to sign in once.")
		if f.editing.PasswordAuth && !f.forgetPassword {
			ui.Row(c.Key("password")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					widget.CardNote(c, "A password is saved in the keychain for this host.", 0)
				})
				if widget.GhostButton(c, "Forget password").Clicked() {
					f.forgetPassword = true
				}
			})
		}
		widget.CardNote(c.Key("about"), "enowX signs in with your SSH keys and asks for a password if the server wants one. It installs its daemon in ~/"+transport.HomeName+" on the server; sessions keep running there when you disconnect.", 0)
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
	}, func() {
		if byTailnet {
			return
		}
		label := "Add and connect"
		if editing {
			label = "Save and connect"
		}
		if widget.PrimaryButton(c, nil, label, "").Clicked() {
			submit = true
		}
	})
	if submit {
		a.saveHost()
	}
}

func (a *App) saveHost() {
	f := &a.addHost
	if a.opts.Hosts == nil {
		f.err = "This build keeps no host list."
		return
	}
	if strings.TrimSpace(f.address) == "" {
		f.err = "Type the server's SSH address."
		return
	}
	h := f.editing // keeps the ID, the run-as choice and a saved password
	h.Name, h.Address, h.IdentityFile, h.Jump = f.name, f.address, strings.TrimSpace(f.identity), strings.TrimSpace(f.jump)
	h.Kind, h.Tailnet = "", f.tailnet
	if f.device {
		h.Kind = hosts.KindDevice
	}
	if f.forgetPassword {
		h.PasswordAuth = false
	}
	h.KeepRoot, h.RunAs = f.asRoot, ""
	if !f.asRoot {
		h.RunAs = strings.TrimSpace(f.runAs)
		if h.RunAs != "" && !hosts.ValidUserName(h.RunAs) {
			f.err = "Run sessions as: use lowercase letters, digits, - and _, starting with a letter."
			return
		}
	}
	var err error
	if h.ID != "" {
		if err = a.opts.Hosts.Update(h); err == nil {
			h, _ = a.opts.Hosts.Get(h.ID)
			if f.forgetPassword {
				a.forgetPassword(f.editing)
			}
		}
	} else {
		h, err = a.opts.Hosts.Add(h)
	}
	if err != nil {
		f.err = err.Error()
		return
	}
	f.open = false
	a.reloadHosts()
	a.connect(h)
}

// openEditHost opens the host dialog on a saved host.
func (a *App) openEditHost(h hosts.Host) {
	a.addHost = addHostForm{open: true, manual: true, editing: h, name: h.Name, address: h.Address, identity: h.IdentityFile, jump: h.Jump, device: h.Kind == hosts.KindDevice, tailnet: h.Tailnet,
		asRoot: h.KeepRoot, runAs: h.RunAs}
}

func (a *App) storedHost(id string) (hosts.Host, bool) {
	if a.opts.Hosts == nil {
		return hosts.Host{}, false
	}
	return a.opts.Hosts.Get(id)
}

// sshConfigSection offers the hosts ~/.ssh/config names that aren't in
// the list yet: picking one fills the form with its alias, which ssh's
// file resolves (user, port, key, jump host).
func (a *App) sshConfigSection(c *ui.Context) {
	f := &a.addHost
	var offered []transport.SSHConfigHost
	for _, h := range transport.SSHConfigHosts() {
		if !slices.ContainsFunc(a.hostList, func(o hosts.Host) bool { return o.Address == h.Alias }) {
			offered = append(offered, h)
		}
	}
	if len(offered) == 0 {
		return
	}
	widget.Field(c, "From ~/.ssh/config", func() {
		ui.Row(c).FillWidth().Gap(theme.S1).Wrap().Children(func() {
			for _, h := range offered {
				on := f.address == h.Alias
				b := ui.ButtonBase(c.Key(h.Alias)).Height(theme.ControlHeight).PaddingX(theme.S3).Gap(theme.S1).Radius(theme.RadiusFull).
					Cursor(ui.CursorPointer).Label(h.Alias).Tooltip(h.Real.String())
				widget.Well(b, false)
				if on {
					b.Background(theme.Selected)
				}
				b.Children(func() {
					ui.Text(c, h.Alias).FontSize(theme.TextUI).SingleLine()
				})
				if b.Clicked() {
					f.address, f.name, f.err = h.Alias, h.Alias, ""
				}
			}
		})
	})
}

// runAsField picks whom sessions run as when SSH signs in as root: a
// regular user, as agents want, or root.
func (a *App) runAsField(c *ui.Context, f *addHostForm) {
	widget.Field(c, "Run sessions as (when signed in as root)", func() {
		ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
			cur := 0
			if f.asRoot {
				cur = 1
			}
			ui.Row(c.Key("seg")).FillWidth().Shrink(0).Children(func() {
				if i := widget.Segmented(c, []string{"A regular user", "root"}, cur, nil); i >= 0 {
					f.asRoot = i == 1
				}
			})
			if f.asRoot {
				widget.CardNote(c.Key("root"), "The daemon and every session run as root. Claude Code won't skip permission prompts as root, and agents can change anything on the server.", 0).TextColor(theme.Warning)
				return
			}
			widget.Input(c, &f.runAs, defaultRunAs+" (made if missing)").FillWidth()
			widget.CardNote(c.Key("user"), "SSH still signs in as root; enowX runs its daemon and sessions as this user. Sessions started as another user stay with it.", 0)
		})
	})
}

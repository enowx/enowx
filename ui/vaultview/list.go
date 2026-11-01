package vaultview

import (
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// row is one credential: its platform's mark, its name and account, the
// end of its value, its status, where it may be used, and what to do.
func (v *View) row(c *ui.Context, o Options, cr vault.Credential) {
	p := vault.Lookup(cr.Platform)
	now := time.Now()
	card := widget.Card(c).FillWidth().Padding(12, 14).Gap(12).Row().AlignItems(ui.Center)
	card.Children(func() {
		Logo(c, p, 34)
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, cr.Name).FontSize(theme.TextUI).FontWeight(600).TextColor(theme.TextStrong).SingleLine()
				if cr.Default {
					widget.Tag(c, "default", theme.Accent)
				}
				if cr.Auto {
					widget.Tag(c, "every session", ui.Color{})
				}
			})
			detail := p.Name
			if cr.Account != "" {
				detail += " · " + cr.Account
			}
			if h := vault.Hint(mainValue(cr)); h != "" {
				detail += " · " + h
			}
			ui.Text(c, detail).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				st := cr.State(now)
				widget.Badge(c, widget.Status{Label: statusLabel(st), Color: statusColor(st), Hollow: st == proto.CredUnchecked})
				ui.Text(c, "·").FontSize(theme.TextLabel).TextColor(theme.Muted)
				ui.Text(c, whereLabel(cr, o.Hosts)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
				if !cr.Expires.IsZero() && st != proto.CredExpired {
					ui.Text(c, "·").FontSize(theme.TextLabel).TextColor(theme.Muted)
					col := theme.TextSoft
					if cr.ExpiresSoon(now, 7*24*time.Hour) {
						col = theme.Warning
					}
					ui.Text(c, "expires "+cr.Expires.Local().Format("2 Jan 2006")).FontSize(theme.TextLabel).TextColor(col).SingleLine()
				}
			})
		})
		if o.Testing[cr.ID] {
			widget.Spinner(c, 14)
		} else if widget.Button(c, widget.Secondary, icon.Check, "Test", "").Tooltip("Check it still works, with "+p.Name).Clicked() && o.Actions.Test != nil {
			o.Actions.Test(cr.ID)
		}
		if widget.IconButton(c, icon.Pencil, "Edit "+cr.Name, false).Clicked() {
			v.form = editForm(cr)
		}
		widget.MoreMenu(c, &v.menu, "cred:"+cr.ID, func(done func()) {
			if !cr.Default && widget.MenuItem(c, icon.Check, "Make it "+p.Name+"'s default", false) {
				o.Actions.SetDefault(cr.ID)
				done()
			}
			label, rev := "Revoke: no host may use it", true
			if cr.Revoked {
				label, rev = "Allow it again", false
			}
			if widget.MenuItem(c, icon.Stop, label, false) {
				o.Actions.Revoke(cr.ID, rev)
				done()
			}
			widget.MenuSeparator(c)
			if widget.MenuItem(c, icon.Trash, "Remove from the vault", true) {
				o.Actions.Remove(cr.ID)
				done()
			}
		})
	})
}

// mainValue is the value a hint is made of.
func mainValue(cr vault.Credential) string {
	for _, f := range vault.Lookup(cr.Platform).Fields {
		if f.Secret && cr.Fields[f.Key] != "" {
			return cr.Fields[f.Key]
		}
	}
	return ""
}

func statusLabel(st string) string {
	switch st {
	case proto.CredValid:
		return "Works"
	case proto.CredInvalid:
		return "Refused"
	case proto.CredExpired:
		return "Expired"
	case proto.CredRevoked:
		return "Revoked"
	}
	return "Not checked"
}

func statusColor(st string) ui.Color {
	switch st {
	case proto.CredValid:
		return theme.Done
	case proto.CredInvalid, proto.CredExpired, proto.CredRevoked:
		return theme.Error
	}
	return theme.Muted
}

// whereLabel says where a credential may be used without asking.
func whereLabel(cr vault.Credential, hosts []Host) string {
	names := make([]string, 0, len(cr.Hosts))
	for _, id := range cr.Hosts {
		for _, h := range hosts {
			if h.ID == id {
				names = append(names, h.Name)
			}
		}
	}
	scope := ""
	if len(cr.Projects) > 0 {
		scope = " · only " + strings.Join(cr.Projects, ", ")
	}
	switch {
	case len(names) == 0 && cr.HostsOnly:
		return "no host" + scope
	case len(names) == 0:
		return "asks on each host" + scope
	case cr.HostsOnly:
		return "only on " + strings.Join(names, ", ") + scope
	}
	return "allowed on " + strings.Join(names, ", ") + ", asks elsewhere" + scope
}

// Logo is a platform's mark: its own on a tile of its brand's color, else
// its initials on a tile of a color of its own.
func Logo(c *ui.Context, p vault.Platform, size float32) {
	if svg, brand, ok := logoOf(p.ID); ok {
		fg := readable(brand)
		ui.Row(c).Size(size, size).Shrink(0).Radius(size/4).Center().Background(theme.Fade(brand, 0.14)).
			Border(1, theme.Fade(fg, 0.3)).Label(p.Name).Children(func() {
			ui.Icon(c, svg).FontSize(size * 0.5).TextColor(fg)
		})
		return
	}
	h := fnv.New32a()
	h.Write([]byte(p.ID))
	hue := float64(h.Sum32()%360) / 360
	bg := hsl(hue, 0.55, 0.45, 0.16)
	fg := hsl(hue, 0.6, 0.62, 1)
	ui.Row(c).Size(size, size).Shrink(0).Radius(size/4).Center().Background(bg).Border(1, theme.Fade(fg, 0.3)).Children(func() {
		if p.ID == "ssh" {
			ui.Icon(c, icon.Key).FontSize(size * 0.45).TextColor(fg)
			return
		}
		ui.Text(c, initials(p.Name)).FontSize(size * 0.36).FontWeight(700).TextColor(fg)
	})
}

// initials is a name's first letters: "GitHub" GH, "Docker Hub" DH.
func initials(name string) string {
	var out []rune
	words := strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '/' || r == '.' || r == '-' })
	for _, w := range words {
		rs := []rune(w)
		out = append(out, unicode.ToUpper(rs[0]))
		if len(words) == 1 {
			for _, r := range rs[1:] {
				if unicode.IsUpper(r) {
					out = append(out, r)
					break
				}
			}
		}
		if len(out) >= 2 {
			break
		}
	}
	if len(out) == 1 && len([]rune(words[0])) > 1 {
		out = append(out, unicode.ToLower([]rune(words[0])[1]))
	}
	return string(out)
}

// audit is the last uses on the host connected to.
func (v *View) audit(c *ui.Context, o Options) {
	if len(o.Audit) == 0 {
		return
	}
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		widget.SectionLabel(c, "USED ON "+strings.ToUpper(o.Current.Name), theme.Muted)
		card := widget.Card(c).FillWidth().Padding(4, 0)
		card.Children(func() {
			list := o.Audit
			if len(list) > 12 {
				list = list[len(list)-12:]
			}
			for i := len(list) - 1; i >= 0; i-- {
				e := list[i]
				ui.Row(c.Key(i)).FillWidth().Height(32).PaddingX(14).Gap(10).AlignItems(ui.Center).Children(func() {
					col := theme.Done
					if !e.Allowed {
						col = theme.Error
					}
					widget.Dot(c, col, false, 6)
					ui.Text(c, auditText(e)).FontSize(theme.TextLabel).TextColor(theme.Text).SingleLine().Grow(1).MinWidth(0)
					ui.Text(c, e.Time.Local().Format("2 Jan 15:04")).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
				})
			}
		})
	})
}

func auditText(e proto.VaultAuditEntry) string {
	what := map[string]string{proto.VaultEnv: "variables", proto.VaultSession: "a new session", proto.VaultGit: "git", proto.VaultDocker: "docker",
		proto.VaultSign: "ssh", proto.VaultAdd: "adding", proto.VaultRemove: "removing", proto.VaultTest: "a check", proto.VaultLogin: "a login"}[e.Action]
	if what == "" {
		what = e.Action
	}
	name := e.Credential
	if name == "" {
		name = "a credential"
	}
	s := fmt.Sprintf("%s for %s", name, what)
	if e.Detail != "" {
		s += ": " + e.Detail
	}
	if !e.Allowed {
		s = "Refused " + s
	}
	return s
}

// hsl is a color of hue h, saturation s and lightness l (0 to 1).
func hsl(h, s, l float64, alpha float32) ui.Color {
	f := func(n float64) uint8 {
		k := math.Mod(n+h*12, 12)
		a := s * min(l, 1-l)
		return uint8(255 * (l - a*max(-1, min(k-3, 9-k, 1))))
	}
	return ui.RGBA(f(0), f(8), f(4), alpha)
}

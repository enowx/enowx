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

// row is one credential: its platform's mark and its name, its status, the end
// of its value, where it may be used, and what to do.
func (v *View) row(c *ui.Context, o Options, cr vault.Credential) {
	p := vault.Lookup(cr.Platform)
	now := time.Now()
	st := cr.State(now)
	card := widget.Card(c)
	if statusColor(st) == theme.Error {
		card.Background(theme.Fade(theme.Error, 0.13))
	}
	card.Children(func() {
		ui.Row(c).FillWidth().Shrink(0).MinHeight(24).Gap(theme.S2).Children(func() {
			Logo(c, p, 24)
			ui.Row(c).Grow(1).MinWidth(0).Children(func() {
				widget.Running(c, title(cr, p), widget.RunStyle{Size: theme.TextRead, Weight: 600})
			})
		})
		label, col := statusLabel(st), statusColor(st)
		if (st == proto.CredValid || st == proto.CredUnchecked) && cr.ExpiresSoon(now, expiringWithin) {
			label, col = "Rotate soon", theme.Warning
		}
		widget.StatusLine(c, col, false, label)
		ui.Row(c).FillWidth().Shrink(0).MinWidth(0).Children(func() {
			widget.Running(c.Key("value"), valueLine(cr, p), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		})
		if line, warn := useLine(cr, o, now); line != "" {
			col := theme.TextSoft
			if warn {
				col = theme.Warning
			}
			ui.Row(c).FillWidth().Shrink(0).MinWidth(0).Children(func() {
				widget.Running(c.Key("use"), line, widget.RunStyle{Size: theme.TextLabel, Color: col})
			})
		}
		ui.Row(c).FillWidth().Shrink(0).Wrap().Gap(theme.S1).Tooltip(whereLabel(cr, o.Hosts)).Children(func() {
			if cr.Default {
				widget.Chip(c, "default", ui.Color{})
			}
			if cr.Auto {
				widget.Chip(c, "every session", ui.Color{})
			}
			for _, name := range whereChips(cr, o.Hosts) {
				widget.Chip(c, name, ui.Color{})
			}
		})
		ui.Spacer(c) // the foot stays at the card's bottom, however tall the row
		widget.CardFoot(c, func() {
			if o.Testing[cr.ID] {
				widget.Spinner(c, 14)
			} else if widget.GhostButton(c, "Test").Tooltip("Check it still works, with "+p.Name).Clicked() && o.Actions.Test != nil {
				o.Actions.Test(cr.ID)
			}
			if widget.GhostButton(c, "Details").Label("Details of " + cr.Name).Clicked() {
				v.OpenDetails(cr.ID)
			}
			ui.Spacer(c)
			v.rowMenu(c, o, cr, p)
		})
	})
}

// rowMenu is what else can be done with a credential.
func (v *View) rowMenu(c *ui.Context, o Options, cr vault.Credential, p vault.Platform) {
	key := "cred:" + cr.ID
	b := widget.IconButton(c.Key("more:"+key), icon.More, "More for "+cr.Name, false)
	v.menu.OpenBelow(b, key, theme.ControlHeight)
	v.menu.Show(c, b, key, func(done func()) {
		if widget.MenuItem(c, icon.Pencil, "Edit…", false) {
			v.form = editForm(cr)
			done()
		}
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
}

// title is what a credential's card is called: its platform, when it goes by
// the platform's own name, else the name the user gave it.
func title(cr vault.Credential, p vault.Platform) string {
	if strings.EqualFold(cr.Name, p.ID) {
		return p.Name
	}
	return cr.Name
}

// valueLine says what kind of value it holds, whose, and how it ends.
func valueLine(cr vault.Credential, p vault.Platform) string {
	parts := []string{mainLabel(cr, p)}
	if cr.Account != "" {
		parts = append(parts, cr.Account)
	}
	if h := vault.Hint(mainValue(cr)); h != "" {
		parts = append(parts, h)
	}
	return strings.Join(parts, " · ")
}

// useLine is the card's line on its use: its expiry, else how much it was used
// today, else the user's note. warn says the expiry is close.
func useLine(cr vault.Credential, o Options, now time.Time) (line string, warn bool) {
	if !cr.Expires.IsZero() && cr.State(now) != proto.CredExpired {
		return expiryLine(cr.Expires, now), cr.ExpiresSoon(now, expiringWithin)
	}
	n := 0
	for _, e := range o.Audit {
		if e.Credential == cr.Name && e.Allowed && sameDay(e.Time, now) {
			n++
		}
	}
	switch {
	case n == 1:
		return "Used once today", false
	case n > 1:
		return fmt.Sprintf("Used %d times today", n), false
	}
	return cr.Notes, false
}

// expiryLine says when a credential expires: in days while that's close, else
// its date.
func expiryLine(at, now time.Time) string {
	days := int(math.Ceil(at.Sub(now).Hours() / 24))
	switch {
	case days <= 1:
		return "Expires within a day"
	case days <= 60:
		return fmt.Sprintf("Expires in %d days", days)
	}
	return "Expires " + at.Local().Format("2 Jan 2006")
}

// whereChips are where it may be used, shortly; whereLabel says it whole.
func whereChips(cr vault.Credential, hosts []Host) []string {
	var out []string
	for _, id := range cr.Hosts {
		for _, h := range hosts {
			if h.ID == id {
				out = append(out, h.Name)
			}
		}
	}
	if len(out) == 0 {
		out = append(out, map[bool]string{false: "asks on each host", true: "no host"}[cr.HostsOnly])
	}
	for _, p := range cr.Projects {
		out = append(out, "only "+p)
	}
	return out
}

// mainLabel names the value a hint is made of.
func mainLabel(cr vault.Credential, p vault.Platform) string {
	for _, f := range p.Fields {
		if f.Secret && cr.Fields[f.Key] != "" {
			return f.Label
		}
	}
	return p.Name
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
	return theme.Idle
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

// Logo is a platform's mark, in the text's color so that every one reads on any
// surface; without one, its initials on a tile of a color of its own.
func Logo(c *ui.Context, p vault.Platform, size float32) {
	if svg, _, ok := logoOf(p.ID); ok {
		ui.Row(c).Size(size, size).Shrink(0).Center().Label(p.Name).Children(func() {
			ui.Icon(c, svg).FontSize(size * 5 / 6).TextColor(theme.TextStrong)
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
	ui.Column(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		ui.Text(c, "Latest uses").FontSize(theme.TextUI).FontWeight(500)
		list := o.Audit
		if len(list) > auditShown {
			list = list[len(list)-auditShown:]
		}
		ui.Column(c).FillWidth().Children(func() {
			for i := len(list) - 1; i >= 0; i-- {
				auditRow(c.Key(i), list[i], i == len(list)-1)
			}
		})
	})
}

// auditShown is how many uses show.
const auditShown = 12

// auditRow is a use: when, what, and whether it was let.
func auditRow(c *ui.Context, e proto.VaultAuditEntry, first bool) {
	row := ui.Row(c).FillWidth().Shrink(0).MinHeight(32).Gap(theme.S2)
	if first {
		row.BorderWidth(0.5, 0, 0.5, 0)
	} else {
		row.BorderWidth(0, 0, 0.5, 0)
	}
	row.BorderColor(theme.Border).Children(func() {
		ui.Text(c, auditWhen(e.Time)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Width(44).Shrink(0).SingleLine()
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			widget.Running(c, auditText(e), widget.RunStyle{Size: theme.TextLabel})
		})
		if e.Allowed {
			widget.StatusLine(c.Key("state"), theme.Done, false, "Allowed")
		} else {
			widget.StatusLine(c.Key("state"), theme.Error, false, "Refused")
		}
	})
}

// auditWhen is the hour of a use today, the day of an older one.
func auditWhen(t time.Time) string {
	if sameDay(t, time.Now()) {
		return t.Local().Format("15:04")
	}
	return t.Local().Format("2 Jan")
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
	s := name + " · " + what
	if e.Detail != "" {
		s += " · " + e.Detail
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

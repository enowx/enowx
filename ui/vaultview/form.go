package vaultview

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// form is the Add or Edit credential dialog: a platform picked first,
// then its values and where it may be used.
type form struct {
	open      bool
	edit      *vault.Credential // nil: a new one
	platform  *vault.Platform
	query     string
	name      string
	account   string
	values    map[string]string
	expires   string // YYYY-MM-DD
	hosts     map[string]bool
	hostsOnly bool
	auto      bool
	projects  string
	err       string
	device    *deviceLogin // signing in in the browser
	scanned   bool         // the host's tools were asked for their logins
	page      *tokenPage   // the platform's token page opened (quick.go)
}

// deviceLogin is a sign-in in the browser under way.
type deviceLogin struct {
	clientID string
	code     string // what to type
	url      string // where
	waiting  bool
	done     bool // the token came, and is filled in
	err      string
}

func (f *form) pick(p vault.Platform) {
	f.platform = &p
	if f.name == "" {
		f.name = p.ID
	}
}

// editForm is the form for a credential the vault has.
func editForm(cr vault.Credential) *form {
	f := &form{open: true, edit: &cr, name: cr.Name, account: cr.Account, values: maps.Clone(cr.Fields),
		hosts: map[string]bool{}, hostsOnly: cr.HostsOnly, auto: cr.Auto, projects: strings.Join(cr.Projects, ", ")}
	p := vault.Lookup(cr.Platform)
	f.platform = &p
	for _, h := range cr.Hosts {
		f.hosts[h] = true
	}
	if !cr.Expires.IsZero() {
		f.expires = cr.Expires.Local().Format(time.DateOnly)
	}
	return f
}

func (v *View) dialogs(c *ui.Context, o Options) {
	v.formDialog(c, o)
	v.importDialog(c, o)
	v.changeDialog(c, o)
}

func (v *View) formDialog(c *ui.Context, o Options) {
	f := v.form
	if f == nil || !f.open {
		v.form = nil
		return
	}
	if f.platform == nil {
		v.platformPicker(c, f)
		return
	}
	p := *f.platform
	if f.edit == nil {
		f.name = freeName(f.name, o.Credentials)
	}
	title := "Add " + p.Name
	if f.edit != nil {
		title = "Edit " + f.edit.Name
	}
	// A form of its own each time: its inputs start from its values, not
	// from the last form's.
	widget.FormSheet(c.Key(fmt.Sprintf("form-%p", f)), &f.open, 560, title, "Kept encrypted on this device; hosts get it when a session needs it, after you allow them.", func() {
		if f.edit == nil {
			v.quickWays(c, o, f, p)
			widget.Divider(c)
		}
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			ui.Column(c).Grow(1).Basis(0).Children(func() {
				widget.Field(c, "Name (used in commands)", func() { widget.Input(c, &f.name, p.ID).FillWidth().Font(theme.MonoFont) })
			})
			ui.Column(c).Grow(1).Basis(0).Children(func() {
				widget.Field(c, "Account (optional)", func() { widget.Input(c, &f.account, "you@example.com").FillWidth() })
			})
		})
		for _, fl := range p.Fields {
			label := fl.Label
			if fl.Optional {
				label += " (optional)"
			}
			if f.edit != nil && fl.Secret {
				label += " (empty keeps it)"
			}
			widget.Field(c, label, func() {
				val := f.values[fl.Key]
				if f.edit != nil && fl.Secret {
					val = f.values["\x00"+fl.Key]
				}
				if fl.Multiline {
					ui.TextArea(c, &val).FillWidth().Height(96).Padding(8, 11).Radius(theme.RadiusControl).Background(theme.Sunken).
						Border(1, theme.BorderStrong).Font(theme.MonoFont).FontSize(theme.TextLabel).Label("Paste the " + strings.ToLower(fl.Label))
				} else {
					in := widget.Input(c, &val, "Paste the "+strings.ToLower(fl.Label)).FillWidth().Font(theme.MonoFont)
					if fl.Secret {
						in.Password()
					}
				}
				if f.edit != nil && fl.Secret {
					f.values["\x00"+fl.Key] = val
				} else {
					f.values[fl.Key] = val
				}
			})
		}
		if env := envOf(p); env != "" {
			ui.Text(c, "Sets "+env+" in a session.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		if p.Link != "" && (f.edit != nil || o.Actions.OpenURL == nil) {
			ui.Text(c, "Make one at "+p.Link).FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		widget.Field(c, "Expires (YYYY-MM-DD, optional)", func() { widget.Input(c, &f.expires, "2027-01-31").FillWidth().Font(theme.MonoFont) })
		widget.Field(c, "May be used without asking on", func() {
			ui.Row(c).FillWidth().Wrap().Gap(12).Children(func() {
				for _, h := range o.Hosts {
					on := f.hosts[h.ID]
					if widget.Checkbox(c.Key(h.ID), &on, h.Name) {
						f.hosts[h.ID] = on
					}
				}
			})
		})
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			widget.Switch(c, &f.hostsOnly, "Only these hosts")
			ui.Text(c, "Only these hosts: others are refused without asking.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		})
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			widget.Switch(c, &f.auto, "In every session")
			ui.Text(c, "In every session: its variables are in each new session of those hosts.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		})
		widget.Field(c, "Only for projects (folders or names, comma-separated; empty for any)", func() {
			widget.Input(c, &f.projects, "work-api, /srv/billing").FillWidth().Font(theme.MonoFont)
		})
		if f.err != "" {
			ui.Text(c, f.err).FontSize(theme.TextLabel).TextColor(theme.Error)
		}
	}, func() {
		label := "Add"
		if f.edit != nil {
			label = "Save"
		}
		if widget.PrimaryButton(c, icon.Check, label, "").Clicked() {
			cr, err := f.credential()
			if err == "" {
				err = errText(o.Actions.Save(cr))
			}
			if f.err = err; err == "" {
				v.form = nil
			}
		}
	})
}

// credential is the form's credential, or why it can't be one.
func (f *form) credential() (vault.Credential, string) {
	var cr vault.Credential
	if f.edit != nil {
		cr = *f.edit
	}
	cr.Platform, cr.Name, cr.Account = f.platform.ID, strings.TrimSpace(f.name), strings.TrimSpace(f.account)
	fields := map[string]string{}
	for _, fl := range f.platform.Fields {
		v := strings.TrimSpace(f.values[fl.Key])
		if f.edit != nil && fl.Secret {
			v = strings.TrimSpace(f.values["\x00"+fl.Key])
			if v == "" {
				v = f.edit.Fields[fl.Key]
			}
		}
		if v == "" && !fl.Optional {
			return cr, fl.Label + " is empty."
		}
		if v != "" {
			fields[fl.Key] = v
		}
	}
	if f.edit != nil && !maps.Equal(fields, f.edit.Fields) {
		cr.Rotate(fields)
	} else {
		cr.Fields = fields
	}
	cr.Type = f.platform.Type
	cr.Expires = time.Time{}
	if e := strings.TrimSpace(f.expires); e != "" {
		t, err := time.ParseInLocation(time.DateOnly, e, time.Local)
		if err != nil {
			return cr, "The expiry date reads as YYYY-MM-DD."
		}
		cr.Expires = t.Add(24*time.Hour - time.Second).UTC()
	}
	cr.Hosts = nil
	for id, on := range f.hosts {
		if on {
			cr.Hosts = append(cr.Hosts, id)
		}
	}
	slices.Sort(cr.Hosts)
	cr.HostsOnly, cr.Auto = f.hostsOnly, f.auto
	cr.Projects = nil
	for _, p := range strings.Split(f.projects, ",") {
		if p = strings.TrimSpace(p); p != "" {
			cr.Projects = append(cr.Projects, p)
		}
	}
	return cr, ""
}

// envOf lists the variables a platform's credentials set.
func envOf(p vault.Platform) string {
	names := slices.Collect(maps.Keys(p.Env))
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// platformPicker is the form's first step: the platforms by group, and a
// search.
func (v *View) platformPicker(c *ui.Context, f *form) {
	widget.FormSheet(c, &f.open, 640, "Add a credential", "Pick its platform; Generic covers any other.", func() {
		widget.SearchField(c, &f.query, "GitHub, AWS, Cloudflare, OpenAI…", icon.Search)
		found := vault.Search(f.query)
		for _, g := range vault.Groups {
			var in []vault.Platform
			for _, p := range found {
				if p.Group == g {
					in = append(in, p)
				}
			}
			if len(in) == 0 {
				continue
			}
			ui.Column(c.Key(g)).FillWidth().Gap(8).Children(func() {
				widget.SectionLabel(c, strings.ToUpper(g), theme.Muted)
				ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
					for _, p := range in {
						b := ui.ButtonBase(c.Key(p.ID)).Width(184).Height(44).PaddingX(10).Gap(10).Radius(theme.RadiusControl).
							Border(1, theme.Border).Background(theme.Raised).Cursor(ui.CursorPointer).Label(p.Name).Justify(ui.Start)
						if b.Hovered() {
							b.Border(1, theme.AccentLine).Background(theme.Hover)
						}
						b.Children(func() {
							Logo(c, p, 26)
							ui.Text(c, p.Name).FontSize(theme.TextUI).TextColor(theme.Text).SingleLine().Shrink(1)
						})
						if b.Clicked() {
							f.pick(p)
						}
					}
				})
			})
		}
	}, func() {})
}

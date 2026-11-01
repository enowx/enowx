package vaultview

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// importer is the Import from host dialog: the logins the host's tools
// keep, to pick.
type importer struct {
	open  bool
	picks map[int]bool
	err   string
	seen  int // how many were found when the picks were last filled
}

func (v *View) importDialog(c *ui.Context, o Options) {
	f := v.importer
	if f == nil || !f.open {
		v.importer = nil
		return
	}
	if f.seen != len(o.Found) {
		// Everything found is picked, but what the vault has already.
		f.seen = len(o.Found)
		for i, found := range o.Found {
			f.picks[i] = !has(o.Credentials, found)
		}
	}
	widget.FormSheet(c, &f.open, 480, "Import from "+o.Current.Name,
		"The logins "+o.Current.Name+"'s tools keep (gh, aws, wrangler, docker, npm, git, SSH keys…). Picked ones are copied into the vault; the tools keep theirs.", func() {
			switch {
			case o.Scanning:
				ui.Row(c).Gap(theme.S2).Children(func() {
					widget.Spinner(c, 14)
					ui.Text(c, "Looking…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
				})
				return
			case len(o.Found) == 0:
				ui.Text(c, "Nothing found: log in with a tool there first (gh auth login, aws configure…), or add it by hand.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
				return
			}
			widget.CardLines(c, func() {
				for i, found := range o.Found {
					p := vault.Lookup(found.Platform)
					title := p.Name
					if found.Account != "" {
						title += " · " + found.Account
					}
					about := "from " + found.Source
					if found.Note != "" {
						about += " · " + found.Note
					}
					if has(o.Credentials, found) {
						about = "in the vault already · " + about
					}
					widget.CardLine(c.Key(i), i == 0, title, about, func() {
						on := f.picks[i]
						if widget.Switch(c, &on, "Import "+p.Name) {
							f.picks[i] = on
						}
					})
				}
			})
			if f.err != "" {
				widget.CardNote(c, f.err, 0).TextColor(theme.Error)
			}
		}, func() {
			n := 0
			for i := range o.Found {
				if f.picks[i] {
					n++
				}
			}
			b := widget.PrimaryButton(c, nil, fmt.Sprintf("Import %d", n), "")
			if n == 0 || o.Scanning {
				b.Disabled(true).Opacity(0.45)
			} else if b.Clicked() {
				var picked []proto.VaultFound
				for i, found := range o.Found {
					if f.picks[i] {
						picked = append(picked, found)
					}
				}
				if f.err = errText(o.Actions.Import(picked)); f.err == "" {
					v.importer = nil
				}
			}
		})
}

// has reports whether the vault holds found already: its platform with the
// same main value.
func has(list []vault.Credential, found proto.VaultFound) bool {
	for _, cr := range list {
		if cr.Platform != found.Platform {
			continue
		}
		for k, v := range found.Fields {
			if vault.Lookup(found.Platform).Fields[0].Key == k && cr.Fields[k] == v {
				return true
			}
		}
	}
	return false
}

package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/vaultview"
	"github.com/enowx/enowx/ui/widget"
)

// Notion imported again on its own: the app keeps when, and which of the
// vault's Notion credentials to use; the token is read from the vault at
// each import and sent to the host for it alone, never kept elsewhere.

// notionSync is an import from Notion's API made again every Hours.
type notionSync struct {
	ID       string    `json:"id"`
	Host     string    `json:"host"`     // the daemon's ID
	HostName string    `json:"hostName"` // as the list shows it
	Cred     string    `json:"cred"`     // the vault credential's ID
	Group    string    `json:"group,omitempty"`
	Project  string    `json:"project,omitempty"`
	Rows     bool      `json:"rows,omitempty"`
	Hours    int       `json:"hours"`
	Last     time.Time `json:"last,omitempty"`
	Err      string    `json:"err,omitempty"`
}

// notionEvery are the choices of how often, in hours (0: never again).
var notionEvery = []int{0, 1, 6, 24}

var notionEveryLabels = []string{"Once", "Hourly", "Every 6 hours", "Daily"}

// notionCreds are the vault's Notion credentials in use, nil while it is
// locked.
func (a *App) notionCreds() []vault.Credential {
	v := a.vaultOf()
	if v.data == nil {
		return nil
	}
	var out []vault.Credential
	for _, cr := range v.data.Credentials {
		if cr.Platform == "notion" && cr.Fields["token"] != "" && !cr.Revoked {
			out = append(out, cr)
		}
	}
	return out
}

// notionToken is the token the import dialog sends: the picked
// credential's, else the one typed.
func (a *App) notionToken(f *noteImportForm) string {
	if f.cred != "" {
		if v := a.vaultOf(); v.data != nil {
			if cr, ok := v.data.Find(f.cred); ok {
				return cr.Fields["token"]
			}
		}
		return ""
	}
	return strings.TrimSpace(f.token)
}

// notionTokenField is the dialog's token: one of the vault's, or typed
// and, if asked, kept in the vault.
func (a *App) notionTokenField(c *ui.Context, f *noteImportForm) {
	creds := a.notionCreds()
	widget.Field(c, "Integration token", func() {
		if len(creds) > 0 {
			ui.Row(c).FillWidth().Wrap().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "From the vault:").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
				for _, cr := range creds {
					b := ui.ButtonBase(c.Key(cr.ID)).Height(theme.ControlHeight).PaddingX(theme.S3).Gap(theme.S2).Radius(theme.RadiusControl).Background(theme.Sunken).Cursor(ui.CursorPointer).Label("Use " + cr.Name)
					if f.cred == cr.ID {
						b.Background(theme.Selected)
					} else if b.Hovered() {
						b.Background(theme.Hover)
					}
					b.Children(func() {
						vaultview.Logo(c, vault.Lookup(cr.Platform), 16)
						ui.Text(c, cr.Name).FontSize(theme.TextUI).SingleLine()
					})
					if widget.Pressable(b).Clicked() {
						f.cred, f.token, f.keep = cr.ID, "", false
					}
				}
			})
		}
		if f.cred == "" {
			widget.Input(c, &f.token, "ntn_… or secret_…").FillWidth().Password()
		} else if widget.Button(c, widget.Secondary, nil, "Type another token", "").Clicked() {
			f.cred = ""
		}
		help := "Make an integration at notion.so/my-integrations and share your pages with it. The token goes to Notion from " + a.host.Name + " for this import only."
		if f.cred != "" {
			help = "The vault's token goes to Notion from " + a.host.Name + " for each import, and isn't kept there."
		}
		ui.Text(c, help).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	})
	if f.cred == "" && a.vaultOf().data != nil {
		switchRow(c, &f.keep, "Keep the token in the vault", "As a Notion credential, for importing again without typing it.")
	}
	if f.cred != "" || f.keep {
		widget.Field(c, "Import again", func() {
			if i := widget.Segmented(c, notionEveryLabels, f.every, nil); i >= 0 {
				f.every = i
			}
		})
	} else if a.vaultOf().file != nil && a.vaultOf().data == nil {
		ui.Text(c, "Unlock the vault (Credentials) to use a token kept there, or to import again on a schedule.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	}
	a.notionSyncList(c)
}

// keepNotionToken adds the typed token to the vault as a Notion
// credential, then uses it.
func (a *App) keepNotionToken(f *noteImportForm) error {
	v := a.vaultOf()
	if v.data == nil {
		return fmt.Errorf("the vault is locked")
	}
	name := "notion"
	for i := 2; ; i++ {
		if _, taken := v.data.Find(name); !taken {
			break
		}
		name = fmt.Sprintf("notion-%d", i)
	}
	cr, err := v.data.Add(vault.Credential{Platform: "notion", Type: vault.Lookup("notion").Type, Name: name, Fields: map[string]string{"token": strings.TrimSpace(f.token)}})
	if err != nil {
		return err
	}
	if err := a.vaultSave(); err != nil {
		return err
	}
	f.cred, f.token, f.keep = cr.ID, "", false
	return nil
}

// scheduleNotion keeps the dialog's import to be made again, if asked.
func (a *App) scheduleNotion(f *noteImportForm) {
	hours := notionEvery[f.every]
	if hours == 0 || f.cred == "" {
		return
	}
	s := notionSync{ID: newSyncID(), Host: a.daemonInfo.ID, HostName: a.host.Name, Cred: f.cred,
		Group: strings.TrimSpace(f.group), Project: f.project, Rows: f.rows, Hours: hours, Last: time.Now()}
	list := slices.DeleteFunc(a.state.NotionSyncs, func(o notionSync) bool {
		return o.Host == s.Host && o.Cred == s.Cred && o.Group == s.Group && o.Project == s.Project
	})
	a.state.NotionSyncs = append(list, s)
	a.saveState()
}

func newSyncID() string { return fmt.Sprintf("n%x", time.Now().UnixNano()) }

// notionSyncList lists the imports made again on their own, each to stop.
func (a *App) notionSyncList(c *ui.Context) {
	if len(a.state.NotionSyncs) == 0 {
		return
	}
	widget.Field(c, "Imported again on their own", func() {
		for _, s := range slices.Clone(a.state.NotionSyncs) {
			ui.Row(c.Key(s.ID)).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icon.Sync).FontSize(12).TextColor(theme.TextSoft)
				line := notionEveryLabels[slices.Index(notionEvery, s.Hours)] + " into " + s.HostName
				if s.Group != "" {
					line += " · " + s.Group
				}
				if !s.Last.IsZero() {
					line += " · last " + textfmt.Age(s.Last, time.Now())
				}
				col := theme.TextSoft
				if s.Err != "" {
					line, col = line+" · failed: "+s.Err, theme.Warning
				}
				ui.Row(c).Grow(1).MinWidth(0).Children(func() {
					widget.Running(c, line, widget.RunStyle{Size: theme.TextLabel, Color: col})
				})
				if widget.IconButton(c, icon.Close, "Stop importing again", false).Clicked() {
					a.state.NotionSyncs = slices.DeleteFunc(a.state.NotionSyncs, func(o notionSync) bool { return o.ID == s.ID })
					a.saveState()
				}
			})
		}
	})
}

// notionSyncTick makes again the imports due, on hosts reached, when the
// vault is open or its key remembered.
func (a *App) notionSyncTick() {
	now := time.Now()
	var due []notionSync
	for _, s := range a.state.NotionSyncs {
		if now.Sub(s.Last) >= time.Duration(s.Hours)*time.Hour && !a.notionRunning[s.ID] {
			due = append(due, s)
		}
	}
	if len(due) == 0 {
		return
	}
	a.vaultFromKeychain(func() {
		v := a.vaultOf()
		if v.data == nil {
			return // locked: the next tick tries again
		}
		for _, s := range due {
			h, ok := a.reach(s.Host)
			if !ok {
				continue
			}
			cr, found := v.data.Find(s.Cred)
			if !found || cr.Revoked {
				a.notionSyncDone(s.ID, "the vault no longer has its Notion credential")
				continue
			}
			if a.notionRunning == nil {
				a.notionRunning = map[string]bool{}
			}
			a.notionRunning[s.ID] = true
			p := proto.NoteImportParams{From: proto.ImportNotionAPI, Token: cr.Fields["token"], Group: s.Group,
				Project: s.Project, Mode: proto.ImportOverwrite, RowsAsNotes: s.Rows}
			go func() {
				_, err := h.cl.ImportNotes(p)
				a.onMain(func() {
					delete(a.notionRunning, s.ID)
					msg := ""
					if err != nil {
						msg = err.Error()
					}
					a.notionSyncDone(s.ID, msg)
				})
			}()
		}
	})
}

// notionSyncDone records an import made again, and how it went.
func (a *App) notionSyncDone(id, failed string) {
	for i := range a.state.NotionSyncs {
		if a.state.NotionSyncs[i].ID == id {
			a.state.NotionSyncs[i].Last, a.state.NotionSyncs[i].Err = time.Now(), failed
		}
	}
	a.saveState()
}

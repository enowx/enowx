// Package vaultview is the Credentials page: the user's credential vault,
// made, unlocked, its logins listed by platform, added, edited, checked,
// imported from a host's tools, and the questions it asks the user when a
// host wants one. It gets the vault and what to do from the app through
// Options; the app keeps the key.
package vaultview

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// The vault's states (Options.State).
const (
	StateNone   = "none"   // no vault yet: make one
	StateLocked = "locked" // its key isn't in memory
	StateOpen   = "open"
)

// Host is a host credentials may be used on.
type Host struct{ ID, Name string }

// Options are the vault as the page shows it, and what it does.
type Options struct {
	State       string
	Credentials []vault.Credential
	Hosts       []Host
	Current     Host // the host connected to
	Audit       []proto.VaultAuditEntry
	Testing     map[string]bool   // credential IDs being checked
	Remember    bool              // the key is kept in the keychain
	ClientIDs   map[string]string // platform → the user's OAuth app, for signing in in the browser
	Found       []proto.VaultFound
	Scanning    bool
	Actions     Actions
}

// Actions are what the page does; the app does them.
type Actions struct {
	Create     func(password string, remember bool) error
	Unlock     func(password string, remember bool) error
	Lock       func()
	Save       func(c vault.Credential) error // an empty ID adds it
	Remove     func(id string)
	Test       func(id string)
	SetDefault func(id string)
	Revoke     func(id string, revoked bool)
	Scan       func()
	Import     func([]proto.VaultFound) error
	Login      func(platform string) // runs its own login in a session; nil for none
	// OpenURL opens a page in the browser: where a platform's token is
	// made.
	OpenURL func(url string)
	// DeviceLogin signs in in the browser with the user's OAuth app
	// clientID: code tells what to type where (the page opened), done the
	// token. Nil for none.
	DeviceLogin    func(platform, clientID string, code func(userCode, url string), done func(token string, err error))
	ChangePassword func(old, password string) error
}

// View is the page's state, owned by the main thread.
type View struct {
	search   string
	password string
	confirm  string
	remember bool
	err      string
	form     *form
	importer *importer
	change   *change
	details  *detailsSheet
	menu     widget.ContextMenu
	folded   map[string]bool
}

// OpenAdd opens the form adding a credential of platform ("" to pick).
func (v *View) OpenAdd(platform string) {
	f := &form{open: true, values: map[string]string{}, hosts: map[string]bool{}}
	if platform != "" {
		f.pick(vault.Lookup(platform))
	}
	v.form = f
}

// OpenImport opens the import dialog; Options.Found fills it.
func (v *View) OpenImport() { v.importer = &importer{open: true, picks: map[int]bool{}} }

// OpenChange opens the Change master password dialog.
func (v *View) OpenChange() { v.change = &change{open: true} }

// OpenDetails opens the dialog of credential id.
func (v *View) OpenDetails(id string) { v.details = &detailsSheet{open: true, id: id} }

// Body is the page under its header.
func (v *View) Body(c *ui.Context, o Options) {
	switch o.State {
	case StateNone:
		v.centered(c, func() { v.create(c, o) })
	case StateLocked:
		v.centered(c, func() { v.unlock(c, o) })
	default:
		v.open(c, o)
	}
	v.dialogs(c, o)
}

// centered holds a card in the middle of the frame, scrolling when the frame
// is shorter than it.
func (v *View) centered(c *ui.Context, card func()) {
	body := ui.Scroll(c.Key("vault-body")).Grow(1).FillWidth().MinHeight(0)
	room := body.Bounds().H
	body.Children(func() {
		ui.Column(c).FillWidth().MinHeight(room).Padding(theme.S4, theme.S5).AlignItems(ui.Center).Justify(ui.Center).Children(card)
	})
}

// Toolbar is the header's actions: the vault's own in a menu, and adding.
func (v *View) Toolbar(c *ui.Context, o Options) {
	if o.State != StateOpen {
		return
	}
	b := widget.IconButton(c.Key("vault-more"), icon.More, "More for the vault", false)
	v.menu.OpenBelow(b, "vault", theme.ControlHeight)
	v.menu.Show(c, b, "vault", func(done func()) {
		if o.Actions.Scan != nil && widget.MenuItemTip(c, icon.ArrowDown, "Import from "+o.Current.Name+"…", "Find the logins "+o.Current.Name+"'s tools keep (gh, aws, wrangler, docker, npm, SSH keys…)", false) {
			v.OpenImport()
			o.Actions.Scan()
			done()
		}
		if o.Actions.ChangePassword != nil && widget.MenuItem(c, icon.Key, "Change master password…", false) {
			v.OpenChange()
			done()
		}
		if o.Actions.Lock != nil && widget.MenuItem(c, icon.Lock, "Lock the vault", false) {
			o.Actions.Lock()
			done()
		}
	})
	if widget.PrimaryButton(c, icon.Plus, "Add credential", "").Clicked() {
		v.OpenAdd("")
	}
}

// figures are the vault's numbers over its list.
func figures(c *ui.Context, o Options, now time.Time) {
	var working, soon, refused, today int
	for _, cr := range o.Credentials {
		switch st := cr.State(now); {
		case st == proto.CredInvalid:
			refused++
		case st == proto.CredValid:
			working++
		}
		if cr.ExpiresSoon(now, expiringWithin) {
			soon++
		}
	}
	for _, e := range o.Audit {
		if e.Allowed && sameDay(e.Time, now) {
			today++
		}
	}
	tones := map[int]ui.Color{}
	if soon > 0 {
		tones[2] = theme.Warning
	}
	if refused > 0 {
		tones[3] = theme.Error
	}
	widget.FigsBandToned(c, []widget.Fig{
		{Label: "Credentials", Value: strconv.Itoa(len(o.Credentials))},
		{Label: "Work now", Value: strconv.Itoa(working)},
		{Label: "Rotate soon", Value: strconv.Itoa(soon)},
		{Label: "Refused", Value: strconv.Itoa(refused)},
		{Label: "Used today", Value: strconv.Itoa(today)},
	}, tones)
}

func sameDay(a, b time.Time) bool {
	a, b = a.Local(), b.Local()
	return a.YearDay() == b.YearDay() && a.Year() == b.Year()
}

// expiringWithin is when a credential's expiry starts to be told.
const expiringWithin = 7 * 24 * time.Hour

// flatUpTo is how many credentials show as one grid; more go in folding groups
// by platform kind, to keep them findable.
const flatUpTo = 9

func (v *View) open(c *ui.Context, o Options) {
	now := time.Now()
	if len(o.Credentials) == 0 {
		v.centered(c, func() {
			widget.EmptyState(c, icon.Shield, "No credentials yet",
				"Add a token, a key or a login once; sessions on any host get it when they need it, after you allow that host.", func() {
					ui.Row(c).Gap(theme.S2).Children(func() {
						if widget.PrimaryButton(c, icon.Plus, "Add credential", "").Clicked() {
							v.OpenAdd("")
						}
						if o.Actions.Scan != nil && widget.Button(c, widget.Secondary, icon.ArrowDown, "Import from "+o.Current.Name, "").Clicked() {
							v.OpenImport()
							o.Actions.Scan()
						}
					})
				})
		})
		return
	}
	figures(c, o, now)
	ui.Scroll(c.Key("vault-body")).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(theme.S4, theme.S5, theme.S5, theme.S5).Gap(theme.S4).Children(func() {
			v.rotateBanner(c, o, now)
			ui.Row(c).Width(320).Shrink(1).MinWidth(0).Children(func() {
				widget.SearchField(c, &v.search, "Search credentials", icon.Search)
			})
			v.list(c, o)
			v.audit(c, o)
		})
	})
}

// rotateBanner names the credentials expired or about to.
func (v *View) rotateBanner(c *ui.Context, o Options, now time.Time) {
	var names []string
	for _, cr := range o.Credentials {
		if cr.ExpiresSoon(now, expiringWithin) || cr.State(now) == proto.CredExpired {
			names = append(names, title(cr, vault.Lookup(cr.Platform)))
		}
	}
	if len(names) > 0 {
		widget.Banner(c, theme.Warning, "Rotate soon: "+strings.Join(names, ", ")+". Expired, or expiring within a week: make a new token and edit it here.", nil)
	}
}

// list is the credentials the search leaves, as one grid or, past flatUpTo, in
// groups that fold.
func (v *View) list(c *ui.Context, o Options) {
	q := strings.ToLower(strings.TrimSpace(v.search))
	var shown []vault.Credential
	for _, cr := range o.Credentials {
		p := vault.Lookup(cr.Platform)
		if q == "" || strings.Contains(strings.ToLower(cr.Name+" "+p.Name+" "+cr.Account), q) {
			shown = append(shown, cr)
		}
	}
	slices.SortFunc(shown, func(a, b vault.Credential) int { return strings.Compare(a.Name, b.Name) })
	if len(o.Credentials) <= flatUpTo {
		v.grid(c, o, shown, q == "")
		return
	}
	if v.folded == nil {
		v.folded = map[string]bool{}
	}
	for _, g := range vault.Groups {
		var in []vault.Credential
		for _, cr := range shown {
			if vault.Lookup(cr.Platform).Group == g {
				in = append(in, cr)
			}
		}
		if len(in) == 0 {
			continue
		}
		open := !v.folded[g]
		widget.Group(c.Key(g), g, len(in), &open, func() { v.grid(c, o, in, false) })
		v.folded[g] = !open
	}
}

// grid is cards of credentials in three columns, and the way to add another.
func (v *View) grid(c *ui.Context, o Options, list []vault.Credential, addOne bool) {
	ui.Grid(c).FillWidth().Columns(3).Gap(theme.S3).Children(func() {
		for _, cr := range list {
			v.row(c.Key(cr.ID), o, cr)
		}
		if addOne {
			widget.Card(c.Key("add-one")).Children(func() {
				widget.CardTitle(c, nil, "Add one", nil)
				widget.CardNote(c, "Add a token, a key or a login once; sessions on any host get it when they need it, after you allow that host.", 0)
				widget.CardFoot(c, func() {
					if widget.Button(c, widget.Secondary, icon.Plus, "Add credential", "").Clicked() {
						v.OpenAdd("")
					}
				})
			})
		}
	})
}

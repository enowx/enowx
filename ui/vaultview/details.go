package vaultview

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// detailsSheet is the dialog of one credential: what the card says shortly, and
// where it was used. Values stay masked here.
type detailsSheet struct {
	open bool
	id   string
}

func (v *View) detailsDialog(c *ui.Context, o Options) {
	d := v.details
	if d == nil || !d.open {
		v.details = nil
		return
	}
	var cr vault.Credential
	found := false
	for _, x := range o.Credentials {
		if x.ID == d.id {
			cr, found = x, true
		}
	}
	if !found {
		v.details = nil
		return
	}
	p := vault.Lookup(cr.Platform)
	widget.DetailSheet(c, &d.open, 480, title(cr, p), valueLine(cr, p), func() {
		widget.KV(c, "Name", cr.Name, false)
		if val := mainValue(cr); val != "" {
			widget.KV(c, "Value", vault.Mask(val), true)
		}
		widget.KV(c, "Status", statusLabel(cr.State(time.Now())), false)
		if cr.Notes != "" {
			widget.KV(c, "Note", cr.Notes, false)
		}
		widget.KV(c, "Hosts", whereLabel(cr, o.Hosts), false)
		ui.Text(c, "Used on "+o.Current.Name).FontSize(theme.TextUI).FontWeight(500)
		widget.CardLines(c, func() {
			n := 0
			for i := len(o.Audit) - 1; i >= 0 && n < auditShown; i-- {
				if e := o.Audit[i]; e.Credential == cr.Name {
					detailUse(c.Key(i), e, n == 0)
					n++
				}
			}
			if n == 0 {
				widget.CardLine(c, true, "Nothing yet.", "", nil)
			}
		})
	}, func() {
		if o.Testing[cr.ID] {
			widget.Spinner(c, 14)
		} else if widget.GhostButton(c, "Test").Clicked() && o.Actions.Test != nil {
			o.Actions.Test(cr.ID)
		}
	})
}

// detailUse is one use of a credential.
func detailUse(c *ui.Context, e proto.VaultAuditEntry, first bool) {
	widget.CardLine(c, first, auditText(e), "", func() {
		col := theme.TextSoft
		label := auditWhen(e.Time)
		if !e.Allowed {
			col, label = theme.Error, "Refused · "+label
		}
		ui.Text(c, label).FontSize(theme.TextLabel).TextColor(col).SingleLine()
	})
}

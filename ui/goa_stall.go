package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// goaStallForm is how long a task may not move, on the host shown, being edited.
type goaStallForm struct {
	open               bool
	blank, idle, stuck string
	loaded, saving     bool
	err                string
}

// openGOAStall reads the host's limits into their dialog.
func (a *App) openGOAStall() {
	cl := a.client
	if cl == nil {
		return
	}
	f := &goaStallForm{open: true}
	a.goa.stall = f
	go func() {
		var l proto.GOAStall
		err := cl.Call(proto.MethodGOAStall, nil, &l)
		a.onMain(func() {
			if err != nil {
				// Not read: saving would put back what wasn't asked for.
				f.err = client.Explain(err)
				return
			}
			f.loaded = true
			f.blank, f.idle, f.stuck = goa.StallText(l.Blank), goa.StallText(l.Idle), goa.StallText(l.Stuck)
		})
	}()
}

func (a *App) goaStallDialog(c *ui.Context) {
	f := a.goa.stall
	if f == nil {
		return
	}
	if !f.open {
		a.goa.stall = nil
		return
	}
	widget.FormSheet(c, &f.open, 480, "When a task counts as stalled", "How long a task of GOD of Agents may not move on "+a.host.Name+" before it shows as stalled and GOD of Agents is told. A long build doesn't count while it uses the CPU.", func() {
		if f.loaded {
			widget.Field(c, "Blank: its screen still empty", func() {
				widget.Input(c.Key("stall-blank"), &f.blank, "Default "+goa.StallText(goa.StallBlank))
			})
			widget.Field(c, "Idle: hibernated or gone, or its agent not at work", func() {
				widget.Input(c.Key("stall-idle"), &f.idle, "Default "+goa.StallText(goa.StallIdle))
			})
			widget.Field(c, "Stuck: its agent says it works, but nothing shows or runs", func() {
				widget.Input(c.Key("stall-stuck"), &f.stuck, "Default "+goa.StallText(goa.StallStuck))
			})
		}
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
	}, func() {
		b := widget.PrimaryButton(c, nil, "Save", "")
		if f.saving || !f.loaded {
			b.Disabled(true)
		} else if b.Clicked() {
			a.saveGOAStall(f)
		}
	})
}

func (a *App) saveGOAStall(f *goaStallForm) {
	cl := a.client
	if cl == nil {
		return
	}
	var l proto.GOAStall
	for _, field := range []struct {
		text  string
		limit *int
	}{{f.blank, &l.Blank}, {f.idle, &l.Idle}, {f.stuck, &l.Stuck}} {
		seconds, err := goa.ParseStall(field.text)
		if err != nil {
			f.err = strings.TrimPrefix(err.Error(), "goa: ")
			return
		}
		*field.limit = seconds
	}
	f.saving, f.err = true, ""
	go func() {
		err := cl.Call(proto.MethodGOAStallSave, l, nil)
		a.onMain(func() {
			f.saving = false
			if err != nil {
				f.err = strings.TrimPrefix(client.Explain(err), "goa: ")
				return
			}
			f.open = false
			a.status = "Saved: tasks are held to these from now on"
		})
	}()
}

package ui

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Always allow: a session the user lets hibernate and close sessions without a
// card. The asking session's daemon holds the choice and the rule; the app
// offers it, shows it in Settings, and checks another host's sessions.

// alwaysAllowNote says in a card what its Always allow covers.
func alwaysAllowNote(asker string) string {
	return " Always allow is for “" + asker + "” only, hibernating and closing what loses nothing."
}

// allowAlways keeps the choice on the daemon of the asking session, then does
// what the user allowed this time, whatever came of keeping it.
func (a *App) allowAlways(cl *client.Client, session string, then func()) {
	go func() {
		err := cl.ControlAllow(session, false)
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
			}
			then()
		})
	}()
}

// whenNothingLost runs allowed if stopping what p asks, on another of the
// user's hosts, loses nothing there, as that host says; else ask, for the user
// to say. Anything it can't find out asks.
func (a *App) whenNothingLost(p proto.HostCallParams, allowed, ask func()) {
	var cl *client.Client
	for _, h := range a.reachable() {
		if h.info.ID == p.Host || h.host.ID == p.Host || strings.EqualFold(h.host.Name, p.Host) {
			cl = h.cl
		}
	}
	dry, err := stoppingPlanOf(p)
	if cl == nil || err != nil {
		ask()
		return
	}
	go func() {
		lost := true
		res, err := cl.HibernateSessions(dry)
		list, errList := cl.Sessions()
		if err == nil && errList == nil {
			lost = slices.ContainsFunc(res.Plans, func(pl proto.HibernatePlan) bool {
				i := slices.IndexFunc(list, func(s proto.SessionInfo) bool { return s.ID == pl.ID })
				switch {
				case i < 0 || list[i].Member != nil:
					return true
				case pl.Blocked == "":
					return pl.Busy()
				}
				// What can't hibernate stays as it is when hibernating; closed, it
				// goes, and its plan says nothing of what runs in it.
				return p.Method == proto.MethodSessionKill && !list[i].Exited
			})
		}
		a.onMain(func() {
			if lost {
				ask()
				return
			}
			allowed()
		})
	}()
}

// stoppingPlanOf is the dry run that says what p, a close or a hibernate, stops.
func stoppingPlanOf(p proto.HostCallParams) (proto.SessionHibernateParams, error) {
	switch p.Method {
	case proto.MethodSessionKill:
		var ref proto.SessionRef
		if err := json.Unmarshal(p.Params, &ref); err != nil {
			return proto.SessionHibernateParams{}, err
		}
		return proto.SessionHibernateParams{IDs: []string{ref.ID}, DryRun: true}, nil
	case proto.MethodSessionHibernate:
		var args proto.SessionHibernateParams
		if err := json.Unmarshal(p.Params, &args); err != nil {
			return proto.SessionHibernateParams{}, err
		}
		args.DryRun, args.From = true, ""
		return args, nil
	}
	return proto.SessionHibernateParams{}, errors.New("control: not a way of stopping a session")
}

// alwaysAllowedState is the Always allowed sheet of Settings → Agents. It
// belongs to the main thread.
type alwaysAllowedState struct {
	open bool
	list []proto.ControlAllow
	err  string
}

// openAlwaysAllowed shows the sheet, reading this host's choices.
func (a *App) openAlwaysAllowed() {
	a.alwaysAllowedSheet = alwaysAllowedState{open: true}
	a.loadAlwaysAllowed()
}

func (a *App) loadAlwaysAllowed() {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		list, err := cl.ControlAllowed()
		a.onMain(func() {
			st := &a.alwaysAllowedSheet
			st.list, st.err = list, ""
			if err != nil {
				st.err = client.Explain(err)
			}
		})
	}()
}

// alwaysAllowedView is the sheet: the sessions always allowed, each to revoke.
func (a *App) alwaysAllowedView(c *ui.Context) {
	st := &a.alwaysAllowedSheet
	if !st.open {
		return
	}
	widget.DetailSheet(c, &st.open, 480, "Always allowed", "These sessions hibernate and close sessions without asking, when nothing would be lost. Push, merge and the rest still ask.", func() {
		if st.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, st.err, "", nil)
		}
		if len(st.list) == 0 && st.err == "" {
			ui.Text(c, "No session is always allowed on "+a.host.Name+".").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		}
		widget.CardLines(c, func() {
			for i, al := range st.list {
				widget.CardLine(c.Key(al.Session), i == 0, al.Name, "since "+al.Since.Format("2 Jan 15:04"), func() {
					if widget.Button(c.Key("revoke"), widget.Secondary, nil, "Revoke", "").Clicked() {
						a.revokeAlways(al.Session)
					}
				})
			}
		})
	}, nil)
}

// revokeAlways brings a session's cards back.
func (a *App) revokeAlways(session string) {
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		err := cl.ControlAllow(session, true)
		a.onMain(func() {
			if err != nil {
				a.alwaysAllowedSheet.err = client.Explain(err)
				return
			}
			a.loadAlwaysAllowed()
		})
	}()
}

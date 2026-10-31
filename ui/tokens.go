package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	tokenuse "github.com/enowx/enowx/core/tokens"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/tokenview"
)

// Token usage in the app (docs/tokens.md): the Tokens tab of Resource
// usage, every host's together (those reached asked now, the others by
// their synced daily totals), and each session's use beside it.

// tokensStale is how long the Tokens tab's numbers stand before they are
// asked again while it shows.
const tokensStale = 30 * time.Second

// tokensState is the app's token usage. It belongs to the main thread.
type tokensState struct {
	// pageTab is the Resource usage page's tab.
	pageTab string
	view    tokenview.View
	data    tokenview.Data
	gen     int // the connection the data is of
	fetched time.Time
	busy    bool
	// sessions is the shown host's sessions' use so far, asked once a
	// connection, then told by tokens.changed.
	sessions    map[string]proto.TokenCounts
	sessionsGen int
}

// reachedHost is a host the app reaches now, to ask its numbers.
type reachedHost struct {
	name string
	cl   *client.Client
}

// reachedHosts are the shown host and the watched ones online.
func (a *App) reachedHosts() []reachedHost {
	var out []reachedHost
	if a.client != nil {
		out = append(out, reachedHost{a.host.Name, a.client})
	}
	for _, w := range a.watches {
		if w.cl != nil && w.status.state == hostOnline {
			out = append(out, reachedHost{w.host.Name, w.cl})
		}
	}
	return out
}

// fetchTokens asks every reached host for the Tokens tab's numbers, and
// the shown host for its settings, budgets, prices and coverage.
func (a *App) fetchTokens() {
	t := &a.tok
	cl := a.client
	if cl == nil || t.busy {
		return
	}
	t.busy, t.data.Loading, t.fetched, t.gen = true, true, time.Now(), a.gen
	q := t.view.Query(time.Now())
	reached := a.reachedHosts()
	gen := a.gen
	go func() {
		d := collectTokens(cl, reached, q)
		a.onMain(func() {
			t.busy = false
			if gen != a.gen {
				return
			}
			d.Backfilled = t.data.Backfilled
			t.data = d
		})
	}()
}

// collectTokens is the view's data: each reached host's summary of q,
// the others' from the synced totals, all merged.
func collectTokens(cl *client.Client, reached []reachedHost, q proto.TokensQuery) tokenview.Data {
	var d tokenview.Data
	ask := q
	if q.By == proto.TokensByHost {
		ask.By = ""
	}
	var parts []proto.TokensSummary
	seen := map[string]bool{}
	var errs []string
	for _, h := range reached {
		var s proto.TokensSummary
		if err := h.cl.Call(proto.MethodTokensSummary, ask, &s); err != nil {
			errs = append(errs, h.name+": "+client.Explain(err))
			continue
		}
		s.Host = h.name
		seen[s.HostID] = true
		parts = append(parts, s)
		d.Hosts = append(d.Hosts, tokenview.HostTotal{Name: h.name, Live: true, On: s.On, Total: s.Total})
	}
	var synced []proto.TokensHostDays
	if cl.Call(proto.MethodTokensHosts, nil, &synced) == nil {
		for _, h := range synced {
			if seen[h.Host] {
				continue
			}
			s := tokenuse.SummarizeRows(h.Rows, ask, time.Now())
			s.Host, s.HostID, s.On = h.Name, h.Host, true
			parts = append(parts, s)
			d.Hosts = append(d.Hosts, tokenview.HostTotal{Name: h.Name, Updated: h.Updated, On: true, Total: s.Total})
		}
	}
	d.Summary = tokenuse.Merge(q, parts)
	if err := cl.Call(proto.MethodTokensSettings, nil, &d.Settings); err != nil {
		errs = append(errs, client.Explain(err))
	}
	cl.Call(proto.MethodTokensBudgets, nil, &d.Budgets)
	cl.Call(proto.MethodTokensPrices, nil, &d.Prices)
	cl.Call(proto.MethodTokensCoverage, nil, &d.Coverage)
	d.Err = strings.Join(errs, "; ")
	return d
}

// tokensTab is Resource usage's Tokens tab.
func (a *App) tokensTab(c *ui.Context) {
	t := &a.tok
	if t.gen != a.gen || time.Since(t.fetched) > tokensStale {
		a.fetchTokens()
	}
	cl := a.client
	t.view.Draw(c, t.data, tokenview.Options{
		HostName: a.host.Name,
		Refresh:  func() { t.fetched = time.Time{} },
		Save: func(st proto.TokensSettings) {
			t.data.Settings = st
			go func() {
				err := cl.Call(proto.MethodTokensSetSettings, st, nil)
				a.onMain(func() {
					if err != nil {
						t.data.Err = client.Explain(err)
					}
					t.fetched = time.Time{}
				})
			}()
		},
		Backfill: func() {
			t.data.Backfilled = "Reading back…"
			go func() {
				var res proto.TokensBackfillResult
				err := cl.Call(proto.MethodTokensBackfill, proto.TokensBackfill{}, &res)
				a.onMain(func() {
					t.data.Backfilled = fmt.Sprintf("Read back %d files: %d new calls.", res.Files, res.Calls)
					if err != nil {
						t.data.Backfilled = client.Explain(err)
					}
					t.fetched = time.Time{}
				})
			}()
		},
		Confirm: a.confirm,
		Agent:   a.agentName,
	})
}

// openProjectTokens shows the Tokens tab of project path's use.
func (a *App) openProjectTokens(path string) {
	a.page, a.tok.pageTab = pageUsage, usageTabTokens
	a.tok.view.Tab, a.tok.view.Project, a.tok.fetched = tokenview.TabOverview, path, time.Time{}
}

// sessionTokens is session id's use so far on the shown host, asked once
// a connection.
func (a *App) sessionTokens(id string) (proto.TokenCounts, bool) {
	t := &a.tok
	if t.sessionsGen != a.gen && a.client != nil {
		t.sessionsGen = a.gen
		cl, gen := a.client, a.gen
		go func() {
			var list []proto.TokenSessionTotal
			if cl.Call(proto.MethodTokensSessions, nil, &list) != nil {
				return
			}
			a.onMain(func() {
				if gen != a.gen {
					return
				}
				t.sessions = map[string]proto.TokenCounts{}
				for _, s := range list {
					t.sessions[s.Session] = s.TokenCounts
				}
			})
		}()
	}
	u, ok := t.sessions[id]
	return u, ok && u.Calls > 0
}

// tokensEvent keeps sessions' use as their host tells it, and notifies
// of a budget at 80% or 100%.
func (a *App) tokensEvent(ev proto.Event) {
	switch ev.Name {
	case proto.EventTokensChanged:
		var ch proto.TokensChanged
		if json.Unmarshal(ev.Data, &ch) != nil {
			return
		}
		a.onMain(func() {
			if a.tok.sessions == nil {
				a.tok.sessions = map[string]proto.TokenCounts{}
			}
			for _, s := range ch.Sessions {
				a.tok.sessions[s.Session] = s.TokenCounts
			}
		})
	case proto.EventTokensBudget:
		var b proto.TokenBudgetState
		if json.Unmarshal(ev.Data, &b) != nil {
			return
		}
		a.onMain(func() { a.budgetAlert(b) })
	}
}

// budgetAlert tells of a budget at 80% or 100%: in the status bar, and
// as a notification.
func (a *App) budgetAlert(b proto.TokenBudgetState) {
	what := "everything"
	if b.Scope != proto.BudgetAll {
		what = b.Scope + " " + b.Target
	}
	body := fmt.Sprintf("The %s token budget for %s is at %d%%.", b.Period, what, int(b.Fraction*100))
	if b.Level >= 100 {
		switch b.AtLimit {
		case proto.BudgetAsk:
			body += " New sessions ask first."
		case proto.BudgetStop:
			body += " New agent sessions are stopped until it resets."
		}
	}
	a.status = body
	a.tok.fetched = time.Time{}
	if a.state.NotifyOff || a.notifySettings().NoBanner {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{ID: "tokens:" + b.ID, Title: "Token budget", Subtitle: a.host.Name, Body: body})
	n.OnClick(func() {
		a.onMain(func() { a.page, a.tok.pageTab, a.tok.view.Tab = pageUsage, usageTabTokens, tokenview.TabBudgets })
		a.showWindow()
	})
	go func() { _ = n.Show() }()
}

// budgetAsks reports whether err is a budget at its limit asking before
// a new session.
func budgetAsks(err error) bool {
	return err != nil && strings.Contains(err.Error(), proto.TokenBudgetAsks)
}

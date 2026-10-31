package ui

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/widget"
)

// An agent's configs on the connected host (proto/config.go): its other
// accounts and model APIs, picked per session; Settings → Agents shows
// them (agents.go), New session picks one.

// accountsState is Settings → Agents'. It belongs to the main thread.
type accountsState struct {
	host   string
	asked  bool
	loaded bool
	list   []proto.Account
	info   proto.AccountsInfo // states, what they have left, preferences
	log    []proto.AccountLogEntry
	tab    int    // 0 the configs, 1 the history
	agent  string // the agent whose page shows
	err    string
	only   map[string]*string // account ID → its projects, being set
	edit   *configForm        // a config being made or changed
	busy   bool
}

func (a *App) accountsOf() *accountsState {
	if a.accts.host != a.host.ID || !a.accts.asked {
		a.accts = accountsState{host: a.host.ID, asked: true, only: map[string]*string{}}
		a.loadAccounts()
	}
	return &a.accts
}

func (a *App) loadAccounts() {
	cl, host := a.client, a.host.ID
	if cl == nil {
		return
	}
	go func() {
		list, err := cl.Accounts()
		info, ierr := cl.AccountsInfo()
		log, _ := cl.AccountLog()
		a.onMain(func() {
			if a.accts.host != host {
				return
			}
			a.accts.loaded, a.accts.list, a.accts.err = true, list, ""
			if ierr == nil {
				a.accts.info, a.accts.log = info, log
			}
			if err != nil {
				a.accts.err = client.Explain(err)
			}
		})
	}()
}

// accountsOfAgent are agent's configs but its official one (agent may be
// a variant, as Claude Agent Teams, of the agent they are of).
func (st *accountsState) accountsOfAgent(id string) []proto.Account {
	var out []proto.Account
	for _, acc := range st.list {
		if acc.Agent == agent.Base(id) && acc.Member == "" {
			out = append(out, acc)
		}
	}
	return out
}

// signInAccount starts a session signing agent in as acc.
func (a *App) signInAccount(agentID string, acc proto.Account) {
	cl := a.client
	if cl == nil {
		return
	}
	p, _ := a.currentProject()
	go func() {
		info, err := cl.CreateSession(proto.SessionCreateParams{Project: p.Path, Agent: agentID, SignIn: true, Account: acc.ID,
			Name: "Sign in " + acc.Name, BranchMode: proto.BranchNone, Cols: 120, Rows: 36})
		a.onMain(func() {
			if err != nil {
				a.status = "Couldn't start signing in: " + client.Explain(err)
				return
			}
			a.upsert(info)
			a.selectSession(info)
		})
	}()
}

func (a *App) removeAccount(st *accountsState, acc proto.Account) {
	cl := a.client
	if cl == nil {
		return
	}
	st.busy = true
	go func() {
		err := cl.RemoveAccount(acc.ID)
		a.onMain(func() {
			st.busy = false
			if err != nil {
				st.err = client.Explain(err)
			}
			a.loadAccounts()
		})
	}()
}

// accountField picks the config the new session's agent runs with:
// Official, or one of its own (picked by the preferences until changed).
func (a *App) accountField(c *ui.Context) {
	f := &a.newSession
	list := a.accountsOf().accountsOfAgent(f.agent)
	if len(list) == 0 {
		f.account = ""
		return
	}
	if f.account != "" && !slices.ContainsFunc(list, func(acc proto.Account) bool { return acc.ID == f.account }) {
		f.accountPicked = false // picked for another agent
	}
	if !f.accountPicked {
		f.account = a.defaultAccountFor(f.agent, f.project.Path) // by the preferences, until picked
	}
	widget.Field(c, "Config", func() {
		labels := []string{"Official"}
		cur := 0
		for i, acc := range list {
			labels = append(labels, acc.Name)
			if acc.ID == f.account {
				cur = i + 1
			}
		}
		if len(labels) <= 4 {
			if i := widget.Segmented(c, labels, cur, nil); i >= 0 {
				f.account, f.accountPicked = "", true
				if i > 0 {
					f.account = list[i-1].ID
				}
			}
			return
		}
		options, names := []string{""}, map[string]string{"": "Official: the agent as installed"}
		for _, acc := range list {
			options = append(options, acc.ID)
			names[acc.ID] = acc.Name + " · " + configKindLabel(acc)
		}
		v := f.account
		hookSelect(c.Key("config"), "Config", &v, options, names)
		if v != f.account {
			f.account, f.accountPicked = v, true
		}
	})
}

// configKindLabel says what config c is, in a few words.
func configKindLabel(c proto.Account) string {
	switch {
	case c.Kind == proto.ConfigKindAPI:
		return "model API"
	case c.Cred != "":
		return "API key"
	}
	return "account"
}

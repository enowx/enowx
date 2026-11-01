package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/vaultview"
	"github.com/enowx/enowx/ui/widget"
)

// A host's request for a credential comes to the app holding the vault:
// allowed on that host already, it is answered at once; otherwise the user
// is asked (Allow once, Always on the host, Refuse), unlocking the vault
// first if it is locked. Adding or removing one asks the user too.

// vaultRequestAsk is a request waiting for the user.
type vaultRequestAsk struct {
	cl    *client.Client
	host  hosts.Host
	req   proto.VaultRequest
	creds []string // the credentials (IDs) it would lend, asked about
	lend  proto.VaultLend
	ask   vaultview.Ask
	done  func(answer int) // after the user answered; nil for lending creds
	until time.Time        // past it, the host waits no more
}

// vaultEvent hands a host's request (on connection cl to h) to the vault.
func (a *App) vaultEvent(cl *client.Client, h hosts.Host, req proto.VaultRequest) {
	if cl == nil {
		return
	}
	reply := func(lend proto.VaultLend, err error) {
		r := proto.VaultReply{ID: req.ID, Lend: lend}
		if err != nil {
			r.Error = err.Error()
		}
		go func() { _ = cl.VaultReply(r) }()
	}
	v := a.vaultOf()
	if v.file == nil {
		if req.Action == proto.VaultLogin {
			a.askVaultLogin(req.Platform)
			reply(proto.VaultLend{Message: "The user has no vault yet: they were asked to make one in enowX, then add " + vault.Lookup(req.Platform).Name + "."}, nil)
			return
		}
		reply(proto.VaultLend{}, errors.New("vault: the user has no credential vault yet: they make one in enowX → Credentials"))
		return
	}
	a.vaultFromKeychain(func() {
		if v.key == nil {
			a.vaultQueue(&vaultRequestAsk{cl: cl, host: h, req: req, ask: vaultview.Ask{Locked: true,
				Title: "Unlock the vault", About: a.vaultWho(h, req) + " asks for " + vaultWhat(req) + "."}})
			return
		}
		a.vaultDecide(cl, h, req, reply)
	})
}

// vaultWho says who asks: the agent, its session, the host.
func (a *App) vaultWho(h hosts.Host, req proto.VaultRequest) string {
	who := "A program"
	if req.Agent != "" {
		who = a.agentName(req.Agent)
	}
	if req.SessionName != "" {
		who += " in “" + req.SessionName + "”"
	}
	return who + " on " + h.Name
}

func vaultWhat(req proto.VaultRequest) string {
	switch req.Action {
	case proto.VaultGit:
		return "a git login to " + req.Host
	case proto.VaultDocker:
		return "a login to the registry " + req.Host
	case proto.VaultSign:
		return "an SSH signature"
	case proto.VaultSession:
		return "the credentials a new session starts with"
	case proto.VaultAdd:
		return "keeping " + req.Name + " in the vault"
	case proto.VaultRemove:
		return "removing " + req.Name
	}
	if req.Name != "" {
		return req.Name
	}
	return "a credential"
}

// vaultDecide answers req with the vault unlocked.
func (a *App) vaultDecide(cl *client.Client, h hosts.Host, req proto.VaultRequest, reply func(proto.VaultLend, error)) {
	v := a.vaultOf()
	v.used = time.Now()
	now := time.Now()
	switch req.Action {
	case proto.VaultTest:
		a.vaultTest(req.Name, func(status, msg string) {
			if status == "" {
				reply(proto.VaultLend{}, errors.New(msg))
				return
			}
			reply(proto.VaultLend{Message: req.Name + ": " + status + ": " + msg}, nil)
		})
		return
	case proto.VaultLogin:
		a.askVaultLogin(req.Platform)
		reply(proto.VaultLend{Message: "Asked the user to log in to " + vault.Lookup(req.Platform).Name + " in enowX: creds list shows it once they have."}, nil)
		return
	case proto.VaultAdd:
		a.vaultQueue(&vaultRequestAsk{cl: cl, host: h, req: req, ask: vaultview.Ask{Title: "Keep " + req.Name + " in the vault?",
			About: a.vaultWho(h, req) + " asks to keep a new " + vault.Lookup(req.Platform).Name + " credential, " + req.Name + "."},
			done: func(answer int) {
				if answer != vaultview.AnswerOnce {
					reply(proto.VaultLend{}, errors.New("vault: the user refused"))
					return
				}
				c, err := v.data.Add(vault.Credential{Name: req.Name, Platform: req.Platform, Account: req.Account, Fields: req.Fields})
				if err == nil {
					c.Hosts = []string{h.ID}
					err = a.vaultSave()
				}
				reply(proto.VaultLend{Message: "Kept " + req.Name + " in the vault."}, err)
			}})
		return
	case proto.VaultRemove:
		if _, ok := v.data.Find(req.Name); !ok {
			reply(proto.VaultLend{}, fmt.Errorf("vault: no credential %q", req.Name))
			return
		}
		a.vaultQueue(&vaultRequestAsk{cl: cl, host: h, req: req, ask: vaultview.Ask{Title: "Remove " + req.Name + " from the vault?", Danger: true,
			About: a.vaultWho(h, req) + " asks to remove it. Hosts can't use it afterwards."},
			done: func(answer int) {
				if answer != vaultview.AnswerOnce {
					reply(proto.VaultLend{}, errors.New("vault: the user refused"))
					return
				}
				err := v.data.Remove(req.Name)
				if err == nil {
					err = a.vaultSave()
				}
				reply(proto.VaultLend{Message: "Removed " + req.Name + "."}, err)
			}})
		return
	}
	creds, err := a.vaultPick(req)
	if err != nil {
		reply(proto.VaultLend{}, err)
		return
	}
	var lend proto.VaultLend
	var ask []string
	for _, c := range creds {
		switch d, why := c.Allowed(h.ID, req.Project, now); d {
		case vault.Deny:
			if req.Action != proto.VaultSession {
				reply(proto.VaultLend{}, errors.New("vault: "+why))
				return
			}
		case vault.Ask:
			ask = append(ask, c.ID)
		case vault.Allow:
			if err := a.vaultAddLend(&lend, c, req); err != nil {
				reply(proto.VaultLend{}, err)
				return
			}
		}
	}
	if len(ask) == 0 {
		reply(lend, nil)
		return
	}
	names := make([]string, len(ask))
	for i, id := range ask {
		c, _ := v.data.Find(id)
		names[i] = c.Name
	}
	about := a.vaultWho(h, req) + " asks for it"
	if req.Command != "" {
		about += ", for " + req.Command
	}
	a.vaultQueue(&vaultRequestAsk{cl: cl, host: h, req: req, creds: ask, lend: lend, ask: vaultview.Ask{Always: true, Host: h.Name,
		Title: "Let " + h.Name + " use " + strings.Join(names, ", ") + "?", About: about + ". The values go to that session only."}})
}

// vaultPick is the credentials req is about.
func (a *App) vaultPick(req proto.VaultRequest) ([]*vault.Credential, error) {
	d := a.vaultOf().data
	var match func(*vault.Credential) bool
	switch req.Action {
	case proto.VaultSession:
		var out []*vault.Credential
		for i := range d.Credentials {
			c := &d.Credentials[i]
			if c.Auto || slices.ContainsFunc(req.Names, func(n string) bool { return n == c.ID || strings.EqualFold(n, c.Name) }) {
				out = append(out, c)
			}
		}
		return out, nil
	case proto.VaultEnv:
		if c, ok := d.Find(req.Name); ok {
			return []*vault.Credential{c}, nil
		}
		return nil, fmt.Errorf("vault: no credential %q: creds list shows them", req.Name)
	case proto.VaultGit:
		match = func(c *vault.Credential) bool { _, _, ok := c.GitLogin(req.Host); return ok }
	case proto.VaultDocker:
		match = func(c *vault.Credential) bool { _, _, ok := c.DockerLogin(req.Host); return ok }
	case proto.VaultSign:
		match = func(c *vault.Credential) bool {
			return c.Type == vault.TypeSSHKey && vault.KeyMatches(c.Fields["private_key"], req.KeyBlob)
		}
	default:
		return nil, fmt.Errorf("vault: this app doesn't know %q", req.Action)
	}
	for _, c := range d.Pick("", "", req.Project, match) {
		if st := c.State(time.Now()); st != proto.CredRevoked && st != proto.CredExpired {
			return []*vault.Credential{c}, nil
		}
	}
	return nil, fmt.Errorf("vault: no credential for %s", vaultWhat(req))
}

// vaultAddLend adds what credential c lends for req.
func (a *App) vaultAddLend(lend *proto.VaultLend, c *vault.Credential, req proto.VaultRequest) error {
	lend.Credentials = append(lend.Credentials, c.Name)
	switch req.Action {
	case proto.VaultGit:
		lend.Username, lend.Secret, _ = c.GitLogin(req.Host)
	case proto.VaultDocker:
		lend.Username, lend.Secret, _ = c.DockerLogin(req.Host)
	case proto.VaultSign:
		sig, err := vault.SignSSH(c.Fields["private_key"], req.Data, req.Flags)
		if err != nil {
			return err
		}
		lend.Signature, lend.SigFormat = sig.Blob, sig.Format
	default:
		env, files := c.Lend()
		if lend.Env == nil {
			lend.Env, lend.Files = map[string]string{}, map[string]string{}
		}
		for k, v := range env {
			lend.Env[k] = v
		}
		for k, v := range files {
			lend.Files[k] = v
		}
	}
	return nil
}

// vaultQueue puts a question to the user, telling them when the window
// isn't in front.
func (a *App) vaultQueue(q *vaultRequestAsk) {
	v := a.vaultOf()
	q.until = askUntil(vaultAskLife)
	v.asks = append(v.asks, q)
	if a.win != nil && !a.win.IsFocused() && !a.state.NotifyOff {
		n := mygo.NewNotification(mygo.NotificationOptions{ID: "vault", Title: q.ask.Title, Body: q.ask.About})
		n.OnClick(func() { a.showWindow() })
		go func() { _ = n.Show() }()
	}
}

// vaultAnswerWaiting answers the requests that waited for the vault to
// unlock.
func (a *App) vaultAnswerWaiting() {
	v := a.vaultOf()
	var keep []*vaultRequestAsk
	waiting := v.asks
	v.asks = nil
	for _, q := range waiting {
		if !q.ask.Locked {
			keep = append(keep, q)
			continue
		}
		cl, req := q.cl, q.req
		a.vaultDecide(cl, q.host, req, func(lend proto.VaultLend, err error) {
			r := proto.VaultReply{ID: req.ID, Lend: lend}
			if err != nil {
				r.Error = err.Error()
			}
			go func() { _ = cl.VaultReply(r) }()
		})
		// What it asks now ends when it does: the host's wait began with it.
		for _, later := range v.asks {
			if later.req.ID == req.ID {
				later.until = q.until
			}
		}
	}
	v.asks = append(keep, v.asks...)
}

// vaultAsk is the card of the first question waiting.
func (a *App) vaultAsk(c *ui.Context) *widget.Ask {
	v := a.vlt
	if v == nil || len(v.asks) == 0 {
		return nil
	}
	q := v.asks[0]
	return v.view.AskCard(c, "vault:"+q.req.ID, q.ask, func(password string) error {
		return a.vaultUnlock(password, a.state.VaultRemember)
	}, func(answer int) {
		if len(v.asks) > 0 && v.asks[0] == q {
			v.asks = v.asks[1:]
			a.vaultAnswer(q, answer)
		}
	})
}

// vaultAnswer replies to q as the user answered.
func (a *App) vaultAnswer(q *vaultRequestAsk, answer int) {
	if q.done != nil {
		q.done(answer)
		return
	}
	reply := func(lend proto.VaultLend, err error) {
		r := proto.VaultReply{ID: q.req.ID, Lend: lend}
		if err != nil {
			r.Error = err.Error()
		}
		go func() { _ = q.cl.VaultReply(r) }()
	}
	v := a.vaultOf()
	if answer == vaultview.AnswerDeny || v.data == nil {
		if q.req.Action == proto.VaultSession {
			reply(q.lend, nil) // starts without them
			return
		}
		if q.ask.Locked {
			reply(proto.VaultLend{}, errors.New("vault: the vault is locked, and the user didn't unlock it: ask them to unlock it in enowX → Credentials"))
			return
		}
		reply(proto.VaultLend{}, fmt.Errorf("vault: the user refused it on %s", q.host.Name))
		return
	}
	lend := q.lend
	changed := false
	for _, id := range q.creds {
		c, ok := v.data.Find(id)
		if !ok {
			continue
		}
		if answer == vaultview.AnswerAlways && !slices.Contains(c.Hosts, q.host.ID) {
			c.Hosts, changed = append(c.Hosts, q.host.ID), true
		}
		if err := a.vaultAddLend(&lend, c, q.req); err != nil {
			reply(proto.VaultLend{}, err)
			return
		}
	}
	if changed {
		_ = a.vaultSave()
	}
	reply(lend, nil)
}

// askVaultLogin is an agent asking the user to log in to platform: a card they
// may click, not a move.
func (a *App) askVaultLogin(platform string) {
	a.toastAction("vault:"+platform, vault.Lookup(platform).Name, "An agent asks you to log in here: click to add it", func() { a.openVaultFor(platform) })
}

// openVaultFor opens the Credentials page on adding platform's.
func (a *App) openVaultFor(platform string) {
	a.page = pageVault
	v := a.vaultOf()
	if v.file != nil && platform != "" {
		v.view.OpenAdd(platform)
	}
	a.showWindow()
}

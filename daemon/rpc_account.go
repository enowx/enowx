package daemon

import (
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

var accountHandlers = map[string]handler{
	proto.MethodAccountList: bare(func(c *conn) (any, error) { return c.srv.accounts.List(), nil }),
	proto.MethodAccountAdd: with(func(c *conn, p proto.AccountAddParams) (any, error) {
		if p.Cred != "" {
			return c.srv.accounts.AddKey(p.Agent, p.Name, p.Cred)
		}
		return c.srv.accounts.Add(p.Agent, p.Name)
	}),
	proto.MethodAccountInfo:     bare(func(c *conn) (any, error) { return c.srv.accountsInfo(), nil }),
	proto.MethodAccountSetPrefs: with(func(c *conn, p proto.AccountPrefsParams) (any, error) { return c.srv.setAccountPrefs(p) }),
	proto.MethodAccountSwitch: with(func(c *conn, p proto.AccountSwitchParams) (any, error) {
		if p.Reason == "" {
			p.Reason = "the user switched"
		}
		info, err := c.srv.switchAccount(p)
		if err == nil && p.Agent == "" {
			c.srv.acctWatch.clear(agent.Base(info.Agent), c.srv.accountName(p.Account)) // picked: taken as working
		}
		return info, err
	}),
	proto.MethodAccountLog: bare(func(c *conn) (any, error) { return c.srv.accountLog(), nil }),
	proto.MethodAccountRemove: with(func(c *conn, p proto.AccountRef) (any, error) {
		return ok, c.srv.accounts.Remove(p.ID)
	}),
	proto.MethodAccountSave: with(func(c *conn, p proto.Account) (any, error) { return c.srv.saveConfig(p) }),
}

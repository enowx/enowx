package client

import "github.com/enowx/enowx/proto"

// Accounts lists the host's agents' other accounts.
func (c *Client) Accounts() ([]proto.Account, error) {
	var out []proto.Account
	err := c.Call(proto.MethodAccountList, nil, &out)
	return out, err
}

// AddAccount keeps a new account of agent, to sign in with next.
func (c *Client) AddAccount(agent, name string) (proto.Account, error) {
	var out proto.Account
	err := c.Call(proto.MethodAccountAdd, proto.AccountAddParams{Agent: agent, Name: name}, &out)
	return out, err
}

// RemoveAccount forgets account id, with its sign-in.
func (c *Client) RemoveAccount(id string) error {
	return c.Call(proto.MethodAccountRemove, proto.AccountRef{ID: id}, nil)
}

// SaveConfig adds an agent's config (its ID empty) or changes one.
func (c *Client) SaveConfig(cfg proto.Account) (proto.Account, error) {
	var out proto.Account
	err := c.Call(proto.MethodAccountSave, cfg, &out)
	return out, err
}

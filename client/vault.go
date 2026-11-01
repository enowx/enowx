package client

import "github.com/enowx/enowx/proto"

// VaultIndex is what the host's copy of the vault holds, no values.
func (c *Client) VaultIndex() (proto.VaultIndex, error) {
	var out proto.VaultIndex
	err := c.Call(proto.MethodVaultIndex, nil, &out)
	return out, err
}

// VaultGet is the host's copy of the vault, encrypted.
func (c *Client) VaultGet() (proto.VaultCopy, error) {
	var out proto.VaultCopy
	err := c.Call(proto.MethodVaultGet, nil, &out)
	return out, err
}

// VaultPut replaces the host's copy of the vault.
func (c *Client) VaultPut(v proto.VaultCopy) (proto.VaultIndex, error) {
	var out proto.VaultIndex
	err := c.Call(proto.MethodVaultPut, v, &out)
	return out, err
}

// ServeVault tells the daemon this app answers the vault's requests,
// which come as proto.EventVaultRequest events.
func (c *Client) ServeVault() error { return c.Call(proto.MethodVaultServe, nil, nil) }

// VaultReply answers a proto.EventVaultRequest.
func (c *Client) VaultReply(r proto.VaultReply) error { return c.Call(proto.MethodVaultReply, r, nil) }

// VaultAsk asks the app holding the vault, through the host's daemon.
func (c *Client) VaultAsk(a proto.VaultAsk) (proto.VaultLend, error) {
	var out proto.VaultLend
	err := c.Call(proto.MethodVaultAsk, a, &out)
	return out, err
}

// KeysToVault moves the keys typed on this host to the vault.
func (c *Client) KeysToVault() (proto.HostKeysMoved, error) {
	var out proto.HostKeysMoved
	err := c.Call(proto.MethodHostKeysToVault, nil, &out)
	return out, err
}

// VaultAudit is the host's last limit uses of the vault (0: all).
func (c *Client) VaultAudit(limit int) ([]proto.VaultAuditEntry, error) {
	var out []proto.VaultAuditEntry
	err := c.Call(proto.MethodVaultAudit, proto.VaultAuditParams{Limit: limit}, &out)
	return out, err
}

// VaultScan finds the logins the host's tools keep, to import.
func (c *Client) VaultScan() ([]proto.VaultFound, error) {
	var out []proto.VaultFound
	err := c.Call(proto.MethodVaultScan, nil, &out)
	return out, err
}

// VaultAttach lends credentials names to a session.
func (c *Client) VaultAttach(session string, names []string) ([]string, error) {
	var out []string
	err := c.Call(proto.MethodVaultAttach, proto.VaultAttach{Session: session, Names: names}, &out)
	return out, err
}

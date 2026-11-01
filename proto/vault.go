package proto

import "time"

// The credential vault (docs/vault.md): the app keeps the user's logins
// encrypted with a key that never leaves the device. Hosts keep the
// ciphertext and an index of what it holds, no secret values, and ask the
// app for a credential when one of their sessions needs it: the app asks
// the user the first time on each host, then lends the values, which the
// daemon hands to that session alone and forgets with it.
const (
	// MethodVaultIndex is what the vault holds, without secret values.
	MethodVaultIndex = "vault.index" // → VaultIndex
	// MethodVaultGet is the host's copy of the vault, encrypted.
	MethodVaultGet = "vault.get" // → VaultCopy
	// MethodVaultPut replaces the host's copy (the app, after a change).
	MethodVaultPut = "vault.put" // VaultCopy → VaultIndex
	// MethodVaultServe tells the daemon this connection (the app) answers
	// the vault's requests; the latest to say so answers them.
	MethodVaultServe = "vault.serve"
	// MethodVaultAsk asks the serving app for a credential, or to do what
	// only it can (add, remove, test one, have the user log in).
	MethodVaultAsk   = "vault.ask"   // VaultAsk → VaultLend
	MethodVaultReply = "vault.reply" // VaultReply
	// MethodVaultAudit is the host's record of credentials used there.
	MethodVaultAudit = "vault.audit" // VaultAuditParams → []VaultAuditEntry
	// MethodVaultScan finds logins the host's tools keep (gh, aws,
	// wrangler, docker, npm, SSH keys…), with their values, to import.
	MethodVaultScan = "vault.scan" // → []VaultFound
	// MethodVaultAttach gives a session credentials (by name): lent now,
	// and again whenever its program starts.
	MethodVaultAttach = "vault.attach" // VaultAttach → []string
	// MethodHostKeysToVault moves the keys typed on this host (a judge's,
	// RAG's) to the vault, for every host to read: → HostKeysMoved.
	MethodHostKeysToVault = "host.keysToVault"
	// EventVaultRequest asks the serving app: VaultRequest.
	EventVaultRequest = "vault.request"
	// EventVaultChanged says the host's copy changed: VaultIndex.
	EventVaultChanged = "vault.changed"
)

// What a VaultAsk asks for.
const (
	VaultEnv     = "env"     // Name's variables (or the session's, Name empty)
	VaultSession = "session" // a new session's: those it should start with
	VaultGit     = "git"     // a login for Host, for git
	VaultDocker  = "docker"  // a login for registry Host
	VaultSign    = "sign"    // sign Data with SSH key KeyBlob
	VaultAdd     = "add"     // keep Fields as a new credential Name
	VaultRemove  = "rm"      // remove credential Name
	VaultTest    = "test"    // check credential Name still works
	VaultLogin   = "login"   // have the user log in to Platform
)

// Credential statuses (CredentialInfo.Status).
const (
	CredUnchecked = "unchecked"
	CredValid     = "valid"
	CredInvalid   = "invalid"
	CredExpired   = "expired"
	CredRevoked   = "revoked"
)

// VaultIndex is what a vault holds, without values. Present is false on a
// host that has none yet.
type VaultIndex struct {
	Present     bool             `json:"present"`
	Updated     time.Time        `json:"updated,omitzero"`
	Credentials []CredentialInfo `json:"credentials"`
}

// CredentialInfo is one credential, its secret values left out: Hint is
// the end of its main value, enough to tell two apart.
type CredentialInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Account  string `json:"account,omitempty"`
	Type     string `json:"type"`
	Hint     string `json:"hint,omitempty"`
	// Env is the variables it sets in a session.
	Env     []string  `json:"env,omitempty"`
	Status  string    `json:"status"`
	Checked time.Time `json:"checked,omitzero"`
	Expires time.Time `json:"expires,omitzero"`
	Revoked bool      `json:"revoked,omitempty"`
	// Hosts are the hosts (IDs of the app's list) it may be used on
	// without asking; with HostsOnly, on no other.
	Hosts     []string `json:"hosts,omitempty"`
	HostsOnly bool     `json:"hostsOnly,omitempty"`
	// Projects limits it to sessions of these project folders.
	Projects []string `json:"projects,omitempty"`
	// Auto puts it in every session of the hosts it may be used on.
	Auto bool `json:"auto,omitempty"`
	// Default: the platform's credential picked when none is named.
	Default    bool     `json:"default,omitempty"`
	GitHosts   []string `json:"gitHosts,omitempty"`
	Registries []string `json:"registries,omitempty"`
	// PublicKey is an SSH key's, as in authorized_keys.
	PublicKey string `json:"publicKey,omitempty"`
}

// VaultCopy is a host's copy of the vault: the encrypted file and its
// index.
type VaultCopy struct {
	Data  []byte     `json:"data"`
	Index VaultIndex `json:"index"`
}

// VaultAsk is a request to the app holding the vault. Session is the
// session asking, which the app tells the user of.
type VaultAsk struct {
	Action   string            `json:"action"`
	Name     string            `json:"name,omitempty"`
	Names    []string          `json:"names,omitempty"` // VaultSession's asked for
	Platform string            `json:"platform,omitempty"`
	Account  string            `json:"account,omitempty"`
	Type     string            `json:"type,omitempty"`
	Fields   map[string]string `json:"fields,omitempty"` // VaultAdd's values
	Session  string            `json:"session,omitempty"`
	// Host is the server git or docker logs in to.
	Host string `json:"host,omitempty"`
	// KeyBlob, Data and Flags are an SSH signing request's.
	KeyBlob []byte `json:"keyBlob,omitempty"`
	Data    []byte `json:"data,omitempty"`
	Flags   uint32 `json:"flags,omitempty"`
	// Command is what it is for, shown to the user.
	Command string `json:"command,omitempty"`
}

// VaultRequest is a VaultAsk handed to the app, with where it comes from.
type VaultRequest struct {
	ID string `json:"id"`
	VaultAsk
	Hostname    string `json:"hostname,omitempty"`
	SessionName string `json:"sessionName,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Project     string `json:"project,omitempty"`
}

// VaultLend is what the app answers. Env and Files are a session's
// variables, Files' values the content of a file the variable names (the
// daemon writes it, readable by the user alone, and removes it with the
// session). Username and Secret are a git or docker login.
type VaultLend struct {
	Credentials []string          `json:"credentials,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Files       map[string]string `json:"files,omitempty"`
	Username    string            `json:"username,omitempty"`
	Secret      string            `json:"secret,omitempty"`
	Signature   []byte            `json:"signature,omitempty"`
	SigFormat   string            `json:"sigFormat,omitempty"`
	Message     string            `json:"message,omitempty"`
}

// VaultReply is the app's answer to request ID.
type VaultReply struct {
	ID    string    `json:"id"`
	Lend  VaultLend `json:"lend"`
	Error string    `json:"error,omitempty"`
}

// VaultAuditParams asks for the last Limit uses (0: all kept).
type VaultAuditParams struct {
	Limit int `json:"limit,omitempty"`
}

// VaultAuditEntry is one use of the vault on a host.
type VaultAuditEntry struct {
	Time       time.Time `json:"time"`
	Action     string    `json:"action"`
	Credential string    `json:"credential,omitempty"`
	Session    string    `json:"session,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	Allowed    bool      `json:"allowed"`
}

// VaultFound is a login a host's tool keeps, to import.
type VaultFound struct {
	Name     string            `json:"name"`
	Platform string            `json:"platform"`
	Account  string            `json:"account,omitempty"`
	Type     string            `json:"type"`
	Fields   map[string]string `json:"fields"`
	Source   string            `json:"source"`
	Note     string            `json:"note,omitempty"`
}

// VaultAttach gives session Session credentials Names.
type VaultAttach struct {
	Session string   `json:"session"`
	Names   []string `json:"names"`
}

// HostKeysMoved is what keys moved to the vault, by the credential that now
// holds each, and what stayed typed and why. No value is in it.
type HostKeysMoved struct {
	Moved  []HostKeyMoved  `json:"moved"`
	Failed []HostKeyFailed `json:"failed"`
}

// HostKeyMoved is a key that moved: what it is for, and its credential.
type HostKeyMoved struct {
	What       string `json:"what"`
	Credential string `json:"credential"`
}

// HostKeyFailed is a key that stayed where it was.
type HostKeyFailed struct {
	What string `json:"what"`
	Err  string `json:"err"`
}

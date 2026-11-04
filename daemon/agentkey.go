package daemon

import (
	"slices"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
)

// An agent whose official form runs on a credential of the vault (Preset.VaultPlatform):
// its sessions get it as the vault lends to any session, with the app's consent, in
// their process's environment alone. Their saved command holds no value and they keep
// its name (Meta.Creds), so a wake or a restart has it lent again. With none in the vault
// a session still starts, and the agent says what is missing. docs/claude-platform.md.

// agentKeyPlatform is the vault's platform session of agentID needs a credential of, ""
// when it needs none, as its config (account) is a model API with a key of its own.
func (s *Server) agentKeyPlatform(agentID, account string) string {
	preset, ok := agent.Find(agentID)
	if !ok || preset.VaultPlatform == "" {
		return ""
	}
	if c, ok := s.accounts.Get(account); ok && c.Kind == proto.ConfigKindAPI && c.API != nil {
		return ""
	}
	return preset.VaultPlatform
}

// agentCreds is creds, a session's, with the credential its agent runs on when it has
// none yet: one added to the vault since it started counts too.
func (s *Server) agentCreds(agentID, project, account string, creds []string) []string {
	platform := s.agentKeyPlatform(agentID, account)
	if platform == "" {
		return creds
	}
	pick, _ := vault.AgentKey(s.vaultIndex().Credentials, platform, project, creds, time.Now())
	if pick == "" {
		return creds
	}
	return append(slices.Clone(creds), pick)
}

// keyAwaitsApp says session id (of meta), to start now, would go without the key it has
// in the vault: no app to lend it, and no lease holding it.
func (s *Server) keyAwaitsApp(id string, meta session.Meta) bool {
	platform := s.agentKeyPlatform(meta.Agent, meta.Account)
	if platform == "" || meta.Member != nil || s.vault.served() {
		return false
	}
	idx := s.vaultIndex()
	if _, has := vault.AgentKey(idx.Credentials, platform, meta.Project, meta.Creds, time.Now()); !has {
		return false
	}
	l := s.vaultLease(id)
	if l == nil {
		return true
	}
	s.vault.mu.Lock()
	defer s.vault.mu.Unlock()
	return !slices.ContainsFunc(idx.Credentials, func(c proto.CredentialInfo) bool {
		return c.Platform == platform && slices.Contains(l.names, c.Name)
	})
}

// restartVaultEnv is what session id (of meta) starts with again from the vault, and the
// names of its credentials afterwards: one it was given here shows only once lent.
func (s *Server) restartVaultEnv(id string, meta session.Meta) (env, names []string) {
	creds := s.agentCreds(meta.Agent, meta.Project, meta.Account, meta.Creds)
	env = s.vaultEnvFor(id, meta.Project, creds, meta.Agent)
	if len(creds) == len(meta.Creds) {
		return env, meta.Creds
	}
	if l := s.vaultLease(id); l != nil {
		s.vault.mu.Lock()
		defer s.vault.mu.Unlock()
		return env, slices.Clone(l.names)
	}
	return env, meta.Creds
}

// waitForKey has session id continue once an app serves the vault.
func (s *Server) waitForKey(id string) {
	s.vault.mu.Lock()
	defer s.vault.mu.Unlock()
	if s.vault.keyWait == nil {
		s.vault.keyWait = map[string]bool{}
	}
	s.vault.keyWait[id] = true
}

// continueKeyWaiters continues the sessions that came back from a restart waiting for an
// app to lend their key; the app asks the user where it must.
func (s *Server) continueKeyWaiters() {
	s.vault.mu.Lock()
	waiting := s.vault.keyWait
	s.vault.keyWait = nil
	s.vault.mu.Unlock()
	for id := range waiting {
		if _, err := s.continueSession(id); err != nil {
			s.logger().Warn("could not continue a session waiting for its key", "id", id, "err", err)
		}
	}
}

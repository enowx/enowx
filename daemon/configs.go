package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/enowx/enowx/core/accounts"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/agentapi"
	"github.com/enowx/enowx/core/decide"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A session's config (proto/config.go, docs/configs.md): applied each
// time its agent starts, through its environment and command line, the
// agent's own files untouched. A model API config's key is read from the
// vault then, kept in memory for the session (masked like what the vault
// lends), never written.

// configRun is what a session's agent starts with for its config.
type configRun struct {
	info     *proto.SessionConfig
	api      bool           // a model API: the agent's own API set in its files doesn't apply
	args     []string       // added to its command line, no secret among them
	env      []string       // added to its environment, its key among them
	settings map[string]any // Claude Code's, merged into its --settings
}

// configKeys are the keys of the model APIs sessions run with now, by
// session, to mask them like lent credentials. Safe from any goroutine.
type configKeys struct {
	mu   sync.Mutex
	keys map[string]string
}

func (k *configKeys) set(session, key string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.keys == nil {
		k.keys = map[string]string{}
	}
	if key == "" {
		delete(k.keys, session)
		return
	}
	k.keys[session] = key
}

func (k *configKeys) all() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	var out []string
	for _, v := range k.keys {
		out = append(out, v)
	}
	return out
}

// configInfo is what is shown of config c on a session.
func configInfo(c proto.Account) *proto.SessionConfig {
	out := &proto.SessionConfig{ID: c.ID, Name: c.Name, Kind: c.Kind, Icon: c.Icon, Color: c.Color}
	if c.API != nil {
		out.Provider, out.BaseURL, out.Model = decide.ProviderOfURL(c.API.BaseURL), c.API.BaseURL, c.API.Model
	}
	return out
}

// configRunFor is what session id, of meta, starts with for its config:
// nil for its agent's official one, and for a team's member's session.
func (s *Server) configRunFor(id string, meta session.Meta) (*configRun, error) {
	s.cfgKeys.set(id, "") // the key of the config it ran with before
	if meta.Member != nil || meta.Account == "" {
		return nil, nil
	}
	c, ok := s.accounts.Get(meta.Account)
	if !ok {
		return nil, fmt.Errorf("configs: the session's config is gone: pick another (switch its config)")
	}
	if c.Agent != agent.Base(meta.Agent) {
		return nil, fmt.Errorf("configs: %s is a config of %s, not %s", c.Name, agentName(c.Agent), agentName(meta.Agent))
	}
	run := &configRun{info: configInfo(c), env: slices.Clone(c.Env)}
	// Its effort by the agent's flag (an agent with none takes it with its
	// API, as Aider).
	run.args = append(append(run.args, agent.LaunchArgs(meta.Agent, "", c.Effort)...), c.Args...)
	if c.Kind != proto.ConfigKindAPI || c.API == nil {
		return run, nil
	}
	key := ""
	if c.API.KeyCred != "" {
		var err error
		if key, err = s.configKey(id, c); err != nil {
			return nil, err
		}
	}
	l, err := agentapi.SessionLaunch(meta.Agent, accounts.APIOf(c, key), s.configRunDir(id))
	if err != nil {
		return nil, err
	}
	s.cfgKeys.set(id, key)
	run.api = true
	run.args = append(run.args, l.Args...)
	run.env = append(run.env, l.Env...)
	run.settings = l.Settings
	return run, nil
}

// configRunDir holds what session id's config needs as files (none with
// a key in it).
func (s *Server) configRunDir(id string) string { return filepath.Join(s.dataDir, "configs-run", id) }

// configForget drops what session id's config left: its key, its files.
func (s *Server) configForget(id string) {
	s.cfgKeys.set(id, "")
	_ = os.RemoveAll(s.configRunDir(id))
}

// configKey reads config c's key from the vault for session id.
func (s *Server) configKey(id string, c proto.Account) (string, error) {
	lend, err := s.vaultAsk(proto.VaultAsk{Action: proto.VaultEnv, Name: c.API.KeyCred, Session: id,
		Command: "the key of " + agentName(c.Agent) + "'s config " + c.Name})
	if err != nil {
		return "", fmt.Errorf("configs: %s's key (%s in the vault): %w", c.Name, c.API.KeyCred, err)
	}
	for _, v := range lend.Env {
		if v != "" {
			return v, nil
		}
	}
	if lend.Secret != "" {
		return lend.Secret, nil
	}
	return "", fmt.Errorf("configs: the credential %s has no value", c.API.KeyCred)
}

// environ is run's variables, none for nil.
func (r *configRun) environ() []string {
	if r == nil {
		return nil
	}
	return r.env
}

// sessionInfo is run's config as shown on its session, nil for none.
func (r *configRun) sessionInfo() *proto.SessionConfig {
	if r == nil {
		return nil
	}
	return r.info
}

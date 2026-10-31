package daemon

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/enowx/enowx/core/agentapi"
	"github.com/enowx/enowx/proto"
)

// Model APIs set before configs were written into the agents' own files
// (core/agentapi Set). Each becomes a config of the agent's: its key kept
// in the vault (the app asks the user), the config the agent's default
// for new sessions so they go on as before, and the agent's files taken
// back to what the user had (agentapi.Unset). Until the vault took the
// key, the files stay as they are and the API keeps working from them;
// it is tried again whenever an app connects to lend credentials.

// apiMigration serializes migrations: one at a time.
var apiMigration sync.Mutex

// migrateAPIs moves the model APIs set in agents' files into configs, on
// the user's daemon (daemon.Run), not those tests start.
func (s *Server) migrateAPIs() {
	if !s.migrateAPIsOn || !apiMigration.TryLock() {
		return
	}
	defer apiMigration.Unlock()
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	for _, id := range agentapi.Supported() {
		if !agentapi.PerSession(id) {
			continue // set only in its files: it stays there (Settings → Agents)
		}
		a, err := agentapi.Of(home, id)
		if err != nil || a.Current == nil {
			continue
		}
		if err := s.migrateAPI(home, id, *a.Current); err != nil {
			s.logger().Info("a model API stays in the agent's settings for now", "agent", id, "err", err)
		}
	}
}

// migrateAPI makes api, agent id's, a config and takes it out of its
// files.
func (s *Server) migrateAPI(home, id string, api agentapi.API) error {
	cfg := proto.Account{Agent: id, Kind: proto.ConfigKindAPI, Effort: api.Effort,
		API: &proto.ConfigAPI{BaseURL: api.BaseURL, Protocol: api.Protocol, Model: api.Model, SmallModel: api.SmallModel}}
	for _, m := range api.Models {
		cfg.API.Models = append(cfg.API.Models, proto.AgentAPIModel{ID: m.ID, Vision: m.Vision, Context: m.Context, Output: m.Output, Effort: m.Effort})
	}
	// A config made by an earlier try that stopped short: the same API.
	existing, found := proto.Account{}, false
	for _, c := range s.accounts.List() {
		if c.Agent == id && c.Kind == proto.ConfigKindAPI && c.API != nil && c.API.BaseURL == api.BaseURL && strings.HasPrefix(c.Name, "Model API") {
			existing, found = c, true
		}
	}
	if api.APIKey != "" {
		cred := id + "-model-api"
		if found && existing.API.KeyCred != "" {
			cred = existing.API.KeyCred
		}
		if !slices.ContainsFunc(s.vaultIndex().Credentials, func(c proto.CredentialInfo) bool { return c.Name == cred }) {
			_, err := s.vaultAsk(proto.VaultAsk{Action: proto.VaultAdd, Name: cred, Platform: "api-key", Fields: map[string]string{"value": api.APIKey},
				Command: "the key of the model API " + agentName(id) + " is set to, kept in the vault as its config's"})
			if err != nil {
				return fmt.Errorf("keeping its key in the vault: %w", err)
			}
		}
		cfg.API.KeyCred = cred
	}
	if found {
		cfg.ID, cfg.Name, cfg.Icon, cfg.Color = existing.ID, existing.Name, existing.Icon, existing.Color
	} else {
		cfg.Name = s.freeConfigName(id, "Model API")
	}
	saved, err := s.accounts.Save(cfg)
	if err != nil {
		return err
	}
	pr := s.accountPrefs.Of(id)
	if pr.Default == "" && len(pr.Projects) == 0 {
		// Its sessions used the API: new ones go on doing so.
		pr.Default = saved.Name
		if err := s.accountPrefs.Set(id, pr); err != nil {
			return err
		}
	}
	restored, err := agentapi.Unset(home, id)
	if err != nil {
		return fmt.Errorf("taking it out of %s's settings: %w", agentName(id), err)
	}
	s.logger().Info("a model API became a config", "agent", id, "config", saved.Name, "files restored", restored)
	return nil
}

// freeConfigName is name, or name with a number, that none of agent's
// configs has.
func (s *Server) freeConfigName(agent, name string) string {
	taken := func(n string) bool { _, ok := s.accounts.ByName(agent, n); return ok }
	if !taken(name) {
		return name
	}
	for i := 2; ; i++ {
		if n := fmt.Sprintf("%s %d", name, i); !taken(n) {
			return n
		}
	}
}

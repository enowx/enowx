package accounts

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/agentapi"
	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
)

// The user's configs, in the synced settings: a sync from another host
// may change the file at any time, so it is read on each use.

func (s *Store) configsFile() string { return filepath.Join(s.dir, "settings", "configs.json") }

// readConfigs is the configs on disk; s.mu held.
func (s *Store) readConfigs() []proto.Account {
	var out []proto.Account
	if data, err := os.ReadFile(s.configsFile()); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

// writeConfigs keeps cfgs; s.mu held.
func (s *Store) writeConfigs(cfgs []proto.Account) error {
	if cfgs == nil {
		cfgs = []proto.Account{}
	}
	data, err := json.MarshalIndent(cfgs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.configsFile()), 0o700); err != nil {
		return err
	}
	if err := envsync.WriteFileAtomic(s.configsFile(), data, 0o600); err != nil {
		return fmt.Errorf("accounts: %w", err)
	}
	return nil
}

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Save adds config c (its ID empty) or changes the one of its ID, after
// checking it: its kind and the agent can take it, its name new for the
// agent, its API as the agent takes one. An account made here gets its
// home on this host.
func (s *Store) Save(c proto.Account) (proto.Account, error) {
	c, err := check(c)
	if err != nil {
		return proto.Account{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfgs := s.readConfigs()
	i := -1
	if c.ID != "" {
		if i = slices.IndexFunc(cfgs, func(a proto.Account) bool { return a.ID == c.ID }); i < 0 {
			return proto.Account{}, fmt.Errorf("accounts: no config %s", c.ID)
		}
		if cfgs[i].Agent != c.Agent {
			return proto.Account{}, errors.New("accounts: a config stays its agent's: duplicate it for another")
		}
		if (cfgs[i].Kind == proto.ConfigKindAPI) != (c.Kind == proto.ConfigKindAPI) {
			return proto.Account{}, errors.New("accounts: a config stays the kind it is")
		}
	}
	if slices.ContainsFunc(cfgs, func(a proto.Account) bool {
		return a.ID != c.ID && a.Agent == c.Agent && strings.EqualFold(a.Name, c.Name)
	}) {
		return proto.Account{}, fmt.Errorf("accounts: %s already has a config named %s", c.Agent, c.Name)
	}
	if i < 0 {
		c.ID = newID()
		if c.Kind != proto.ConfigKindAPI && c.Cred == "" {
			if err := os.MkdirAll(s.Home(c.ID), 0o700); err != nil {
				return proto.Account{}, fmt.Errorf("accounts: %w", err)
			}
		}
		cfgs = append(cfgs, c)
	} else {
		cfgs[i] = c
	}
	if err := s.writeConfigs(cfgs); err != nil {
		return proto.Account{}, err
	}
	return c, nil
}

// check is c as kept: trimmed, its API as its agent takes one.
func check(c proto.Account) (proto.Account, error) {
	c.Name, c.Cred, c.Effort = strings.TrimSpace(c.Name), strings.TrimSpace(c.Cred), strings.TrimSpace(c.Effort)
	c.Member, c.Agent = "", agentapi.Base(c.Agent)
	if c.Name == "" {
		return c, errors.New("accounts: name the config")
	}
	if c.Agent == "" {
		return c, errors.New("accounts: which agent's config?")
	}
	if strings.EqualFold(c.Name, "default") || strings.EqualFold(c.Name, "official") {
		return c, fmt.Errorf("accounts: %q names the agent's own config", c.Name)
	}
	var env []string
	for _, e := range c.Env {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		k, _, ok := strings.Cut(e, "=")
		if !ok || !envNameRe.MatchString(k) {
			return c, fmt.Errorf("accounts: %q is not NAME=VALUE", e)
		}
		env = append(env, e)
	}
	c.Env = env
	c.Args = slices.DeleteFunc(c.Args, func(a string) bool { return strings.TrimSpace(a) == "" })
	switch c.Kind {
	case proto.ConfigKindAccount:
		c.API = nil
		if c.Cred == "" && !Supported(c.Agent) {
			return c, fmt.Errorf("accounts: %s can't have other accounts", c.Agent)
		}
	case proto.ConfigKindAPI:
		c.Cred = ""
		if c.API == nil {
			return c, errors.New("accounts: a model API config needs its API")
		}
		if !agentapi.PerSession(c.Agent) {
			return c, fmt.Errorf("accounts: %s can't take a model API per session", c.Agent)
		}
		api, err := agentapi.Prepare(c.Agent, APIOf(c, ""))
		if err != nil {
			return c, err
		}
		cred := strings.TrimSpace(c.API.KeyCred)
		c.API = &proto.ConfigAPI{BaseURL: api.BaseURL, Protocol: api.Protocol, KeyCred: cred, Model: api.Model, SmallModel: api.SmallModel}
		for _, m := range api.Models {
			c.API.Models = append(c.API.Models, proto.AgentAPIModel{ID: m.ID, Vision: m.Vision, Context: m.Context, Output: m.Output, Effort: m.Effort})
		}
	default:
		return c, fmt.Errorf("accounts: no kind of config %q", c.Kind)
	}
	return c, nil
}

// APIOf is model API config c's API, key its key (read from the vault).
func APIOf(c proto.Account, key string) agentapi.API {
	if c.API == nil {
		return agentapi.API{}
	}
	api := agentapi.API{BaseURL: c.API.BaseURL, APIKey: key, Model: c.API.Model, SmallModel: c.API.SmallModel, Protocol: c.API.Protocol, Effort: c.Effort}
	for _, m := range c.API.Models {
		api.Models = append(api.Models, agentapi.Model{ID: m.ID, Vision: m.Vision, Context: m.Context, Output: m.Output, Effort: m.Effort})
	}
	return api
}

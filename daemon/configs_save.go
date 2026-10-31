package daemon

import (
	"slices"
	"strings"

	"github.com/enowx/enowx/proto"
)

// saveConfig adds config c or changes it (account.save); a config
// renamed keeps its place in the preferences, which are by name.
func (s *Server) saveConfig(c proto.Account) (proto.Account, error) {
	old, had := proto.Account{}, false
	if c.ID != "" {
		old, had = s.accounts.Get(c.ID)
	}
	saved, err := s.accounts.Save(c)
	if err != nil {
		return proto.Account{}, err
	}
	if had && old.Name != saved.Name {
		if err := s.renameInPrefs(saved.Agent, old.Name, saved.Name); err != nil {
			return saved, err
		}
	}
	return saved, nil
}

// renameInPrefs has agent's preferences name config to where they named
// from.
func (s *Server) renameInPrefs(agent, from, to string) error {
	pr := s.accountPrefs.Of(agent)
	same := func(n string) bool { return n != "" && strings.EqualFold(n, from) }
	pr.Order = slices.Clone(pr.Order)
	for i, n := range pr.Order {
		if same(n) {
			pr.Order[i] = to
		}
	}
	if same(pr.Default) {
		pr.Default = to
	}
	for p, n := range pr.Projects {
		if same(n) {
			pr.Projects[p] = to
		}
	}
	if only, ok := pr.Only[from]; ok {
		delete(pr.Only, from)
		pr.Only[to] = only
	}
	return s.accountPrefs.Set(agent, pr)
}

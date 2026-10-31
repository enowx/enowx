// Package decide is enowX's judge: a small, cheap model asked instead of
// guessing (a session's state, an agent's command, a long output, a loop),
// which answers typed: a verdict, how sure, why. The judges are found on
// the host (agents' one-shot commands, model APIs with keys in the vault,
// local model servers) and tried in the user's order within their policy;
// with none answering, the caller does what it did without. Settings are
// synced; the log stays on the host. See docs/decide.md.
package decide

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
)

// Settings keeps the decisions' settings, settings/decide.json, synced.
// Safe from any goroutine.
type Settings struct {
	dir string
	mu  sync.Mutex
}

// OpenSettings is the settings kept in dir, a daemon's folder.
func OpenSettings(dir string) *Settings { return &Settings{dir: dir} }

func (s *Settings) file() string { return filepath.Join(s.dir, "settings", "decide.json") }

// Get is the settings, defaults filled in.
func (s *Settings) Get() proto.DecideSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out proto.DecideSettings
	if data, err := os.ReadFile(s.file()); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	if out.Policy == "" {
		out.Policy = proto.DecideSameProvider
	}
	if out.Uses == nil {
		out.Uses = map[string]proto.DecideUse{}
	}
	return out
}

// Set keeps the settings.
func (s *Settings) Set(st proto.DecideSettings) error {
	if err := checkPolicy(st.Policy, st.Provider); err != nil {
		return err
	}
	for use, u := range st.Uses {
		if !slices.Contains(proto.DecideUses, use) {
			return fmt.Errorf("decide: no use %q", use)
		}
		if u.MinConfidence < 0 || u.MinConfidence > 1 || u.DailyCalls < 0 {
			return fmt.Errorf("decide: %s: how sure is 0 to 1, calls a day not negative", use)
		}
	}
	for _, p := range st.Projects {
		if err := checkPolicy(p.Policy, p.Provider); err != nil {
			return err
		}
	}
	for i, j := range st.Own {
		if j.Model == "" || j.Kind != proto.JudgeAPI && j.Kind != proto.JudgeLocal {
			return fmt.Errorf("decide: a judge added is a model API or a local server, with its model")
		}
		if j.Kind == proto.JudgeLocal && j.URL == "" {
			return fmt.Errorf("decide: a local judge needs its server's address")
		}
		st.Own[i] = ownJudge(j)
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.file()), 0o700); err != nil {
		return err
	}
	return envsync.WriteFileAtomic(s.file(), data, 0o600)
}

func checkPolicy(policy, provider string) error {
	switch policy {
	case "", proto.DecideSameProvider, proto.DecideAnyProvider, proto.DecideLocalOnly:
	case proto.DecideOneProvider:
		if provider == "" {
			return fmt.Errorf("decide: pick the one provider allowed")
		}
	default:
		return fmt.Errorf("decide: no policy %q", policy)
	}
	return nil
}

// Use is use's settings, its defaults filled in.
func Use(st proto.DecideSettings, use string) proto.DecideUse {
	u := st.Uses[use]
	if u.MinConfidence == 0 {
		u.MinConfidence = 0.75
		if use == proto.DecideApprove {
			u.MinConfidence = 0.9 // allowing without the user wants more
		}
	}
	if u.DailyCalls == 0 {
		u.DailyCalls = 300
	}
	return u
}

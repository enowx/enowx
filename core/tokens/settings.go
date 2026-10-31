package tokens

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
)

// DefaultBackfillDays is how far back turning tracking on reads.
const DefaultBackfillDays = 30

// Settings keeps tracking's settings, settings/tokens.json under a
// daemon's folder: synced, so prices and budgets are the same on every
// host. Safe from any goroutine.
type Settings struct {
	dir string
	mu  sync.Mutex
}

// OpenSettings is the settings kept in dir, a daemon's folder.
func OpenSettings(dir string) *Settings { return &Settings{dir: dir} }

func (s *Settings) file() string { return filepath.Join(s.dir, "settings", "tokens.json") }

// Get is the settings, defaults filled in.
func (s *Settings) Get() proto.TokensSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out proto.TokensSettings
	if data, err := os.ReadFile(s.file()); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	if out.BackfillDays == 0 {
		out.BackfillDays = DefaultBackfillDays
	}
	return out
}

// Set checks and keeps the settings; budgets without an ID get one.
func (s *Settings) Set(st proto.TokensSettings) (proto.TokensSettings, error) {
	if st.BackfillDays < 0 || st.BackfillDays > KeepDays {
		return st, fmt.Errorf("tokens: read back 0 to %d days", KeepDays)
	}
	for _, p := range st.Prices {
		if strings.TrimSpace(p.Model) == "" {
			return st, fmt.Errorf("tokens: a price names its model (or a prefix ending in *)")
		}
		if p.Input < 0 || p.Output < 0 || p.CacheRead < 0 || p.CacheWrite < 0 {
			return st, fmt.Errorf("tokens: %s: prices aren't negative", p.Model)
		}
	}
	st.Prices = slices.DeleteFunc(st.Prices, func(p proto.TokenPrice) bool { return p.Shipped })
	for i, b := range st.Budgets {
		if err := checkBudget(b); err != nil {
			return st, err
		}
		if b.ID == "" {
			var id [4]byte
			_, _ = rand.Read(id[:])
			st.Budgets[i].ID = hex.EncodeToString(id[:])
		}
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.file()), 0o700); err != nil {
		return st, err
	}
	return st, envsync.WriteFileAtomic(s.file(), data, 0o600)
}

func checkBudget(b proto.TokenBudget) error {
	if !slices.Contains([]string{proto.BudgetDay, proto.BudgetWeek, proto.BudgetMonth}, b.Period) {
		return fmt.Errorf("tokens: a budget is per day, week or month, not %q", b.Period)
	}
	switch b.Scope {
	case proto.BudgetAll:
	case proto.BudgetAgent, proto.BudgetConfig, proto.BudgetProject:
		if b.Target == "" {
			return fmt.Errorf("tokens: a budget for one %s names it", b.Scope)
		}
	default:
		return fmt.Errorf("tokens: a budget is for everything, an agent, a config or a project, not %q", b.Scope)
	}
	if b.Tokens <= 0 && b.Cost <= 0 {
		return fmt.Errorf("tokens: a budget sets tokens or a cost")
	}
	switch b.AtLimit {
	case "", proto.BudgetNotify, proto.BudgetAsk, proto.BudgetStop:
	default:
		return fmt.Errorf("tokens: at its limit a budget notifies, asks or stops, not %q", b.AtLimit)
	}
	return nil
}

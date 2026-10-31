package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/core/tokens"
	"github.com/enowx/enowx/proto"
)

// Each host writes its daily totals to settings/tokens-hosts/<its ID>.json,
// synced with the user's data, so that the app shows a host it doesn't
// reach by its last totals. Only this host writes its own file.

func (s *Server) hostDaysDir() string { return filepath.Join(s.dataDir, "settings", "tokens-hosts") }

// writeHostDays writes this host's totals of the last hostDaysKept days.
func (s *Server) writeHostDays(now time.Time, prices *tokens.Prices) {
	id, err := s.userdata.ID()
	if err != nil {
		return
	}
	since := now.AddDate(0, 0, -hostDaysKept)
	calls := s.tok.store.Calls(since, now.Add(time.Minute))
	s.tok.store.Forget()
	if len(calls) == 0 {
		return
	}
	days := proto.TokensHostDays{Host: id, Name: s.userdata.Name, Updated: now.UTC().Truncate(time.Second), Rows: tokens.DayRows(calls, prices)}
	data, err := json.Marshal(days)
	if err != nil {
		return
	}
	file := filepath.Join(s.hostDaysDir(), id+".json")
	_ = os.MkdirAll(filepath.Dir(file), 0o700)
	if err := envsync.WriteFileAtomic(file, data, 0o600); err != nil {
		s.logger().Warn("tokens: host totals", "err", err)
	}
}

// hostDays is every host's synced totals this host has, its own written
// fresh first.
func (s *Server) hostDays() []proto.TokensHostDays {
	if s.tokensOn() {
		s.writeHostDays(time.Now(), s.prices())
	}
	entries, _ := os.ReadDir(s.hostDaysDir())
	out := []proto.TokensHostDays{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.hostDaysDir(), e.Name()))
		if err != nil {
			continue
		}
		var d proto.TokensHostDays
		if json.Unmarshal(data, &d) == nil && d.Host != "" {
			out = append(out, d)
		}
	}
	return out
}

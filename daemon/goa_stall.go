package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// A task that isn't moving: nothing changes a session's state then, so GOA, told
// only of changes, would wait for ever, and the board go on saying working.

const (
	stallEvery   = 15 * time.Second
	stallCPUIdle = 1.0 // percent of a core a tree working uses at least
	stallAgain   = 3   // a further line after this many times the limit
)

// goaStallFile keeps the limits apart from the synced settings: how long a
// worker may take depends on the host it watches from.
func (s *Server) goaStallFile() string { return filepath.Join(s.dataDir, "goa-stall.json") }

// goaStall are this host's limits, the defaults for any not set.
func (s *Server) goaStall() proto.GOAStall {
	var l proto.GOAStall
	if data, err := os.ReadFile(s.goaStallFile()); err == nil {
		_ = json.Unmarshal(data, &l)
	}
	return goa.StallOr(l)
}

// saveGOAStall keeps l, a limit of 0 going back to its default.
func (s *Server) saveGOAStall(l proto.GOAStall) (proto.GOAStall, error) {
	if err := goa.CheckStall(l); err != nil {
		return proto.GOAStall{}, err
	}
	data, err := json.MarshalIndent(goa.StallOr(l), "", "  ")
	if err != nil {
		return proto.GOAStall{}, err
	}
	if err := writeTeamFile(s.goaStallFile(), data); err != nil {
		return proto.GOAStall{}, fmt.Errorf("goa: stall limits: %w", err)
	}
	select {
	case s.goa.stallSet <- struct{}{}:
	default:
	}
	return s.goaStall(), nil
}

// stallSeconds is a limit as a time.
func stallSeconds(seconds int) time.Duration { return time.Duration(seconds) * time.Second }

// stallLook is what a task's session is like now.
type stallLook struct {
	Info  proto.SessionInfo
	Blank bool
	Still time.Duration // since its screen last changed
	CPU   float64
	Age   time.Duration // since the task was handed over
}

// stallCase names how a task isn't moving, and after how long that counts; ""
// when it is moving, as far as can be told. A long build shows no output, but
// uses the CPU; a slow model's reply shows the agent's own timer, on its screen.
func stallCase(l stallLook, lim proto.GOAStall) (string, time.Duration) {
	blank, idle, stuck := stallSeconds(lim.Blank), stallSeconds(lim.Idle), stallSeconds(lim.Stuck)
	switch {
	case l.Info.Exited && l.Info.Hibernated:
		return "idle: its session is hibernated", idle
	case l.Info.Exited:
		return "", 0 // the board has it as failed or stopped
	case l.Blank && l.Age >= blank:
		return "never started: its screen is blank", blank
	case l.Info.State == proto.StateWorking:
		if l.Still >= stuck && l.CPU < stallCPUIdle {
			return "stuck: nothing has changed on its screen, and nothing runs", stuck
		}
		return "", 0
	case l.Info.State == "" && l.Still >= idle && l.CPU < stallCPUIdle:
		return "idle: its agent isn't working on it", idle
	}
	return "", 0
}

// stallFollow is what was told of a task. Only watchStalls touches it.
type stallFollow struct {
	screen string
	since  time.Time
	told   time.Time
}

// watchStalls looks at the tasks not settled until done closes.
func (s *Server) watchStalls(done <-chan struct{}) {
	follows := map[string]*stallFollow{}
	var sampler procstat.Sampler
	tick := time.NewTicker(stallEvery)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
		}
		seen := map[string]bool{}
		lim := s.goaStall()
		_ = sampler.Sample()
		for _, t := range s.goa.board.Working() {
			if t.Host != "" || t.Session == "" || (t.Status != proto.GOATaskWorking && t.Status != proto.GOATaskStalled) {
				continue
			}
			sess, err := s.sessions.Get(t.Session)
			if err != nil {
				continue
			}
			seen[t.ID] = true
			f := follows[t.ID]
			if f == nil {
				f = &stallFollow{since: time.Now()}
				follows[t.ID] = f
			}
			info := sess.Info()
			text := sess.Terminal().Text()
			if text != f.screen {
				f.screen, f.since = text, time.Now()
			}
			why, limit := stallCase(stallLook{Info: info, Blank: strings.TrimSpace(text) == "", Still: time.Since(f.since),
				CPU: taggedCPU(&sampler, info.ID), Age: time.Since(t.Created)}, lim)
			if why == "" {
				if t.Status == proto.GOATaskStalled {
					_, _ = s.goa.board.Update(t.ID, func(t *proto.GOATask) { t.Status = proto.GOATaskWorking })
					s.goaChanged()
				}
				f.told = time.Time{}
				continue
			}
			if !f.told.IsZero() && time.Since(f.told) < stallAgain*limit {
				continue
			}
			f.told = time.Now()
			_, _ = s.goa.board.Update(t.ID, func(t *proto.GOATask) { t.Status = proto.GOATaskStalled })
			s.goaChanged()
			line := fmt.Sprintf("%s, for %s", why, time.Since(goa.Later(t.Created, f.since)).Round(time.Second))
			go s.fireHooks(info, proto.HookEvent{Event: proto.HookStalled, Text: line})
			s.goaTell(fmt.Sprintf("[enowX GOA] Task %s %q (session %s) isn't moving: %s. Its screen ends: %s. Read it (read %s), then restart or move the task and tell the user; don't repeat what already failed.",
				t.ID, t.Title, t.Session, line, clip(oneLine(lastLines(text, 3)), 200), t.Session))
		}
		for id := range follows {
			if !seen[id] {
				delete(follows, id)
			}
		}
	}
}

// taggedCPU is the percent of a core a session's processes use.
func taggedCPU(sampler *procstat.Sampler, id string) float64 {
	cpu := 0.0
	for _, pid := range procstat.Tagged(session.EnvSession, id) {
		cpu += sampler.One(pid).CPU
	}
	return cpu
}

// lastLines are text's last n lines with something on them.
func lastLines(text string, n int) string {
	var out []string
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			out = append([]string{t}, out...)
		}
	}
	return strings.Join(out, " / ")
}

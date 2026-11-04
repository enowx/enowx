package daemon

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/core/selfprofile"
	"github.com/enowx/enowx/proto"
)

func init() { slowMethods[proto.MethodDaemonProfile] = true } // it waits, and it reads every mapping

// profile measures the daemon, the latest app attached, and the programs sessions run.
// It changes nothing. Measuring both over the same window is what makes their CPU
// comparable, so the app is asked while the daemon measures itself.
func (s *Server) profile(p proto.ProfileParams) proto.Profile {
	type answer struct {
		app *proto.ProcessProfile
		err string
	}
	asked := make(chan answer, 1)
	go func() {
		var got proto.ProcessProfile
		raw, err := s.askApp("profile", p, time.Duration(p.CPUSeconds+30)*time.Second)
		if err == nil {
			err = json.Unmarshal(raw, &got)
		}
		if err != nil {
			asked <- answer{err: err.Error()}
			return
		}
		asked <- answer{app: &got}
	}()
	out := proto.Profile{Daemon: selfprofile.Take(selfprofile.Options{Kind: "daemon", Version: Version, Params: p, Areas: s.profileAreas})}
	a := <-asked
	out.App, out.AppError = a.app, a.err
	out.Programs = s.sessionPrograms()
	return out
}

// profileAreas are what the daemon keeps, by part.
func (s *Server) profileAreas() []proto.ProfileArea {
	conversations, entries := s.transcripts.held()
	return []proto.ProfileArea{
		{Name: "sessions (each up to about 13 MB of scrollback, outside Go)", Count: len(s.sessions.List())},
		{Name: "conversations held for chats", Count: conversations},
		{Name: "their entries", Count: entries},
	}
}

// sessionPrograms sums what sessions' processes hold, by program, the most first.
func (s *Server) sessionPrograms() []proto.ProfileProgram {
	s.stats()
	st := &s.statsState
	byName := map[string]*proto.ProfileProgram{}
	st.mu.Lock()
	defer st.mu.Unlock()
	for child, id := range st.owners {
		if id == "" {
			continue
		}
		for _, pid := range st.sampler.Descendants(child) {
			u := st.sampler.One(pid)
			name := programLabel(procstat.Args(pid))
			g := byName[name]
			if g == nil {
				g = &proto.ProfileProgram{Name: name}
				byName[name] = g
			}
			g.Count++
			g.RSS += u.RSS
		}
	}
	out := make([]proto.ProfileProgram, 0, len(byName))
	for _, g := range byName {
		out = append(out, *g)
	}
	slices.SortFunc(out, func(a, b proto.ProfileProgram) int {
		if a.RSS != b.RSS {
			if a.RSS > b.RSS {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out[:min(len(out), 25)]
}

// runsScripts are programs named by what they run. Only a script's file name is shown
// of a command line, whose other arguments may hold anything, a token too.
var runsScripts = []string{"node", "bun", "deno", "python", "python3", "uv", "uvx", "npx", "ruby"}

func programLabel(args []string) string {
	if len(args) == 0 {
		return "?"
	}
	name := filepath.Base(args[0])
	if slices.Contains(runsScripts, name) {
		for _, a := range args[1:] {
			if !strings.HasPrefix(a, "-") {
				return diag.Scrub(name + " " + filepath.Base(a))
			}
		}
	}
	return diag.Scrub(name)
}

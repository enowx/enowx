package daemon

import (
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/proto"
)

// A session in bypass doesn't stop to ask about routine work, whatever asks: another
// hook, a plugin's, or the agent's own check. What still prompts is answered here.
// What goes on reaching the user: a hook's deny, which never gets this far; what GOA's
// urgent rules hold for the owner (can't be undone, spends money, publishes, touches
// secrets); and what the judge, sure of itself, doesn't allow. A session not in bypass
// asks as it always did. Every automatic answer is in the decide log, use "bypass".

// bypassAnswer says a prompt of session info for event ev is answered allow. An agent
// that reports its mode goes by that, or by how it was started.
func (s *Server) bypassAnswer(info proto.SessionInfo, ev proto.HookEvent) bool {
	if info.GOA || !userSession(info) || !workerRunsUnasked(info, ev) {
		return false
	}
	return s.bypassPolicy(info, ev.Tool, ev.Command, ev.Paths, ev.Scratchpad, "its permission prompt, from another hook's ask or its own check: it doesn't say which")
}

// bypassPolicy decides and logs a prompt of a session in bypass: tool's command and
// paths, asked by asker; scratchpad is the agent's own, if it says.
func (s *Server) bypassPolicy(info proto.SessionInfo, tool, command string, paths []string, scratchpad, asker string) bool {
	asked := strings.TrimSpace(diag.Scrub(s.maskLent(tool + " " + command + " " + strings.Join(paths, " "))))
	record := func(allowed bool, why string) bool {
		verdict, acted := proto.VerdictAllow, proto.ActedAllowed
		if !allowed {
			verdict, acted = proto.VerdictAsk, proto.ActedLeft
		}
		d := proto.Decision{ID: "b" + newRenderID(), Use: proto.DecideBypass, Session: info.ID, Project: info.Project, Agent: agent.Base(info.Agent),
			Time: time.Now(), Verdict: verdict, Why: why, Input: clip(asked, 2000) + " (asked by " + asker + ")", Acted: acted}
		s.judge.Record(d)
		s.broadcast(proto.EventDecision, d)
		return allowed
	}
	if urgent := goa.Urgent(tool, command, paths); urgent != "" && !removesOnlyInside(command, info.Dir, scratchpad) {
		return record(false, "for the owner: "+urgent)
	}
	if s.useOn(proto.DecideApprove) && command != "" {
		if d, err := s.decideFor(info, proto.DecideAsk{Use: proto.DecideApprove, Command: command}); err == nil && d.Judge != "" && s.sure(d) && d.Verdict != proto.VerdictAllow {
			return record(false, "for the owner: the judge says "+d.Verdict+": "+d.Why)
		}
	}
	return record(true, "routine in bypass: not urgent, and the judge doesn't object")
}

// sessionInBypass says session info runs in bypass, as far as its agent has said.
func (s *Server) sessionInBypass(info proto.SessionInfo) bool {
	if e, ok := s.agentModes.get(info.ID); ok && e.mode != "" {
		return e.mode == agentModeBypass
	}
	return info.SkipPermissions
}

// shellSpecial is what makes a command more than its words: expansions, globs, chains.
const shellSpecial = "$`*?[]{}()<>|&;~\\\"'\n"

// removesOnlyInside says command is a plain rm whose every path is inside the folders
// given: the session's own project or scratchpad, which is routine, unlike what can't be
// undone outside them. Anything it can't read for sure doesn't count.
func removesOnlyInside(command string, folders ...string) bool {
	if strings.ContainsAny(command, shellSpecial) {
		return false
	}
	words := strings.Fields(command)
	if len(words) < 2 || words[0] != "rm" {
		return false
	}
	found := false
	for _, w := range words[1:] {
		if strings.HasPrefix(w, "-") {
			continue
		}
		found = true
		if !slices.ContainsFunc(folders, func(dir string) bool { return pathInside(w, dir) }) {
			return false
		}
	}
	return found
}

// pathInside says path p, relative to dir if it isn't absolute, is under dir, not dir.
func pathInside(p, dir string) bool {
	if dir == "" {
		return false
	}
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	rel, err := filepath.Rel(dir, filepath.Clean(p))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

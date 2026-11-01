package ui

import (
	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// Forking a session from its menu: a new session goes on from a compacted
// copy of its conversation (daemon/fork.go), in the same folder and branch,
// which the user is warned of first.

// agentForks says whether the host's enowX forks sessions of agent id.
func agentForks(id string) bool {
	p, ok := agent.Find(id)
	return ok && len(p.Fork) > 0
}

// askFork forks session s once the user agrees, or says it can't.
func (a *App) askFork(s proto.SessionInfo) {
	if !agentForks(s.Agent) {
		a.notify("Forking isn't supported for " + a.agentName(s.Agent) + " yet: Claude Code and Codex only")
		return
	}
	a.confirm("Fork “"+a.sessionName(s)+"”?",
		"A new session goes on from a compacted copy of its conversation. This one keeps its whole history and stays as it is. Both work in the same folder and branch, so two agents can overwrite each other's changes: let one finish before the other edits.",
		"Fork", func() { a.forkSession(s) })
}

func (a *App) forkSession(s proto.SessionInfo) {
	cl := a.client
	if cl == nil {
		return
	}
	a.notify("Forking “" + a.sessionName(s) + "”: compacting a copy of its conversation…")
	go func() {
		info, err := cl.ForkSession(s.ID)
		a.onMain(func() {
			if err != nil {
				a.status = client.Explain(err)
				return
			}
			a.upsert(info)
			a.notify("Forked as “" + a.sessionName(info) + "”")
			// Compacting takes a while: only if they're still where they asked.
			if !a.goaOn() && a.selected == s.ID && a.page == "" {
				a.selectSession(info)
			}
		})
	}()
}

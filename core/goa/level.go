// Package goa is what GOD of Agents (docs/goa.md) needs that isn't the
// daemon's: how far each agent holds GOA to delegating, which of its tool
// calls are enowX's own, which of its workers' calls are the user's to
// answer, and its task board on disk.
package goa

import (
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

// LevelOf is how far agent id holds GOA to delegating on this host, and
// why: its own hooks refuse the rest (enforced), it starts with enowX's
// tools only (limited), or GOA is only asked and watched.
func LevelOf(id string) (level, why string) {
	hooks := agent.HookSupport(id)[proto.HookToolBefore] == proto.HookInAgent
	limits := agent.GOALimitsTools(id)
	switch {
	case agent.IsOpenCode(id):
		return proto.GOAEnforced, "its config offers it enowX's tools, reading, and editing its own folder only, and refuses the rest"
	case hooks && limits:
		return proto.GOAEnforced, "its hooks refuse every tool but enowX's and those reading and editing files, and it starts with those only"
	case hooks:
		return proto.GOAEnforced, "its hooks refuse every tool but enowX's before it runs"
	case limits:
		return proto.GOALimited, "it starts with enowX's tools and those reading and editing files only"
	}
	return proto.GOAWatched, "it can't be held to enowX's tools: what it runs itself and the files it changes in projects are flagged to you, to revert"
}

// provenAgents are those run for real as GOA and seen to hold: a model pressed to work
// by itself got nowhere. The others may follow by the same test.
var provenAgents = map[string]bool{"claude": true, "claude-agent-teams": true, "codex": true, "opencode": true, "opencode2": true, agent.PlatformID: true}

// NotProven says why GOA isn't offered on agent id, "" when it is.
func NotProven(id string) string {
	if provenAgents[id] {
		return ""
	}
	return "not tested yet to hold GOD of Agents to delegating: it runs on Claude Code, Codex and OpenCode"
}

package agent

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/enowx/enowx/proto"
)

// EnvBin names the variable holding the program the agents' hooks run, as
// "$ENOWX_BIN hook …". The daemon sets it in every session, so a command
// saved for a restart keeps working when enowX moves or updates.
const EnvBin = "ENOWX_BIN"

// hookCommand is the shell command a hook runs to report state. Outside
// enowX (no ENOWX_BIN) it does nothing.
func hookCommand(state string) string {
	return `[ -z "$` + EnvBin + `" ] || "$` + EnvBin + `" hook ` + state
}

// eventCommand is the shell command a hook runs to hand the daemon event
// (proto.Hook*), the agent's JSON on its stdin, with the state it means
// ("" for none). Its exit code and output are the user's hooks' answer
// (see daemon.Hook).
func eventCommand(event, state string) string {
	cmd := `[ -z "$` + EnvBin + `" ] || "$` + EnvBin + `" hook --event ` + event
	if state != "" {
		cmd += " --state " + state
	}
	return cmd
}

// HookArgs returns the arguments that make agent id report its state
// (proto.State*) and hand the daemon its events through its own hooks, or
// nil when enowX has none for it. They add to the user's configuration
// and replace none of it. See docs/agents.md and docs/hooks.md.
func HookArgs(id string) []string {
	id = Base(id)
	if !hooksSupported() {
		return nil
	}
	switch id {
	case "claude":
		return claudeHookArgs()
	case "codex":
		return codexHookArgs(codexNotify())
	}
	return nil
}

// agentHook is one of an agent's hook handlers, as Claude Code and Codex
// take them.
type agentHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// agentMatcher is a group of handlers for the events matcher matches.
type agentMatcher struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []agentHook `json:"hooks"`
}

// waitTimeout is how long an agent waits for the user's hooks on an event
// they can answer (refuse, add context): the daemon bounds each hook.
const waitTimeout = 300

// on is a group running event's command for what match matches.
func on(match, event, state string, timeout int) agentMatcher {
	return agentMatcher{Matcher: match, Hooks: []agentHook{{Type: "command", Command: eventCommand(event, state), Timeout: timeout}}}
}

// claudeHookArgs passes Claude Code hooks in --settings, which it merges
// with the user's settings: a prompt sent and a tool run mean working,
// a permission or a question asked means waiting, the end of a turn done;
// each hands the daemon its event.
func claudeHookArgs() []string {
	settings := map[string]any{"hooks": map[string][]agentMatcher{
		"SessionStart":       {on("", proto.HookSessionStart, "", waitTimeout)},
		"UserPromptSubmit":   {on("", proto.HookPromptStart, proto.StateWorking, waitTimeout)},
		"PreToolUse":         {on("", proto.HookToolBefore, "", waitTimeout)},
		"PostToolUse":        {on("", proto.HookToolAfter, proto.StateWorking, waitTimeout)},
		"PostToolUseFailure": {on("", proto.HookToolAfter, proto.StateWorking, waitTimeout)},
		"PermissionRequest":  {on("", proto.HookPermission, "", waitTimeout)},
		"Notification":       {on("permission_prompt|elicitation_dialog", proto.HookWaiting, proto.StateWaiting, waitTimeout)},
		"Stop":               {on("", proto.HookTurnDone, proto.StateDone, waitTimeout)},
		"PreCompact":         {on("", proto.HookCompact, "", waitTimeout)},
		"SessionEnd":         {on("", proto.HookSessionEnd, "", 5)},
	}}
	return []string{"--settings", marshal(settings)}
}

// codexEvents are Codex's hook events enowX listens to: its name, the
// label its trust is kept under, the event and state it means, and how
// long it may take.
var codexEvents = []struct {
	name, label, event, state string
	timeout                   int
}{
	{"SessionStart", "session_start", proto.HookSessionStart, "", waitTimeout},
	{"UserPromptSubmit", "user_prompt_submit", proto.HookPromptStart, proto.StateWorking, waitTimeout},
	{"PreToolUse", "pre_tool_use", proto.HookToolBefore, "", waitTimeout},
	{"PostToolUse", "post_tool_use", proto.HookToolAfter, proto.StateWorking, waitTimeout},
	{"PermissionRequest", "permission_request", proto.HookPermission, proto.StateWaiting, waitTimeout},
	{"Stop", "stop", proto.HookTurnDone, "", waitTimeout},
	{"PreCompact", "pre_compact", proto.HookCompact, "", waitTimeout},
	{"SessionEnd", "session_end", proto.HookSessionEnd, "", 3},
}

// JudgeApprovesByHook says a PreToolUse hook can spare agent id's user its
// permission prompt. Codex's can't: it takes no plain allow, and an allow with
// updatedInput only rewrites the call, which then goes through its approval as
// any other (codex-rs/core/src/tools/registry.rs, as of 0.166.0). Its
// PermissionRequest hook is what answers its prompts.
func JudgeApprovesByHook(id string) bool { return Base(id) != "codex" }

// codexOn builds the same definition for the command line and its trust hash.
func codexOn(event, state string, timeout int) agentMatcher {
	group := on("", event, state, timeout)
	group.Hooks[0].Command += " --agent=codex"
	return group
}

// codexHookArgs sets Codex's hooks, each handing the daemon its event,
// and its notify, which it runs at the end of each turn with the turn as
// a last argument, to report done and then run the user's own notify
// command, if any, as Codex would have. Codex runs a hook only once
// trusted: TrustCodexHooks trusts these.
func codexHookArgs(user []string) []string {
	script := hookCommand(proto.StateDone) + `; [ $# -lt 2 ] || exec "$@"`
	argv := append([]string{"/bin/sh", "-c", script, "enowx-notify"}, user...)
	// A JSON array of strings is a TOML array too.
	args := []string{"-c", "notify=" + marshal(argv)}
	for _, e := range codexEvents {
		args = append(args, "-c", "hooks."+e.name+"="+tomlInline([]agentMatcher{codexOn(e.event, e.state, e.timeout)}))
	}
	return args
}

// codexNotify is the notify command of the user's Codex configuration.
func codexNotify() []string {
	var cfg struct {
		Notify []string `toml:"notify"`
	}
	if _, err := toml.DecodeFile(filepath.Join(codexHome(), "config.toml"), &cfg); err != nil {
		return nil
	}
	return cfg.Notify
}

// marshal is JSON without HTML escapes, which TOML would keep literally.
func marshal(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSpace(b.String())
}

// outsideEvents are the events the daemon sees of any agent, after the
// fact: its program starting, working, ending (with an error, or
// hibernated) and waking, a prompt typed, a turn ending as its screen
// goes quiet, files changed, Git, and the host's own.
var outsideEvents = []string{proto.HookSessionStart, proto.HookPromptStart, proto.HookWorking, proto.HookTurnDone, proto.HookFileChanged,
	proto.HookCommit, proto.HookPush, proto.HookMerge, proto.HookConflict, proto.HookError, proto.HookHibernate, proto.HookWake,
	proto.HookSessionEnd, proto.HookStalled, proto.HookPort, proto.HookSync, proto.HookNotify, proto.HookSchedule, proto.HookFallback}

// HookSupport is how agent id gets each event (proto.HookInAgent,
// proto.HookOutside); an event missing it doesn't get.
func HookSupport(id string) map[string]string {
	out := map[string]string{}
	for _, e := range outsideEvents {
		out[e] = proto.HookOutside
	}
	var in []string
	if len(HookArgs(id)) > 0 {
		switch Base(id) {
		case "claude":
			in = []string{proto.HookSessionStart, proto.HookPromptStart, proto.HookToolBefore, proto.HookToolAfter, proto.HookPermission,
				proto.HookWaiting, proto.HookCompact, proto.HookTurnDone, proto.HookSessionEnd}
		case "codex":
			for _, e := range codexEvents {
				in = append(in, e.event)
			}
		}
	}
	for _, e := range in {
		out[e] = proto.HookInAgent
	}
	if len(in) > 0 {
		// Told by the agent's tool and permission events.
		out[proto.HookGitBefore], out[proto.HookAnswered] = proto.HookInAgent, proto.HookInAgent
	}
	return out
}

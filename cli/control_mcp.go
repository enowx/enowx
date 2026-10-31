package cli

import (
	"maps"
	"slices"
)

// Control's commands as MCP tools, each on any host (host: as the app
// names it; this one when left out).

var hostProp = mcpProp{"string", "The host, as the app names it (sessions_everywhere lists them); this one when left out. Other hosts need the enowX app open"}

var yesProp = mcpProp{"boolean", "true, only when the user asked for it: it can't be undone (from an agent the user also says yes in the app)"}

func idProps(more map[string]mcpProp) map[string]mcpProp {
	out := map[string]mcpProp{"id": {"string", "The session: its ID, the start of one, or its name"}, "host": hostProp}
	maps.Copy(out, more)
	return out
}

// sessionArgs are cmd ID, then the flags a's keys name.
func sessionArgs(cmd string, a map[string]any, flags ...string) []string {
	args := []string{cmd, str(a, "id")}
	for _, f := range flags {
		args = withFlag(args, a, f, f)
	}
	return withFlag(args, a, "host", "host")
}

var controlTools = []mcpTool{
	{name: "session_interrupt", description: "Stop what a session's agent is doing, as its user would (Escape, Ctrl+C); it then waits for a prompt.",
		props: idProps(nil), required: []string{"id"},
		args: func(a map[string]any) []string { return sessionArgs("interrupt", a) }},
	{name: "session_stop", description: "End a session's agent now, even mid-work, keeping the session: session_continue brings it back on its conversation.",
		props: idProps(nil), required: []string{"id"},
		args: func(a map[string]any) []string { return sessionArgs("stop", a) }},
	{name: "session_continue", description: "Start an ended or hibernated session's agent again, on its conversation.",
		props: idProps(nil), required: []string{"id"},
		args: func(a map[string]any) []string { return sessionArgs("continue", a) }},
	{name: "session_rename", description: "Rename a session, as the user sees it in enowX.",
		props: idProps(map[string]mcpProp{"name": {"string", "Its new name"}}), required: []string{"id", "name"},
		args: func(a map[string]any) []string { return append(sessionArgs("rename", a), str(a, "name")) }},
	{name: "session_close", description: "Close a session: its terminal, tabs and what runs in them end, for good. Only when the user asked; needs yes, and the user confirms in the app.",
		props: idProps(map[string]mcpProp{"yes": yesProp}), required: []string{"id", "yes"},
		args: func(a map[string]any) []string { return sessionArgs("close", a, "yes") }},
	{name: "session_chat", description: "A session's last messages (40 by default), as its agent recorded them: what it was asked and answered, the tools it used.",
		props: idProps(map[string]mcpProp{"last": {"number", "How many of its last messages"}}), required: []string{"id"},
		args: func(a map[string]any) []string { return sessionArgs("chat", a, "last") }},
	{name: "session_ask", description: "What a session's agent waits for: the tool it asks to use (and what it would do) or its questions, and the answers it takes.",
		props: idProps(nil), required: []string{"id"},
		args: func(a map[string]any) []string { return sessionArgs("answer", a) }},
	{name: "session_answer", description: "Answer what a session's agent asks (session_ask first): allow (once), always, deny, or a question's option number.",
		props: idProps(map[string]mcpProp{"answer": {"string", "allow, always, deny, or an option's number"}}), required: []string{"id", "answer"},
		args: func(a map[string]any) []string { return append(sessionArgs("answer", a), str(a, "answer")) }},
	{name: "session_switch", description: "Change a running session's model, reasoning effort, permission mode or account, as its user would in the agent.",
		props: idProps(map[string]mcpProp{"model": {"string", "The agent's name for the model"}, "effort": {"string", "low, medium, high, xhigh, max"},
			"mode": {"string", "The agent's name for the mode (Claude Code: default, acceptEdits, plan, auto, bypassPermissions)"}, "account": {"string", "An account of the agent (accounts list)"}}),
		required: []string{"id"},
		args:     func(a map[string]any) []string { return sessionArgs("switch", a, "model", "effort", "mode", "account") }},
	{name: "session_changes", description: "A session's branch against its base (ahead, behind, uncommitted, unpushed), or with diff what it changed.",
		props:    idProps(map[string]mcpProp{"diff": {"boolean", "The diff, uncommitted changes included"}, "files": {"boolean", "With diff: the files only"}, "base": {"string", "Compare with this branch"}}),
		required: []string{"id"},
		args: func(a map[string]any) []string {
			args := sessionArgs("changes", a, "base", "files")
			if str(a, "diff") == "true" {
				args = append(args, "diff")
			}
			return args
		}},
	{name: "session_commit", description: "Commit every change in a session's folder.",
		props: idProps(map[string]mcpProp{"message": {"string", "The commit's message"}}), required: []string{"id", "message"},
		args: func(a map[string]any) []string {
			return append(sessionArgs("changes", a), "commit", "-m", str(a, "message"))
		}},
	{name: "session_push", description: "Push a session's branch. Only when the user asked; needs yes, and the user confirms in the app.",
		props: idProps(map[string]mcpProp{"yes": yesProp}), required: []string{"id", "yes"},
		args: func(a map[string]any) []string { return append(sessionArgs("changes", a, "yes"), "push") }},
	{name: "session_pr", description: "Push a session's branch and open a pull request into its base (gh); prints its address. Only when the user asked; needs yes, and the user confirms in the app.",
		props:    idProps(map[string]mcpProp{"title": {"string", "Its title (default: from the commits)"}, "body": {"string", "Its description"}, "base": {"string", "The branch it goes into"}, "yes": yesProp}),
		required: []string{"id", "yes"},
		args: func(a map[string]any) []string {
			return append(sessionArgs("changes", a, "title", "body", "base", "yes"), "pr")
		}},
	{name: "session_merge", description: "Merge a session's branch into its base, in the project folder. Only when the user asked; needs yes, and the user confirms in the app.",
		props: idProps(map[string]mcpProp{"base": {"string", "The branch it goes into"}, "yes": yesProp}), required: []string{"id", "yes"},
		args: func(a map[string]any) []string { return append(sessionArgs("changes", a, "base", "yes"), "merge") }},
	{name: "session_discard", description: "End the sessions in a session's worktree and remove it, its branch too with delete_branch. Only when the user asked; needs yes, and the user confirms in the app.",
		props: idProps(map[string]mcpProp{"delete_branch": {"boolean", "Delete its branch too"}, "yes": yesProp}), required: []string{"id", "yes"},
		args: func(a map[string]any) []string {
			return append(withFlag(sessionArgs("changes", a, "yes"), a, "delete_branch", "delete-branch"), "discard")
		}},
	{name: "host_settings", description: "A host's settings (agents at once, hibernating idle agents, its terminals' shell), or one changed: key max-agents, hibernate-after (minutes), auto-hibernate (on/off), shell.",
		props: map[string]mcpProp{"key": {"string", "The setting to change"}, "value": {"string", "Its new value"}, "host": hostProp},
		args: func(a map[string]any) []string {
			args := []string{"settings"}
			if k := str(a, "key"); k != "" {
				args = append(args, "set", k, str(a, "value"))
			}
			return withFlag(args, a, "host", "host")
		}},
	{name: "host_env", description: "The variables every session on a host gets (values masked), or one set (a secret: the user says yes in the app) or unset.",
		props: map[string]mcpProp{"name": {"string", "The variable to set or unset"}, "value": {"string", "Its value; empty unsets it"}, "host": hostProp},
		args: func(a map[string]any) []string {
			args := []string{"settings", "env"}
			switch n, v := str(a, "name"), str(a, "value"); {
			case n != "" && v != "":
				args = append(args, "set", n, v)
			case n != "":
				args = append(args, "unset", n)
			}
			return withFlag(args, a, "host", "host")
		}},
	{name: "app_settings", description: "The enowX app's own settings (New session's agent, worktree or project folder, skipping permissions, time budget, terminal font and size, theme, notifications, editor…) with what each takes, or one changed. Needs the app open.",
		props: map[string]mcpProp{"key": {"string", "The setting to change"}, "value": {"string", "Its new value"}},
		args: func(a map[string]any) []string {
			if k := str(a, "key"); k != "" {
				return []string{"settings", "app", "set", k, str(a, "value")}
			}
			return []string{"settings", "app"}
		}},
	{name: "project_settings", description: "A project's settings (.enowx.toml, committed with it): the files git ignores that new worktrees get, and the script that readies one; or changed.",
		props: map[string]mcpProp{"project": {"string", "The project's folder (whole, on another host)"}, "copy": {"string", "Files or globs, comma separated (\"\" none)"},
			"setup": {"string", "The setup script (\"\" none)"}, "host": hostProp},
		args: func(a map[string]any) []string {
			args := []string{"project", "settings", str(a, "project")}
			for _, k := range []string{"copy", "setup"} {
				if v, ok := a[k]; ok && v != nil {
					args = append(args, "--"+k, str(a, k))
				}
			}
			return withFlag(args, a, "host", "host")
		}},
	{name: "control_log", description: "The requests sessions made through control on a host lately: from which session, to which host, what, and how it ended.",
		props: map[string]mcpProp{"host": hostProp},
		args:  func(a map[string]any) []string { return withFlag([]string{"control", "log"}, a, "host", "host") }},
}

// onAnyHost are tools also on another host: a host argument, passed as
// --host after prefix and their own arguments, for runner.command; those
// named in except stay as they are, but for prefix.
func onAnyHost(prefix []string, tools []mcpTool, except ...string) []mcpTool {
	out := slices.Clone(tools)
	for i, t := range out {
		if slices.Contains(except, t.name) {
			args := t.args
			t.args = func(a map[string]any) []string { return append(slices.Clone(prefix), args(a)...) }
			out[i] = t
			continue
		}
		props := maps.Clone(t.props)
		if props == nil {
			props = map[string]mcpProp{}
		}
		props["host"] = hostProp
		args := t.args
		if t.name == "session_spawn" {
			// On another host it needs the project's folder there.
			props["project"] = mcpProp{"string", "The project's folder (needed on another host: whole)"}
			props["name"] = mcpProp{"string", "The session's name"}
			props["model"] = mcpProp{"string", "The model to start on"}
			props["effort"] = mcpProp{"string", "The reasoning effort to start with"}
			spawn := t.args
			args = func(a map[string]any) []string {
				orig := spawn(a)
				head, prompt := slices.Clone(orig[:len(orig)-1]), orig[len(orig)-1]
				for _, k := range []string{"project", "name", "model", "effort"} {
					head = withFlag(head, a, k, k)
				}
				return append(head, prompt)
			}
		}
		t.props = props
		t.args = func(a map[string]any) []string {
			return withFlag(append(slices.Clone(prefix), args(a)...), a, "host", "host")
		}
		out[i] = t
	}
	return out
}

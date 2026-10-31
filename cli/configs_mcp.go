package cli

import "fmt"

// configTools are the configs as MCP tools.
var configTools = []mcpTool{
	{name: "configs_list", description: "Each agent's configs on this host: the official one (the agent as installed), its other accounts, its model APIs (address, models, the vault credential holding the key), their state at a limit.",
		props: map[string]mcpProp{"agent": {"string", "Only this agent's (claude, codex, …)"}},
		args:  func(a map[string]any) []string { return withFlag([]string{"list"}, a, "agent", "agent") }},
	{name: "configs_add", description: "Save a config of an agent when the user asks: a model API (url, key_cred, models) or another account (account=true). The agent's own settings stay untouched; sessions pick it as they start.",
		props:    configProps(map[string]mcpProp{"account": {"boolean", "Another sign-in of the agent's, signed in from the app"}}),
		required: []string{"agent", "name"},
		args:     func(a map[string]any) []string { return configArgs("add", a) }},
	{name: "configs_edit", description: "Change an agent's config when the user asks: only what is given changes; new_name renames it.",
		props:    configProps(map[string]mcpProp{"new_name": {"string", "Its new name"}}),
		required: []string{"agent", "name"},
		args:     func(a map[string]any) []string { return configArgs("edit", a) }},
	{name: "configs_remove", description: "Remove an agent's config when the user asks (an account's sign-in on this host goes with it).",
		props: map[string]mcpProp{"agent": {"string", "The agent's id"}, "name": {"string", "The config's name"}}, required: []string{"agent", "name"},
		args: func(a map[string]any) []string { return []string{"rm", str(a, "agent"), str(a, "name")} }},
}

func configProps(more map[string]mcpProp) map[string]mcpProp {
	out := map[string]mcpProp{"agent": {"string", "The agent's id: claude, codex, opencode…"}, "name": {"string", "The config's name"},
		"url": {"string", "A model API's base URL"}, "key_cred": {"string", "The vault's credential holding its key (creds_list)"},
		"models":      {"array", "Its models, the first the default, each ID[,vision][,context=TOKENS][,output=TOKENS][,effort=E]"},
		"small_model": {"string", "A model for quick work, where the agent has one"}, "protocol": {"string", "anthropic, openai, openai-responses or gemini"},
		"effort": {"string", "The agent's reasoning effort"}, "env": {"array", "NAME=VALUE added to its sessions"}, "color": {"string", "Its badge's color, #RRGGBB"}}
	for k, v := range more {
		out[k] = v
	}
	return out
}

func configArgs(cmd string, a map[string]any) []string {
	args := []string{cmd, str(a, "agent"), str(a, "name")}
	for _, k := range [][2]string{{"url", "url"}, {"key_cred", "key-cred"}, {"small_model", "small-model"}, {"protocol", "protocol"},
		{"effort", "effort"}, {"color", "color"}, {"new_name", "name"}, {"account", "account"}} {
		args = withFlag(args, a, k[0], k[1])
	}
	for _, list := range [][2]string{{"models", "model"}, {"env", "env"}} {
		items, _ := a[list[0]].([]any)
		for _, it := range items {
			args = append(args, "--"+list[1], fmt.Sprint(it))
		}
	}
	return args
}

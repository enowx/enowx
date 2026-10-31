package tokens

import "github.com/enowx/enowx/proto"

// adapters read the agents whose records hold their tokens.
var adapters = []adapter{
	{agent: "claude", files: claudeLike([]string{"CLAUDE_CONFIG_DIR"}, ".claude"), read: readClaudeAs("claude")},
	{agent: "openclaude", files: claudeLike([]string{"OPENCLAUDE_CONFIG_DIR"}, ".openclaude"), read: readClaudeAs("openclaude")},
	{agent: "zcode", files: claudeLike(nil, ".zcode"), read: readClaudeAs("zcode")},
	{agent: "qoder", files: claudeLike([]string{"QODER_CONFIG_DIR"}, ".qoder"), read: readClaudeAs("qoder")},
	{agent: "codex", files: codexFiles, read: readCodex},
	{agent: "pi", files: piFiles, read: readPi},
	{agent: "omp", files: ompFiles, read: readPi},
	{agent: "prime-agent", files: primeFiles, read: readPi},
	{agent: "openclaw", files: openclawFiles, read: readOpenClaw},
	{agent: "opencode", files: opencodeStore("OPENCODE_DB", "opencode"), read: readOpenCode},
	{agent: "opencode2", files: opencodeStore("", "opencode2"), read: readOpenCode},
	{agent: "kilo", files: opencodeStore("KILO_DB", "kilo"), read: readOpenCode},
	{agent: "mimo-code", files: opencodeStore("", "mimocode"), read: readOpenCode},
	{agent: "gemini", files: geminiFiles, read: readGemini},
	{agent: "qwen-code", files: qwenFiles, read: readQwen},
	{agent: "goose", files: gooseFiles, read: readGoose},
	{agent: "copilot", files: oneFile(".copilot", "session-store.db"), read: readCopilot},
	{agent: "devin", files: oneFile(".local", "share", "devin", "cli", "sessions.db"), read: readDevin},
	{agent: "grok", files: oneFile(".grok", "grok.db"), read: readGrokDB},
	{agent: "grok", files: oneFile(".grok", "logs", "unified.jsonl"), read: readEventLines("grok", "shell.turn.inference_done", grokLogCall)},
	{agent: "cline", files: clineFiles, read: readCline},
	{agent: "continue", files: continueFiles, read: readContinue},
	{agent: "amp", files: ampFiles, read: readAmp},
	{agent: "aug", files: auggieFiles, read: readAuggie},
	{agent: "kiro", files: kiroFiles, read: readKiro},
	{agent: "jcode", files: jcodeFiles, read: readJcode},
	{agent: "kimi", files: kimiFiles, read: readKimi},
	{agent: "mistral-vibe", files: vibeFiles, read: readEventLines("vibe", "completion_succeeded", vibeCall)},
	{agent: "muse", files: museFiles, read: readEventLines("muse", "model_completed", museCall)},
	{agent: "command-code", files: commandCodeFiles, read: readCommandCode},
	{agent: "dsh", files: dshFiles, read: readDSH},
	{agent: "codebuddy", files: codebuddyFiles, read: readCodeBuddy},
	{agent: "aider", files: aiderFiles, read: readAider},
}

const (
	recordsOfOpenCode = "each model call (step): input, output, reasoning, cache read, cache write, cost; provider, model"
	recordsOfPi       = "each message: input, output, cache read, cache write, cost; provider, model"
	recordsOfClaude   = "each message: input, cache read, cache write, output, thinking; model"
	notStandard       = "none: it talks to its maker's own service, not a model API whose answers can be read"
)

// coverage is how each agent enowX knows is tracked, by preset ID: its
// layer, what its records hold, where, and whether that was checked
// against a real install (else its documentation and source only).
var coverage = []proto.TokenCoverage{
	{Agent: "claude", Layer: proto.TokenLayerRecords, Checked: true, Records: recordsOfClaude, Where: "~/.claude/projects/<folder>/*.jsonl"},
	{Agent: "claude-agent-teams", Layer: proto.TokenLayerRecords, Checked: true, Records: "Claude Code's, its teammates' included", Where: "~/.claude/projects/<folder>/*.jsonl"},
	{Agent: "openclaude", Layer: proto.TokenLayerRecords, Records: recordsOfClaude, Where: "~/.openclaude/projects/<folder>/*.jsonl"},
	{Agent: "zcode", Layer: proto.TokenLayerRecords, Records: "Claude Code's shape, its usage when it writes one (unconfirmed)", Where: "~/.zcode/projects/<folder>/*.jsonl"},
	{Agent: "qoder", Layer: proto.TokenLayerRecords, Records: "Claude Code's shape, its usage when it writes one (undocumented)", Where: "~/.qoder/projects/<folder>/*.jsonl"},
	{Agent: "codex", Layer: proto.TokenLayerRecords, Checked: true, Records: "each call (token_count): input, cached, cache write, output, reasoning; model; plan", Where: "~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl"},
	{Agent: "pi", Layer: proto.TokenLayerRecords, Records: recordsOfPi, Where: "~/.pi/agent/sessions/<folder>/*.jsonl"},
	{Agent: "omp", Layer: proto.TokenLayerRecords, Records: recordsOfPi, Where: "~/.omp/agent/sessions/<folder>/*.jsonl"},
	{Agent: "prime-agent", Layer: proto.TokenLayerRecords, Records: recordsOfPi, Where: "~/.prime/agent/sessions/*.jsonl"},
	{Agent: "openclaw", Layer: proto.TokenLayerRecords, Records: recordsOfPi, Where: "~/.openclaw/agents/<agent>/agent/openclaw-agent.sqlite (before: sessions/*.jsonl)"},
	{Agent: "opencode", Layer: proto.TokenLayerRecords, Checked: true, Records: recordsOfOpenCode, Where: "~/.local/share/opencode/opencode.db (before: storage/part/*/*.json)"},
	{Agent: "opencode2", Layer: proto.TokenLayerRecords, Records: "OpenCode's, if it keeps them as OpenCode does (unconfirmed)", Where: "~/.local/share/opencode2/*.db"},
	{Agent: "kilo", Layer: proto.TokenLayerRecords, Records: recordsOfOpenCode, Where: "~/.local/share/kilo/kilo.db"},
	{Agent: "mimo-code", Layer: proto.TokenLayerRecords, Records: recordsOfOpenCode, Where: "~/.local/share/mimocode/mimocode.db"},
	{Agent: "gemini", Layer: proto.TokenLayerRecords, Records: "each message: input, cached, output, thoughts, tool; model", Where: "~/.gemini/tmp/<project>/chats/session-*.jsonl"},
	{Agent: "qwen-code", Layer: proto.TokenLayerRecords, Records: "each message: prompt, cached, candidates, thoughts; model", Where: "~/.qwen/projects/<folder>/chats/*.jsonl"},
	{Agent: "goose", Layer: proto.TokenLayerRecords, Records: "each call (usage_ledger): input, output, cache read, cache write, cost; model", Where: "~/.local/share/goose/sessions/sessions.db"},
	{Agent: "copilot", Layer: proto.TokenLayerRecords, Records: "each call: input, output, cache read, cache write, reasoning; model", Where: "~/.copilot/session-store.db"},
	{Agent: "devin", Layer: proto.TokenLayerRecords, Records: "each message: input, output, cache read, cache write; model", Where: "~/.local/share/devin/cli/sessions.db"},
	{Agent: "grok", Layer: proto.TokenLayerRecords, Records: "each call: input, output, cost (Grok CLI); prompt, cached, completion, reasoning (Grok Build's log)", Where: "~/.grok/grok.db, ~/.grok/logs/unified.jsonl"},
	{Agent: "cline", Layer: proto.TokenLayerRecords, Records: "each message: input, output, cache read, cache write, cost; model", Where: "~/.cline/data/sessions/<id>/<id>.messages.json (before: tasks/<id>/ui_messages.json)"},
	{Agent: "continue", Layer: proto.TokenLayerRecords, Records: "each answer: prompt, completion, cached, cache write (no time: the session's last change)", Where: "~/.continue/sessions/<id>.json"},
	{Agent: "amp", Layer: proto.TokenLayerRecords, Records: "older builds only: each call's input, output, cache read, cache write; threads are now on Amp's servers", Where: "~/.local/share/amp/threads/T-*.json"},
	{Agent: "aug", Layer: proto.TokenLayerRecords, Records: "each finished turn: input, output, cache read, cache write; model", Where: "~/.augment/sessions/<id>.json"},
	{Agent: "kiro", Layer: proto.TokenLayerRecords, Records: "each turn: input, output, cache read, cache write, when Kiro counts them (often zeros: it meters credits)", Where: "~/.kiro/sessions/cli/<id>.json"},
	{Agent: "jcode", Layer: proto.TokenLayerRecords, Records: "each message: input, output, cache read, cache write, reasoning; model", Where: "~/.jcode/sessions/session_*.json"},
	{Agent: "kimi", Layer: proto.TokenLayerRecords, Records: "each answer: input, output, cache read, cache write (no model)", Where: "~/.kimi/sessions/<folder>/<id>/wire.jsonl"},
	{Agent: "mistral-vibe", Layer: proto.TokenLayerRecords, Records: "Vibe 3: each call's input, cached, cache write, output, reasoning (Vibe 2 keeps only a session's totals)", Where: "~/.vibe/logs/session/unified/<id>/journal/*.jsonl"},
	{Agent: "muse", Layer: proto.TokenLayerRecords, Records: "model_completed events' usage, where its version writes them", Where: "~/.local/share/muse/sessions/YYYY/MM/DD/<id>/session.jsonl"},
	{Agent: "command-code", Layer: proto.TokenLayerRecords, Records: "each message: input, output, cache read, cache write, cost; model", Where: "~/.commandcode/projects/<folder>/*.jsonl"},
	{Agent: "dsh", Layer: proto.TokenLayerRecords, Records: "each answer: input, output, cache read; model", Where: "~/.dsh/sessions/<folder>/<id>/session.jsonl(.zstd)"},
	{Agent: "codebuddy", Layer: proto.TokenLayerRecords, Checked: true, Records: "each message: input, cache read, cache write, output, reasoning; model", Where: "~/.codebuddy/projects/<folder>/*.jsonl"},
	{Agent: "aider", Layer: proto.TokenLayerRecords, Records: "each answer's \"Tokens: … sent, … received\" line, rounded as printed, and its cost", Where: "<project>/.aider.chat.history.md"},

	{Agent: "crush", Layer: proto.TokenLayerTraffic, Records: "a session's last call only (and its cost so far): its traffic is metered", Where: "<project>/.crush/crush.db"},
	{Agent: "hermes", Layer: proto.TokenLayerTraffic, Records: "a session's totals per model only: its traffic is metered", Where: "~/.hermes/state.db"},
	{Agent: "droid", Layer: proto.TokenLayerTraffic, Records: "a session's totals only: its traffic is metered", Where: "~/.factory/sessions/*.settings.json"},
	{Agent: "autohand", Layer: proto.TokenLayerTraffic, Records: "a session's totals only: its traffic is metered", Where: "~/.autohand/sessions/<id>/metadata.json"},
	{Agent: "rovo", Layer: proto.TokenLayerTraffic, Records: "undocumented: its traffic is metered when it speaks a model API", Where: "~/.rovodev/sessions/<id>/"},
	{Agent: "ante", Layer: proto.TokenLayerTraffic, Records: "none known: its traffic is metered when it speaks a model API"},
	{Agent: "qoder-cn", Layer: proto.TokenLayerTraffic, Records: "where it keeps its sessions is undocumented: its traffic is metered"},

	{Agent: "cursor", Layer: proto.TokenLayerNone, Records: notStandard + " (usage is only in its account's dashboard)"},
	{Agent: "trae", Layer: proto.TokenLayerNone, Records: notStandard + " (usage is only in its account)"},
	{Agent: "antigravity", Layer: proto.TokenLayerNone, Records: notStandard + " (its records are protobuf)"},
	{Agent: "codebuff", Layer: proto.TokenLayerNone, Records: notStandard + " (its records hold credits, not tokens)"},
	{Agent: "freebuff", Layer: proto.TokenLayerNone, Records: notStandard},
}

// LayerOf is how agent's tokens are tracked: the traffic layer for an
// agent the table doesn't list.
func LayerOf(agent string) string {
	for _, c := range coverage {
		if c.Agent == agent {
			return c.Layer
		}
	}
	return proto.TokenLayerTraffic
}

// Coverage is the coverage of agents (preset IDs, in their order): those
// the table doesn't list are metered from their traffic.
func Coverage(agents []string) []proto.TokenCoverage {
	out := make([]proto.TokenCoverage, 0, len(agents))
	for _, id := range agents {
		c := proto.TokenCoverage{Agent: id, Layer: proto.TokenLayerTraffic,
			Records: "none known: its traffic is metered when it speaks a model API"}
		for _, k := range coverage {
			if k.Agent == id {
				c = k
			}
		}
		out = append(out, c)
	}
	return out
}

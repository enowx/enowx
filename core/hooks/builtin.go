package hooks

import "github.com/enowx/enowx/proto"

// TrimCommands finds the commands whose output is long: tests, builds,
// installs (a hook's Text).
const TrimCommands = `^\s*(npm|pnpm|yarn|bun)\s+(test|run\s+(test|build|lint)|install|ci)\b|^\s*go\s+(test|build|vet)\b|^\s*(make|cargo\s+(build|test)|pytest|mvn|gradle|\./gradlew)\b`

// Built-in hooks: what enowX does before a tool runs, its judge's uses on
// (docs/decide.md), when the user set no hook of the same action, which
// takes their place.
var (
	BuiltinJudge = proto.Hook{ID: "builtin-judge", Name: "enowX's judge", On: true, Event: proto.HookToolBefore,
		Action: proto.HookActionJudge, Tool: `^(Bash|shell|exec_command)$`}
	BuiltinTrim = proto.Hook{ID: "builtin-trim", Name: "enowX's trim", On: true, Event: proto.HookToolBefore,
		Action: proto.HookActionTrim, Text: TrimCommands}
)

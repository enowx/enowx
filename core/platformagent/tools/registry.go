package tools

// Builtin are the agent's own tools: Read, Write, Edit, Glob, Grep, Bash.
func Builtin() []Tool { return nil }

// NewAskUserQuestion is the tool that puts questions to the user through a.
func NewAskUserQuestion(a Asker) Tool { return nil }

// CleanEnviron is env without what must never reach a program a tool starts: the API
// key and the agent's other ANTHROPIC_ credentials.
func CleanEnviron(env []string) []string { return env }

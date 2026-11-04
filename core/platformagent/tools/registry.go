package tools

// Builtin are the agent's own tools, in the fixed order the API's cached prefix needs:
// Read, Write, Edit, Glob, Grep, Bash.
func Builtin() []Tool {
	return []Tool{readTool{}, writeTool{}, editTool{}, globTool{}, grepTool{}, bashTool{}}
}

// NewAskUserQuestion is the tool that puts questions to the user through a.
func NewAskUserQuestion(a Asker) Tool { return nil }

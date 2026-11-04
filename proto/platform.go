package proto

// Claude Platform's system prompt on a host, for the page of the agent's settings.
const (
	// MethodPlatformPrompt reads it: → PlatformPrompt.
	MethodPlatformPrompt = "platform.prompt"
	// MethodPlatformPromptSave keeps it, or with Reset goes back to the built-in one:
	// PlatformPrompt → PlatformPrompt. Sessions started after use it.
	MethodPlatformPromptSave = "platform.prompt.save"
)

// PlatformPrompt is the user's system prompt for Claude Platform on a host.
type PlatformPrompt struct {
	Text string `json:"text,omitempty"`
	// Custom says the host has one; otherwise sessions use the built-in prompt.
	Custom bool `json:"custom,omitempty"`
	// Path is the file that keeps it on the host, as it answers.
	Path string `json:"path,omitempty"`
	// TooLong says the file is over what can be edited here: no Text, but it can be reset
	// or written over.
	TooLong bool `json:"tooLong,omitempty"`
	// Reset, saving, removes it.
	Reset bool `json:"reset,omitempty"`
}

package client

import "github.com/enowx/enowx/proto"

// PlatformPrompt reads the host's system prompt for Claude Platform.
func (c *Client) PlatformPrompt() (proto.PlatformPrompt, error) {
	var out proto.PlatformPrompt
	err := c.Call(proto.MethodPlatformPrompt, nil, &out)
	return out, err
}

// SavePlatformPrompt keeps text as that prompt.
func (c *Client) SavePlatformPrompt(text string) (proto.PlatformPrompt, error) {
	var out proto.PlatformPrompt
	err := c.Call(proto.MethodPlatformPromptSave, proto.PlatformPrompt{Text: text}, &out)
	return out, err
}

// ResetPlatformPrompt removes it: sessions go back to the built-in one.
func (c *Client) ResetPlatformPrompt() (proto.PlatformPrompt, error) {
	var out proto.PlatformPrompt
	err := c.Call(proto.MethodPlatformPromptSave, proto.PlatformPrompt{Reset: true}, &out)
	return out, err
}

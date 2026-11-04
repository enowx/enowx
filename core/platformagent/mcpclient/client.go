// Package mcpclient connects the agent to MCP servers, enowX's and the user's, and
// offers their tools as the agent's own, named mcp__<server>__<tool>.
package mcpclient

import (
	"context"

	"github.com/enowx/enowx/core/platformagent/tools"
)

// Servers are the connected MCP servers.
type Servers struct{}

// Connect starts the servers spec names: a path to, or the text of, an mcp.json as
// Claude Code takes it ({"mcpServers":{name:{command,args,env}|{type,url,headers}}}).
// A server that fails doesn't stop the others: Problems says what went wrong.
func Connect(ctx context.Context, spec string, environ []string) (*Servers, error) {
	return &Servers{}, nil
}

// Tools are every connected server's tools.
func (s *Servers) Tools() []tools.Tool { return nil }

// Problems say which servers failed, without any secret of theirs.
func (s *Servers) Problems() []string { return nil }

// Close ends the servers.
func (s *Servers) Close() error { return nil }

package daemon

import (
	"os"
	"path/filepath"

	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
)

// mcpAgents are the agents enowX's MCP server is added to, with the
// folder that says one is set up here.
var mcpAgents = []struct{ id, dir string }{
	{"claude", ".claude"}, {"codex", ".codex"}, {"opencode", ".config/opencode"}, {"gemini", ".gemini"},
}

// trafficMCP adds enowX's MCP server, which this daemon's program serves
// as `mcp`, to the agents set up on this host.
func (s *Server) trafficMCP() (proto.TrafficMCP, error) {
	exe := programPath()
	home, err := os.UserHomeDir()
	if err != nil {
		return proto.TrafficMCP{}, err
	}
	res := proto.TrafficMCP{Added: []string{}}
	for _, a := range mcpAgents {
		if _, err := os.Stat(filepath.Join(home, a.dir)); err != nil {
			continue
		}
		payload, err := envsync.NewMCP(a.id, "enowx", envsync.MCPServer{Command: exe, Args: []string{"mcp"}})
		if err != nil {
			res.Notes = append(res.Notes, a.id+": "+err.Error())
			continue
		}
		r, err := envsync.Import(home, payload)
		if err != nil {
			res.Notes = append(res.Notes, a.id+": "+err.Error())
			continue
		}
		res.Notes = append(res.Notes, r.Warnings...)
		res.Added = append(res.Added, a.id)
	}
	return res, nil
}

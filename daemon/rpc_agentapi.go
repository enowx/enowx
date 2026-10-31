package daemon

import (
	"fmt"
	"os"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/agentapi"
	"github.com/enowx/enowx/proto"
)

var agentAPIHandlers = map[string]handler{
	proto.MethodAgentAPIList: bare(func(c *conn) (any, error) { return agentAPIList() }),
	proto.MethodAgentAPISet: with(func(c *conn, p proto.AgentAPISetParams) (any, error) {
		if agentapi.PerSession(p.Agent) {
			// Its official settings stay as installed: an API is a config.
			return nil, fmt.Errorf("agentapi: %s takes a model API as a config of its own now, its settings untouched: Settings → Agents, or enowx-app configs add %s --url …", agentName(p.Agent), p.Agent)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		a := p.API
		if a.APIKey == proto.AgentAPIKeepKey {
			a.APIKey = ""
			if cur, err := agentapi.Of(home, p.Agent); err == nil && cur.Current != nil {
				a.APIKey = cur.Current.APIKey
			}
		}
		api := agentapi.API{BaseURL: a.BaseURL, APIKey: a.APIKey, Model: a.Model, SmallModel: a.SmallModel, Effort: a.Effort, Protocol: a.Protocol}
		for _, m := range a.Models {
			api.Models = append(api.Models, agentapi.Model{ID: m.ID, Vision: m.Vision, Context: m.Context, Output: m.Output, Effort: m.Effort})
		}
		if err := agentapi.Set(home, p.Agent, api); err != nil {
			return nil, err
		}
		return agentAPIInfo(home, p.Agent), nil
	}),
	proto.MethodAgentAPIClear: with(func(c *conn, p proto.AgentAPIRef) (any, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		if err := agentapi.Clear(home, p.Agent); err != nil {
			return nil, err
		}
		return agentAPIInfo(home, p.Agent), nil
	}),
	proto.MethodAgentAPIRestore: with(func(c *conn, p proto.AgentAPIRestoreParams) (any, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		id := p.Backup
		if id == "" {
			bs, err := agentapi.Backups(home, p.Agent)
			if err != nil {
				return nil, err
			}
			for _, b := range bs {
				if b.Original {
					id = b.ID
				}
			}
		}
		if err := agentapi.Restore(home, p.Agent, id); err != nil {
			return nil, err
		}
		return agentAPIInfo(home, p.Agent), nil
	}),
}

func agentAPIList() ([]proto.AgentAPIInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var out []proto.AgentAPIInfo
	for _, id := range agentapi.Supported() {
		out = append(out, agentAPIInfo(home, id))
	}
	return out, nil
}

// agentAPIInfo is agent id's API, its key masked: it never leaves the
// host whole.
func agentAPIInfo(home, id string) proto.AgentAPIInfo {
	info := proto.AgentAPIInfo{Agent: id, Name: id, PerSession: agentapi.PerSession(id)}
	if p, ok := agent.Find(id); ok {
		info.Name, info.Installed = p.Name, p.Installed()
	}
	a, err := agentapi.Of(home, id)
	info.Protocols, info.Files, info.Note, info.Small, info.NoModel = a.Protocols, a.Files, a.Note, a.Small, a.NoModel
	caps := a.Caps
	info.Caps = proto.AgentAPICaps{Multi: caps.Multi, Vision: caps.Vision, Context: caps.Context, Output: caps.Output, Effort: caps.Effort, Efforts: caps.Efforts}
	if err != nil {
		info.Error = err.Error()
	}
	if c := a.Current; c != nil {
		info.Current = &proto.AgentAPI{BaseURL: c.BaseURL, APIKey: agentapi.MaskKey(c.APIKey), Model: c.Model, SmallModel: c.SmallModel, Effort: c.Effort, Protocol: c.Protocol}
		for _, m := range c.Models {
			info.Current.Models = append(info.Current.Models, proto.AgentAPIModel{ID: m.ID, Vision: m.Vision, Context: m.Context, Output: m.Output, Effort: m.Effort})
		}
	}
	bs, _ := agentapi.Backups(home, id)
	for _, b := range bs {
		info.Backups = append(info.Backups, proto.AgentAPIBackup{ID: b.ID, Time: b.Time, Original: b.Original, Note: b.Note})
	}
	return info
}

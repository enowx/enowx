package client

import "github.com/enowx/enowx/proto"

// RestartPlan says what replacing the daemon would stop now. A daemon too old to say is
// judged by its sessions, every terminal among them as it can't tell what runs there.
func (c *Client) RestartPlan() (proto.RestartPlan, error) {
	var plan proto.RestartPlan
	err := c.Call(proto.MethodDaemonRestartPlan, nil, &plan)
	if !IsUnknownMethod(err) {
		return plan, err
	}
	list, err := c.Sessions()
	if err != nil {
		return proto.RestartPlan{}, err
	}
	plan = proto.RestartPlan{Unknown: true}
	for _, s := range list {
		item := proto.RestartItem{Session: s.ID, Name: s.Name, What: s.Agent}
		switch {
		case s.Exited || s.Queued:
		case s.Agent == "" || s.Agent == "shell":
			item.What = "a terminal: this daemon can't say what runs in it"
			plan.Commands = append(plan.Commands, item)
		case (s.State == proto.StateWorking && !s.StateStale) || s.State == proto.StateWaiting:
			plan.Agents = append(plan.Agents, item)
		}
	}
	return plan, nil
}

package client

import "github.com/enowx/enowx/proto"

// Stats measures the daemon's host and sessions.
func (c *Client) Stats() (proto.DaemonStats, error) {
	var out proto.DaemonStats
	err := c.Call(proto.MethodDaemonStats, struct{}{}, &out)
	return out, err
}

// StopProcess stops a process the daemon's stats name as left running.
func (c *Client) StopProcess(p proto.LeftProcess) error {
	return c.Call(proto.MethodDaemonStopProcess, proto.StopProcessParams{PID: p.PID, Start: p.Start}, nil)
}

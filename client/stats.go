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

// CacheList says what the host's folder takes, and what can go of it.
func (c *Client) CacheList(p proto.CacheListParams) (proto.CacheReport, error) {
	var out proto.CacheReport
	err := c.Call(proto.MethodCacheList, p, &out)
	return out, err
}

// CacheClean takes away what p asks.
func (c *Client) CacheClean(p proto.CacheCleanParams) (proto.CacheCleanResult, error) {
	var out proto.CacheCleanResult
	err := c.Call(proto.MethodCacheClean, p, &out)
	return out, err
}

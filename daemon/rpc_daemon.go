package daemon

import (
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/proto"
)

var daemonHandlers = map[string]handler{
	proto.MethodDaemonInfo:    bare(func(c *conn) (any, error) { return c.srv.info(), nil }),
	proto.MethodDaemonStats:   bare(func(c *conn) (any, error) { return c.srv.stats(), nil }),
	proto.MethodOnboardingGet: bare(func(c *conn) (any, error) { return c.srv.onboarding(), nil }),
	proto.MethodOnboardingSet: with(func(c *conn, p proto.OnboardingState) (any, error) {
		return c.srv.setOnboarding(p)
	}),
	proto.MethodCacheList: with(func(c *conn, p proto.CacheListParams) (any, error) {
		return c.srv.cacheList(p), nil
	}),
	proto.MethodCacheClean: with(func(c *conn, p proto.CacheCleanParams) (any, error) {
		return c.srv.cacheClean(p), nil
	}),
	proto.MethodDaemonStopProcess: with(func(c *conn, p proto.StopProcessParams) (any, error) {
		return ok, c.srv.stopLeftProcess(p)
	}),
	proto.MethodDaemonSettings: bare(func(c *conn) (any, error) { return c.srv.sched.get(), nil }),
	proto.MethodDaemonSetSettings: with(func(c *conn, p proto.DaemonSettings) (any, error) {
		return c.srv.sched.set(p)
	}),
	proto.MethodDaemonStopAgents: bare(func(c *conn) (any, error) {
		return proto.StopAgentsResult{Stopped: c.srv.stopAgents()}, nil
	}),
}

var agentHandlers = map[string]handler{
	proto.MethodAgentList: bare(func(c *conn) (any, error) {
		// Agents installed since the daemon started show up.
		refreshLoginPath(c.srv.logger())
		return agent.List(), nil
	}),
}

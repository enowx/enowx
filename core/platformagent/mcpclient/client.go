// Package mcpclient connects the agent to MCP servers, enowX's and the user's, and
// offers their tools as the agent's own, named mcp__<server>__<tool>.
//
// The tools are listed once, at Connect, and stay as they were: the model's prompt
// cache depends on their bytes, so a server's later list changes are not followed.
package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/platformagent/tools"
)

// startTimeout bounds one server's start, so a slow one can't hold up the agent's.
const startTimeout = 20 * time.Second

// Servers are the connected MCP servers. Its methods are safe for concurrent use.
type Servers struct {
	conns    []*conn
	tools    []tools.Tool
	problems []string
	close    sync.Once
}

// Connect starts the servers spec names, side by side: a path to, or the text of, an
// mcp.json as Claude Code takes it
// ({"mcpServers":{name:{command,args,env}|{type,url,headers}}}), "" for none. ${VAR} and
// ${VAR:-default} in a server's entry expand from environ, and a program it runs has
// environ and its own env, nothing of this process's besides.
//
// A server that fails, or doesn't answer in time, doesn't stop the others: its tools
// are missing and Problems says why. The error is for a spec that can't be read at all.
func Connect(ctx context.Context, spec string, environ []string) (*Servers, error) {
	return connectWithin(ctx, spec, environ, startTimeout)
}

func connectWithin(ctx context.Context, spec string, environ []string, timeout time.Duration) (*Servers, error) {
	entries, err := readSpec(spec)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	slices.Sort(names)

	type started struct {
		conn     *conn
		tools    []listedTool
		problems []string
	}
	results := make([]started, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := &results[i]
			cfg, err := parseServer(name, entries[name], environ)
			if err != nil {
				res.problems = []string{problem(name, err.Error())}
				return
			}
			for _, v := range cfg.missing {
				res.problems = append(res.problems, problem(name, "${"+v+"} isn't set"))
			}
			c := newConn(cfg, timeout)
			listed, err := c.startAndList(ctx)
			if err != nil {
				c.close()
				res.problems = append(res.problems, problem(name, startFailure(ctx, err, timeout, cfg)))
				return
			}
			if len(listed) == 0 {
				c.close() // nothing it could be asked: no process to keep
				return
			}
			res.conn, res.tools = c, listed
		}()
	}
	wg.Wait()

	s := &Servers{}
	var all []listedTool
	for _, r := range results {
		s.problems = append(s.problems, r.problems...)
		if r.conn != nil {
			s.conns = append(s.conns, r.conn)
			all = append(all, r.tools...)
		}
	}
	s.tools = buildTools(all)
	return s, nil
}

func problem(server, why string) string { return "MCP server " + server + ": " + why }

// startFailure says why a server didn't start, without any value of its config.
func startFailure(ctx context.Context, err error, timeout time.Duration, cfg serverConfig) string {
	if ctx.Err() != nil {
		return "the start was interrupted; its tools aren't available this session"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("no answer within %s; its tools aren't available this session", timeout)
	}
	return cfg.hide(err.Error()) + "; its tools aren't available this session"
}

// Tools are every connected server's tools, sorted by name.
func (s *Servers) Tools() []tools.Tool { return slices.Clone(s.tools) }

// Problems say which servers failed and why, without any secret of theirs.
func (s *Servers) Problems() []string { return slices.Clone(s.problems) }

// Close ends the servers: a program and what it started are gone when it returns.
func (s *Servers) Close() error {
	s.close.Do(func() {
		var wg sync.WaitGroup
		for _, c := range s.conns {
			wg.Go(c.close)
		}
		wg.Wait()
	})
	return nil
}

// oneLine is s on one line, at most n runes.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

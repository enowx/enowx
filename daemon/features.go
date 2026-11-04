package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/display"
	"github.com/enowx/enowx/core/features"
	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// The features the user has on decide what a session's agent is told
// (agent.GuideFor) and which MCP tools it gets (cli's mcp asks
// features.guide).

// projectKey names project folder path's switches the same on every host
// it's cloned on: its Git origin, else its folder.
func projectKey(path string) string {
	if path == "" {
		return ""
	}
	if remote, err := git.Run(path, "config", "--get", "remote.origin.url"); err == nil && strings.TrimSpace(remote) != "" {
		return strings.TrimSpace(remote)
	}
	return "path:" + path
}

// resolveFeatures is every feature's state for a session of project with
// switches own.
func (s *Server) resolveFeatures(project string, own map[string]bool) (map[string]features.Resolved, features.Settings, string) {
	st, _ := s.features.Settings()
	host, _ := s.features.Host()
	ok, _ := display.Available()
	key := projectKey(project)
	return features.Resolve(st, host, key, own, !ok), st, key
}

// guideFor is the guide a session of meta's project gets, with its
// switches; "" leaves agents the full one (no switches read).
func (s *Server) guideFor(meta session.Meta) string { return s.guideIn(meta, "") }

// guideIn is guideFor a session whose folder is dir: a folder of no project's has
// rules too, a team's maybe.
func (s *Server) guideIn(meta session.Meta, dir string) string {
	if s.features == nil {
		return agent.Guide
	}
	r, st, _ := s.resolveFeatures(meta.Project, meta.Features)
	return agent.GuideFor(features.On(r), st.LazyOn()) + s.projectGuide(rulesFolder(meta, dir))
}

// featuresState is the switches as Settings shows them.
func (s *Server) featuresState(p proto.FeaturesParams) (proto.FeaturesState, error) {
	project, own := p.Project, map[string]bool(nil)
	if p.Session != "" {
		sess, err := s.sessions.Get(p.Session)
		if err != nil {
			return proto.FeaturesState{}, err
		}
		m := sess.Meta()
		if project == "" {
			project = m.Project
		}
		own = m.Features
	}
	r, st, key := s.resolveFeatures(project, own)
	out := proto.FeaturesState{Lazy: st.LazyOn(), Project: key}
	for _, f := range agent.Features {
		cost := agent.GuideTokens(f.Guide)
		if st.LazyOn() && f.Lazy && f.Summary != "" {
			cost = agent.GuideTokens(f.Summary)
		}
		out.Features = append(out.Features, proto.FeatureInfo{ID: f.ID, Name: f.Name, Group: f.Group, About: f.About,
			Default: f.Default, Core: f.Core, Lazy: f.Lazy, Requires: f.Requires, On: r[f.ID].On, From: r[f.ID].From, Tokens: cost})
	}
	out.Tokens = agent.GuideTokens(agent.GuideFor(features.On(r), st.LazyOn()))
	out.AllCost = agent.GuideTokens(agent.Guide)
	return out, nil
}

// setFeature sets a switch: a session's in its meta, the others in the
// store.
func (s *Server) setFeature(p proto.FeatureSetParams) error {
	if _, ok := agent.FindFeature(p.ID); !ok {
		return errors.New("features: no feature " + p.ID)
	}
	switch p.Scope {
	case features.ScopeSession:
		sess, err := s.sessions.Get(p.Session)
		if err != nil {
			return err
		}
		m := sess.Meta()
		own := map[string]bool{}
		for k, v := range m.Features {
			own[k] = v
		}
		if p.On == nil {
			delete(own, p.ID)
		} else {
			own[p.ID] = *p.On
		}
		s.sessions.SetFeatures(sess, own)
	case features.ScopeProject:
		return s.features.Set(p.ID, p.On, p.Scope, projectKey(p.Project))
	default:
		return s.features.Set(p.ID, p.On, p.Scope, "")
	}
	return nil
}

// featureAsks are agents' requests to turn a feature on, waiting for the
// user's answer.
type featureAsks struct {
	mu      sync.Mutex
	pending map[string]chan proto.FeatureAnswer
}

// ask sends req to the apps and waits for the user's answer, at most
// wait.
func (s *Server) askFeature(req proto.FeatureRequest, wait time.Duration) (proto.FeatureAnswer, error) {
	var b [6]byte
	rand.Read(b[:])
	req.RequestID = hex.EncodeToString(b[:])
	ch := make(chan proto.FeatureAnswer, 1)
	s.featAsks.mu.Lock()
	if s.featAsks.pending == nil {
		s.featAsks.pending = map[string]chan proto.FeatureAnswer{}
	}
	s.featAsks.pending[req.RequestID] = ch
	s.featAsks.mu.Unlock()
	defer func() {
		s.featAsks.mu.Lock()
		delete(s.featAsks.pending, req.RequestID)
		s.featAsks.mu.Unlock()
	}()
	s.broadcast(proto.EventFeaturesRequested, req)
	select {
	case a := <-ch:
		return a, nil
	case <-time.After(wait):
		return proto.FeatureAnswer{}, errors.New("features: the user didn't answer; ask them in the chat to turn it on in Settings → Features")
	case <-s.done:
		return proto.FeatureAnswer{}, errors.New("features: the daemon stopped")
	}
}

// answerFeature hands the user's answer to the request waiting for it.
func (s *Server) answerFeature(a proto.FeatureAnswer) {
	s.featAsks.mu.Lock()
	ch := s.featAsks.pending[a.RequestID]
	s.featAsks.mu.Unlock()
	if ch != nil {
		ch <- a
	}
}

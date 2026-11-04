package daemon

import (
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/features"
	"github.com/enowx/enowx/proto"
)

var featureHandlers = map[string]handler{
	proto.MethodFeaturesList: with(func(c *conn, p proto.FeaturesParams) (any, error) { return c.srv.featuresState(p) }),
	proto.MethodFeaturesSet: with(func(c *conn, p proto.FeatureSetParams) (any, error) {
		if err := c.srv.setFeature(p); err != nil {
			return nil, err
		}
		return c.srv.featuresChanged(proto.FeaturesParams{Project: p.Project, Session: p.Session})
	}),
	proto.MethodFeaturesLazy: with(func(c *conn, p proto.FeaturesLazyParams) (any, error) {
		if err := c.srv.features.SetLazy(p.Lazy); err != nil {
			return nil, err
		}
		return c.srv.featuresChanged(proto.FeaturesParams{})
	}),
	proto.MethodFeaturesPreset: with(func(c *conn, p proto.FeaturesPresetParams) (any, error) {
		sw, ok := features.PresetSwitches(p.Name)
		if !ok {
			return nil, errUnknownPreset(p.Name)
		}
		for id, on := range sw {
			on := on
			if f, _ := agent.FindFeature(id); f.Core {
				continue
			}
			if err := c.srv.setFeature(proto.FeatureSetParams{ID: id, On: &on, Scope: p.Scope, Project: p.Project}); err != nil {
				return nil, err
			}
		}
		return c.srv.featuresChanged(proto.FeaturesParams{Project: p.Project})
	}),
	proto.MethodFeaturesGuide: with(func(c *conn, p proto.FeaturesParams) (any, error) {
		project, own := p.Project, map[string]bool(nil)
		if p.Session != "" {
			if sess, err := c.srv.sessions.Get(p.Session); err == nil {
				m := sess.Meta()
				project, own = m.Project, m.Features
			}
		}
		r, st, _ := c.srv.resolveFeatures(project, own)
		on := features.On(r)
		return proto.FeaturesGuide{Guide: agent.GuideFor(on, st.LazyOn()) + c.srv.guideReadRules(p), On: on}, nil
	}),
	proto.MethodFeaturesRequest: with(func(c *conn, p proto.FeatureRequest) (any, error) {
		return c.srv.askFeature(p, 5*time.Minute)
	}),
	proto.MethodFeaturesAnswer: with(func(c *conn, p proto.FeatureAnswer) (any, error) {
		c.srv.answerFeature(p)
		return ok, nil
	}),
}

// featuresChanged tells the apps the switches changed, and answers with
// them for p.
func (s *Server) featuresChanged(p proto.FeaturesParams) (any, error) {
	s.broadcast(proto.EventFeaturesChanged, struct{}{})
	return s.featuresState(p)
}

type errUnknownPreset string

func (e errUnknownPreset) Error() string { return "features: no preset " + string(e) }

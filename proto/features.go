package proto

// Features the user turns on or off (Settings → Features): one off is
// left out of what agents are told and the MCP tools they get.
const (
	MethodFeaturesList    = "features.list"    // FeaturesParams → FeaturesState
	MethodFeaturesSet     = "features.set"     // FeatureSetParams → FeaturesState
	MethodFeaturesLazy    = "features.lazy"    // FeaturesLazyParams → FeaturesState
	MethodFeaturesPreset  = "features.preset"  // FeaturesPresetParams → FeaturesState
	MethodFeaturesGuide   = "features.guide"   // FeaturesParams → FeaturesGuide
	MethodFeaturesRequest = "features.request" // FeatureRequest → FeatureAnswer, once the user answered
	MethodFeaturesAnswer  = "features.answer"  // FeatureAnswer: the user's, to a request
)

// Feature events.
const (
	// EventFeaturesChanged says the switches changed (no data).
	EventFeaturesChanged = "features.changed"
	// EventFeaturesRequested carries a FeatureRequest an agent made, for
	// the app to ask the user.
	EventFeaturesRequested = "features.requested"
)

// FeaturesParams name what to resolve the switches for: a project's
// folder, or a session (its project and its own switches).
type FeaturesParams struct {
	Project string `json:"project,omitempty"`
	Session string `json:"session,omitempty"`
	// Reading (MethodFeaturesGuide): the session's agent ran the guide, to read
	// it, rather than a program asking for its text.
	Reading bool `json:"reading,omitempty"`
}

// FeatureInfo is a feature and whether it is on, From the scope that
// decided ("" its default: global, host, project, session).
type FeatureInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Group    string   `json:"group"`
	About    string   `json:"about"`
	Default  bool     `json:"default"`
	Core     bool     `json:"core,omitempty"`
	Lazy     bool     `json:"lazy,omitempty"`
	Requires []string `json:"requires,omitempty"`
	On       bool     `json:"on"`
	From     string   `json:"from,omitempty"`
	Tokens   int      `json:"tokens"` // what its part of the guide costs, about
}

// FeaturesState is every feature's state, and what the guide costs.
type FeaturesState struct {
	Features []FeatureInfo `json:"features"`
	Lazy     bool          `json:"lazy"`
	Tokens   int           `json:"tokens"`            // the guide of those on
	AllCost  int           `json:"allTokens"`         // the guide of every feature
	Project  string        `json:"project,omitempty"` // the project's key, its switches'
}

// FeatureSetParams turn feature ID on or off (On nil: as the wider scope
// has it) in Scope (global, host, project, session); Project is a
// project's folder, Session a session's ID.
type FeatureSetParams struct {
	ID      string `json:"id"`
	On      *bool  `json:"on,omitempty"`
	Scope   string `json:"scope"`
	Project string `json:"project,omitempty"`
	Session string `json:"session,omitempty"`
}

// FeaturesLazyParams turn lazy guides on or off.
type FeaturesLazyParams struct {
	Lazy bool `json:"lazy"`
}

// FeaturesPresetParams set every switch of Scope to preset Name.
type FeaturesPresetParams struct {
	Name    string `json:"name"`
	Scope   string `json:"scope"`
	Project string `json:"project,omitempty"`
}

// FeaturesGuide is the guide a session gets, with its switches.
type FeaturesGuide struct {
	Guide string          `json:"guide"`
	On    map[string]bool `json:"on"`
}

// FeatureRequest is an agent asking the user to turn feature ID on, for
// its session or wider.
type FeatureRequest struct {
	RequestID string `json:"requestId,omitempty"`
	ID        string `json:"id"`
	Session   string `json:"session,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// FeatureAnswer is the user's answer to request RequestID.
type FeatureAnswer struct {
	RequestID string `json:"requestId"`
	Allowed   bool   `json:"allowed"`
	Scope     string `json:"scope,omitempty"` // where it was turned on
}

package proto

// A project's rules: Markdown every session in it reads first, whatever
// its agent. A project of the user's own keeps them in the user's synced
// settings; a team's project in the team's space, the same for every
// member, edited by its members or only its admins as the admins choose.
const (
	MethodProjectRules     = "project.rules"      // ProjectRulesRef → ProjectRules
	MethodProjectRulesSave = "project.rules.save" // ProjectRules (Path, Rules, and for a team's admins Editors) → ProjectRules
	// MethodRulesDraft has an AI write rules from what the user says they
	// want: RulesDraftParams → RulesDraft, nothing saved.
	MethodRulesDraft = "rules.draft"
)

// What rules a draft is for (RulesDraftParams.For).
const (
	RulesForProject = "project"
	RulesForGOA     = "goa"
)

// RulesDraftParams ask for rules: Ask what the user wants, in their
// words; Current the rules so far, kept and improved; Path the project's
// folder (RulesForProject).
type RulesDraftParams struct {
	For     string `json:"for"`
	Ask     string `json:"ask"`
	Current string `json:"current,omitempty"`
	Path    string `json:"path,omitempty"`
}

// RulesDraft is the rules an AI wrote, and which (a judge's label).
type RulesDraft struct {
	Rules string `json:"rules"`
	By    string `json:"by"`
}

// Who edits a team's project's rules (ProjectRules.Editors).
const (
	RulesByMembers = "members"
	RulesByAdmins  = "admins"
)

// ProjectRulesRef names a project by its folder on the host.
type ProjectRulesRef struct {
	Path string `json:"path"`
}

// ProjectRules is a project's rules.
type ProjectRules struct {
	Path  string `json:"path"`
	Rules string `json:"rules"`
	// Team is the team whose project it is, "" the user's own; Project
	// its name there.
	Team    string `json:"team,omitempty"`
	Project string `json:"project,omitempty"`
	// Editors are who edit a team's project's rules: RulesByMembers
	// (the default) or RulesByAdmins; CanEdit says the user may.
	Editors string `json:"editors,omitempty"`
	CanEdit bool   `json:"canEdit,omitempty"`
}

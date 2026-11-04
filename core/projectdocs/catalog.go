package projectdocs

import (
	"embed"
	"fmt"
	"strings"
)

// Areas of the catalog, in their order.
const (
	AreaProduct  = "Business and product"
	AreaUX       = "UX and UI"
	AreaArch     = "Architecture"
	AreaData     = "Data"
	AreaAPI      = "API and integrations"
	AreaSecurity = "Security and compliance"
	AreaQuality  = "Quality"
	AreaOps      = "Release and operations"
	AreaPlan     = "Planning"
	AreaLaunch   = "Launch and documentation"
)

// Areas is the catalog's areas, in order.
var Areas = []string{AreaProduct, AreaUX, AreaArch, AreaData, AreaAPI, AreaSecurity, AreaQuality, AreaOps, AreaPlan, AreaLaunch}

// Entry is a document the catalog has a template for.
type Entry struct {
	Key  string // the template's file, templates/<Key>.md
	Name string // its title, and the kind in its first line
	Area string
	Core bool // in the core pack as well as the full
	Gate bool // must be approved before building, when the project requires it
	// Diagram: the document holds a diagram drawn on a canvas and embedded.
	Diagram bool
	// When says what kind of project needs it, for an agent choosing, and
	// its absence is why it may be skipped.
	When string
	Icon string
}

// Catalog is every built-in document.
var Catalog = []Entry{
	{Key: "brief", Name: "Brief", Area: AreaProduct, Core: true, Icon: "lucide:lightbulb", When: "every project"},
	{Key: "market-research", Name: "Market and competitor research", Area: AreaProduct, Icon: "lucide:telescope", When: "a product entering a market with alternatives"},
	{Key: "brd", Name: "Business Requirements", Area: AreaProduct, Icon: "lucide:briefcase", When: "a product with a business case or stakeholders to convince"},
	{Key: "lean-canvas", Name: "Lean Canvas", Area: AreaProduct, Icon: "lucide:layout-grid", When: "a new business or a product with a business model to test"},
	{Key: "prd", Name: "PRD", Area: AreaProduct, Core: true, Gate: true, Icon: "lucide:file-text", When: "every project with users"},
	{Key: "personas", Name: "Personas and User Journey", Area: AreaProduct, Icon: "lucide:users", When: "a product with several kinds of user"},
	{Key: "user-stories", Name: "User Stories and Acceptance Criteria", Area: AreaProduct, Core: true, Icon: "lucide:list-checks", When: "every project with users"},
	{Key: "prioritization", Name: "Feature Priorities and Story Map", Area: AreaProduct, Icon: "lucide:list-ordered", When: "a backlog larger than one release"},
	{Key: "srs", Name: "Software Requirements Specification", Area: AreaProduct, Icon: "lucide:scroll-text", When: "a project that needs formal requirements: a contract, an audit, a regulator"},
	{Key: "glossary", Name: "Glossary", Area: AreaProduct, Icon: "lucide:book-a", When: "a domain with terms of its own"},

	{Key: "user-flow", Name: "User Flow", Area: AreaUX, Core: true, Diagram: true, Icon: "lucide:workflow", When: "every project with a UI"},
	{Key: "sitemap", Name: "Sitemap and Information Architecture", Area: AreaUX, Diagram: true, Icon: "lucide:network", When: "a UI of more than a few pages"},
	{Key: "wireframe", Name: "Wireframes", Area: AreaUX, Diagram: true, Icon: "lucide:square-dashed", When: "a new UI whose layout isn't settled"},
	{Key: "mockup", Name: "Mockups and Prototype", Area: AreaUX, Diagram: true, Icon: "lucide:monitor-smartphone", When: "a UI whose look matters before it is built"},
	{Key: "design-system", Name: "Design System", Area: AreaUX, Icon: "lucide:palette", When: "a UI with more than a few screens, or several people building it"},
	{Key: "copy-guide", Name: "Content and Copy Guide", Area: AreaUX, Icon: "lucide:pen-line", When: "a product whose words matter: marketing, onboarding, several languages"},

	{Key: "tech-spec", Name: "Technical Spec", Area: AreaArch, Core: true, Gate: true, Icon: "lucide:cpu", When: "every project"},
	{Key: "c4", Name: "Architecture Diagram (C4)", Area: AreaArch, Diagram: true, Icon: "lucide:boxes", When: "more than one service, or a system others will work on"},
	{Key: "adr", Name: "Architecture Decision Records", Area: AreaArch, Core: true, Icon: "lucide:milestone", When: "every project"},
	{Key: "dfd", Name: "Data Flow Diagram", Area: AreaArch, Diagram: true, Icon: "lucide:git-fork", When: "data crossing services or trust boundaries, or personal data"},
	{Key: "sequence", Name: "Sequence Diagrams", Area: AreaArch, Diagram: true, Icon: "lucide:arrow-right-left", When: "flows across services: payment, sign-in, webhooks"},
	{Key: "use-case", Name: "Use Case Diagram", Area: AreaArch, Diagram: true, Icon: "lucide:user-round-check", When: "several actors with different permissions"},
	{Key: "state", Name: "State Diagrams", Area: AreaArch, Diagram: true, Icon: "lucide:toggle-right", When: "entities with statuses, such as orders or payments"},
	{Key: "activity", Name: "Activity Diagram", Area: AreaArch, Diagram: true, Icon: "lucide:route", When: "a business process with branches and several parties"},
	{Key: "class", Name: "Class Diagram", Area: AreaArch, Diagram: true, Icon: "lucide:shapes", When: "a rich domain model in object-oriented code"},
	{Key: "deployment-diagram", Name: "Deployment and Infrastructure Diagram", Area: AreaArch, Diagram: true, Icon: "lucide:server", When: "more than one server or managed service"},

	{Key: "erd", Name: "ERD", Area: AreaData, Core: true, Diagram: true, Icon: "lucide:database", When: "every project with stored data"},
	{Key: "data-dictionary", Name: "Database Schema and Data Dictionary", Area: AreaData, Icon: "lucide:table", When: "a schema others will read or query"},
	{Key: "migration-plan", Name: "Data Migration and Seed Plan", Area: AreaData, Icon: "lucide:database-backup", When: "existing data to move, or seed data to start with"},
	{Key: "data-retention", Name: "Data Retention and Classification", Area: AreaData, Icon: "lucide:archive", When: "personal or regulated data"},

	{Key: "api-spec", Name: "API Spec", Area: AreaAPI, Core: true, Icon: "lucide:plug", When: "every project with an API or events"},
	{Key: "integrations", Name: "Third-party Integrations and Webhooks", Area: AreaAPI, Icon: "lucide:cable", When: "any third-party service or webhook, payment included"},
	{Key: "api-conventions", Name: "Error Format, API Versions and Rate Limits", Area: AreaAPI, Icon: "lucide:gauge", When: "an API others depend on"},

	{Key: "threat-model", Name: "Threat Model", Area: AreaSecurity, Icon: "lucide:shield-alert", When: "accounts, money, personal data, or anything on the internet"},
	{Key: "rbac", Name: "Roles and Permissions Matrix", Area: AreaSecurity, Icon: "lucide:key-round", When: "more than one role"},
	{Key: "secrets", Name: "Secret Handling", Area: AreaSecurity, Icon: "lucide:lock-keyhole", When: "any API key, token or password"},
	{Key: "privacy", Name: "Privacy and Compliance", Area: AreaSecurity, Icon: "lucide:scale", When: "personal data: UU PDP, GDPR and the like"},

	{Key: "test-plan", Name: "Test Plan", Area: AreaQuality, Core: true, Icon: "lucide:flask-conical", When: "every project"},
	{Key: "test-cases", Name: "Test Cases", Area: AreaQuality, Icon: "lucide:clipboard-check", When: "acceptance tested by hand, or by someone other than the builder"},
	{Key: "performance", Name: "Performance and Load Targets", Area: AreaQuality, Icon: "lucide:activity", When: "expected load, or latency users would notice"},
	{Key: "accessibility", Name: "Accessibility", Area: AreaQuality, Icon: "lucide:accessibility", When: "a UI for the public, or a contract requiring it"},

	{Key: "deployment-plan", Name: "Deployment Plan", Area: AreaOps, Core: true, Icon: "lucide:rocket", When: "every project that runs somewhere"},
	{Key: "runbook", Name: "Runbook and Incident Response", Area: AreaOps, Icon: "lucide:siren", When: "a service someone must keep running"},
	{Key: "monitoring", Name: "Monitoring, Logs, SLOs and Alerts", Area: AreaOps, Icon: "lucide:line-chart", When: "a service in production"},
	{Key: "backup-dr", Name: "Backup and Disaster Recovery", Area: AreaOps, Icon: "lucide:hard-drive-download", When: "data that can't be lost"},
	{Key: "rollback", Name: "Rollback Plan", Area: AreaOps, Icon: "lucide:undo-2", When: "releases to users who can't wait for a fix"},
	{Key: "cost-estimate", Name: "Cost Estimate", Area: AreaOps, Icon: "lucide:coins", When: "paid infrastructure or services"},
	{Key: "changelog", Name: "Release Notes and Changelog", Area: AreaOps, Icon: "lucide:history", When: "releases others follow"},

	{Key: "roadmap", Name: "Roadmap and Milestones", Area: AreaPlan, Core: true, Icon: "lucide:map", When: "every project"},
	{Key: "work-breakdown", Name: "Work Breakdown and Backlog", Area: AreaPlan, Core: true, Icon: "lucide:kanban", When: "every project"},
	{Key: "risk-register", Name: "Risk Register", Area: AreaPlan, Icon: "lucide:triangle-alert", When: "risks worth tracking: money, dates, new technology"},
	{Key: "definition-of-done", Name: "Definition of Done", Area: AreaPlan, Icon: "lucide:circle-check-big", When: "several people or agents working on it"},
	{Key: "estimate-raci", Name: "Time Estimate and RACI", Area: AreaPlan, Icon: "lucide:calendar-clock", When: "work in a team, or a date promised"},

	{Key: "readme", Name: "README and Contributing Guide", Area: AreaLaunch, Icon: "lucide:book-open", When: "code others will run or change"},
	{Key: "user-docs", Name: "User and API Documentation", Area: AreaLaunch, Icon: "lucide:library", When: "users or developers learning it without help"},
	{Key: "launch-plan", Name: "Launch Plan", Area: AreaLaunch, Icon: "lucide:megaphone", When: "a public release"},
	{Key: "terms", Name: "Terms of Service and Privacy Policy", Area: AreaLaunch, Icon: "lucide:gavel", When: "a public service, or personal data"},
}

//go:embed templates/*.md
var templates embed.FS

// Find is the catalog's entry with this key or name, nil for none.
func Find(name string) *Entry {
	for i := range Catalog {
		if strings.EqualFold(Catalog[i].Key, name) || strings.EqualFold(Catalog[i].Name, name) {
			return &Catalog[i]
		}
	}
	return nil
}

// Template is entry key's template, without its first line.
func Template(key string) (string, error) {
	data, err := templates.ReadFile("templates/" + key + ".md")
	if err != nil {
		return "", fmt.Errorf("projectdocs: no template %q", key)
	}
	return string(data), nil
}

// Pack is the catalog's entries of a size.
func Pack(size string) []Entry {
	var out []Entry
	for _, e := range Catalog {
		if size == SizeFull || e.Core {
			out = append(out, e)
		}
	}
	return out
}

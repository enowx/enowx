package projectdocs

// Topic is something to settle about a project: a question an agent asks one
// at a time while brainstorming, and the section of a document its answer
// goes to. Heading is a heading the document's template must have.
type Topic struct {
	Area, Name, Question string
	Doc, Heading         string // Entry.Key and the section
}

// Topics is every topic, in the order they're best asked.
var Topics = []Topic{
	{"Idea", "Problem", "What problem does it solve, and for whom?", "brief", "Problem and audience"},
	{"Idea", "Goals", "What is it for, and how will you know it worked?", "brief", "Goals and success measures"},
	{"Idea", "Non-goals", "What will it deliberately not do?", "brief", "Non-goals"},
	{"Idea", "Assumptions and risks", "What must be true for this to work, and what is the biggest risk?", "brief", "Assumptions and risks"},
	{"Idea", "Alternatives", "What do people use today instead?", "brief", "Competitors and alternatives"},
	{"Idea", "Business model", "Does it make money? How, and at what price?", "brief", "Business model and pricing"},

	{"Product", "Users", "Who uses it? Describe the main kinds of user.", "prd", "Personas and users"},
	{"Product", "Stories", "What are the main things they do, step by step?", "user-stories", "Main flows"},
	{"Product", "Features", "Which features are in the first version, and which later?", "prd", "Features and priorities"},
	{"Product", "Acceptance", "For each feature: how do we test it is done?", "user-stories", "Acceptance criteria"},
	{"Product", "Roles", "What roles are there, and what may each do?", "prd", "Roles and permissions"},
	{"Product", "Edge states", "What shows when empty, on an error, or at a limit?", "prd", "Empty, error and limit states"},
	{"Product", "Non-functional", "Which needs on performance, security, privacy, accessibility and language?", "prd", "Non-functional requirements"},

	{"Design", "Navigation", "What pages are there, and how do people move between them?", "user-flow", "Pages and navigation"},
	{"Design", "Canvases", "Which wireframes, mockups or directions exist or are needed?", "user-flow", "Canvases"},
	{"Design", "Design system", "Which colors, type and components?", "tech-spec", "Design system"},
	{"Design", "Copy", "What tone and language does it speak in?", "prd", "Copy and tone"},

	{"Technical", "Stack", "Which stack, and why?", "tech-spec", "Stack and reasons"},
	{"Technical", "Architecture", "What are the main parts and how do they talk?", "tech-spec", "Architecture"},
	{"Technical", "Data", "What data is stored, and how does it change over time?", "tech-spec", "Data model and migrations"},
	{"Technical", "API", "What are the API's endpoints or events?", "api-spec", "Contracts"},
	{"Technical", "Auth", "How do people sign in, and how is access decided?", "tech-spec", "Authentication and authorization"},
	{"Technical", "Integrations", "Which third parties, payment included?", "tech-spec", "Third-party integrations"},
	{"Technical", "Conventions", "Which folder structure, code standards and project rules?", "tech-spec", "Structure and conventions"},
	{"Technical", "Security", "What could be attacked, and how is each handled?", "tech-spec", "Security"},
	{"Technical", "Tests", "What is tested at unit, integration and end-to-end level?", "test-plan", "Levels of testing"},
	{"Technical", "Observability", "Which logs, metrics and alerts?", "tech-spec", "Observability"},

	{"Operations", "Environments", "Which environments and variables, and where are secrets kept?", "deployment-plan", "Environments and secrets"},
	{"Operations", "Delivery", "How does it get built and deployed, and on which domain?", "deployment-plan", "CI/CD, domain and deploy"},
	{"Operations", "Backup", "How is it backed up, and has restoring been tried?", "deployment-plan", "Backup and restore"},
	{"Operations", "Rollback", "How is a bad release undone?", "deployment-plan", "Rollback"},
	{"Operations", "Cost", "What does it cost per month, and what is the limit?", "deployment-plan", "Monthly cost and limit"},
	{"Operations", "Legal", "Which terms, privacy policy and licenses?", "deployment-plan", "Legal"},
	{"Operations", "Launch", "How is it announced, documented and supported?", "deployment-plan", "Launch"},

	{"Plan", "Milestones", "What are the stages?", "roadmap", "Milestones"},
	{"Plan", "Tasks", "What tasks does each stage have, and what depends on what?", "work-breakdown", "Tasks"},
	{"Plan", "Decisions", "What was decided, why, and over what?", "adr", "Decisions"},
	{"Plan", "Open questions", "What is still unanswered?", "roadmap", "Open questions"},
	{"Plan", "Change history", "What changed in these documents, and why?", "roadmap", "Change history"},
}

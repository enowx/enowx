package proto

import "time"

// Decisions: a small, cheap model (a judge) the daemon asks instead of
// guessing, given a situation: whether a session needs the user, whether
// an agent's command may run, what of a long output matters, whether an
// agent loops. It answers typed (a verdict, how sure, why); enowX acts on
// it, and with no answer does what it did without. See docs/decide.md.
const (
	MethodDecideSettings    = "decide.settings"    // → DecideSettings
	MethodDecideSetSettings = "decide.setSettings" // DecideSettings → DecideSettings
	MethodDecideJudges      = "decide.judges"      // → []Judge: those found on the host, the user's own, as last tested
	MethodDecideTest        = "decide.test"        // JudgeRef → []Judge: tested now (all when ID is "")
	MethodDecideAsk         = "decide.ask"         // DecideAsk → Decision
	MethodDecideLog         = "decide.log"         // DecideLogParams → []Decision, the latest first
	MethodDecideMark        = "decide.mark"        // DecideMark: a decision the user says was wrong (or right after all)
	MethodDecideRoute       = "decide.route"       // DecideRouteParams → RouteChoice: a model and effort for a prompt
	// EventDecision is a decision acted on in a session: the apps show it
	// where it acted, with its reason and a way back.
	EventDecision = "decide.made" // Decision
)

// Uses of the judge: what it is asked.
const (
	DecideState   = "state"   // a session gone quiet: done, waiting for the user, stuck, failed
	DecideApprove = "approve" // a command an agent is about to run: allow, ask the user, refuse
	DecideTrim    = "trim"    // a long command output: what the agent needs of it
	DecideLoop    = "loop"    // an agent's last steps: looping, or getting on
	DecideRoute   = "route"   // a prompt: how hard, so a model and an effort to match
	DecideContext = "context" // a project's search results: which bear on a prompt
	DecideMemory  = "memory"  // what an agent said last: a decision worth keeping
	DecideChat    = "chat"    // a team chat message: urgent, normal or quiet
	DecideFlaky   = "flaky"   // a failed check's output: flaky or a real failure
)

// DecideUses are the uses, in the order they're offered.
var DecideUses = []string{DecideState, DecideApprove, DecideTrim, DecideLoop, DecideRoute, DecideContext, DecideMemory, DecideChat, DecideFlaky}

// Verdicts.
const (
	VerdictDone     = "done"
	VerdictWaiting  = "waiting"
	VerdictStuck    = "stuck"
	VerdictFailed   = "failed"
	VerdictAllow    = "allow"
	VerdictAsk      = "ask"
	VerdictRefuse   = "refuse"
	VerdictTrimmed  = "trimmed"
	VerdictKeep     = "keep"
	VerdictLooping  = "looping"
	VerdictProgress = "progress"
	VerdictEasy     = "easy"
	VerdictMedium   = "medium"
	VerdictHard     = "hard"
	VerdictRelevant = "relevant" // some of the results bear on the prompt (Decision.Keep)
	VerdictNone     = "none"
	VerdictDecision = "decision" // the answer holds one (Decision.Text, distilled)
	VerdictNothing  = "nothing"
	VerdictUrgent   = "urgent"
	VerdictNormal   = "normal"
	VerdictQuiet    = "quiet"
	VerdictFlaky    = "flaky"
	VerdictReal     = "real"
)

// Policies: which providers may judge.
const (
	DecideSameProvider = "agent" // the session's agent's own (Claude Code: Anthropic), local models too
	DecideOneProvider  = "one"   // DecideSettings.Provider only, local models too
	DecideAnyProvider  = "any"
	DecideLocalOnly    = "local" // nothing leaves the host
)

// Kinds of judge.
const (
	JudgeAgent = "agent" // an agent's one-shot command (claude -p, codex exec), its own sign-in
	JudgeAPI   = "api"   // a model API, its key a vault credential
	JudgeLocal = "local" // a model server on the host (Ollama, LM Studio)
	// JudgeDecision is a decision model (TypeSafe's Jev, Cloudflare's
	// Clef): typed questions answered with probabilities over the System
	// One API, asked before any other judge.
	JudgeDecision = "decision"
)

// Providers of judges.
const (
	ProviderAnthropic  = "anthropic"
	ProviderOpenAI     = "openai"
	ProviderGoogle     = "google"
	ProviderOpenRouter = "openrouter"
	ProviderLocal      = "local"
	ProviderTypeSafe   = "typesafe"
	ProviderCloudflare = "cloudflare"
)

// Judge is a model that can judge: how it is reached, and how it did when
// last tested.
type Judge struct {
	// ID is "agent:claude:claude-haiku-5-5", "api:<credential>:<model>",
	// "local:<address>:<model>"; Own that the user added it.
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Provider string `json:"provider"`
	// Agent is the agent of a JudgeAgent; Cred the vault's credential of a
	// JudgeAPI; URL the address of an API or local server
	// (OpenAI-compatible; "" for the provider's own).
	Agent string `json:"agent,omitempty"`
	Cred  string `json:"cred,omitempty"`
	URL   string `json:"url,omitempty"`
	Model string `json:"model"`
	// Protocol is how its API is spoken: "anthropic" (Messages) or ""
	// (OpenAI-compatible chat completions; Anthropic's own API aside); for
	// a decision model, "" (System One) or "openai" (OpenAI's Decisions).
	Protocol string `json:"protocol,omitempty"`
	Label    string `json:"label"`
	Own      bool   `json:"own,omitempty"`
	// The last test: when, whether it answered as asked, why not, how long
	// it took, what it cost.
	Tested time.Time `json:"tested,omitzero"`
	OK     bool      `json:"ok,omitempty"`
	Error  string    `json:"error,omitempty"`
	Ms     int64     `json:"ms,omitempty"`
	Tokens int       `json:"tokens,omitempty"`
	Cost   float64   `json:"cost,omitempty"` // US dollars a call, when its service says
}

// JudgeRef names a judge.
type JudgeRef struct {
	ID string `json:"id,omitempty"`
}

// DecideUse is a use's settings: on, which judge ("" the chain), how sure
// the judge must be for enowX to act, and how many calls a day at most.
type DecideUse struct {
	On            bool    `json:"on,omitempty"`
	Judge         string  `json:"judge,omitempty"`
	MinConfidence float64 `json:"minConfidence,omitempty"` // 0: the use's default
	DailyCalls    int     `json:"dailyCalls,omitempty"`    // 0: 300
}

// DecideProject is a project's own policy ("" the host's).
type DecideProject struct {
	Policy   string `json:"policy,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// DecideSettings are the host's (synced): which providers may judge, the
// judges in the order tried (the next when one fails), those turned off,
// those the user added, each use's, and projects' own policies by project
// ID.
type DecideSettings struct {
	Policy   string                   `json:"policy,omitempty"` // DecideSameProvider when ""
	Provider string                   `json:"provider,omitempty"`
	Order    []string                 `json:"order,omitempty"`
	Off      []string                 `json:"off,omitempty"`
	Own      []Judge                  `json:"own,omitempty"`
	Uses     map[string]DecideUse     `json:"uses,omitempty"`
	Projects map[string]DecideProject `json:"projects,omitempty"`
}

// DecideAsk asks for a decision of Use about session Session (or project
// Project): a command (Command, its Output and Exit), a screen (Screen), an
// agent's last steps (Steps).
type DecideAsk struct {
	Use     string   `json:"use"`
	Session string   `json:"session,omitempty"`
	Project string   `json:"project,omitempty"`
	Command string   `json:"command,omitempty"`
	Output  string   `json:"output,omitempty"`
	Exit    int      `json:"exit,omitempty"`
	Screen  string   `json:"screen,omitempty"`
	Steps   []string `json:"steps,omitempty"`
	// Agent is whose prompt Prompt is (route) when no session says; Parts
	// the search results to weigh against it (context).
	Agent  string   `json:"agent,omitempty"`
	Prompt string   `json:"prompt,omitempty"`
	Parts  []string `json:"parts,omitempty"`
	// Text is an agent's last answer (memory) or a chat message (chat),
	// From its author, Place the chat it was said in, Mention that it
	// names or answers the user.
	Text    string `json:"text,omitempty"`
	From    string `json:"from,omitempty"`
	Place   string `json:"place,omitempty"`
	Mention bool   `json:"mention,omitempty"`
	// Judge asks this judge alone ("" the chain), as a test does.
	Judge string `json:"judge,omitempty"`
}

// Decision is a judge's answer and what came of it.
type Decision struct {
	ID         string    `json:"id"`
	Use        string    `json:"use"`
	Session    string    `json:"session,omitempty"`
	Project    string    `json:"project,omitempty"`
	Agent      string    `json:"agent,omitempty"`
	Time       time.Time `json:"time"`
	Verdict    string    `json:"verdict,omitempty"`
	Confidence float64   `json:"confidence,omitempty"`
	Why        string    `json:"why,omitempty"`
	// Text is a trim's output, what the agent reads; a memory's decision,
	// distilled.
	Text string `json:"text,omitempty"`
	// Keep are the parts that bear on a prompt (context), numbered from 1.
	Keep []int `json:"keep,omitempty"`
	// Sure says enowX acts on it: answered, as sure as its use asks.
	Sure bool `json:"sure,omitempty"`
	// Judge answered, in Ms, for Tokens and Cost; Tried are those that
	// failed first ("id: why"); Error why none answered.
	Judge  string   `json:"judge,omitempty"`
	Ms     int64    `json:"ms,omitempty"`
	Tokens int      `json:"tokens,omitempty"`
	Cost   float64  `json:"cost,omitempty"`
	Tried  []string `json:"tried,omitempty"`
	Error  string   `json:"error,omitempty"`
	// Input is what the judge was given, cut short, secrets masked; Acted
	// what enowX did of it ("" nothing: not sure enough, or no answer).
	Input string `json:"input,omitempty"`
	Acted string `json:"acted,omitempty"`
	// Saved is the characters a trim kept from the agent.
	Saved int    `json:"saved,omitempty"`
	Wrong bool   `json:"wrong,omitempty"`
	Note  string `json:"note,omitempty"`
}

// What enowX did of a decision (Decision.Acted).
const (
	ActedAllowed = "allowed"
	ActedRefused = "refused"
	ActedWaiting = "marked waiting"
	ActedAlerted = "alerted"
	ActedTrimmed = "trimmed"
	ActedPaused  = "paused"
	ActedRouted  = "suggested"         // a model and effort for a prompt
	ActedChosen  = "kept some"         // only the search results that bear on the prompt
	ActedDropped = "dropped"           // no context, or no memory offered
	ActedOffered = "offered"           // a memory offered, its text distilled
	ActedUrgent  = "notified urgently" // sound and banner, whatever the chat's settings
	ActedQuiet   = "kept quiet"        // counted as unread, nothing more
	ActedFlaky   = "marked flaky"
	ActedReal    = "marked real"
	ActedNothing = ""
)

// DecideLogParams filters the log: by Use, by Session, Limit at most (0:
// 200).
type DecideLogParams struct {
	Use     string `json:"use,omitempty"`
	Session string `json:"session,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// DecideMark marks decision ID wrong (or not), with the user's note.
type DecideMark struct {
	ID    string `json:"id"`
	Wrong bool   `json:"wrong"`
	Note  string `json:"note,omitempty"`
}

// DecideRouteParams asks which model and effort suit Prompt for Agent (or
// session Session's agent), in Project.
type DecideRouteParams struct {
	Agent   string `json:"agent,omitempty"`
	Session string `json:"session,omitempty"`
	Project string `json:"project,omitempty"`
	Prompt  string `json:"prompt"`
}

// RouteChoice is a model and effort for a prompt, from how hard the judge
// says it is. Model or Effort "" keep the agent's own; with no answer
// (Error), or the judge not sure enough, both are "".
type RouteChoice struct {
	Agent      string  `json:"agent"`
	Difficulty string  `json:"difficulty,omitempty"` // VerdictEasy, VerdictMedium, VerdictHard
	Model      string  `json:"model,omitempty"`
	Effort     string  `json:"effort,omitempty"`
	Why        string  `json:"why,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	// Config is a session config of the user's to run it with, when one
	// suits better than the agent's own models ("" for none: not chosen
	// by the judge yet).
	Config string `json:"config,omitempty"`
	// Switch is what to type in a running session of the agent for this
	// model and effort (its /model, /effort); a new session takes them as
	// SessionCreateParams.Model and Effort.
	Switch   []string `json:"switch,omitempty"`
	Decision string   `json:"decision,omitempty"` // the decision's ID in the log
	Error    string   `json:"error,omitempty"`
}

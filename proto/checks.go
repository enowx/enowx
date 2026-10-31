package proto

import "time"

// Checks: the commands that tell whether a session's work holds up (its
// tests, linters, build), run in the session's folder.
const (
	MethodChecksList = "checks.list" // SessionRef → ChecksInfo
	MethodChecksRun  = "checks.run"  // ChecksRunParams → ChecksInfo
	MethodChecksStop = "checks.stop" // SessionRef
	MethodChecksSet  = "checks.set"  // ChecksSetParams → ChecksInfo
)

// EventChecksChanged carries a SessionRef when a check of the session
// starts or ends.
const EventChecksChanged = "checks.changed"

// Check is a command to run, by name.
type Check struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// ChecksInfo is a session's checks and how their last runs went. Custom
// says the user set the checks for the session's project; else they are
// found from its files.
type ChecksInfo struct {
	Checks []Check    `json:"checks"`
	Custom bool       `json:"custom,omitempty"`
	Runs   []CheckRun `json:"runs,omitempty"`
}

// Check run states.
const (
	CheckQueued  = "queued"
	CheckRunning = "running"
	CheckPassed  = "passed"
	CheckFailed  = "failed"
	CheckStopped = "stopped"
)

// CheckRun is the last run of a check.
type CheckRun struct {
	Name     string        `json:"name"`
	Command  string        `json:"command"`
	State    string        `json:"state"`
	ExitCode int           `json:"exitCode,omitempty"`
	Started  time.Time     `json:"started,omitzero"`
	Duration time.Duration `json:"duration,omitempty"`
	Output   string        `json:"output,omitempty"` // its end, when long
	// Judged is the judge's word on a failure, VerdictFlaky or VerdictReal
	// (docs/decide.md), JudgedWhy its reason; Judging while it reads.
	Judged    string `json:"judged,omitempty"`
	JudgedWhy string `json:"judgedWhy,omitempty"`
	Judging   bool   `json:"judging,omitempty"`
}

// ChecksRunParams runs the checks named, or all of them.
type ChecksRunParams struct {
	Session string   `json:"session"`
	Names   []string `json:"names,omitempty"`
}

// ChecksSetParams sets the checks of the session's project; none goes back
// to those found from its files.
type ChecksSetParams struct {
	Session string  `json:"session"`
	Checks  []Check `json:"checks"`
}

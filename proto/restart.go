package proto

// MethodDaemonRestartPlan says what replacing a host's daemon would cost now, for an
// update to wait or ask: → RestartPlan.
const MethodDaemonRestartPlan = "daemon.restartPlan"

// RestartItem is one thing a daemon's restart stops or stopped.
type RestartItem struct {
	Session string `json:"session"`
	Name    string `json:"name,omitempty"`
	// What is an agent's name, or a command as run, secrets masked.
	What string `json:"what"`
}

// RestartPlan is what a restart of a host's daemon would stop now. Agents come back on
// their conversations; what runs in terminals ends, unless it is an agent, which comes
// back too.
type RestartPlan struct {
	Agents   []RestartItem `json:"agents,omitempty"`   // at work or asking: their turn is cut short
	Commands []RestartItem `json:"commands,omitempty"` // running in terminals
	// Lost is what the last restart ended, and couldn't bring back.
	Lost []RestartItem `json:"lost,omitempty"`
	// Unknown: a daemon too old to look into terminals, whose every terminal is listed.
	Unknown bool `json:"unknown,omitempty"`
}

// Empty says a restart would stop nothing of the user's now.
func (p RestartPlan) Empty() bool { return len(p.Agents) == 0 && len(p.Commands) == 0 }

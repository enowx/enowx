package proto

import (
	"slices"
	"time"
)

// Hibernating a session by hand: its agent stops, what it held (memory, a
// model's session) is freed, and it comes back on its conversation when
// woken, as the daemon does on its own with idle agents
// (DaemonSettings.HibernateAfter).
const (
	MethodSessionHibernate     = "session.hibernate"      // SessionHibernateParams → SessionHibernateResult
	MethodSessionWake          = "session.wake"           // SessionRef → SessionInfo: woken on its conversation
	MethodSessionAutoHibernate = "session.autoHibernate"  // SessionAutoHibernateParams
	MethodHibernatePrefs       = "session.hibernatePrefs" // → HibernatePrefs
)

// HibernatePrefs are the user's hibernation preferences, the same on
// every host: whether new sessions keep from hibernating on their own.
type HibernatePrefs struct {
	NeverByDefault bool `json:"neverByDefault"`
}

// SessionHibernateParams hibernates sessions IDs, or with Idle every
// session whose agent is done and that runs nothing else. Sessions that
// would lose work (an agent working, a dev server, an app on a display,
// other programs) are left as they are and planned in the result, unless
// Force stops them anyway, or WhenDone hibernates them once their agent
// is done. DryRun only plans. From is the session asking (an agent):
// the user confirms in the app first.
type SessionHibernateParams struct {
	IDs      []string `json:"ids,omitempty"`
	Idle     bool     `json:"idle,omitempty"`
	Force    bool     `json:"force,omitempty"`
	WhenDone bool     `json:"whenDone,omitempty"`
	DryRun   bool     `json:"dryRun,omitempty"`
	From     string   `json:"from,omitempty"`
}

// SessionHibernateResult is what was hibernated, what waits for its agent
// to be done, and the plan for each session asked about.
type SessionHibernateResult struct {
	Hibernated []string        `json:"hibernated,omitempty"`
	Waiting    []string        `json:"waiting,omitempty"`
	Plans      []HibernatePlan `json:"plans"`
}

// HibernatePlan is what hibernating a session would stop and free.
// Blocked says why it can't be hibernated at all; Working, Task, Procs, Ports
// and Display are the work it would lose. Task is GOA's task under way there.
type HibernatePlan struct {
	ID      string          `json:"id"`
	Name    string          `json:"name,omitempty"`
	Blocked string          `json:"blocked,omitempty"`
	Working bool            `json:"working,omitempty"`
	Task    string          `json:"task,omitempty"`
	Procs   []HibernateProc `json:"procs,omitempty"`
	Ports   []int           `json:"ports,omitempty"`
	Display bool            `json:"display,omitempty"`
	// Background are the agents in its background, by name, which stop with it and may lose work.
	Background []string `json:"background,omitempty"`
	RSS        uint64   `json:"rss"` // bytes freed, about
	CPU        float64  `json:"cpu"` // percent of a core freed
}

// Busy reports whether hibernating it now would stop work: its agent
// working, a task of GOA's under way, something served, an app shown, or a program it left running
// in the background. Its agent's own helpers stopping with it are not.
func (p HibernatePlan) Busy() bool {
	return p.Working || p.Task != "" || len(p.Ports) > 0 || p.Display || len(p.Background) > 0 || slices.ContainsFunc(p.Procs, func(pr HibernateProc) bool { return pr.Left })
}

// HibernateProc is a program a session started that would stop with it;
// Left when it left the session's process tree (nohup, tmux, setsid).
type HibernateProc struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
	RSS  uint64 `json:"rss"`
	Left bool   `json:"left,omitempty"`
}

// SessionAutoHibernateParams keeps session ID from hibernating on its own
// (Never), or lets it again; with Default, sets whether new sessions start
// that way, on every host (the user's synced settings).
type SessionAutoHibernateParams struct {
	ID      string `json:"id,omitempty"`
	Never   bool   `json:"never"`
	Default bool   `json:"default,omitempty"`
}

// HibernateState is how a session hibernated, for SessionInfo: by hand
// or on its own, and since when.
type HibernateState struct {
	Manual bool      `json:"manual,omitempty"`
	Since  time.Time `json:"since,omitempty"`
}

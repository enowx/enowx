package proto

// MethodDaemonStats measures what the daemon and its sessions use, and the
// host's memory and CPU: → DaemonStats.
const MethodDaemonStats = "daemon.stats"

// Usage is what a process, or a process tree, uses.
type Usage struct {
	RSS   uint64  `json:"rss"` // bytes of memory
	CPU   float64 `json:"cpu"` // percent of one core
	Procs int     `json:"procs"`
}

// HostUsage is a machine's memory and CPU.
type HostUsage struct {
	CPUs     int     `json:"cpus"`
	MemTotal uint64  `json:"memTotal"`
	MemUsed  uint64  `json:"memUsed"`
	CPU      float64 `json:"cpu"` // percent of all cores
	// DiskTotal and DiskUsed are of the filesystem holding the daemon's
	// home folder, where projects and worktrees go.
	DiskTotal uint64 `json:"diskTotal,omitempty"`
	DiskUsed  uint64 `json:"diskUsed,omitempty"`
}

// DaemonStats is a measurement of a host.
type DaemonStats struct {
	Supported bool           `json:"supported"` // false where enowX can't measure
	Host      HostUsage      `json:"host"`
	Daemon    Usage          `json:"daemon"`   // the daemon alone
	Sessions  []SessionUsage `json:"sessions"` // each with all it started
	// Left is what sessions started and still runs with none looking after it.
	Left []LeftProcess `json:"left,omitempty"`
}

// MethodDaemonStopProcess stops a process DaemonStats.Left names, and no other.
const MethodDaemonStopProcess = "daemon.stopProcess"

// LeftProcess is a process a session started that outlived it, or left from under it.
type LeftProcess struct {
	PID     int    `json:"pid"`
	Start   string `json:"start"` // with PID it names this process
	Name    string `json:"name"`
	Session string `json:"session"`
	Usage
}

// StopProcessParams names a process to stop.
type StopProcessParams struct {
	PID   int    `json:"pid"`
	Start string `json:"start"`
}

// SessionUsage is what a session's processes use.
type SessionUsage struct {
	ID string `json:"id"`
	Usage
}

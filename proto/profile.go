package proto

// MethodDaemonProfile measures the running daemon and the app attached to it, changing
// nothing and restarting nothing: ProfileParams → Profile.
const MethodDaemonProfile = "daemon.profile"

// ProfileParams says how to measure.
type ProfileParams struct {
	// CPUSeconds is the window CPU is measured over (at most 60); 0 reads what a
	// process has used in all without waiting.
	CPUSeconds int `json:"cpuSeconds,omitempty"`
	// Raw asks for pprof profiles too (heap, goroutines; CPU with CPUSeconds).
	Raw bool `json:"raw,omitempty"`
}

// ProfileMemory is what the system says a process holds, in bytes. Which fields are set
// depends on the system: PSS and the page kinds on Linux, Footprint on macOS, where
// clean file pages (code) don't count against it.
type ProfileMemory struct {
	RSS           uint64          `json:"rss"`
	PSS           uint64          `json:"pss,omitempty"`
	PrivateDirty  uint64          `json:"privateDirty,omitempty"`
	PrivateClean  uint64          `json:"privateClean,omitempty"`
	SharedClean   uint64          `json:"sharedClean,omitempty"`
	SharedDirty   uint64          `json:"sharedDirty,omitempty"`
	Anon          uint64          `json:"anon,omitempty"`
	Swap          uint64          `json:"swap,omitempty"`
	Footprint     uint64          `json:"footprint,omitempty"`
	FootprintPeak uint64          `json:"footprintPeak,omitempty"`
	PeakRSS       uint64          `json:"peakRss,omitempty"`
	Regions       []ProfileRegion `json:"regions,omitempty"`
	Note          string          `json:"note,omitempty"` // why something couldn't be read
}

// ProfileRegion is a kind of mapping: a file (a program, a library) or anonymous memory.
type ProfileRegion struct {
	Name         string `json:"name"`
	RSS          uint64 `json:"rss"`
	PrivateDirty uint64 `json:"privateDirty,omitempty"`
}

// ProfileGo is what the Go runtime holds, in bytes.
type ProfileGo struct {
	HeapAlloc    uint64  `json:"heapAlloc"`
	HeapInuse    uint64  `json:"heapInuse"`
	HeapIdle     uint64  `json:"heapIdle"`
	HeapReleased uint64  `json:"heapReleased"` // given back to the system, and still counted in Sys
	HeapSys      uint64  `json:"heapSys"`
	NextGC       uint64  `json:"nextGC"`
	StackInuse   uint64  `json:"stackInuse"`
	Sys          uint64  `json:"sys"` // address space ever taken: it only rises
	Objects      uint64  `json:"objects"`
	NumGC        uint32  `json:"numGC"`
	PauseMillis  uint64  `json:"pauseMillis"`
	GCPercent    int     `json:"gcPercent"`             // GOGC
	MemoryLimit  int64   `json:"memoryLimit,omitempty"` // GOMEMLIMIT, when one is set
	Goroutines   int     `json:"goroutines"`
	Threads      int     `json:"threads"`
	GCCPUPercent float64 `json:"gcCpuPercent"` // of the time since it started
}

// ProfileCost is what one thing costs: the live bytes of the heap's sampled allocations
// by the code that made them, or the goroutines of the code running them.
type ProfileCost struct {
	Name  string `json:"name"`
	Bytes uint64 `json:"bytes,omitempty"`
	Count int64  `json:"count"`
}

// ProfileArea is what a part of the program keeps: an open page, its terminals, a cache.
type ProfileArea struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Bytes uint64 `json:"bytes,omitempty"`
}

// ProfileRaw holds pprof files, for `go tool pprof`.
type ProfileRaw struct {
	Heap       []byte `json:"heap,omitempty"`
	Goroutines []byte `json:"goroutines,omitempty"`
	CPU        []byte `json:"cpu,omitempty"`
}

// ProcessProfile is one process measured.
type ProcessProfile struct {
	Kind          string        `json:"kind"` // "daemon" or "app"
	Version       string        `json:"version"`
	OS            string        `json:"os"`
	Arch          string        `json:"arch"`
	PID           int           `json:"pid"`
	UptimeSeconds int64         `json:"uptimeSeconds"`
	CPUSeconds    float64       `json:"cpuSeconds"`           // used since it started
	CPUWindow     float64       `json:"cpuWindow,omitempty"`  // the window's seconds
	CPUPercent    float64       `json:"cpuPercent,omitempty"` // of one core, over the window
	Memory        ProfileMemory `json:"memory"`
	Go            ProfileGo     `json:"go"`
	HeapBy        []ProfileCost `json:"heapBy,omitempty"`
	GoroutinesBy  []ProfileCost `json:"goroutinesBy,omitempty"`
	Areas         []ProfileArea `json:"areas,omitempty"`
	Raw           *ProfileRaw   `json:"raw,omitempty"`
}

// ProfileProgram is one program sessions run, with how many of it: what they cost
// beside the app and the daemon, such as an MCP server each session starts for itself.
type ProfileProgram struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	RSS   uint64 `json:"rss"`
}

// Profile is a host's running daemon, the app attached to it, and what sessions run.
type Profile struct {
	Daemon   ProcessProfile   `json:"daemon"`
	App      *ProcessProfile  `json:"app,omitempty"`
	AppError string           `json:"appError,omitempty"` // why App is missing
	Programs []ProfileProgram `json:"programs,omitempty"`
}

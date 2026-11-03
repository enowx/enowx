package proto

// MethodDaemonDiagnostics reads a host's logs for a bug report, masked: → Diagnostics.
const MethodDaemonDiagnostics = "daemon.diagnostics"

// DiagFile is a log, its end if it is large: Size is the whole file's.
type DiagFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Text string `json:"text"`
}

// Diagnostics is a host's logs.
type Diagnostics struct {
	Version string     `json:"version"`
	Files   []DiagFile `json:"files"`
}

package daemon

// goaProcesses is a snapshot used by one watcher tick. Start times keep a
// reused PID from hiding a new process. Unknown ancestry is never exempted.
type goaProcesses struct {
	known  map[int]bool
	starts map[int]string
}

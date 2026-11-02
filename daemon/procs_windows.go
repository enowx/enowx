package daemon

import "os"

// stopProcess ends process pid: Windows has no asking. Nothing calls this yet, as
// processes aren't measured here, so none are on the list.
func stopProcess(pid int, _ bool) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}

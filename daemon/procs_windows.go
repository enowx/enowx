package daemon

import "os"

// stopProcess ends process pid when forced. Windows has no signal to ask with: closing
// a session's console is what asks (CTRL_CLOSE_EVENT), reapGrace before this forces.
func stopProcess(pid int, force bool) {
	if !force {
		return
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}

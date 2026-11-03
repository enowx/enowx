//go:build !windows

package daemon

// commandLimit is how long an agent's command may be: 0, none to speak of here.
var commandLimit = 0

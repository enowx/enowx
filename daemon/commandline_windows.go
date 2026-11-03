package daemon

// commandLimit is how long an agent's command may be: cmd.exe stops at 8,191 characters
// and an agent installed by npm starts through it, its own .cmd adding its path. 0 where
// there's no such limit.
var commandLimit = 7500

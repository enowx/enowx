package agent

// OpenCode 2 runs one background server per user, which owns its configuration: a
// session's OPENCODE_CONFIG and OPENCODE_CONFIG_CONTENT reach only the client, and are
// ignored once that server runs (seen on 2.2.5: the config's models weren't listed, and
// its first start took them only by chance of starting the server). Its --standalone
// gives the session a private server, the user's own left alone. 1.x has no server, and
// doesn't start with that flag.

// OpenCodeArgs are what command, an OpenCode, needs on its command line for what its
// session's environment says to count: --standalone from version 2.
func OpenCodeArgs(command string) []string {
	if OpenCodeMajor(command) < 2 {
		return nil
	}
	return []string{"--standalone"}
}

package agent

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"github.com/enowx/enowx/internal/quietcmd"
)

// OpenCode 2 runs one background server per user, which owns its configuration: a
// session's OPENCODE_CONFIG and OPENCODE_CONFIG_CONTENT reach only the client, and are
// ignored once that server runs (seen on 2.2.5: the config's models weren't listed, and
// its first start took them only by chance of starting the server). Its --standalone
// gives the session a private server, the user's own left alone. 1.x has no server, and
// doesn't start with that flag.

var openCodeVersion = regexp.MustCompile(`(\d+)\.\d+\.\d+`)

// OpenCodeArgs are what command, an OpenCode, needs on its command line for what its
// session's environment says to count: --standalone from version 2.
func OpenCodeArgs(command string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := quietcmd.CommandContext(ctx, command, "--version").Output()
	if err != nil {
		return nil
	}
	m := openCodeVersion.FindSubmatch(out)
	if m == nil {
		return nil
	}
	if major, err := strconv.Atoi(string(m[1])); err != nil || major < 2 {
		return nil
	}
	return []string{"--standalone"}
}

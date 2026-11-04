//go:build !windows

package daemon

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/internal/standinapi"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
)

// readAncestors is a hostile Bash call: every process above it, its environment read as
// /proc lets a program of the same user read it.
const readAncestors = `p=$PPID; while [ "$p" -gt 1 ]; do tr '\\0' '\\n' < /proc/$p/environ 2>/dev/null; p=$(cut -d' ' -f4 /proc/$p/stat); done`

// The real program on a key the vault lent: the API gets it, and nothing the agent runs
// can read it from any process above it, wrapper shells included.
func e2eVaultKey(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	e := startE2E(t, e2eOptions{noKey: true})
	sock, err := transport.SocketPathIn(e.data)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := transport.DialLocal(sock, nil)
	if err != nil {
		t.Fatal(err)
	}
	var app *client.Client
	app, err = client.New(conn, func(ev proto.Event) {
		var req proto.VaultRequest
		if ev.Name != proto.EventVaultRequest || json.Unmarshal(ev.Data, &req) != nil {
			return
		}
		go func() {
			_ = app.VaultReply(proto.VaultReply{ID: req.ID, Lend: proto.VaultLend{
				Credentials: []string{"anthropic"}, Env: map[string]string{"ANTHROPIC_API_KEY": e2eKey},
			}})
		}()
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	if err := app.ServeVault(); err != nil {
		t.Fatal(err)
	}
	putVault(t, app, vault.Credential{Name: "anthropic", Platform: "anthropic", Fields: map[string]string{"api_key": e2eKey}})

	id := e.spawn(proto.SessionCreateParams{Agent: agent.PlatformID, SkipPermissions: true})
	input, _ := json.Marshal(map[string]string{"command": readAncestors})
	e.api.Enqueue(standinapi.Calls(standinapi.ToolUse("toolu_p", "Bash", string(input))))
	e.say(id, "look around", 2)

	bash, ok := lastTool(e.entries(id), "Bash")
	if !ok || !bash.Done {
		t.Fatalf("Bash: %s", e.dumpEntries(id))
	}
	if !strings.Contains(bash.Result, "ENOWX_SESSION") {
		t.Fatalf("read no process above it, so this proves nothing: %q", bash.Result)
	}
	if strings.Contains(bash.Result, e2eKey) || strings.Contains(bash.Result, "ANTHROPIC_API_KEY") {
		t.Fatal("a tool read the key from a process above it")
	}
	req := lastRequest(t, e.api)
	if got := req.Header.Get("X-Api-Key"); got != e2eKey {
		t.Fatalf("the API got key %q", got)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("the key also went as a bearer token")
	}
	e.assertNoKeyInCommandLines()
}

package ui

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// tokenDaemon is a host's daemon with tracking on, its records read
// back, its data folder dir.
func tokenDaemon(t *testing.T, dir string) *client.Client {
	t.Helper()
	srv, err := daemon.NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a, b := net.Pipe()
	go srv.ServeConn(a)
	cl, err := client.New(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	if err := cl.Call(proto.MethodTokensSetSettings, proto.TokensSettings{On: true}, nil); err != nil {
		t.Fatal(err)
	}
	if err := cl.Call(proto.MethodTokensBackfill, proto.TokensBackfill{}, nil); err != nil {
		t.Fatal(err)
	}
	return cl
}

// Resource usage → Tokens shows every host's numbers together: the
// shown host's and a watched one's asked now, a third's as it synced;
// split by host, each apart. A budget added there is kept.
func TestTokensTabEveryHost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "XDG_DATA_HOME"} {
		t.Setenv(v, "")
	}
	// Both daemons run here, so both read this transcript: each host
	// counts 1200 tokens of input.
	now := time.Now().UTC()
	file := filepath.Join(home, ".claude", "projects", "-work", "c.jsonl")
	os.MkdirAll(filepath.Dir(file), 0o700)
	os.WriteFile(file, []byte(fmt.Sprintf(`{"type":"assistant","timestamp":%q,"cwd":"/work","message":{"id":"m1","model":"claude-opus-5-5","usage":{"input_tokens":1200,"output_tokens":300}}}`+"\n", now.Format(time.RFC3339Nano))), 0o600)

	dirA := t.TempDir()
	synced := proto.TokensHostDays{Host: "vps-1", Name: "Old VPS", Updated: now.Add(-3 * time.Hour), Rows: []proto.TokenDayRow{
		{Day: now.Local().Format(time.DateOnly), Agent: "codex", Model: "gpt-5.1", TokenCounts: proto.TokenCounts{Input: 5000, Output: 100, Calls: 2}}}}
	data, _ := json.Marshal(synced)
	os.MkdirAll(filepath.Join(dirA, "settings", "tokens-hosts"), 0o700)
	os.WriteFile(filepath.Join(dirA, "settings", "tokens-hosts", "vps-1.json"), data, 0o600)
	clA := tokenDaemon(t, dirA)
	clB := tokenDaemon(t, t.TempDir())

	app := New(Options{})
	posted := make(chan func(), 64)
	app.onMain = func(fn func()) { posted <- fn }
	app.client, app.agents, app.gen = clA, agent.List(), 1
	app.host.Name = "this Mac"
	app.watches = map[string]*hostWatch{"b": {host: hosts.Host{ID: "b", Name: "Server B"}, status: hostStatus{state: hostOnline}, cl: clB}}
	app.tok.pageTab = usageTabTokens
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(1000).Height(1100).Background(theme.Background).TextColor(theme.Text).Children(func() {
			app.tokensTab(c)
			app.confirmDialog(c)
		})
	}, 1000, 1100)
	frames := func(until func() bool, what string) {
		deadline := time.Now().Add(15 * time.Second)
		for !until() {
			select {
			case fn := <-posted:
				fn()
			case <-time.After(30 * time.Millisecond):
			}
			tt.Frame()
			if time.Now().After(deadline) {
				t.Fatalf("never: %s: %q", what, tt.Texts())
			}
		}
	}
	tt.Frame()
	frames(func() bool { return hasText(tt, "Server B") && hasText(tt, "Old VPS") }, "every host listed")
	if got := app.tok.data.Summary.Total.Input; got != 1200+1200+5000 {
		t.Fatalf("every host's input together: %d", got)
	}
	if err := tt.Click("Host"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool { return len(app.tok.data.Summary.Groups) == 3 }, "split by host")
	shot(t, tt, "tokens-overview.png")

	if err := tt.Click("Budgets"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Add budget"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("5000000"); err != nil {
		t.Fatal(err)
	}
	tt.Type("2000")
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool {
		var st proto.TokensSettings
		return clA.Call(proto.MethodTokensSettings, nil, &st) == nil && len(st.Budgets) == 1 && st.Budgets[0].Tokens == 2000
	}, "the budget kept")
	frames(func() bool { return hasText(tt, "75%") }, "the budget's use shown")
	shot(t, tt, "tokens-budgets.png")

	if err := tt.Click("Coverage"); err != nil {
		t.Fatal(err)
	}
	frames(func() bool { return hasText(tt, "Claude Code") && hasText(tt, "Not tracked") }, "the coverage table")
	shot(t, tt, "tokens-coverage.png")
}

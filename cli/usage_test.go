package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
)

// An agent asks what was spent: this host's tokens by agent, every host's
// together (the others by their synced daily totals), one other host's.
func TestUsageFromTheCommandLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "XDG_DATA_HOME"} {
		t.Setenv(v, "")
	}
	now := time.Now().UTC()
	transcript := filepath.Join(home, ".claude", "projects", "-work", "c.jsonl")
	os.MkdirAll(filepath.Dir(transcript), 0o700)
	os.WriteFile(transcript, []byte(fmt.Sprintf(`{"type":"assistant","timestamp":%q,"cwd":"/work","message":{"id":"m1","model":"claude-opus-5-5","usage":{"input_tokens":1000,"output_tokens":200}}}`+"\n", now.Format(time.RFC3339Nano))), 0o600)

	dir := t.TempDir()
	other := proto.TokensHostDays{Host: "vps-1", Name: "vps", Updated: now, Rows: []proto.TokenDayRow{
		{Day: now.Local().Format(time.DateOnly), Agent: "codex", Model: "gpt-5.1", TokenCounts: proto.TokenCounts{Input: 5000, Output: 100, Calls: 3}}}}
	data, _ := json.Marshal(other)
	os.MkdirAll(filepath.Join(dir, "settings", "tokens-hosts"), 0o700)
	os.WriteFile(filepath.Join(dir, "settings", "tokens-hosts", "vps-1.json"), data, 0o600)

	srv, err := daemon.NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a, b := net.Pipe()
	go srv.ServeConn(a)
	c, err := client.New(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Call(proto.MethodTokensSetSettings, proto.TokensSettings{On: true}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(proto.MethodTokensBackfill, proto.TokensBackfill{}, nil); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	r := runner{c: c, out: &out}
	if err := r.usageCmd([]string{"--by", "agent", "--since", "7d"}); err != nil {
		t.Fatal(err)
	}
	if s := out.String(); !strings.Contains(s, "Claude Code") || !strings.Contains(s, "1000 in") {
		t.Fatalf("by agent:\n%s", s)
	}
	out.Reset()
	if err := r.usageCmd([]string{"--by", "host"}); err != nil {
		t.Fatal(err)
	}
	if s := out.String(); !strings.Contains(s, "(this host)") || !strings.Contains(s, "vps (synced") || !strings.Contains(s, "6000 in") {
		t.Fatalf("every host:\n%s", s)
	}
	out.Reset()
	if err := r.usageCmd([]string{"--host", "vps", "--json"}); err != nil {
		t.Fatal(err)
	}
	var s proto.TokensSummary
	if json.Unmarshal(out.Bytes(), &s) != nil || s.Total.Input != 5000 || s.Host != "vps" {
		t.Fatalf("one host: %s", out.String())
	}
	if _, err := parseSince("2w", now); err != nil {
		t.Fatal(err)
	}
	if _, err := parseSince("soon", now); err == nil {
		t.Fatal("a bad --since taken")
	}
}

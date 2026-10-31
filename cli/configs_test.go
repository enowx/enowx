package cli

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
)

// Configs from the command line and over MCP: a model API saved, changed,
// renamed and listed, then removed.
func TestConfigsFromTheCommandLine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv, err := daemon.NewServer(t.TempDir())
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
	var out bytes.Buffer
	r := runner{c: c, out: &out}
	run := func(args ...string) string {
		t.Helper()
		out.Reset()
		if err := r.configsCmd(args); err != nil {
			t.Fatalf("%q: %v", args, err)
		}
		return out.String()
	}
	run("add", "claude", "GLM", "--url", "https://openrouter.ai/api", "--key-cred", "openrouter", "--model", "z-ai/glm-5,effort=high", "--model", "kimi-k3",
		"--env", "FOO=1", "--color", "#22aa66")
	run("edit", "claude", "glm", "--name", "GLM 5", "--small-model", "z-ai/glm-5-air")
	list := run("list", "--agent", "claude")
	if !strings.Contains(list, "GLM 5") || !strings.Contains(list, "https://openrouter.ai/api · z-ai/glm-5 · key openrouter") || !strings.Contains(list, "official") {
		t.Fatalf("list:\n%s", list)
	}
	cfg, err := r.config("claude", "GLM 5")
	if err != nil || cfg.API.SmallModel != "z-ai/glm-5-air" || len(cfg.API.Models) != 2 || cfg.Env[0] != "FOO=1" || cfg.Color != "#22aa66" {
		t.Fatalf("kept: %+v %v", cfg, err)
	}
	if err := r.configsCmd([]string{"add", "claude", "Nothing"}); err == nil {
		t.Fatal("a config of no kind")
	}
	res := r.callTool(configTools[2], runner.configsCmd, map[string]any{"agent": "claude", "name": "GLM 5", "models": []any{"z-ai/glm-5.1"}})
	if res["isError"] == true {
		t.Fatalf("configs_edit: %+v", res)
	}
	if cfg, _ = r.config("claude", "GLM 5"); cfg.API.Model != "z-ai/glm-5.1" || len(cfg.API.Models) != 1 {
		t.Fatalf("edited over MCP: %+v", cfg.API)
	}
	run("rm", "claude", "GLM 5")
	if list, _ := c.Accounts(); len(list) != 0 {
		t.Fatalf("left: %+v", list)
	}
}

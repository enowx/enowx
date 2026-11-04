package mcpclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestMain makes this test binary the fake server when run with FAKE_MCP set: the tests
// name os.Args[0] as a server's command.
func TestMain(m *testing.M) {
	switch mode := os.Getenv("FAKE_MCP"); mode {
	case "":
		os.Exit(m.Run())
	case "sleep": // a child a server starts
		time.Sleep(time.Minute)
	default:
		fakeServer(mode)
	}
}

// fakeServer speaks MCP on stdin and stdout. Each request runs on a goroutine of its own,
// so answers can come out of order, as a real server's do.
func fakeServer(mode string) {
	fmt.Fprintln(os.Stderr, "starting with", os.Getenv("FAKE_SECRET")) // what must go nowhere
	switch mode {
	case "die":
		fmt.Fprintln(os.Stderr, "fatal:", os.Getenv("FAKE_SECRET"))
		os.Exit(3)
	case "hang":
		time.Sleep(time.Hour)
	}
	logTo := func(line string) {
		if p := os.Getenv("FAKE_LOG"); p != "" {
			if f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				fmt.Fprintln(f, line)
				_ = f.Close()
			}
		}
	}
	logTo("start " + strconv.Itoa(os.Getpid()))
	if mode == "child" {
		c := exec.Command(os.Args[0])
		c.Env = append(os.Environ(), "FAKE_MCP=sleep")
		if err := c.Start(); err != nil {
			os.Exit(4)
		}
		logTo("child " + strconv.Itoa(c.Process.Pid))
	}
	var (
		mu      sync.Mutex
		release = make(chan struct{})
		once    sync.Once
	)
	write := func(v any) {
		b, _ := json.Marshal(v)
		mu.Lock()
		defer mu.Unlock()
		_, _ = os.Stdout.Write(append(b, '\n'))
	}
	lines := bufio.NewScanner(os.Stdin)
	lines.Buffer(make([]byte, 1<<20), 1<<24)
	fmt.Fprintln(os.Stdout, "a log line, not a message")
	for lines.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
				RequestID json.RawMessage `json:"requestId"`
				Cursor    string          `json:"cursor"`
			} `json:"params"`
		}
		if json.Unmarshal(lines.Bytes(), &req) != nil {
			continue
		}
		answer := func(result any) {
			write(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		}
		text := func(s string) map[string]any {
			return map[string]any{"content": []any{map[string]any{"type": "text", "text": s}}}
		}
		switch req.Method {
		case "initialize":
			answer(map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "fake", "version": "1"}})
		case "notifications/cancelled":
			logTo("cancelled " + string(req.Params.RequestID))
		case "tools/list":
			if req.Params.Cursor == "" {
				write(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"nextCursor": "more", "tools": []any{
					map[string]any{"name": "echo", "description": "Echoes.", "inputSchema": json.RawMessage(`{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"integer","maximum":12345678901234567890}}}`), "annotations": map[string]any{"readOnlyHint": true}},
					map[string]any{"name": "fail"}, map[string]any{"name": "image"},
				}}})
				continue
			}
			answer(map[string]any{"tools": []any{map[string]any{"name": "block"}, map[string]any{"name": "open"}, map[string]any{"name": "crash"}, map[string]any{"name": "serverask"}}})
		case "tools/call":
			switch req.Params.Name {
			case "echo":
				answer(text(string(req.Params.Arguments)))
			case "fail":
				answer(map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": "it went wrong"}}})
			case "image":
				answer(map[string]any{"content": []any{
					map[string]any{"type": "text", "text": "a picture"},
					map[string]any{"type": "image", "data": "aGk=", "mimeType": "image/png"},
					map[string]any{"type": "image", "data": "aGk=", "mimeType": "image/svg+xml"},
				}})
			case "block":
				go func() { <-release; answer(text("released")) }()
			case "open":
				once.Do(func() { close(release) })
				answer(text("opened"))
			case "crash":
				os.Exit(5)
			case "serverask":
				write(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "ping"})
				answer(text("asked"))
			default:
				write(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32602, "message": "unknown tool"}})
			}
		default:
			if len(req.ID) > 0 && req.Method != "" {
				write(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "no such method"}})
			}
			if len(req.ID) > 0 && req.Method == "" {
				logTo("client answered " + string(req.ID))
			}
		}
	}
	os.Exit(0)
}

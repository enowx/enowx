package cli

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/designbrief"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/version"
)

// enowx-app mcp serves enowX's tools to agents over MCP (JSON-RPC on
// stdin and stdout, a message a line): the traffic the host caught, the
// session's Browser, the host's Android devices, and other sessions. Each tool runs the command line's command of the
// same name, so both say the same.

// mcpTool is a tool: its schema, and the command line it runs.
type mcpTool struct {
	name, description string
	props             map[string]mcpProp
	required          []string
	args              func(a map[string]any) []string
	// image: the command writes a PNG to the path it is given last, which
	// the tool answers with, unless the agent asked for a path itself.
	image bool
}

type mcpProp struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// schemaOf is a property's JSON schema: an array's is of strings.
func schemaOf(p mcpProp) map[string]any {
	out := map[string]any{"type": p.Type, "description": p.Description}
	if p.Type == "array" {
		out["items"] = map[string]any{"type": "string"}
	}
	return out
}

func str(a map[string]any, k string) string {
	switch v := a[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

// withFlag adds --name value when the argument is given.
func withFlag(args []string, a map[string]any, k, name string) []string {
	if v := str(a, k); v != "" && v != "false" {
		if v == "true" {
			return append(args, "--"+name)
		}
		return append(args, "--"+name, v)
	}
	return args
}

var filterProp = mcpProp{"string", "A filter: domain:example.com (and subdomains), host:*.example.com, path:/api/*, method:POST, status:4xx, status:>=500, type:json|ws|grpc|sse, header:name=value, body:\"text\", size:>1mb, time:>500ms, since:10m, is:mocked|error|paused; -x negates."}

var mcpTools = []mcpTool{
	{name: "traffic_list", description: "List the HTTP exchanges enowX caught for this session (all of the host's with all=true), newest last: number, status, method, URL, type, size, time.",
		props: map[string]mcpProp{"filter": filterProp, "all": {"boolean", "Every source's traffic, not only this session's."}, "limit": {"number", "How many, newest (default 100)."}},
		args: func(a map[string]any) []string {
			return withFlag(withFlag(withFlag([]string{"list"}, a, "filter", "filter"), a, "all", "all"), a, "limit", "limit")
		}},
	{name: "traffic_show", description: "Show one exchange: request and response headers and bodies (decoded, secrets masked), and WebSocket, SSE or gRPC messages.",
		props:    map[string]mcpProp{"id": {"number", "The exchange's number, from traffic_list."}, "body": {"string", "Only this body, whole: request or response."}},
		required: []string{"id"},
		args:     func(a map[string]any) []string { return withFlag([]string{"show", str(a, "id")}, a, "body", "body") }},
	{name: "traffic_watch", description: "Wait for the next exchange matching a filter (or any), as an app makes it, and show it.",
		props: map[string]mcpProp{"filter": filterProp, "timeout_seconds": {"number", "How long to wait (default 60)."}, "all": {"boolean", "Every source's traffic."}},
		args: func(a map[string]any) []string {
			t := str(a, "timeout_seconds")
			if t == "" {
				t = "60"
			}
			return withFlag(withFlag([]string{"watch", "--until-match", "--timeout", t + "s"}, a, "filter", "filter"), a, "all", "all")
		}},
	{name: "traffic_mock", description: "Answer matching requests from a mock, to test an error path or an API not built yet. Saved in the project's rules (.enowx/traffic.json).",
		props: map[string]mcpProp{"method": {"string", "GET, POST… (comma separated)"}, "host": {"string", "api.example.com, *.example.com"}, "path": {"string", "A path prefix, or a glob with *"},
			"status": {"number", "The status to answer (default 200)."}, "body": {"string", "The body to answer."}, "header": {"string", "A header to answer, Name: value."},
			"times": {"number", "Answer only this many times."}, "delay_ms": {"number", "Wait this long first."}},
		args: func(a map[string]any) []string {
			args := []string{"mock"}
			for _, k := range [][2]string{{"method", "method"}, {"host", "host"}, {"path", "path"}, {"status", "status"}, {"body", "body"}, {"header", "header"}, {"times", "times"}, {"delay_ms", "delay"}} {
				args = withFlag(args, a, k[0], k[1])
			}
			return args
		}},
	{name: "traffic_breakpoint", description: "Pause matching requests (or their responses) until the user resumes them in enowX.",
		props: map[string]mcpProp{"method": {"string", "GET, POST…"}, "host": {"string", "A host"}, "path": {"string", "A path prefix or glob"}, "stage": {"string", "request, response or both"}},
		args: func(a map[string]any) []string {
			return withFlag(withFlag(withFlag(withFlag([]string{"breakpoint"}, a, "method", "method"), a, "host", "host"), a, "path", "path"), a, "stage", "stage")
		}},
	{name: "traffic_rules", description: "List the project's traffic rules: mocks, forwards, breakpoints, with their ids and hits.",
		args: func(map[string]any) []string { return []string{"rules"} }},
	{name: "traffic_rule_remove", description: "Remove a traffic rule by its id.",
		props: map[string]mcpProp{"id": {"string", "The rule's id, from traffic_rules."}}, required: []string{"id"},
		args: func(a map[string]any) []string { return []string{"rules", "rm", str(a, "id")} }},
	{name: "traffic_send", description: "Send an HTTP request from the host, through the rules unless direct, and show the exchange.",
		props: map[string]mcpProp{"method": {"string", "GET by default, POST with a body"}, "url": {"string", "https://…"}, "header": {"string", "A header, Name: value"},
			"body": {"string", "The request's body"}, "direct": {"boolean", "Around the rules"}},
		required: []string{"url"},
		args: func(a map[string]any) []string {
			args := []string{"send"}
			if m := str(a, "method"); m != "" {
				args = append(args, "-X", m)
			}
			if h := str(a, "header"); h != "" {
				args = append(args, "-H", h)
			}
			if b := str(a, "body"); b != "" {
				args = append(args, "-d", b)
			}
			args = withFlag(args, a, "direct", "direct")
			return append(args, str(a, "url"))
		}},
	{name: "traffic_export", description: "Export exchanges as a HAR file, secrets masked.",
		props: map[string]mcpProp{"filter": filterProp, "path": {"string", "Where to write it"}}, required: []string{"path"},
		args: func(a map[string]any) []string {
			return withFlag(withFlag([]string{"export"}, a, "filter", "filter"), a, "path", "out")
		}},
	{name: "traffic_attach", description: "Catch more of this session's traffic: terminal (a new tab), session (its new tabs), docker, browser (program chrome, chromium, edge, brave or firefox), electron (program: its executable or .app), android (program: a serial, optional), webview (the Browser tool's previews), manual.",
		props:    map[string]mcpProp{"kind": {"string", "terminal, session, docker, browser, electron, android, webview, manual"}, "program": {"string", "A browser's name, an Electron app's path, a device's serial"}},
		required: []string{"kind"},
		args: func(a map[string]any) []string {
			args := []string{"attach", str(a, "kind")}
			if p := str(a, "program"); p != "" {
				args = append(args, p)
			}
			return args
		}},
	{name: "traffic_clear", description: "Clear caught exchanges (matching a filter, or all).",
		props: map[string]mcpProp{"filter": filterProp},
		args:  func(a map[string]any) []string { return withFlag([]string{"clear"}, a, "filter", "filter") }},
	{name: "traffic_forward", description: "Send matching requests to another origin (staging, a local server), the path kept.",
		props:    map[string]mcpProp{"method": {"string", "GET, POST…"}, "host": {"string", "A host"}, "path": {"string", "A path prefix or glob"}, "to": {"string", "https://staging.example.com"}, "keep_host": {"boolean", "Keep the Host header"}},
		required: []string{"to"},
		args: func(a map[string]any) []string {
			return withFlag(withFlag(ruleMatch([]string{"forward"}, a), a, "to", "to"), a, "keep_host", "keep-host")
		}},
	{name: "traffic_fail", description: "Make matching requests fail: how is 503, close, reset or timeout; to test an app's error handling.",
		props:    map[string]mcpProp{"method": {"string", "GET, POST…"}, "host": {"string", "A host"}, "path": {"string", "A path prefix or glob"}, "how": {"string", "503, close, reset or timeout"}},
		required: []string{"how"},
		args:     func(a map[string]any) []string { return withFlag(ruleMatch([]string{"fail"}, a), a, "how", "how") }},
	{name: "traffic_transform", description: "Change matching requests or their responses on the way: headers set or removed, the status, text replaced, JSON fields set.",
		props: map[string]mcpProp{"method": {"string", "GET, POST…"}, "host": {"string", "A host"}, "path": {"string", "A path prefix or glob"},
			"req_set": {"string", "A request header to set, Name: value"}, "req_remove": {"string", "A request header to remove"},
			"res_set": {"string", "A response header to set, Name: value"}, "res_remove": {"string", "A response header to remove"},
			"res_status": {"number", "The response's status"}, "replace": {"string", "Text of the response replaced, FROM=>TO"}, "res_json": {"string", "A JSON field of the response, a.b=VALUE"}},
		args: func(a map[string]any) []string {
			args := ruleMatch([]string{"transform"}, a)
			for _, k := range [][2]string{{"req_set", "req-set"}, {"req_remove", "req-remove"}, {"res_set", "res-set"}, {"res_remove", "res-remove"}, {"res_status", "res-status"}, {"replace", "replace"}, {"res_json", "res-json"}} {
				args = withFlag(args, a, k[0], k[1])
			}
			return args
		}},
	{name: "traffic_settings", description: "The interceptor's settings, or changed: insecure hosts (certificates not checked), an upstream proxy, names pointed elsewhere (host=address), how much it keeps, error notices.",
		props: map[string]mcpProp{"insecure": {"string", "Hosts, comma separated"}, "upstream": {"string", "A proxy URL, \"\" for none"}, "host": {"string", "NAME=ADDRESS"},
			"max_exchanges": {"number", ""}, "max_mb": {"number", ""}, "notify_errors": {"string", "on or off"}},
		args: func(a map[string]any) []string {
			args := []string{"settings", "set"}
			for _, k := range [][2]string{{"insecure", "insecure"}, {"upstream", "upstream"}, {"host", "host"}, {"max_exchanges", "max-exchanges"}, {"max_mb", "max-mb"}, {"notify_errors", "notify-errors"}} {
				args = withFlag(args, a, k[0], k[1])
			}
			if len(args) == 2 {
				return []string{"settings"}
			}
			return args
		}},
	{name: "traffic_ca", description: "The CA programs must trust for enowX to read their HTTPS: its fingerprint and files, its PEM, or made anew.",
		props: map[string]mcpProp{"pem": {"boolean", "Its certificate as PEM"}, "new": {"boolean", "Make it anew"}},
		args: func(a map[string]any) []string {
			return withFlag(withFlag([]string{"ca"}, a, "pem", "pem"), a, "new", "new")
		}},
	{name: "traffic_pin", description: "Pin an exchange, kept when the rest is cleared (unpin to let it go).",
		props: map[string]mcpProp{"id": {"number", "The exchange's number"}, "unpin": {"boolean", "Unpin it"}}, required: []string{"id"},
		args: func(a map[string]any) []string {
			if str(a, "unpin") == "true" {
				return []string{"unpin", str(a, "id")}
			}
			return []string{"pin", str(a, "id")}
		}},
	{name: "traffic_snippet", description: "An exchange's request as code that sends it again: curl, fetch, python, go, httpie or powershell.",
		props: map[string]mcpProp{"id": {"number", "The exchange's number"}, "as": {"string", "curl, fetch, python, go, httpie or powershell"}}, required: []string{"id", "as"},
		args: func(a map[string]any) []string { return []string{"show", str(a, "id"), "--as", str(a, "as")} }},
	{name: "traffic_resume", description: "Let an exchange paused at a breakpoint go on, changed (status, body) or dropped.",
		props: map[string]mcpProp{"id": {"number", "Its number, from traffic_paused"}, "drop": {"boolean", "Drop it"}, "status": {"number", "Answer this status"}, "body": {"string", "With this body"}}, required: []string{"id"},
		args: func(a map[string]any) []string {
			return withFlag(withFlag(withFlag([]string{"resume", str(a, "id")}, a, "drop", "drop"), a, "status", "status"), a, "body", "body")
		}},
	{name: "traffic_paused", description: "The exchanges paused at breakpoints.",
		args: func(map[string]any) []string { return []string{"paused"} }},
	{name: "traffic_import", description: "Import a HAR file's exchanges.",
		props: map[string]mcpProp{"path": {"string", "The HAR file"}}, required: []string{"path"},
		args: func(a map[string]any) []string { return []string{"import", str(a, "path")} }},
}

// ruleMatch adds a rule's match flags.
func ruleMatch(args []string, a map[string]any) []string {
	return withFlag(withFlag(withFlag(args, a, "method", "method"), a, "host", "host"), a, "path", "path")
}

// browserTools drive the session's Browser in the app.
var browserTools = []mcpTool{
	{name: "browser_open", description: "Open a page in this session's Browser in enowX: a URL, a host, or a path of its dev server.",
		props: map[string]mcpProp{"url": {"string", "What to open"}}, required: []string{"url"},
		args: func(a map[string]any) []string { return []string{"open", str(a, "url")} }},
	{name: "browser_snapshot", description: "The Browser's page as text, its controls numbered for browser_click and browser_fill.",
		args: func(map[string]any) []string { return []string{"snapshot"} }},
	{name: "browser_click", description: "Click a control of the page: its number from browser_snapshot, or a CSS selector.",
		props: map[string]mcpProp{"ref": {"string", "The control"}}, required: []string{"ref"},
		args: func(a map[string]any) []string { return []string{"click", str(a, "ref")} }},
	{name: "browser_fill", description: "Type text into a field of the page.",
		props: map[string]mcpProp{"ref": {"string", "The field"}, "text": {"string", "What to type"}}, required: []string{"ref", "text"},
		args: func(a map[string]any) []string { return []string{"fill", str(a, "ref"), str(a, "text")} }},
	{name: "browser_screenshot", description: "A screenshot of the page, as an image (or saved where path says).",
		props: map[string]mcpProp{"path": {"string", "Where to write it (optional)"}}, image: true,
		args: func(a map[string]any) []string { return []string{"screenshot", str(a, "path")} }},
	{name: "browser_back", description: "Go back in the Browser's history.",
		args: func(map[string]any) []string { return []string{"back"} }},
	{name: "browser_forward", description: "Go forward in the Browser's history.",
		args: func(map[string]any) []string { return []string{"forward"} }},
	{name: "browser_reload", description: "Reload the Browser's page.",
		args: func(map[string]any) []string { return []string{"reload"} }},
	{name: "browser_unslop", description: "What makes the page in the Browser (the project's, running on localhost) look AI-made, by pass in the order to fix them, each with what to avoid and its elements, and a score from 100. In a redesign the app started, say the pass you begin.",
		props: map[string]mcpProp{"pass": {"string", "In a redesign the app started: the pass you begin, 1 to 12, or done"},
			"brief": {"string", "What the page is for, as Markdown, a ## section a field (" + designbrief.Sections() + "): written before pass 1, kept in the project"}},
		args: func(a map[string]any) []string {
			return withFlag(withFlag([]string{"unslop"}, a, "pass", "pass"), a, "brief", "brief")
		}},
	{name: "browser_eval", description: "Run JavaScript in the Browser's page and get its value: read the DOM, localStorage, a variable, what the console would show.",
		props: map[string]mcpProp{"code": {"string", "JavaScript; its last expression is the value"}}, required: []string{"code"},
		args: func(a map[string]any) []string { return []string{"eval", str(a, "code")} }},
	{name: "browser_capture", description: "Send the Browser's previews of the session's dev servers through Traffic (on), or not (off).",
		props: map[string]mcpProp{"on": {"boolean", "On or off"}}, required: []string{"on"},
		args: func(a map[string]any) []string {
			if str(a, "on") == "true" {
				return []string{"capture", "on"}
			}
			return []string{"capture", "off"}
		}},
}

// withSerial adds -s SERIAL when the agent names the device.
func withSerial(args []string, a map[string]any) []string {
	if s := str(a, "serial"); s != "" {
		return append(args, "-s", s)
	}
	return args
}

var serialProp = mcpProp{"string", "The device's serial (optional when one device is online)"}

// androidTools drive the host's Android devices.
var androidTools = []mcpTool{
	{name: "android_screenshot", description: "The device's screen, as an image (or saved where path says).",
		props: map[string]mcpProp{"serial": serialProp, "path": {"string", "Where to write it (optional)"}}, image: true,
		args: func(a map[string]any) []string { return append(withSerial([]string{"screenshot"}, a), str(a, "path")) }},
	{name: "android_controls", description: "What is on the device's screen: its controls numbered, with their text, id, state and center, for android_tap and android_type.",
		props: map[string]mcpProp{"serial": serialProp},
		args:  func(a map[string]any) []string { return withSerial([]string{"controls"}, a) }},
	{name: "android_tap", description: "Tap a control by its number from android_controls, or a point x, y of the screen; long for a long press.",
		props: map[string]mcpProp{"serial": serialProp, "ref": {"number", "A control's number"}, "x": {"number", "Or a point"}, "y": {"number", ""}, "long": {"boolean", "A long press"}},
		args: func(a map[string]any) []string {
			cmd := "tap"
			if str(a, "long") == "true" {
				cmd = "longpress"
			}
			args := withSerial([]string{cmd}, a)
			if ref := str(a, "ref"); ref != "" {
				return append(args, ref)
			}
			return append(args, str(a, "x"), str(a, "y"))
		}},
	{name: "android_swipe", description: "Swipe from x1, y1 to x2, y2: scroll a list (from low to high to scroll down), go back a page.",
		props:    map[string]mcpProp{"serial": serialProp, "x1": {"number", ""}, "y1": {"number", ""}, "x2": {"number", ""}, "y2": {"number", ""}, "ms": {"number", "How long (default 300)"}},
		required: []string{"x1", "y1", "x2", "y2"},
		args: func(a map[string]any) []string {
			return withFlag(withSerial([]string{"swipe", str(a, "x1"), str(a, "y1"), str(a, "x2"), str(a, "y2")}, a), a, "ms", "ms")
		}},
	{name: "android_type", description: "Type text (ASCII) into the field focused, or into control into (tapped first).",
		props: map[string]mcpProp{"serial": serialProp, "text": {"string", "What to type; a new line presses Enter"}, "into": {"number", "A field's number from android_controls"}}, required: []string{"text"},
		args: func(a map[string]any) []string {
			return append(withFlag(withSerial([]string{"type"}, a), a, "into", "into"), str(a, "text"))
		}},
	{name: "android_key", description: "Press a key: back, home, recents, enter, delete, tab, escape, up, down, left, right, menu, power, volume-up, volume-down, or an Android key code. An Android TV is used with its remote's: up, down, left, right, ok, menu, play-pause.",
		props: map[string]mcpProp{"serial": serialProp, "key": {"string", "The key"}}, required: []string{"key"},
		args: func(a map[string]any) []string { return append(withSerial([]string{"key"}, a), str(a, "key")) }},
	{name: "android_catch", description: "Send the device's HTTP through Traffic (traffic_list then shows it), its CA trusted: among the system's with root, else from Settings; off stops.",
		props: map[string]mcpProp{"serial": serialProp, "off": {"boolean", "Stop catching it"}},
		args:  func(a map[string]any) []string { return withFlag(withSerial([]string{"catch"}, a), a, "off", "off") }},
	{name: "android_connect", description: "Connect a phone over Wi-Fi (Developer options → Wireless debugging), pairing it the first time with the pairing address and code it shows.",
		props: map[string]mcpProp{"address": {"string", "Its address, 192.168.1.5:41823"}, "pair": {"string", "Pairing address, the first time"}, "code": {"string", "Pairing code"}}, required: []string{"address"},
		args: func(a map[string]any) []string {
			return withFlag(withFlag([]string{"connect", str(a, "address")}, a, "pair", "pair"), a, "code", "code")
		}},
	{name: "android_status", description: "The host's Android SDK, virtual phones (AVDs) and the devices adb sees, with their serials.",
		args: func(map[string]any) []string { return []string{"status"} }},
	{name: "android_start", description: "Start a virtual phone by its AVD name, from android_status.",
		props: map[string]mcpProp{"avd": {"string", "The AVD's name"}}, required: []string{"avd"},
		args: func(a map[string]any) []string { return []string{"start", str(a, "avd")} }},
	{name: "android_run", description: "Run this session's app (Flutter, Expo, React Native, Capacitor, NativePHP, Gradle) on a device, in a new tab of the session; prints the tab, to read with session_read.",
		props:    map[string]mcpProp{"serial": {"string", "The device's serial, from android_status"}, "command": {"string", "The command to run instead of the project's own"}},
		required: []string{"serial"},
		args: func(a map[string]any) []string {
			return withFlag([]string{"run", str(a, "serial")}, a, "command", "command")
		}},
	{name: "android_logcat", description: "A device's log so far, only an app's with package, at a level and above.",
		props:    map[string]mcpProp{"serial": {"string", "The device's serial"}, "package": {"string", "The app's package, com.example.app"}, "level": {"string", "V, D, I, W or E"}},
		required: []string{"serial"},
		args: func(a map[string]any) []string {
			return withFlag(withFlag([]string{"logcat", str(a, "serial")}, a, "package", "package"), a, "level", "level")
		}},
}

// sessionTools start other agents and follow them, and say where this
// session is at.
var sessionTools = []mcpTool{
	{name: "sessions_list", description: "The sessions on this host: ID, agent, state, project, branch, name.",
		args: func(map[string]any) []string { return []string{"ls"} }},
	{name: "session_spawn", description: "Start another coding agent in a session of its own, which the user sees in enowX; prints its ID.",
		props: map[string]mcpProp{"prompt": {"string", "Its task"}, "agent": {"string", "claude, codex, gemini, opencode… (default claude)"},
			"branch": {"string", "A new branch in a worktree of its own, so it doesn't touch this session's files"}, "yolo": {"boolean", "Skip its permission prompts"},
			"config": {"string", "The agent's config to run with (configs_list), official for the agent as installed; default the user's"}},
		required: []string{"prompt"},
		args: func(a map[string]any) []string {
			return append(withFlag(withFlag(withFlag(withFlag([]string{"spawn"}, a, "agent", "agent"), a, "branch", "branch"), a, "yolo", "yolo"), a, "config", "config"), str(a, "prompt"))
		}},
	{name: "session_wait", description: "Wait until a session's agent is done or asks something.",
		props: map[string]mcpProp{"id": {"string", "The session"}, "timeout": {"string", "How long at most, as 10m (default 30m)"}}, required: []string{"id"},
		args: func(a map[string]any) []string {
			return withFlag([]string{"wait", str(a, "id")}, a, "timeout", "timeout")
		}},
	{name: "session_read", description: "What a session's or a tab's screen shows.",
		props: map[string]mcpProp{"id": {"string", "The session or tab"}}, required: []string{"id"},
		args: func(a map[string]any) []string { return []string{"read", str(a, "id")} }},
	{name: "session_send", description: "Type a message into a session and send it.",
		props: map[string]mcpProp{"id": {"string", "The session"}, "text": {"string", "The message"}}, required: []string{"id", "text"},
		args: func(a map[string]any) []string { return []string{"send", str(a, "id"), str(a, "text")} }},
	{name: "session_note", description: "Say where this session is at, in a line shown under it in enowX (at milestones).",
		props: map[string]mcpProp{"text": {"string", "Where you are"}}, required: []string{"text"},
		args: func(a map[string]any) []string { return []string{"note", "self", str(a, "text")} }},
}

// sessionTools' kin: the Ports tool and the Run menu, and the rest of the
// command line, so no command is out of an agent's reach.
var moreTools = []mcpTool{
	{name: "ports_list", description: "The ports this session listens on: its dev servers.",
		args: func(map[string]any) []string { return []string{"ports"} }},
	{name: "run_list", description: "The project's commands, as the Run menu lists them: saved, run lately, found in its files.",
		args: func(map[string]any) []string { return []string{"run"} }},
	{name: "run_command", description: "Run one of the project's commands (its number or name from run_list) or any command, in a new tab of the session; session_read shows it.",
		props: map[string]mcpProp{"command": {"string", "A number, a name, or a command line"}}, required: []string{"command"},
		args: func(a map[string]any) []string { return []string{"run", str(a, "command")} }},
	{name: "agent_api_list", description: "The agents on this host that can use a model API of the user's (a gateway, OpenRouter, a local server), what each is set to (keys masked), and the backups of their settings.",
		args: func(map[string]any) []string { return []string{"api"} }},
	{name: "agent_api_set", description: "Point an agent that takes a model API only in its own settings at one the user gave (others, as Claude Code and Codex, take it as a config: configs_add): its address, key and model, written into the agent's settings, which are backed up first. Sessions of it started afterwards use it.",
		props: map[string]mcpProp{"agent": {"string", "The agent's id: claude, codex, opencode… (agent_api_list says what each takes)"}, "url": {"string", "The API's base URL"},
			"key": {"string", "Its API key"}, "models": {"array", "Its models, the first the default, each ID[,vision][,context=TOKENS][,output=TOKENS][,effort=E] as the agent takes them"},
			"small_model": {"string", "A model for quick work, where the agent has one"}, "effort": {"string", "The reasoning effort for every model, where the agent takes one for all"},
			"protocol": {"string", "anthropic, openai, openai-responses or gemini, as the agent speaks"}},
		required: []string{"agent", "url"},
		args: func(a map[string]any) []string {
			args := []string{"set", str(a, "agent")}
			for _, k := range [][2]string{{"url", "url"}, {"key", "key"}, {"small_model", "small-model"}, {"effort", "effort"}, {"protocol", "protocol"}} {
				args = withFlag(args, a, k[0], k[1])
			}
			list, _ := a["models"].([]any)
			for _, m := range list {
				args = append(args, "--model", fmt.Sprint(m))
			}
			return append([]string{"api"}, args...)
		}},
	{name: "agent_api_clear", description: "Set an agent back to its own API, the rest of its settings kept (backed up first).",
		props: map[string]mcpProp{"agent": {"string", "The agent's id"}}, required: []string{"agent"},
		args: func(a map[string]any) []string { return []string{"api", "clear", str(a, "agent")} }},
	{name: "agent_api_restore", description: "Put an agent's settings back as a backup kept them: without backup, as the user had them before enowX changed them.",
		props: map[string]mcpProp{"agent": {"string", "The agent's id"}, "backup": {"string", "A backup's id, from agent_api_list"}}, required: []string{"agent"},
		args: func(a map[string]any) []string {
			args := []string{"api", "restore", str(a, "agent")}
			if b := str(a, "backup"); b != "" {
				args = append(args, b)
			}
			return args
		}},
	{name: "enowx", description: "Run any enowx-app command, as on the command line: [\"help\"] lists them all, [\"traffic\", \"help\"] and [\"android\", \"help\"] theirs.",
		props: map[string]mcpProp{"args": {"array", "The command and its arguments, one string each"}}, required: []string{"args"},
		args: func(a map[string]any) []string {
			list, _ := a["args"].([]any)
			var out []string
			for _, v := range list {
				out = append(out, fmt.Sprint(v))
			}
			return out
		}},
}

// mcpGroup is tools with the command line's command each runs under.
type mcpGroup struct {
	tools []mcpTool
	run   func(r runner, args []string) error
}

// mcpGroups are every tool; a function, as the commands they run serve
// mcp too.
func mcpGroups() []mcpGroup {
	return []mcpGroup{
		{mcpTools, runner.traffic},
		{captureTools, runner.traffic},
		{browserTools, runner.browser},
		{androidTools, runner.android},
		{onAnyHost(nil, sessionTools, "session_note"), runner.command},
		{onAnyHost(nil, hibernateTools), runner.command},
		{moreTools, runner.command},
		{noteTools, runner.notes},
		{noteHistoryTools, runner.notes},
		{noteAgentTools, runner.notes},
		{iconTools, runner.icons},
		{notesIOTools, runner.notes},
		{onAnyHost([]string{"project"}, cloneTools), runner.command},
		{syncTools, runner.sync},
		{featureTools, runner.features},
		{notifyTools, runner.notify},
		{hookTools, runner.hooks},
		{accountTools, runner.accountsCmd},
		{configTools, runner.configsCmd},
		{ragTools, runner.ragCmd},
		{decideTools, runner.decideCmd},
		{attachTools, runner.notes},
		{contextTools, runner.projectCmd},
		{canvasTools, runner.canvasCmd},
		{projectLinkTools, runner.projectCmd},
		{onAnyHost([]string{"project"}, rulesTools), runner.command},
		{peekTools, runner.command},
		{teamChatTools, runner.team},
		{credsTools, runner.creds},
		{controlTools, runner.command},
	}
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// mcp serves the tools on in and out until in ends.
func (r runner) mcp(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	enc := json.NewEncoder(out)
	reply := func(id json.RawMessage, result any, rpcErr any) {
		msg := map[string]any{"jsonrpc": "2.0", "id": id}
		if rpcErr != nil {
			msg["error"] = rpcErr
		} else {
			msg["result"] = result
		}
		enc.Encode(msg)
	}
	tools := map[string]mcpTool{}
	run := map[string]func(runner, []string) error{}
	groups, instructions := mcpGroups(), agent.Guide
	if r.c != nil && os.Getenv(proto.EnvNotesPropose) == "" {
		// The features the user has off serve no tools and aren't told of.
		if g, err := r.c.FeaturesGuide(proto.FeaturesParams{Session: os.Getenv(EnvSession)}); err == nil && g.Guide != "" {
			instructions = g.Guide
			for i := range groups {
				groups[i].tools = slices.DeleteFunc(slices.Clone(groups[i].tools), func(t mcpTool) bool {
					f := agent.FeatureOfTool(t.name)
					return f != "" && !g.On[f]
				})
			}
		}
	}
	if os.Getenv(proto.EnvCanvasChat) != "" {
		// The Canvas page's agent: its canvases, nothing else.
		instructions = agent.CanvasChatGuide + agent.CanvasGuide
		groups = []mcpGroup{{canvasTools, runner.canvasCmd}}
	}
	if os.Getenv(proto.EnvNotesPropose) != "" {
		instructions = agent.NotesGuide
		// The Notes page's agent: its notes, nothing else.
		groups = []mcpGroup{
			{slices.DeleteFunc(slices.Clone(noteTools), func(t mcpTool) bool { return t.name == "notes_delete" }), runner.notes},
			{iconTools, runner.icons},
			{slices.DeleteFunc(slices.Clone(attachTools), func(t mcpTool) bool { return t.name != "notes_projects" }), runner.notes},
			{contextTools, runner.projectCmd},
		}
	}
	for _, g := range groups {
		for _, t := range g.tools {
			tools[t.name], run[t.name] = t, g.run
		}
	}
	for sc.Scan() {
		var m rpcMessage
		if json.Unmarshal(sc.Bytes(), &m) != nil || m.ID == nil {
			continue // notifications need no answer
		}
		switch m.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			json.Unmarshal(m.Params, &p)
			if p.ProtocolVersion == "" {
				p.ProtocolVersion = "2025-06-18"
			}
			reply(m.ID, map[string]any{
				"protocolVersion": p.ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "enowx", "version": version.String},
				"instructions":    instructions,
			}, nil)
		case "ping":
			reply(m.ID, map[string]any{}, nil)
		case "tools/list":
			var list []any
			for _, g := range groups {
				for _, t := range g.tools {
					props := map[string]any{}
					for k, p := range t.props {
						props[k] = schemaOf(p)
					}
					schema := map[string]any{"type": "object", "properties": props}
					if len(t.required) > 0 {
						schema["required"] = t.required
					}
					list = append(list, map[string]any{"name": t.name, "description": t.description, "inputSchema": schema})
				}
			}
			reply(m.ID, map[string]any{"tools": list}, nil)
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if err := json.Unmarshal(m.Params, &p); err != nil {
				reply(m.ID, nil, map[string]any{"code": -32602, "message": err.Error()})
				continue
			}
			t, ok := tools[p.Name]
			if !ok {
				reply(m.ID, nil, map[string]any{"code": -32602, "message": "no tool " + p.Name})
				continue
			}
			reply(m.ID, r.callTool(t, run[t.name], p.Arguments), nil)
		default:
			reply(m.ID, nil, map[string]any{"code": -32601, "message": fmt.Sprintf("no method %s", m.Method)})
		}
	}
	return sc.Err()
}

// callTool runs a tool's command line and says what it printed, with the
// image it wrote for a tool that makes one.
func (r runner) callTool(t mcpTool, run func(runner, []string) error, a map[string]any) map[string]any {
	result := func(text string, failed bool) map[string]any {
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "isError": failed}
	}
	for _, k := range t.required {
		if v, ok := a[k]; !ok || v == nil || v == "" {
			return result(k+" is required", true)
		}
	}
	tmp := ""
	if t.image && str(a, "path") == "" {
		f, err := os.CreateTemp("", "enowx-*.png")
		if err != nil {
			return result(err.Error(), true)
		}
		f.Close()
		tmp = f.Name()
		defer os.Remove(tmp)
		a = maps.Clone(a)
		if a == nil {
			a = map[string]any{}
		}
		a["path"] = tmp
	}
	var buf bytes.Buffer
	rr := runner{c: r.c, out: &buf}
	err := run(rr, t.args(a))
	text := strings.TrimSpace(buf.String())
	if err != nil {
		text = strings.TrimSpace(text + "\n" + err.Error())
	}
	if text == "" {
		text = "Done."
	}
	if tmp != "" && err == nil {
		png, rerr := os.ReadFile(tmp)
		if rerr == nil && len(png) > 0 {
			return map[string]any{"content": []any{
				map[string]any{"type": "image", "data": base64.StdEncoding.EncodeToString(png), "mimeType": "image/png"},
			}, "isError": false}
		}
	}
	return result(text, err != nil)
}

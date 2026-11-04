package mcpclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/core/platformagent/tools"
)

// maxToolName is the longest name the Messages API takes for a tool.
const maxToolName = 64

// mcpTool is a server's tool as the agent's own.
type mcpTool struct {
	conn        *conn
	name        string
	remote      string
	description string
	schema      json.RawMessage
	// hintReadOnly is what the server says of it: no ground for allowing anything, since
	// a server would be granting itself no prompts, but enough to run its calls together.
	hintReadOnly bool
}

// buildTools names listed's tools and sorts them. The same tools give the same bytes,
// whatever order they came in: the prompt cache's prefix holds them.
func buildTools(listed []listedTool) []tools.Tool {
	slices.SortStableFunc(listed, func(a, b listedTool) int {
		return strings.Compare(a.conn.cfg.name+"\x00"+a.Name, b.conn.cfg.name+"\x00"+b.Name)
	})
	taken := map[string]bool{}
	var out []tools.Tool
	for _, l := range listed {
		if l.Name == "" {
			continue
		}
		name := toolName(l.conn.cfg.name, l.Name)
		if taken[name] {
			name = hashedToolName(l.conn.cfg.name, l.Name)
			if taken[name] {
				continue // the same tool twice
			}
		}
		taken[name] = true
		out = append(out, &mcpTool{
			conn: l.conn, name: name, remote: l.Name,
			description:  strings.TrimSpace(l.Description),
			schema:       canonicalSchema(l.InputSchema),
			hintReadOnly: l.Annotations.ReadOnlyHint,
		})
	}
	slices.SortFunc(out, func(a, b tools.Tool) int { return strings.Compare(a.Name(), b.Name()) })
	return out
}

var notNameChar = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func sanitize(s string) string { return notNameChar.ReplaceAllString(s, "_") }

// serverPart is a server's name as it goes in a tool's. Only enowX's own may begin
// mcp__enowx__, which is what GOD of Agents goes by: "enowx." or "enowx__x" would pass
// for it once sanitized.
func serverPart(server string) string {
	s := sanitize(server)
	if s != "enowx" && strings.HasPrefix(s+"__", "enowx__") {
		return "srv_" + s
	}
	return s
}

// toolName is mcp__<server>__<tool>, sanitized as Claude Code does, or when that's too
// long its fallback.
func toolName(server, tool string) string {
	name := "mcp__" + serverPart(server) + "__" + sanitize(tool)
	if len(name) <= maxToolName {
		return name
	}
	return hashedToolName(server, tool)
}

// hashedToolName is the name shortened to fit, a hash of the whole keeping it apart from
// any other and the same on every start. A server's name is kept whole where it fits, so
// its prefix still says whose tool it is.
func hashedToolName(server, tool string) string {
	sum := sha256.Sum256([]byte(server + "\x00" + tool))
	suffix := "_" + hex.EncodeToString(sum[:4])
	s, t := serverPart(server), sanitize(tool)
	room := maxToolName - len("mcp__") - len("__") - len(suffix)
	half := room / 2
	switch {
	case len(s)+len(t) <= room:
	case len(s) <= half:
		t = t[:room-len(s)]
	case len(t) <= half:
		s = s[:room-len(t)]
	default:
		s, t = s[:half], t[:room-half]
	}
	return "mcp__" + s + "__" + t + suffix
}

// canonicalSchema is a tool's input schema with sorted keys and its numbers as written,
// the same bytes however a server orders them, always an object as the API requires.
func canonicalSchema(raw json.RawMessage) json.RawMessage {
	schema := map[string]any{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if dec.Decode(&schema) != nil || schema == nil {
		schema = map[string]any{}
	}
	if len(schema) == 0 {
		schema["properties"] = map[string]any{}
	}
	if _, ok := schema["type"]; !ok {
		schema["type"] = "object"
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if enc.Encode(schema) != nil {
		return json.RawMessage(`{"properties":{},"type":"object"}`)
	}
	return json.RawMessage(bytes.TrimSpace(b.Bytes()))
}

func (t *mcpTool) Name() string            { return t.name }
func (t *mcpTool) Description() string     { return t.description }
func (t *mcpTool) Schema() json.RawMessage { return t.schema }

// ReadOnly is false whatever the server says: see hintReadOnly.
func (t *mcpTool) ReadOnly() bool { return false }

// SideBySide says a server calls it read-only.
func (t *mcpTool) SideBySide() bool { return t.hintReadOnly }

// Edits is false: nothing says what a server's tool touches.
func (t *mcpTool) Edits() bool { return false }

// secretField names an argument whose value is never shown.
var secretField = regexp.MustCompile(`(?i)token|secret|passw|api[-_]?key|credential|authorization|cookie|private[-_]?key`)

// Detail is one line: the server, its tool and the arguments, cut short, secrets hidden.
func (t *mcpTool) Detail(input json.RawMessage) string {
	head := t.conn.cfg.name + " " + t.remote
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.UseNumber()
	var args map[string]any
	if dec.Decode(&args) != nil || len(args) == 0 {
		return oneLine(head, 200)
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		value := "[hidden]"
		if !secretField.MatchString(k) {
			value = t.conn.cfg.hide(diag.Scrub(oneLine(argText(args[k]), 60)))
		}
		parts = append(parts, k+"="+value)
	}
	return oneLine(head+": "+strings.Join(parts, ", "), 200)
}

func argText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "?"
	}
	return string(b)
}

// Run calls the tool on its server. env isn't used: the server runs with what it was
// started with. A failure is a Failed result, what the model can act on.
func (t *mcpTool) Run(ctx context.Context, input json.RawMessage, _ tools.Env) tools.Result {
	args := input
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage(`{}`)
	}
	raw, err := t.conn.call(ctx, "tools/call", map[string]any{"name": t.remote, "arguments": args})
	if err != nil {
		return tools.Result{Text: t.failure(ctx, err), Failed: true}
	}
	return resultOf(raw)
}

// failure words what became of a call, without any value of the server's config.
func (t *mcpTool) failure(ctx context.Context, err error) string {
	name := t.conn.cfg.name
	var gone *goneError
	switch {
	case ctx.Err() != nil:
		return fmt.Sprintf("Interrupted: MCP server %s was told to stop the call.", name)
	case errors.Is(err, errClosed):
		return fmt.Sprintf("MCP server %s is closed.", name)
	case errors.As(err, &gone):
		next := "it can't be started again."
		if t.conn.cfg.kind != kindHTTP {
			next = "the next call starts it again."
		}
		return fmt.Sprintf("MCP server %s stopped during the call (%s): the call may have partly run; %s", name, t.conn.cfg.hide(gone.why), next)
	}
	return fmt.Sprintf("MCP server %s: %s", name, t.conn.cfg.hide(err.Error()))
}

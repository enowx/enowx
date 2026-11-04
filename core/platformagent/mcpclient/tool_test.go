package mcpclient

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/core/platformagent/tools"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func TestToolNames(t *testing.T) {
	long := strings.Repeat("x", 80)
	cases := []struct{ server, tool, want string }{
		{"enowx", "session_spawn", "mcp__enowx__session_spawn"},
		{"my.server", "a tool!", "mcp__my_server__a_tool_"},
		{"a-b_C9", "t-1", "mcp__a-b_C9__t-1"},
	}
	for _, c := range cases {
		if got := toolName(c.server, c.tool); got != c.want {
			t.Errorf("%s %s: %s", c.server, c.tool, got)
		}
	}
	for _, c := range [][2]string{{long, "t"}, {"s", long}, {long, long}, {"enowx", long}, {"ünï", "çödé"}} {
		got := toolName(c[0], c[1])
		if !validName.MatchString(got) {
			t.Errorf("%q %q: invalid %q", c[0], c[1], got)
		}
		if got != toolName(c[0], c[1]) {
			t.Errorf("not stable: %q", got)
		}
	}
	if toolName(long, long) == toolName(long, long+"y") {
		t.Error("two long names came out the same")
	}
}

// GOD of Agents lets through mcp__enowx__*: enowX's own server fits it even when long,
// and no other server's name can pass for it.
func TestGOAKnowsEnowXsToolsAlone(t *testing.T) {
	for _, tool := range []string{"session_spawn", strings.Repeat("x", 80)} {
		if ok, why := goa.Allowed(toolName("enowx", tool), "", ""); !ok {
			t.Errorf("enowx's %s: %s", tool, why)
		}
	}
	for _, server := range []string{"enowx.", "enowx__x", "enowx_", "enowx!!x", "enowxy", "other"} {
		if ok, _ := goa.Allowed(toolName(server, "t"), "", ""); ok {
			t.Errorf("server %q passed for enowX's: %s", server, toolName(server, "t"))
		}
	}
}

func TestNamesThatCollideStayApart(t *testing.T) {
	c := newConn(serverConfig{name: "s"}, 0)
	got := buildTools([]listedTool{{conn: c, Name: "a.b"}, {conn: c, Name: "a_b"}, {conn: c, Name: "a_b"}, {conn: c, Name: ""}})
	if len(got) != 2 || got[0].Name() == got[1].Name() {
		t.Fatalf("tools: %d", len(got))
	}
	for _, tool := range got {
		if !validName.MatchString(tool.Name()) {
			t.Errorf("invalid: %s", tool.Name())
		}
	}
}

func TestDetailIsOneLineWithoutSecrets(t *testing.T) {
	const key = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	tool := &mcpTool{remote: "login", conn: newConn(serverConfig{name: "web", secrets: []string{"configured-value"}}, 0)}
	input, _ := json.Marshal(map[string]any{
		"user":     "line one\nline two",
		"password": "correct horse battery",
		"note":     "uses configured-value and " + key,
		"count":    3,
	})
	got := tool.Detail(input)
	if strings.Contains(got, "\n") || len(got) > 300 {
		t.Errorf("not one short line: %q", got)
	}
	for _, leak := range []string{"correct horse", "configured-value", key} {
		if strings.Contains(got, leak) {
			t.Errorf("%q holds a secret: %q", got, leak)
		}
	}
	for _, want := range []string{"web login: ", "count=3", "user=line one line two", "password=[hidden]"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q lacks %q", got, want)
		}
	}
	if got := tool.Detail(nil); got != "web login" {
		t.Errorf("no input: %q", got)
	}
}

func TestBadSchemasBecomeObjects(t *testing.T) {
	want := `{"properties":{},"type":"object"}`
	for _, raw := range []string{"", "null", "[1]", "{broken", "{}"} {
		if got := string(canonicalSchema(json.RawMessage(raw))); got != want {
			t.Errorf("%q: %s", raw, got)
		}
	}
	if got := string(canonicalSchema(json.RawMessage(`{"b":"<x>","properties":{}}`))); got != `{"b":"<x>","properties":{},"type":"object"}` {
		t.Errorf("schema: %s", got)
	}
}

func imageResult(t *testing.T, blocks ...map[string]any) tools.Result {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"content": blocks})
	if err != nil {
		t.Fatal(err)
	}
	return resultOf(raw)
}

// A picture that would fail every later request stays out, a note in its place.
func TestResultImagesAreBounded(t *testing.T) {
	image := func(data string) map[string]any {
		return map[string]any{"type": "image", "mimeType": "image/png", "data": data}
	}
	ok := base64.StdEncoding.EncodeToString(make([]byte, 300))
	big := strings.Repeat("A", tools.ImageBase64Most+4)

	res := imageResult(t, image(ok), image(big), image("not base64!"))
	if len(res.Images) != 1 || !strings.Contains(res.Text, "over the limit") || !strings.Contains(res.Text, "isn't valid base64") {
		t.Errorf("%d images, text %q", len(res.Images), res.Text)
	}
	var many []map[string]any
	for range maxResultImages + 2 {
		many = append(many, image(ok))
	}
	res = imageResult(t, many...)
	if len(res.Images) != maxResultImages || !strings.Contains(res.Text, "at most") {
		t.Errorf("%d images, text %q", len(res.Images), res.Text)
	}
	large := strings.Repeat("A", tools.ImageBase64Most)
	res = imageResult(t, image(large), image(large), image(large), image(large), image(large))
	if total := len(res.Images) * len(large); total > maxImagesBase64 || !strings.Contains(res.Text, "too large together") {
		t.Errorf("%d bytes, text %q", total, res.Text)
	}
}

package libraryview

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
)

func TestSplitCommand(t *testing.T) {
	got := splitCommand(`npx -y "@scope/server files" --root '~/my code'`)
	want := []string{"npx", "-y", "@scope/server files", "--root", "~/my code"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}

// The test of an MCP server says each way it can go, in the panel's own words.
func TestMCPTestPanelStates(t *testing.T) {
	item := proto.EnvItem{ID: "m1", Name: "coolify"}
	for _, tc := range []struct {
		name string
		test mcpTesting
		want []string
	}{
		{"idle", mcpTesting{}, []string{"Test on this Mac", "Runs it where its agent would and lists its tools."}},
		{"running", mcpTesting{forID: "m1", running: true}, []string{"Starting it on this Mac…"}},
		{"error", mcpTesting{forID: "m1", err: "no connection"}, []string{"no connection"}},
		{"answers", mcpTesting{forID: "m1", result: &proto.MCPTest{OK: true, Server: "coolify 1.0", Took: 1200, Tools: []proto.MCPTool{{Name: "deploy", Description: "Deploy an app"}}}},
			[]string{"coolify 1.0 lists 1 tool in 1.2 s", "deploy"}},
		{"fails", mcpTesting{forID: "m1", result: &proto.MCPTest{Error: "it exited", Stderr: "npx: not found"}}, []string{"it exited", "npx: not found"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := &View{test: tc.test}
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Width(600).Height(400).Children(func() { v.mcpTestPanel(c, Options{Host: "this Mac"}, item) })
			}, 600, 400)
			tt.Frame()
			for _, want := range tc.want {
				if !tt.HasText(want) {
					t.Errorf("%q isn't shown: %q", want, tt.Texts())
				}
			}
		})
	}
}

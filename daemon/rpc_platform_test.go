package daemon

import (
	"os"
	"testing"

	"github.com/enowx/enowx/proto"
)

// The prompt is read from its file, kept unless empty, too long or holding a secret, and
// reset removes it.
func TestPlatformPrompt(t *testing.T) {
	srv := startServer(t)
	c := connect(t, srv, nil)
	var got proto.PlatformPrompt
	if err := c.Call(proto.MethodPlatformPrompt, nil, &got); err != nil || got.Custom || got.Text != "" || got.Path == "" {
		t.Fatalf("none yet: %+v %v", got, err)
	}
	if err := c.Call(proto.MethodPlatformPromptSave, proto.PlatformPrompt{Text: "Be brief.\n"}, &got); err != nil || !got.Custom || got.Text != "Be brief.\n" {
		t.Fatalf("saved: %+v %v", got, err)
	}
	data, err := os.ReadFile(got.Path)
	if err != nil || string(data) != "Be brief.\n" {
		t.Fatalf("the file: %q %v", data, err)
	}
	if err := c.Call(proto.MethodPlatformPromptSave, proto.PlatformPrompt{Text: "   "}, nil); err == nil {
		t.Fatal("saved empty")
	}
	if err := c.Call(proto.MethodPlatformPromptSave, proto.PlatformPrompt{Text: string(make([]byte, maxPlatformPrompt+1))}, nil); err == nil {
		t.Fatal("saved too long")
	}
	if err := c.Call(proto.MethodPlatformPromptSave, proto.PlatformPrompt{Text: "key sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789abcdefghijkl"}, nil); err == nil {
		t.Fatal("saved a secret")
	}
	if err := c.Call(proto.MethodPlatformPrompt, nil, &got); err != nil || got.Text != "Be brief.\n" {
		t.Fatalf("a refusal changed it: %+v %v", got, err)
	}
	var reset proto.PlatformPrompt
	if err := c.Call(proto.MethodPlatformPromptSave, proto.PlatformPrompt{Reset: true}, &reset); err != nil || reset.Custom || reset.Text != "" {
		t.Fatalf("reset: %+v %v", reset, err)
	}
	if _, err := os.Stat(reset.Path); !os.IsNotExist(err) {
		t.Fatalf("the file stays: %v", err)
	}
}

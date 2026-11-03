package onboardview

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// With SHOTS set, keeps a picture of each moment in SHOTS_AT (milliseconds, comma
// separated, as the design's are named), to compare with the design's.
func TestPictures(t *testing.T) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		t.Skip("SHOTS isn't set")
	}
	widget.Frosted = true
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	style := os.Getenv("SHOTS_STYLE")
	for _, at := range strings.Split(os.Getenv("SHOTS_AT"), ",") {
		ms, err := strconv.ParseFloat(at, 32)
		if err != nil {
			t.Fatal(err)
		}
		theme.UseStyle(theme.StyleByID(style))
		v := New()
		o := pictureOptions(style)
		v.begin(o)
		v.pictureAt(float32(ms), o)
		tt := ui.NewTester(func(c *ui.Context) { v.Draw(c, o) }, 1440, 900)
		tt.Advance(16 * time.Millisecond) // what's measured from last frame's places
		name := fmt.Sprintf("frame-%sms.png", at)
		if style != "" {
			name = style + "-" + name
		}
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}

// pictureOptions are the design's sample, as Options.
func pictureOptions(style string) Options {
	return Options{
		Flow: FlowNew, HostName: "this Mac", Device: "this Mac", AgentsLoaded: true, StyleID: style,
		Agents: []proto.AgentInfo{
			{ID: "claude", Name: "Claude Code", Command: "claude", Installed: true},
			{ID: "codex", Name: "Codex", Command: "codex", Installed: true},
			{ID: "opencode", Name: "OpenCode", Command: "opencode", Installed: true},
			{ID: "gemini", Name: "Gemini CLI", Command: "gemini", Installed: true},
			{ID: "kiro", Name: "Kiro CLI", Command: "kiro-cli"},
			{ID: "copilot", Name: "GitHub Copilot", Command: "copilot"},
		},
		Projects: []proto.ProjectInfo{{Name: "enowx", Path: "~/code/enowx", IsGit: true, Branch: "main"}},
		Hosts:    []string{"enowxlabs"},
		OpenLink: func(string) {}, OpenFolder: func() {}, OpenClone: func() {}, AddHost: func(string) {},
		SetStyle: func(string) {}, SetPalette: func(string) {}, Done: func() {},
		BackgroundMode: BackgroundDefault, SetBackground: func(string) {}, PanelOpacity: 1, SetPanelOpacity: func(float32) {}, BlurValue: 30, BlurMost: 40, SetBlur: func(float32) {},
		GOAAgents: []proto.GOAAgent{{Agent: "claude", Name: "Claude Code", Installed: true}, {Agent: "codex", Name: "Codex", Installed: true}},
	}
}

// pictureAt puts the clock at ms, at the stop that shows.
func (v *View) pictureAt(ms float32, o Options) {
	v.clock.t, v.clock.target = ms, ms
	if ms > tStyle && ms < tStyleEnd {
		// The design's tour of the styles: a change partway, as if one was just picked.
		seq := []string{"liquid-glass", "ledger", "tonal", "bento", "liquid-glass", "tonal", "ledger", "bento", "tonal", "liquid-glass", "bento", "ledger", "liquid-glass"}
		i := int((ms - tStyle) / 1400)
		v.morph.began = time.Now().Add(-time.Duration(ms-tStyle-float32(i)*1400) * time.Millisecond)
		v.morph.from, v.morph.to = seq[i], seq[i+1]
		v.clock.t, v.clock.target = tStyle, tStyle
		ms = tStyle
	}
	for i, s := range v.stops {
		if s.at <= ms {
			v.at = i
		}
	}
	v.wireOn = true
	if ms >= 41250 {
		v.task.sent, v.task.prompt = true, "Add a short section on make dev to the README."
	}
}

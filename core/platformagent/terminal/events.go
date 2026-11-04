package terminal

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/enowx/enowx/core/platformagent/record"
)

// How much of a thinking block is kept to show it folded.
const (
	thinkKept  = 400
	thinkShown = 60
)

// Event implements platformagent.Front.
func (t *Terminal) Event(r record.Record) {
	t.post(func() { t.show(r) })
}

// show draws record r.
func (t *Terminal) show(r record.Record) {
	if r.Sidechain != "" {
		t.showSidechain(r)
		return
	}
	switch r.Type {
	case record.Meta:
		t.refresh()
	case record.BlockStart:
		t.endThinking()
		t.sc.EndStream()
	case record.Delta:
		t.turnMsg = r.Msg
		if r.Kind == "thinking" {
			t.thinkingDelta(r.Text)
			return
		}
		t.endThinking()
		t.sc.Stream(cleanText(r.Text), "")
	case record.Assistant:
		t.endThinking()
		if r.Msg == "" || r.Msg != t.turnMsg {
			// It didn't stream: its text comes whole.
			if text := AssistantText(r); text != "" {
				t.sc.Stream(cleanText(text), "")
			}
		}
		t.sc.EndStream()
		t.refresh()
	case record.ToolStart:
		t.endThinking()
		t.toolStart(r.Tool, "")
	case record.ToolEnd:
		t.toolEnd(r.Tool, "")
	case record.Notice:
		t.endThinking()
		t.notice(r.Level, r.Text)
	case record.Compact:
		t.sc.Line(t.sc.paint(styleDim, fmt.Sprintf("Compacted (%d tokens before)", r.Before)))
		t.refresh()
	case record.TurnEnd:
		t.endThinking()
		t.sc.EndStream()
		if r.Stop == "interrupted" && !t.saidStop {
			t.saidStop = true
			t.sc.Line(t.sc.paint(styleYellow, "Interrupted"))
		}
	}
}

// showSidechain draws what a subagent does, its tools alone, nested.
func (t *Terminal) showSidechain(r record.Record) {
	switch r.Type {
	case record.ToolStart:
		t.toolStart(r.Tool, "  ")
	case record.ToolEnd:
		t.toolEnd(r.Tool, "  ")
	}
}

// thinkingDelta shows thinking folded to one line, the latest of it.
func (t *Terminal) thinkingDelta(text string) {
	text = strings.ReplaceAll(cleanText(text), "\n", " ")
	if len(t.thinkHead) < thinkKept {
		t.thinkHead += text
	}
	t.thinkTail += text
	if r := []rune(t.thinkTail); len(r) > thinkShown {
		t.thinkTail = string(r[len(r)-thinkShown:])
	}
	t.sc.thinking = "✻ thinking: " + t.thinkTail
}

// endThinking leaves one line of the thinking that ended, its start.
func (t *Terminal) endThinking() {
	if t.sc.thinking == "" {
		return
	}
	t.sc.thinking = ""
	t.sc.Line(t.sc.paint(styleDim, clipCols("✻ thinking: "+oneLine(t.thinkHead), t.sc.width()-1)))
	t.thinkHead, t.thinkTail = "", ""
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// toolStart is a tool's first line.
func (t *Terminal) toolStart(tool *record.Tool, indent string) {
	if tool == nil {
		return
	}
	line := indent + "● " + oneLine(tool.Name)
	if d := oneLine(tool.Detail); d != "" {
		line += " " + d
	}
	t.sc.Line(t.sc.paint(styleCyan, clipCols(line, t.sc.width()-1)))
}

// toolEnd is a tool's last line, its result clipped.
func (t *Terminal) toolEnd(tool *record.Tool, indent string) {
	if tool == nil {
		return
	}
	mark, style := "✓", styleGreen
	if tool.Failed {
		mark, style = "✗", styleRed
	}
	line := indent + "  " + mark + " " + oneLine(tool.Name)
	if tool.Added > 0 || tool.Removed > 0 {
		line += fmt.Sprintf(" +%d -%d", tool.Added, tool.Removed)
	}
	if tool.DurationMs > 0 {
		line += fmt.Sprintf(" %dms", tool.DurationMs)
	}
	if res := oneLine(firstLine(cleanText(tool.Result))); res != "" {
		line += ": " + res
	}
	t.sc.Line(t.sc.paint(style, clipCols(line, t.sc.width()-1)))
}

// notice is a line for the user, coloured by its level.
func (t *Terminal) notice(level, text string) {
	style, mark := styleCyan, ""
	switch level {
	case "warning":
		style, mark = styleYellow, "! "
	case "error":
		style, mark = styleRed, "! "
	}
	t.sc.Line(t.sc.paint(style, mark+cleanText(text)))
}

// AssistantText is the text of an Assistant record's answer.
func AssistantText(r record.Record) string {
	var m struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(r.Message, &m) != nil {
		return ""
	}
	var parts []string
	for _, c := range m.Content {
		if c.Type == "text" && c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

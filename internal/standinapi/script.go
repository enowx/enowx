package standinapi

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Respond is the built-in script: it answers with thinking and text; a prompt that says
// "use a tool" calls the Bash and Read tools the request offers, then, given their
// results, finishes; a request for a <summary> gets one. A line "tool: NAME {input}" in a
// prompt calls that tool with that input, for a person driving a session by hand.
func Respond(req Request) Reply {
	var body struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Required []string `json:"required"`
			} `json:"input_schema"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(req.Body, &body)

	usage := Usage{Input: int64(len(req.Body)/4 + 1)}
	text, gotResults := lastUser(req)
	switch {
	case strings.Contains(text, "<summary>"):
		reply := Text("<summary>The stand-in's summary of the conversation.</summary>")
		reply.Usage = usage
		return reply
	case gotResults:
		reply := Text("The tools ran, and that's done.")
		reply.Usage = usage
		return reply
	}
	if blocks := namedCalls(text); len(blocks) > 0 {
		return Reply{Blocks: blocks, Usage: usage}
	}
	if strings.Contains(strings.ToLower(text), "use a tool") {
		blocks := []Block{Thinking("The user asked for a tool.", "sig-standin")}
		for i, t := range body.Tools {
			if t.Name != "Bash" && t.Name != "Read" {
				continue
			}
			blocks = append(blocks, ToolUse(fmt.Sprintf("toolu_standin_%d", i), t.Name, fillInput(t.InputSchema.Required)))
		}
		if len(blocks) > 1 {
			return Reply{Blocks: blocks, Usage: usage}
		}
	}
	return Reply{
		Blocks: []Block{
			Thinking("A plain question.", "sig-standin"),
			TextBlock("The stand-in API read: " + firstLine(text)),
		},
		Usage: usage,
	}
}

// namedCalls are the calls a prompt's "tool: NAME {input}" lines ask for.
func namedCalls(text string) []Block {
	var blocks []Block
	for i, line := range strings.Split(text, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "tool: ")
		if !ok {
			continue
		}
		name, input, ok := strings.Cut(rest, " ")
		if !ok {
			input = "{}"
		}
		blocks = append(blocks, ToolUse(fmt.Sprintf("toolu_named_%d", i), name, input))
	}
	return blocks
}

// lastUser is the text of the last user message, and whether it holds tool results.
func lastUser(req Request) (string, bool) {
	messages := req.Messages()
	if len(messages) == 0 {
		return "", false
	}
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(messages[len(messages)-1], &m) != nil {
		return "", false
	}
	var plain string
	if json.Unmarshal(m.Content, &plain) == nil {
		return plain, false
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	_ = json.Unmarshal(m.Content, &blocks)
	var texts []string
	results := false
	for _, b := range blocks {
		switch b.Type {
		case "text":
			texts = append(texts, b.Text)
		case "tool_result":
			results = true
		}
	}
	return strings.Join(texts, "\n"), results
}

// fillInput makes an input with the required fields set, plausible for common names.
func fillInput(required []string) string {
	input := map[string]string{}
	for _, name := range slices.Sorted(slices.Values(required)) {
		switch {
		case name == "command":
			input[name] = "echo standin"
		case strings.Contains(name, "path"), strings.Contains(name, "file"):
			input[name] = "README.md"
		case name == "pattern":
			input[name] = "*"
		default:
			input[name] = "standin"
		}
	}
	raw, _ := json.Marshal(input)
	return string(raw)
}

func firstLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	line := lines[len(lines)-1]
	if len(line) > 200 {
		line = line[:200]
	}
	return line
}

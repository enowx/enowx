package mcpclient

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/enowx/enowx/core/platformagent/tools"
)

// maxResultText bounds a result's text: all of it goes to the model on every later turn.
const maxResultText = 256 << 10

// modelImageTypes are the images the Messages API takes: another would fail every
// request from then on, the result being in the history.
var modelImageTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}

type contentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
	URI      string `json:"uri"`
	Name     string `json:"name"`
	Resource struct {
		URI  string `json:"uri"`
		Text string `json:"text"`
	} `json:"resource"`
}

// resultOf turns a tools/call answer into a tool's result.
func resultOf(raw json.RawMessage) tools.Result {
	var answer struct {
		Content           []contentBlock  `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		IsError           bool            `json:"isError"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return tools.Result{Text: "The MCP server's answer can't be read.", Failed: true}
	}
	res := tools.Result{Failed: answer.IsError}
	var texts []string
	for _, b := range answer.Content {
		switch b.Type {
		case "text":
			texts = append(texts, b.Text)
		case "image":
			if modelImageTypes[b.MimeType] && b.Data != "" {
				res.Images = append(res.Images, tools.Image{MediaType: b.MimeType, Base64: b.Data})
			} else {
				texts = append(texts, fmt.Sprintf("[an image of type %q, which can't be shown to the model]", b.MimeType))
			}
		case "resource":
			if b.Resource.Text != "" {
				texts = append(texts, b.Resource.Text)
			} else {
				texts = append(texts, "[resource "+b.Resource.URI+"]")
			}
		case "resource_link":
			texts = append(texts, strings.TrimSpace("[resource "+b.Name+" "+b.URI+"]"))
		default:
			texts = append(texts, "["+b.Type+" content, which can't be shown to the model]")
		}
	}
	if len(answer.Content) == 0 && len(answer.StructuredContent) > 0 {
		texts = append(texts, string(answer.StructuredContent))
	}
	res.Text = strings.Join(texts, "\n")
	if len(res.Text) > maxResultText {
		res.Text = strings.ToValidUTF8(res.Text[:maxResultText], "") + "\n[the rest of this result was cut]"
	}
	return res
}

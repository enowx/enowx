package mcpclient

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/enowx/enowx/core/platformagent/tools"
)

// maxResultText bounds a result's text: all of it goes to the model on every later turn.
const maxResultText = 256 << 10

// A picture over the API's limit, or too many, stays in the history and fails every later
// request until /clear. Their sum is kept well under the API's limit on a whole request.
const (
	maxResultImages = 8
	maxImagesBase64 = 20 << 20
)

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

// refusedImage says why b can't go to the model, count pictures of size bytes being there
// already; "" when it can.
func refusedImage(b contentBlock, count, size int) string {
	switch {
	case !modelImageTypes[b.MimeType] || b.Data == "":
		return fmt.Sprintf("an image of type %q, which can't be shown to the model", b.MimeType)
	case len(b.Data) > tools.ImageBase64Most:
		return fmt.Sprintf("an image of %d MB, over the limit of %d MB", len(b.Data)>>20, tools.ImageBase64Most>>20)
	case count >= maxResultImages:
		return fmt.Sprintf("an image left out: at most %d go with a result", maxResultImages)
	case size+len(b.Data) > maxImagesBase64:
		return "an image left out: the result's images are too large together"
	}
	if _, err := base64.StdEncoding.DecodeString(b.Data); err != nil {
		return "an image whose data isn't valid base64"
	}
	return ""
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
	imagesSize := 0
	for _, b := range answer.Content {
		switch b.Type {
		case "text":
			texts = append(texts, b.Text)
		case "image":
			if why := refusedImage(b, len(res.Images), imagesSize); why != "" {
				texts = append(texts, "["+why+"]")
				continue
			}
			res.Images = append(res.Images, tools.Image{MediaType: b.MimeType, Base64: b.Data})
			imagesSize += len(b.Data)
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

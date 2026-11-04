package platformagent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The API's limits on what a message carries.
const (
	maxImageBytes = 5 << 20
	maxPDFBytes   = 32 << 20
)

// mediaTypes are the files sent as blocks of their own.
var mediaTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".pdf":  "application/pdf",
}

// userMessage builds what goes to the API of a prompt, and says which files went along;
// nil when there is nothing to send. The first message of a conversation opens with the
// repository's instructions. Paths are looked for in own alone, the user's own words:
// text the model or a hook wrote mustn't send a file off unasked, no gate being there.
func (t *thread) userMessage(text, own string, attachments []string) (json.RawMessage, []string) {
	a := t.a
	var blocks []map[string]any
	if len(t.history) == 0 {
		if intro := startOfConversation(a.opts.Dir, time.Now()); intro != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": intro})
		}
	}

	var files []string
	for _, path := range gatherPaths(a.opts.Dir, own, attachments) {
		block, err := attachmentBlock(a.opts.Dir, path)
		if err != nil {
			a.notice("warning", fmt.Sprintf("%s wasn't attached: %v", path, err))
			continue
		}
		blocks = append(blocks, block)
		files = append(files, path)
	}
	if strings.TrimSpace(text) != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": text})
	} else if len(files) == 0 {
		return nil, nil
	}
	return marshalStable(map[string]any{"role": "user", "content": blocks}), files
}

// gatherPaths is the attachments given and the images and PDFs a prompt names: the chat
// sends "text plus paths". A path counts when the file is there; whole lines come first,
// for paths with spaces.
func gatherPaths(dir, text string, attachments []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(path string) {
		if !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}
	for _, path := range attachments {
		add(path)
	}
	consider := func(word string) {
		word = strings.Trim(word, "\"'`,;()[]<>")
		if _, media := mediaTypes[strings.ToLower(filepath.Ext(word))]; !media {
			return
		}
		if info, err := os.Stat(resolvePath(dir, word)); err == nil && !info.IsDir() {
			add(word)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		consider(strings.TrimSpace(line))
		for _, word := range strings.Fields(line) {
			consider(word)
		}
	}
	return out
}

func resolvePath(dir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}

// attachmentBlock is a file as a content block: base64 for images and PDFs, and for
// anything else a line saying where it is, which the Read tool can follow.
func attachmentBlock(dir, path string) (map[string]any, error) {
	full := resolvePath(dir, path)
	media, ok := mediaTypes[strings.ToLower(filepath.Ext(path))]
	if !ok {
		if _, err := os.Stat(full); err != nil {
			return nil, err
		}
		return map[string]any{"type": "text", "text": "Attached file: " + full}, nil
	}
	limit, kind := int64(maxImageBytes), "image"
	if media == "application/pdf" {
		limit, kind = maxPDFBytes, "document"
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("it is over the API's limit of %d MB", limit>>20)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"type": kind,
		"source": map[string]string{
			"type":       "base64",
			"media_type": media,
			"data":       base64.StdEncoding.EncodeToString(data),
		},
	}, nil
}

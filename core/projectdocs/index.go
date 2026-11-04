package projectdocs

import "strings"

// IndexText is a note's body as RAG should embed it: what a document records
// about itself (its first line, its Based on lines) and what its template
// still asks for say nothing of the project, and would come back from a search
// as if they did. A status change then embeds nothing again either. An
// ordinary note stays as it is.
func IndexText(body string) string {
	if _, ok := Parse(body); !ok {
		return body
	}
	lines := strings.Split(body, "\n")
	kept := lines[:0:0]
	for _, line := range lines {
		if isMetaLine(line) || strings.HasPrefix(strings.TrimSpace(line), ToFill) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

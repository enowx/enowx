package platformagent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// validateInput checks a call's input against its tool's schema, "" when it passes.
// Inputs stream unbuffered, so the API hasn't checked them, and one cut short or
// malformed must not run. It checks what a tool can't do without: an object, its
// required fields, and the types of those it names.
func validateInput(schema, input json.RawMessage) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil || fields == nil {
		return "it isn't a JSON object"
	}
	var s struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type any `json:"type"`
		} `json:"properties"`
	}
	if json.Unmarshal(schema, &s) != nil {
		return ""
	}
	var missing []string
	for _, name := range s.Required {
		if raw, ok := fields[name]; !ok || string(raw) == "null" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return "missing required " + strings.Join(missing, ", ")
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		prop, ok := s.Properties[name]
		if !ok {
			continue
		}
		kind, isString := prop.Type.(string)
		if isString && !hasJSONType(fields[name], kind) {
			return fmt.Sprintf("%s should be of type %s", name, kind)
		}
	}
	return ""
}

func hasJSONType(raw json.RawMessage, kind string) bool {
	if len(raw) == 0 {
		return false
	}
	switch c := raw[0]; kind {
	case "string":
		return c == '"'
	case "object":
		return c == '{'
	case "array":
		return c == '['
	case "boolean":
		return c == 't' || c == 'f'
	case "number", "integer":
		return c == '-' || (c >= '0' && c <= '9')
	}
	return true
}

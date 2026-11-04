package usersync

import (
	"bytes"
	"encoding/json"
	"path"
	"slices"
	"strings"
	"unicode/utf8"
)

// MergeFile merges the changes a and b made to base, file p's contents
// (a path as notes/<id>.md), by the kind of file it is; ok is false when
// they don't merge. Syncs other than the user's own use it: a team's space.
func MergeFile(p string, base, a, b []byte) ([]byte, bool) { return mergeFile(p, base, a, b) }

// mergeFile merges the changes a and b made to base, file p's contents,
// by the kind of file it is; ok is false when they don't merge.
func mergeFile(p string, base, a, b []byte) ([]byte, bool) {
	if AppendOnly(p) {
		return mergeLog(a, b), true
	}
	if !utf8.Valid(a) || !utf8.Valid(b) || !utf8.Valid(base) {
		return nil, false
	}
	switch path.Ext(p) {
	case ".md":
		if out, ok := mergeNote(string(base), string(a), string(b)); ok {
			return []byte(out), true
		}
	case ".json":
		if out, ok := mergeJSON(base, a, b); ok {
			return out, true
		}
	}
	if out, ok := merge3(string(base), string(a), string(b)); ok {
		return []byte(out), true
	}
	return nil, false
}

// mergeNote merges a note's header field by field, its last change the
// later of the two, and its body line by line.
func mergeNote(base, a, b string) (string, bool) {
	hb, bb, ok0 := splitNote(base)
	ha, ba, ok1 := splitNote(a)
	hc, bc, ok2 := splitNote(b)
	if !ok1 || !ok2 {
		return "", false
	}
	if !ok0 {
		hb, bb = nil, base
	}
	body, ok := merge3(bb, ba, bc)
	if !ok {
		return "", false
	}
	header, ok := mergeHeader(hb, ha, hc)
	if !ok {
		return "", false
	}
	var out strings.Builder
	out.WriteString("---\n")
	for _, kv := range header {
		out.WriteString(kv[0] + ": " + kv[1] + "\n")
	}
	out.WriteString("---\n")
	out.WriteString(body)
	return out.String(), true
}

// splitNote is a note's header, its "key: value" lines in order, and the
// rest.
func splitNote(s string) ([][2]string, string, bool) {
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		return nil, "", false
	}
	head, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return nil, "", false
	}
	var kvs [][2]string
	for _, line := range strings.Split(head, "\n") {
		k, v, _ := strings.Cut(line, ":")
		kvs = append(kvs, [2]string{strings.TrimSpace(k), strings.TrimSpace(v)})
	}
	return kvs, body, true
}

func mergeHeader(base, a, b [][2]string) ([][2]string, bool) {
	get := func(kvs [][2]string, k string) (string, bool) {
		for _, kv := range kvs {
			if kv[0] == k {
				return kv[1], true
			}
		}
		return "", false
	}
	var keys []string
	for _, kvs := range [][][2]string{a, b} {
		for _, kv := range kvs {
			if !slices.Contains(keys, kv[0]) {
				keys = append(keys, kv[0])
			}
		}
	}
	var out [][2]string
	for _, k := range keys {
		vb, inB := get(base, k)
		va, inA := get(a, k)
		vc, inC := get(b, k)
		var v string
		var keep bool
		switch {
		case inA == inC && va == vc:
			v, keep = va, inA
		case inA == (inB) && va == vb:
			v, keep = vc, inC
		case inC == inB && vc == vb:
			v, keep = va, inA
		case k == "updated":
			v, keep = max(va, vc), true // RFC 3339 in UTC sorts as time does
		case k == "order":
			v, keep = va, inA
		default:
			return nil, false
		}
		if keep {
			out = append(out, [2]string{k, v})
		}
	}
	return out, true
}

// mergeJSON merges JSON objects key by key, and lists of objects named by
// "id" or "name" item by item, the order a's with b's new items after.
func mergeJSON(base, a, b []byte) ([]byte, bool) {
	var vb, va, vc any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vc) != nil {
		return nil, false
	}
	if len(bytes.TrimSpace(base)) > 0 && json.Unmarshal(base, &vb) != nil {
		return nil, false
	}
	v, ok := mergeValue(vb, va, vc)
	if !ok {
		return nil, false
	}
	out, err := json.MarshalIndent(v, "", "  ")
	return out, err == nil
}

func mergeValue(base, a, b any) (any, bool) {
	switch {
	case jsonEqual(a, b):
		return a, true
	case jsonEqual(a, base):
		return b, true
	case jsonEqual(b, base):
		return a, true
	}
	if ma, ok := a.(map[string]any); ok {
		mb, ok := b.(map[string]any)
		if !ok {
			return nil, false
		}
		mbase, _ := base.(map[string]any)
		out := map[string]any{}
		for _, k := range unionKeys(ma, mb) {
			va, inA := ma[k]
			vb, inB := mb[k]
			vbase, inBase := mbase[k]
			switch {
			case inA && inB:
				v, ok := mergeValue(vbase, va, vb)
				if !ok {
					return nil, false
				}
				out[k] = v
			case inA && (!inBase || !jsonEqual(va, vbase)):
				out[k] = va // b deleted what a changed, or a added it
			case inB && (!inBase || !jsonEqual(vb, vbase)):
				out[k] = vb
			}
		}
		return out, true
	}
	la, okA := a.([]any)
	lb, okB := b.([]any)
	if !okA || !okB {
		return nil, false
	}
	lbase, _ := base.([]any)
	ka, kb, kbase := keyed(la), keyed(lb), keyed(lbase)
	if ka == nil || kb == nil || lbase != nil && kbase == nil {
		return nil, false
	}
	var out []any
	var order []string
	for _, it := range la {
		order = append(order, itemKey(it))
	}
	for _, it := range lb {
		if k := itemKey(it); !slices.Contains(order, k) {
			order = append(order, k)
		}
	}
	for _, k := range order {
		va, inA := ka[k]
		vb, inB := kb[k]
		vbase, inBase := kbase[k]
		switch {
		case inA && inB:
			v, ok := mergeValue(vbase, va, vb)
			if !ok {
				return nil, false
			}
			out = append(out, v)
		case inA && (!inBase || !jsonEqual(va, vbase)):
			out = append(out, va)
		case inB && (!inBase || !jsonEqual(vb, vbase)):
			out = append(out, vb)
		}
	}
	if out == nil {
		out = []any{}
	}
	return out, true
}

func keyed(list []any) map[string]any {
	out := map[string]any{}
	for _, it := range list {
		k := itemKey(it)
		if k == "" {
			return nil
		}
		if _, dup := out[k]; dup {
			return nil
		}
		out[k] = it
	}
	return out
}

func itemKey(it any) string {
	m, ok := it.(map[string]any)
	if !ok {
		return ""
	}
	for _, f := range []string{"id", "name"} {
		if s, ok := m[f].(string); ok && s != "" {
			return f + ":" + s
		}
	}
	return ""
}

func unionKeys(a, b map[string]any) []string {
	var keys []string
	for k := range a {
		keys = append(keys, k)
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

func jsonEqual(a, b any) bool {
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

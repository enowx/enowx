package tools

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// The most patterns braces may expand to, so a pattern can't cost what it likes.
const braceMost = 256

// globPattern matches slash-separated paths: * ? [..] within a segment, ** across
// segments, {a,b} for either.
type globPattern struct{ alternatives [][]string }

// newGlob compiles pattern. A pattern without a slash matches at any depth, as models
// expect "*.go" to and gitignore and ripgrep do.
func newGlob(pattern string) (*globPattern, error) {
	return compileGlob(pattern, !strings.Contains(filepath.ToSlash(pattern), "/"))
}

// compileGlob compiles pattern, relative to the walk's start or, if anyDepth, below it.
func compileGlob(pattern string, anyDepth bool) (*globPattern, error) {
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")
	if pattern == "" {
		return nil, fmt.Errorf("empty glob pattern")
	}
	expanded, err := expandBraces(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob pattern %q: %w", pattern, err)
	}
	g := &globPattern{}
	for _, one := range expanded {
		segments := strings.Split(strings.Trim(one, "/"), "/")
		for _, s := range segments {
			if _, err := path.Match(s, ""); err != nil {
				return nil, fmt.Errorf("glob pattern %q: %w", pattern, err)
			}
		}
		if anyDepth {
			segments = append([]string{"**"}, segments...)
		}
		g.alternatives = append(g.alternatives, segments)
	}
	return g, nil
}

// match says the path rel, relative to where the walk began, matches.
func (g *globPattern) match(rel string) bool {
	name := strings.Split(rel, "/")
	for _, segments := range g.alternatives {
		if matchSegments(segments, name) {
			return true
		}
	}
	return false
}

func matchSegments(pattern, name []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			for len(pattern) > 1 && pattern[1] == "**" {
				pattern = pattern[1:]
			}
			if len(pattern) == 1 {
				return true
			}
			for i := range len(name) + 1 {
				if matchSegments(pattern[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, _ := path.Match(pattern[0], name[0]); !ok {
			return false
		}
		pattern, name = pattern[1:], name[1:]
	}
	return len(name) == 0
}

// expandBraces turns "a.{b,c}" into "a.b" and "a.c"; a brace with no comma stays.
func expandBraces(pattern string) ([]string, error) {
	out := []string{}
	pending := []string{pattern}
	for len(pending) > 0 {
		one := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		open, end, commas := outerBrace(one)
		if open < 0 {
			out = append(out, one)
			continue
		}
		start := open + 1
		for _, c := range append(commas, end) {
			pending = append(pending, one[:open]+one[start:c]+one[end+1:])
			start = c + 1
		}
		if len(pending)+len(out) > braceMost {
			return nil, fmt.Errorf("more than %d alternatives", braceMost)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// outerBrace finds the first {..} holding a comma of its own: where it opens, closes,
// and its commas. open is -1 when there is none.
func outerBrace(s string) (open, end int, commas []int) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] != '{' {
			continue
		}
		depth := 0
		commas = nil
		for j := i; j < len(s); j++ {
			switch s[j] {
			case '\\':
				j++
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					if len(commas) > 0 {
						return i, j, commas
					}
					j = len(s)
				}
			case ',':
				if depth == 1 {
					commas = append(commas, j)
				}
			}
		}
	}
	return -1, 0, nil
}

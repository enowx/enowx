package noteblocks

import (
	"fmt"
	"regexp"
	"strings"
)

var todoRe = regexp.MustCompile(`^(\s*[-*+] \[)([ xX])(\] )(.*)$`)

// Todo says line is a to-do, and whether it's done and its text.
func Todo(line string) (done bool, text string, ok bool) {
	m := todoRe.FindStringSubmatch(line)
	if m == nil {
		return false, "", false
	}
	return m[2] != " ", strings.TrimSpace(m[4]), true
}

// progress counts the to-dos of lines, and those done, outside code.
func progress(lines []string) (done, total int) {
	fenced := false
	for _, l := range lines {
		if isFence(l) {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if d, _, ok := Todo(l); ok {
			total++
			if d {
				done++
			}
		}
	}
	return done, total
}

// Progress counts a note's to-dos, and those done.
func Progress(md string) (done, total int) { return progress(Lines(md)) }

// Check ticks the to-do item names (or unticks it, done false): the one
// with that text, else the only one with it in its text. It returns md
// changed and the item's line.
func Check(md, item string, done bool) (string, int, error) {
	lines := Lines(md)
	want := strings.ToLower(strings.TrimSpace(item))
	if want == "" {
		return md, 0, fmt.Errorf("no to-do named")
	}
	var exact, partly []int
	fenced := false
	for i, l := range lines {
		if isFence(l) {
			fenced = !fenced
			continue
		}
		_, text, ok := Todo(l)
		if fenced || !ok {
			continue
		}
		t := strings.ToLower(text)
		switch {
		case t == want:
			exact = append(exact, i)
		case strings.Contains(t, want):
			partly = append(partly, i)
		}
	}
	at := -1
	for _, list := range [][]int{exact, partly} {
		switch len(list) {
		case 0:
			continue
		case 1:
			at = list[0]
		default:
			var names []string
			for _, i := range list {
				_, text, _ := Todo(lines[i])
				names = append(names, fmt.Sprintf("line %d: %q", i+1, text))
			}
			return md, 0, fmt.Errorf("%q names %d to-dos (%s): give more of its text", item, len(list), strings.Join(names, "; "))
		}
		break
	}
	if at < 0 {
		return md, 0, fmt.Errorf("no to-do %q", item)
	}
	mark := " "
	if done {
		mark = "x"
	}
	lines[at] = todoRe.ReplaceAllString(lines[at], "${1}"+mark+"${3}${4}")
	return strings.Join(lines, "\n") + "\n", at, nil
}

// Open are those of items md doesn't have ticked: left open, or no to-do of that
// text at all, an item being found as Check finds it.
func Open(md string, items []string) []string {
	var out []string
	for _, item := range items {
		if after, _, err := Check(md, item, true); err != nil || after != md {
			out = append(out, item)
		}
	}
	return out
}

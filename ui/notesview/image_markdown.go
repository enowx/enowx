package notesview

import "regexp"

// imageMatches excludes escaped markup and inline code from image rendering.
func imageMatches(text string) [][]int {
	if len(text) > longBlock {
		return nil // it's looked through each frame; a block of images is short
	}
	return markdownTokenMatches(text, imageToken, '!')
}

func markdownTokenMatches(text string, token *regexp.Regexp, first byte) [][]int {
	var out [][]int
	code := 0
	for at := 0; at < len(text); {
		switch text[at] {
		case '\\':
			if code == 0 {
				at += 2
				continue
			}
		case '`':
			end := at + 1
			for end < len(text) && text[end] == '`' {
				end++
			}
			if code == 0 {
				code = end - at
			} else if code == end-at {
				code = 0
			}
			at = end
			continue
		case first:
			if code == 0 {
				m := token.FindStringSubmatchIndex(text[at:])
				if len(m) > 0 && m[0] == 0 {
					for j := range m {
						m[j] += at
					}
					out = append(out, m)
					at = m[1]
					continue
				}
			}
		}
		at++
	}
	return out
}

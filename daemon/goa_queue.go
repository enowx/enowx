package daemon

// uniqueGOALines preserves order and task/session identity. A changed report,
// another task or a later permission prompt with different text remains news.
func uniqueGOALines(lines []goaLine) []goaLine {
	seen := make(map[goaLine]bool, len(lines))
	out := make([]goaLine, 0, len(lines))
	for _, line := range lines {
		if !seen[line] {
			seen[line] = true
			out = append(out, line)
		}
	}
	return out
}

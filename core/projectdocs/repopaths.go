package projectdocs

import (
	"fmt"
	"path"
	"strings"

	"github.com/enowx/enowx/core/noteblocks"
)

// RepoPaths is where a repo export writes p, slash-separated and relative to
// its folder: the project note's README.md, then each of p.Docs in order. The
// app lists these before the daemon writes them, so both must name them alike.
func RepoPaths(p *Project) []string {
	out := []string{"README.md"}
	used := map[string]bool{"readme.md": true}
	areas := map[string]string{}
	for _, a := range p.Areas {
		areas[a.ID] = a.Title
	}
	for _, d := range p.Docs {
		name := noteblocks.Slug(d.Title)
		if name == "" {
			name = d.ID
		}
		dir := ""
		if area, ok := areas[d.Parent]; ok {
			if dir = noteblocks.Slug(area); dir == "" {
				dir = d.Parent
			}
		}
		file := path.Join(dir, name+".md")
		for i := 2; used[strings.ToLower(file)]; i++ {
			file = path.Join(dir, fmt.Sprintf("%s-%d.md", name, i))
		}
		used[strings.ToLower(file)] = true
		out = append(out, file)
	}
	return out
}

package notesio

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/autopush"
	"github.com/enowx/enowx/core/iconlib"
	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/core/notes"
	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

// A project's documents written into a repository as Markdown, for what reads
// a repo. Notes stay the source, so each file says so and only a file saying
// so is written over: the folder may hold anyone's work.

// repoMarker starts every file of a repo export, and tells it from others'.
const repoMarker = "<!-- Exported from enowX Project Docs"

// RepoOptions say how to write a repo export.
type RepoOptions struct {
	Force  bool // write over files that aren't a repo export's
	DryRun bool // refuse as it would, and write nothing
}

// RepoExport says what a repo export wrote, or a dry run would.
type RepoExport struct {
	Files []string // relative to the folder, slash-separated
	Notes int
	Size  int64
}

// repoFile is a note's place in the export.
type repoFile struct {
	note         proto.Note
	file, title  string
	status, area string
	version      int
}

// ExportRepo writes the project at note id into folder out.
func ExportRepo(s *notes.Store, id, out string, o RepoOptions) (RepoExport, error) {
	list, err := s.List()
	if err != nil {
		return RepoExport{}, fmt.Errorf("export: %w", err)
	}
	p, err := projectdocs.Load(list, id, s.Get)
	if err != nil {
		return RepoExport{}, fmt.Errorf("export: %w", err)
	}
	files := repoLayout(p)
	rendered := make(map[string]string, len(files))
	var secrets []string
	for _, f := range files {
		text := repoText(p, f, files)
		rendered[f.file] = text
		for _, found := range autopush.ScanSecrets(f.file, []byte(text)) {
			secrets = append(secrets, found.String()) // where and what kind, never the value
		}
	}
	if len(secrets) > 0 {
		return RepoExport{}, fmt.Errorf("export: not written, it looks like it holds a secret (%s): remove it first", strings.Join(secrets, "; "))
	}
	var foreign []string
	for _, f := range files {
		reason, err := repoTargetProblem(filepath.Join(out, filepath.FromSlash(f.file)), o.Force)
		if err != nil {
			return RepoExport{}, err
		}
		if reason != "" {
			foreign = append(foreign, f.file+" ("+reason+")")
		}
	}
	if len(foreign) > 0 {
		return RepoExport{}, fmt.Errorf("export: not written, these aren't files of an export: %s (--force writes over a file)", strings.Join(foreign, ", "))
	}
	res := RepoExport{Notes: len(files)}
	for _, f := range files {
		res.Files = append(res.Files, f.file)
		res.Size += int64(len(rendered[f.file]))
		if o.DryRun {
			continue
		}
		target := filepath.Join(out, filepath.FromSlash(f.file))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return res, fmt.Errorf("export: %w", err)
		}
		if err := os.WriteFile(target, []byte(rendered[f.file]), 0o644); err != nil {
			return res, fmt.Errorf("export: %w", err)
		}
	}
	return res, nil
}

// repoTargetProblem says why target may not be written, "" when it may.
func repoTargetProblem(target string, force bool) (string, error) {
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("export: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("export: %s isn't a plain file: not written", target) // a link could write anywhere, forced or not
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("export: %w", err)
	}
	if !force && !strings.HasPrefix(string(data), repoMarker) {
		return "not from enowX", nil
	}
	return "", nil
}

// repoLayout places the project note as README.md and each document under
// its area's folder when it's in one. The first is the project's.
func repoLayout(p *projectdocs.Project) []repoFile {
	paths := projectdocs.RepoPaths(p)
	files := []repoFile{{note: p.Note, file: paths[0], title: p.Note.Title}}
	areas := map[string]string{}
	for _, a := range p.Areas {
		areas[a.ID] = a.Title
	}
	for i, d := range p.Docs {
		file := paths[i+1]
		status := d.Meta.Status
		if status == "" {
			status = projectdocs.StatusDraft
		}
		files = append(files, repoFile{note: d.Note, file: file, title: d.Title, status: status, area: areas[d.Parent], version: max(d.Meta.Version, 1)})
	}
	return files
}

// repoText is a note as its file: the comment, its title, its body without
// the meta line and with links made relative.
func repoText(p *projectdocs.Project, f repoFile, files []repoFile) string {
	about := "note " + f.note.ID
	if f.status != "" {
		about += fmt.Sprintf(", v%d, %s", f.version, f.status)
	} else {
		about += ", approval " + p.Meta.Approval
	}
	_, body, _ := strings.Cut(f.note.Body, "\n") // a note here has its meta line first
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s). Notes are the source: edit there. -->\n\n# %s\n\n", repoMarker, about, f.title)
	b.WriteString(iconlib.ExportMarkdown(strings.TrimLeft(repoBody(p, f, files, body), "\n")))
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// repoBody rewrites what only the Notes page draws, outside code: links to
// the project's notes, and the project note's list of pages.
func repoBody(p *projectdocs.Project, f repoFile, files []repoFile, body string) string {
	fence := false
	var out []string
	for _, line := range noteblocks.Lines(body) {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			fence = !fence
		}
		if fence {
			out = append(out, line)
			continue
		}
		switch t {
		case "::progress", "::toc":
			continue
		case "::pages":
			if f.file == "README.md" {
				out = append(out, repoIndex(files)...)
			}
			continue
		}
		parts := strings.Split(line, "`")
		for i := 0; i < len(parts); i += 2 { // the parts outside `code`
			parts[i] = wikiLink.ReplaceAllStringFunc(parts[i], func(m string) string { return repoLink(p, f, files, m) })
		}
		out = append(out, strings.Join(parts, "`"))
	}
	return strings.Join(out, "\n")
}

// repoIndex lists the documents, an area at a time.
func repoIndex(files []repoFile) []string {
	var out []string
	var areas []string
	for _, f := range files[1:] {
		if !slices.Contains(areas, f.area) {
			areas = append(areas, f.area)
		}
	}
	for _, area := range areas {
		if area != "" {
			out = append(out, "", "### "+area, "")
		}
		for _, f := range files[1:] {
			if f.area == area {
				out = append(out, fmt.Sprintf("- [%s](%s) (%s, v%d)", f.title, f.file, f.status, f.version))
			}
		}
	}
	return out
}

// repoLink is a wiki link as a relative Markdown link, or as plain text when
// what it names isn't in the export: a dead link helps nobody in a repo.
func repoLink(p *projectdocs.Project, from repoFile, files []repoFile, m string) string {
	inner := wikiLink.FindStringSubmatch(m)[2]
	target, label, labelled := strings.Cut(inner, "|")
	if !labelled {
		label = target
	}
	label = strings.TrimSpace(label)
	target, section, _ := strings.Cut(target, "#")
	target = strings.TrimSpace(target)
	to := from
	if target != "" {
		found := false
		if d := p.Doc(target); d != nil {
			i := slices.IndexFunc(files, func(f repoFile) bool { return f.note.ID == d.ID })
			to, found = files[i], true
		} else if strings.EqualFold(target, p.Note.Title) || target == p.Note.ID {
			to, found = files[0], true
		}
		if !found {
			return label
		}
	}
	href := ""
	if to.file != from.file {
		rel, err := filepath.Rel(filepath.FromSlash(path.Dir(from.file)), filepath.FromSlash(to.file))
		if err != nil {
			return label
		}
		href = filepath.ToSlash(rel)
	}
	if section = strings.TrimSpace(section); section != "" {
		if s, err := noteblocks.Find(noteblocks.Outline(to.note.Body), section); err == nil {
			section = s.Title
		}
		href += "#" + noteblocks.Slug(section)
	}
	return "[" + label + "](" + href + ")"
}

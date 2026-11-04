package designshare

import (
	"errors"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/canvas"
)

// What people take from a design in the library: a component's code, and the
// credit every copy keeps. The cloud makes them from the bundle it holds.

// OthersData is what says, wherever an agent or a command is given something of
// the library's, whose words they are.
const OthersData = CommunityLabel

// LicenseName is a license as people read it.
func LicenseName(license string) string {
	switch license {
	case "mit":
		return "MIT"
	case "cc-by-4.0":
		return "CC BY 4.0"
	case "view-only":
		return "View only"
	}
	return license
}

// OneLine is text on a line of its own: what someone typed can't start another.
func OneLine(s string) string { return strings.Join(strings.Fields(printable(s)), " ") }

// Credit is the line naming a design, by whom.
func Credit(title, handle string) string { return OneLine(title) + " by " + OneLine(handle) }

// Notice is the credit a copy carries, as lines: who made it, where it is, its
// license.
func Notice(title, handle, address, license string) []string {
	return []string{Credit(title, handle), OneLine(address), "License: " + LicenseName(license) + ". Keep this notice."}
}

// unclosable is text that can't end the comment it's put in, whatever a title
// holds.
func unclosable(s string) string {
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "- -")
	}
	s = strings.ReplaceAll(s, ">", "›")
	return strings.ReplaceAll(s, "*/", "* /")
}

// HTMLNotice is notice as an HTML comment.
func HTMLNotice(notice []string) string {
	lines := make([]string, len(notice))
	for i, l := range notice {
		lines[i] = unclosable(l)
	}
	return "<!-- " + strings.Join(lines, "\n     ") + " -->"
}

// CSSNotice is notice as a CSS comment.
func CSSNotice(notice []string) string {
	lines := make([]string, len(notice))
	for i, l := range notice {
		lines[i] = unclosable(l)
	}
	return "/* " + strings.Join(lines, "\n   ") + " */"
}

// Code is a component as people copy it: its HTML and its CSS, each opening with
// the notice, and the files it names that don't come along, whose rights may not
// be the design's.
type Code struct {
	HTML   string
	CSS    string
	Assets []string
}

// ErrNoCode says a design has no HTML to copy: it's drawn in shapes.
var ErrNoCode = errors.New("this design is drawn in shapes: it has no HTML to copy")

// ComponentCode is b's design as code with notice in both copies.
func ComponentCode(b Bundle, notice []string) (Code, error) {
	if strings.TrimSpace(b.Frame.HTML) == "" {
		return Code{}, ErrNoCode
	}
	markup, css, err := canvas.SplitStyles(b.Frame.HTML)
	if err != nil {
		return Code{}, err
	}
	out := Code{HTML: HTMLNotice(notice) + "\n" + markup, CSS: CSSNotice(notice), Assets: filesToProvide(b.Frame)}
	if css != "" {
		out.CSS += "\n" + css
	}
	return out, nil
}

// filesToProvide are the files f shows, sorted.
func filesToProvide(f Frame) []string {
	names := canvas.FilesNamed(f.HTML)
	for _, e := range f.Elements {
		if e.Src != "" && !strings.Contains(e.Src, ":") {
			names = append(names, e.Src)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

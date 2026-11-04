// Package projectdocs documents a whole project as ordinary notes: one
// project note, with its documents under it. What makes them Project Docs is
// a line at the top of the body, not the note's header: a daemon of before
// rewrites a header without keys it doesn't know, whereas a body survives it,
// sync, teams, exports and proposals, and reads as plain text in an app of
// before.
package projectdocs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/enowx/enowx/core/noteblocks"
)

// Marker starts a body's first line.
const Marker = "Project Docs: "

// A document's states.
const (
	StatusDraft    = "Draft"
	StatusReview   = "In review"
	StatusApproved = "Approved"
	StatusOutdated = "Outdated"
)

// Statuses are a document's states, in their order of use.
var Statuses = []string{StatusDraft, StatusReview, StatusApproved, StatusOutdated}

// Whether a project waits for the owner's approval before it is built.
const (
	ApprovalRequired = "required"
	ApprovalOff      = "off"
)

// The two sizes of a project.
const (
	SizeCore = "core"
	SizeFull = "full"
)

// Kinds that aren't documents of the catalog.
const (
	KindProject   = "Project"
	KindArea      = "Area"
	KindTemplates = "Templates"
)

// Meta is what a body's first line says.
type Meta struct {
	Kind string // KindProject, KindArea, KindTemplates, or a document's name
	// A project's.
	Size, Approval string
	// A document's: Hash is ContentHash as approved, "" until then.
	Status  string
	Version int
	Hash    string
}

// IsProject says the note is a project note.
func (m Meta) IsProject() bool { return m.Kind == KindProject }

// IsDocument says the note is a document of a project.
func (m Meta) IsDocument() bool {
	return m.Kind != "" && m.Kind != KindProject && m.Kind != KindArea && m.Kind != KindTemplates
}

var (
	badgeRe   = regexp.MustCompile(`^\{\{(.+)\}\}$`)
	versionRe = regexp.MustCompile(`^v(\d+)$`)
)

// Parse reads the meta of body, ok false for an ordinary note.
func Parse(body string) (Meta, bool) {
	first, _, _ := strings.Cut(body, "\n")
	first = strings.TrimSpace(first)
	rest, ok := strings.CutPrefix(first, Marker)
	if !ok {
		return Meta{}, false
	}
	parts := strings.Split(rest, " · ")
	m := Meta{Kind: strings.TrimSpace(parts[0])}
	if m.Kind == "" {
		return Meta{}, false
	}
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		if b := badgeRe.FindStringSubmatch(p); b != nil {
			switch {
			case m.Kind == KindProject && b[1] == "Full pack":
				m.Size = SizeFull
			case m.Kind == KindProject:
				m.Size = SizeCore
			default:
				m.Status = b[1]
			}
			continue
		}
		if v := versionRe.FindStringSubmatch(p); v != nil {
			m.Version, _ = strconv.Atoi(v[1])
			continue
		}
		if h, ok := strings.CutPrefix(p, "@"); ok {
			m.Hash = h
			continue
		}
		if a, ok := strings.CutPrefix(p, "Approval: "); ok {
			m.Approval = strings.TrimSpace(a)
		}
	}
	if m.Kind == KindProject && m.Approval == "" {
		m.Approval = ApprovalRequired
	}
	return m, true
}

// Line is meta as a body's first line.
func (m Meta) Line() string {
	switch {
	case m.Kind == KindProject:
		pack := "Core pack"
		if m.Size == SizeFull {
			pack = "Full pack"
		}
		approval := m.Approval
		if approval == "" {
			approval = ApprovalRequired
		}
		return fmt.Sprintf("%s%s · {{%s}} · Approval: %s", Marker, m.Kind, pack, approval)
	case !m.IsDocument():
		return Marker + m.Kind
	}
	status := m.Status
	if status == "" {
		status = StatusDraft
	}
	version := max(m.Version, 1)
	line := fmt.Sprintf("%s%s · {{%s}} · v%d", Marker, m.Kind, status, version)
	if m.Hash != "" {
		line += " · @" + m.Hash
	}
	return line
}

// WithMeta is body with its first line saying m, added when it has none.
func WithMeta(body string, m Meta) string {
	if _, ok := Parse(body); ok {
		_, rest, _ := strings.Cut(body, "\n")
		return m.Line() + "\n" + rest
	}
	return m.Line() + "\n\n" + body
}

// Approve is body approved: its status, its hash, and the next version when
// it had been approved before with other content.
func Approve(body string) (string, error) {
	m, ok := Parse(body)
	if !ok || !m.IsDocument() {
		return "", fmt.Errorf("projectdocs: not a document of a project")
	}
	sum := ContentHash(body)
	if m.Hash != "" && m.Hash != sum {
		m.Version = max(m.Version, 1) + 1
	}
	m.Status, m.Hash = StatusApproved, sum
	return WithMeta(body, m), nil
}

// SetStatus is body with status; only Approve makes it approved.
func SetStatus(body, status string) (string, error) {
	if status == StatusApproved {
		return Approve(body)
	}
	m, ok := Parse(body)
	if !ok || !m.IsDocument() {
		return "", fmt.Errorf("projectdocs: not a document of a project")
	}
	known := false
	for _, s := range Statuses {
		known = known || s == status
	}
	if !known {
		return "", fmt.Errorf("projectdocs: no status %q: %s", status, strings.Join(Statuses, ", "))
	}
	m.Status = status
	return WithMeta(body, m), nil
}

// SetApproval is a project note's body with its approval required or off.
func SetApproval(body, approval string) (string, error) {
	m, ok := Parse(body)
	if !ok || !m.IsProject() {
		return "", fmt.Errorf("projectdocs: not a project note")
	}
	if approval != ApprovalRequired && approval != ApprovalOff {
		return "", fmt.Errorf("projectdocs: approval is %s or %s", ApprovalRequired, ApprovalOff)
	}
	m.Approval = approval
	return WithMeta(body, m), nil
}

// ChangedSinceApproval says an approved document's content isn't what was
// approved: edited by an app or daemon that doesn't know Project Docs.
func ChangedSinceApproval(body string) bool {
	m, ok := Parse(body)
	return ok && m.Status == StatusApproved && m.Hash != "" && m.Hash != ContentHash(body)
}

const basedPrefix = "Based on: "

// isMetaLine says a line records rather than documents: changing it isn't
// changing the content.
func isMetaLine(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, Marker) || strings.HasPrefix(t, basedPrefix)
}

// hashText is what content comes to, without what records it and with every
// to-do unticked, since ticking acceptance criteria isn't changing them.
func hashText(lines []string) string {
	h := sha256.New()
	for _, l := range lines {
		if isMetaLine(l) {
			continue
		}
		if _, text, ok := noteblocks.Todo(l); ok {
			l = "- [ ] " + text
		}
		l = strings.TrimRight(l, " \t\r")
		if l == "" {
			continue
		}
		fmt.Fprintln(h, l)
	}
	return hex.EncodeToString(h.Sum(nil))[:8]
}

// ContentHash is a short hash of a document's content.
func ContentHash(body string) string { return hashText(noteblocks.Lines(body)) }

// SectionHash is a short hash of one section's content.
func SectionHash(body, section string) (string, error) {
	s, err := noteblocks.Find(noteblocks.Outline(body), section)
	if err != nil {
		return "", err
	}
	return hashText(strings.Split(noteblocks.Text(body, s), "\n")), nil
}

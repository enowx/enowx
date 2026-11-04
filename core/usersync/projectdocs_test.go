package usersync

import (
	"strings"
	"testing"

	"github.com/enowx/enowx/core/projectdocs"
)

// A document approved on one host while another edits it merges, and the merge
// isn't taken for what was approved: the gate must see the change.
func TestSyncOfAProjectDocument(t *testing.T) {
	p := newPair(t)
	body, err := projectdocs.DocBody(*projectdocs.Find("prd"))
	if err != nil {
		t.Fatal(err)
	}
	const path = "notes/aaaaaaaaaaaa.md"
	p.put(p.a, path, note("PRD", body))
	p.sync()
	if got := p.get(p.b, path); !strings.Contains(got, projectdocs.Marker+"PRD") {
		t.Fatalf("its first line didn't cross: %q", got)
	}

	approved, err := projectdocs.Approve(body)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(body, "## Open questions\n", "## Open questions\n\n- [ ] Which plan?\n", 1)
	if edited == body {
		t.Fatal("the template has no Open questions")
	}
	p.put(p.a, path, note("PRD", approved))
	p.put(p.b, path, note("PRD", edited))
	if rep := p.sync(); rep.Merged != 1 || rep.Conflicts != 0 {
		t.Fatalf("sync = %+v", rep)
	}
	merged := p.get(p.b, path)
	if merged != p.get(p.a, path) {
		t.Fatal("the hosts differ")
	}
	_, merged, _ = strings.Cut(merged, "---\n\n")
	m, ok := projectdocs.Parse(merged)
	if !ok || m.Status != projectdocs.StatusApproved || !strings.Contains(merged, "Which plan?") {
		t.Fatalf("merged: %+v\n%s", m, merged)
	}
	if !projectdocs.ChangedSinceApproval(merged) {
		t.Fatal("an edit merged into an approved document passed as approved")
	}
}

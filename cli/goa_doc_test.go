package cli

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/docs"
)

// goaDocOut runs the command as a user would, with no daemon there.
func goaDocOut(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := Run(append([]string{"goa", "doc"}, args...), &out); err != nil {
		t.Fatalf("goa doc %v: %v", args, err)
	}
	return out.String()
}

func TestGOADocListsEveryDocWithALine(t *testing.T) {
	list := goaDocOut(t)
	names := docs.Names()
	if slices.Contains(names, "PLAN") {
		t.Error("the roadmap ships: it says what is planned as if it were there")
	}
	for _, want := range []string{"rag", "goa-tools", "cli"} {
		if !slices.Contains(names, want) {
			t.Errorf("%s doesn't ship", want)
		}
	}
	for _, name := range names {
		if !regexp.MustCompile(`(?m)^` + name + `\s+\S`).MatchString(list) {
			t.Errorf("the list has no line on %s", name)
		}
		text, err := docs.Text(name)
		if err != nil {
			t.Fatal(err)
		}
		if s := docs.Summary(text); strings.ContainsAny(s, "\n") || !strings.Contains(s, ": ") {
			t.Errorf("%s's line is %q", name, s)
		}
	}
}

func TestGOADocPrintsADocAndItsSection(t *testing.T) {
	if got := goaDocOut(t, "tray"); !strings.HasPrefix(got, "# The tray") {
		t.Errorf("doc starts %.40q", got)
	}
	section := goaDocOut(t, "vault.md", "--section", "keys typed")
	if !strings.HasPrefix(section, "## Keys typed for one host") || strings.Contains(section, "## For agents") {
		t.Errorf("section is %.200q", section)
	}
	// A "#" comment in a code block is no heading.
	for _, name := range docs.Names() {
		text, _ := docs.Text(name)
		for _, h := range docs.Headings(text) {
			if _, err := docs.Section(text, h.Title); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
}

func TestGOADocSaysWhatThereIsWhenWrong(t *testing.T) {
	var out bytes.Buffer
	err := Run([]string{"goa", "doc", "nope"}, &out)
	if err == nil || !strings.Contains(err.Error(), "goa-tools") {
		t.Errorf("unknown doc: %v", err)
	}
	err = Run([]string{"goa", "doc", "vault", "--section", "nope"}, &out)
	if err == nil || !strings.Contains(err.Error(), "Keys typed for one host") {
		t.Errorf("unknown section: %v", err)
	}
}

func TestGOADocSearchFindsTheSection(t *testing.T) {
	got := goaDocOut(t, "--search", "permission rules")
	if !strings.Contains(got, "sync > Settings: the agents' library:") {
		t.Errorf("search found:\n%s", got)
	}
	if got := goaDocOut(t, "--search", "zzzznothing"); !strings.Contains(got, "No doc") {
		t.Errorf("search of nothing: %s", got)
	}
}

// What GOA reads names commands that exist, as far as their first word:
// display and hook are the binaries' own, handled before cli.Commands.
func TestGOADocsNameCommandsThatExist(t *testing.T) {
	named := regexp.MustCompile(`\benowx-app\s+([a-z][a-z-]*)`)
	for _, name := range docs.Names() {
		text, _ := docs.Text(name)
		for _, m := range named.FindAllStringSubmatch(text, -1) {
			if !slices.Contains(Commands, m[1]) && m[1] != "display" && m[1] != "hook" {
				t.Errorf("%s names %q, which isn't a command", name, m[1])
			}
		}
	}
}

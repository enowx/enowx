package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// From an issue fills the New session dialog from one of the project's
// open GitHub issues (project.issues, gh on its host): a prompt with the
// issue, a new branch named after it.

// issuePicker is the list of issues to pick from. It belongs to the main
// thread.
type issuePicker struct {
	open    bool
	search  string
	asked   string // the search the issues answer, or are asked for
	loading bool
	issues  []proto.Issue
	err     string
}

// loadIssues asks for the project's issues matching the search.
func (a *App) loadIssues(project string) {
	p, cl := &a.issues, a.client
	if cl == nil {
		return
	}
	search := p.search
	p.asked, p.loading = search, true
	go func() {
		issues, err := cl.Issues(project, search)
		a.onMain(func() {
			if a.issues.asked != search {
				return
			}
			a.issues.loading, a.issues.issues, a.issues.err = false, issues, ""
			if err != nil {
				a.issues.err = client.Explain(err)
			}
		})
	}()
}

// issueButton opens the list of issues, under the prompt of a git
// project's New session dialog.
func (a *App) issueButton(c *ui.Context) {
	f := &a.newSession
	if !f.project.IsGit {
		return
	}
	b := widget.GhostButton(c, "From a GitHub issue…").Tooltip("Fill the prompt and the branch from an open issue")
	if b.Clicked() {
		a.issues = issuePicker{open: true}
		a.loadIssues(f.project.Path)
	}
	if !a.issues.open {
		return
	}
	ui.PopoverBase(c, b, &a.issues.open, func(panel ui.Element) {
		widget.Popup(panel.AttachTo(b, ui.AnchorTopLeft, ui.AnchorBottomLeft).Margin(theme.S2, 0, 0, 0).Width(520))
		in := widget.Input(c, &a.issues.search, "Search issues (GitHub's search)").FillWidth().AutoFocus()
		if in.Submitted() && a.issues.search != a.issues.asked {
			a.loadIssues(f.project.Path)
		}
		switch {
		case a.issues.err != "":
			ui.Text(c, a.issues.err).FontSize(theme.TextLabel).TextColor(theme.Error).Padding(theme.S2)
		case a.issues.loading:
			ui.Row(c).Gap(theme.S2).Padding(theme.S2).Children(func() {
				widget.Spinner(c, 13)
				ui.Text(c, "Reading the issues…").FontSize(theme.TextLabel).TextColor(theme.Muted)
			})
		case len(a.issues.issues) == 0:
			ui.Text(c, "No open issues match.").FontSize(theme.TextLabel).TextColor(theme.Muted).Padding(theme.S2)
		}
		ui.Scroll(c).FillWidth().MaxHeight(340).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, is := range a.issues.issues {
					item := ui.ButtonBase(c.Key(is.Number)).FillWidth().Label(is.Title).Cursor(ui.CursorPointer)
					widget.PopupItem(item, item.Hovered()).Children(func() {
						ui.Text(c, "#"+strconv.Itoa(is.Number)).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.Muted).Shrink(0)
						ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, is.Title).FontSize(theme.TextUI).SingleLine()
							if len(is.Labels) > 0 {
								ui.Text(c, strings.Join(is.Labels, " · ")).FontSize(theme.TextLabel).TextColor(theme.Muted).SingleLine()
							}
						})
					})
					if item.Clicked() {
						a.fromIssue(is)
						a.issues.open = false
					}
				}
			})
		})
	})
}

// fromIssue fills the New session dialog from issue is.
func (a *App) fromIssue(is proto.Issue) {
	f := &a.newSession
	f.prompt = issuePrompt(is)
	if f.project.IsGit {
		f.branchMode, f.branch = proto.BranchNew, issueBranch(is)
	}
}

// issuePrompt asks the agent to resolve issue is.
func issuePrompt(is proto.Issue) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Resolve GitHub issue #%d: %s\n%s\n", is.Number, is.Title, is.URL)
	if body := strings.TrimSpace(is.Body); body != "" {
		b.WriteString("\n" + body + "\n")
	}
	b.WriteString("\nRead the code it concerns, make the change, and run the project's tests.")
	return b.String()
}

// issueBranch names a branch after issue is: issue-12-fix-the-login-form.
func issueBranch(is proto.Issue) string {
	slug := branchFor(is.Title) // pipeline/<words>
	slug = strings.TrimPrefix(slug, "pipeline/")
	if slug == "" {
		return "issue-" + strconv.Itoa(is.Number)
	}
	return "issue-" + strconv.Itoa(is.Number) + "-" + slug
}

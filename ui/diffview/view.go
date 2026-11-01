package diffview

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Draw shows the panel's part: where the branch stands, what to do with it,
// and the changed files, each opening in the center.
func (v *View) Draw(c *ui.Context, o Options) {
	v.follow(o)
	if v.asked != "" && v.loaded && v.busy == "" {
		v.start(o, v.asked)
		v.asked = ""
	}
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(0, theme.S3, theme.S3, theme.S3).Gap(theme.S2).Children(func() {
		v.toolbar(c, o)
		v.conflictBanner(c, o)
		if v.err != "" {
			widget.Notice(c, nil, theme.Del, v.err, "", nil)
		}
		switch {
		case !v.loaded:
			centered(c, "Reading the changes…")
		case len(v.diff.Files) == 0 && v.err == "":
			msg := "Nothing changed since the last commit."
			if v.status.Base != "" {
				msg = "Nothing differs from " + v.status.Base + "."
			}
			centered(c, msg)
		default:
			v.fileRows(c, o)
		}
	})
	v.commitDialog(c, o)
	v.prDialog(c, o)
	v.mergeDialog(c, o)
	v.discardDialog(c, o)
}

func centered(c *ui.Context, text string) {
	ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(theme.TextSoft).TextAlign(ui.Center)
	})
}

// button is an action of the toolbar.
type button struct {
	action, label string
	kind          widget.ButtonKind
	svg           *ui.SVG
}

// toolbar is how many files changed and what they are compared with, then
// the actions that fit how the branch stands: the first two as buttons, the
// others in a menu. A long summary runs in its frame.
func (v *View) toolbar(c *ui.Context, o Options) {
	st := v.status
	ui.Column(c).Shrink(0).FillWidth().Gap(theme.S2).Children(func() {
		ui.Row(c).FillWidth().MinHeight(theme.ControlHeight).Gap(theme.S2).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Gap(theme.S1).Clip().Children(func() {
				if !v.loaded {
					return
				}
				added, removed := 0, 0
				for _, f := range v.diff.Files {
					added += f.Added
					removed += f.Removed
				}
				widget.Running(c, fmt.Sprintf("%d %s changed", len(v.diff.Files), textfmt.Plural(len(v.diff.Files), "file", "files")), widget.RunStyle{Weight: 500})
				if added > 0 {
					ui.Text(c, "+"+strconv.Itoa(added)).FontSize(theme.TextUI).TextColor(theme.Add).SingleLine().Shrink(0)
				}
				if removed > 0 {
					ui.Text(c, "−"+strconv.Itoa(removed)).FontSize(theme.TextUI).TextColor(theme.Del).SingleLine().Shrink(0)
				}
			})
			v.baseMenu(c, st)
		})
		if v.busy != "" {
			ui.Row(c).FillWidth().Height(theme.ControlHeight).Gap(theme.S2).Children(func() {
				widget.Spinner(c, 14)
				widget.Running(c, v.busy+"…", widget.RunStyle{Color: theme.TextSoft})
			})
			return
		}
		if !v.loaded {
			return
		}
		actions := []button{
			{ActCommit, "Commit…", widget.Primary, icon.Check},
			{ActPush, "Push", widget.Secondary, icon.ArrowUp},
			{ActPR, "Pull request…", widget.Secondary, icon.Globe},
			{ActMerge, "Merge into " + st.Base, widget.Secondary, icon.Git},
		}
		var shown, rest []button
		for _, a := range actions {
			if !v.can(a.action) {
				continue
			}
			if len(shown) < 2 {
				shown = append(shown, a)
			} else {
				rest = append(rest, a)
			}
		}
		ui.Row(c).FillWidth().Height(theme.ControlHeight).Gap(theme.S2).Children(func() {
			for _, a := range shown {
				if widget.Button(c.Key(a.action), a.kind, a.svg, a.label, "").Clicked() {
					v.start(o, a.action)
				}
			}
			ui.Spacer(c)
			more := widget.IconButton(c, icon.More, "More", false)
			v.moreMenu.OpenBelow(more, "more", theme.ControlHeight)
			v.moreMenu.Show(c, more, "more", func(done func()) {
				if sum := v.summary(); sum != "" {
					ui.Text(c, sum).FontSize(theme.TextLabel).TextColor(theme.TextSoft).Padding(theme.S1, theme.S3).MaxWidth(280)
				}
				if st.Uncommitted > 0 {
					ui.Text(c, fmt.Sprintf("%d uncommitted", st.Uncommitted)).FontSize(theme.TextLabel).TextColor(theme.Warning).Padding(theme.S1, theme.S3)
				}
				for _, a := range rest {
					if widget.MenuItem(c.Key(a.action), a.svg, a.label, false) {
						v.start(o, a.action)
						done()
					}
				}
				if widget.MenuItem(c, icon.Sync, "Refresh", false) {
					v.reload()
					done()
				}
				if v.lastPRURL != "" && widget.MenuItem(c, icon.Globe, "Open the pull request", false) {
					o.OpenURL(v.lastPRURL)
					done()
				}
				if st.Worktree && widget.MenuItem(c, icon.Trash, "Discard worktree…", true) {
					v.start(o, ActDiscard)
					done()
				}
			})
		})
	})
}

// Actions a view takes, which the app can also ask for (Ask), as from its
// command palette.
const (
	ActCommit  = "commit"
	ActPush    = "push"
	ActPR      = "pr"
	ActMerge   = "merge"
	ActDiscard = "discard"
)

// Ask takes action once the view has loaded, or says why it can't.
func (v *View) Ask(action string) { v.asked = action }

// can reports whether action fits where the branch stands.
func (v *View) can(action string) bool {
	st := v.status
	switch action {
	case ActCommit:
		return st.Uncommitted > 0
	case ActPush:
		return st.Remote != "" && st.Branch != "" && (st.Upstream == "" || st.Unpushed > 0)
	case ActPR:
		return st.HasGH && st.Remote != "" && st.Base != "" && st.Ahead > 0
	case ActMerge:
		return st.CanMerge && st.Ahead > 0 && st.Uncommitted == 0
	case ActDiscard:
		return st.Worktree
	}
	return false
}

// whyNot says why action doesn't fit now.
func (v *View) whyNot(action string) string {
	st := v.status
	switch action {
	case ActCommit:
		return "Nothing to commit."
	case ActPush:
		if st.Remote == "" {
			return "The repository has no remote to push to."
		}
		return "Nothing to push."
	case ActPR:
		switch {
		case !st.HasGH:
			return "Opening a pull request needs the GitHub CLI (gh) on this host."
		case st.Remote == "":
			return "The repository has no remote."
		case st.Base == "":
			return "Pick a branch to compare with first."
		}
		return st.Branch + " has no commits " + st.Base + " doesn't."
	case ActMerge:
		switch {
		case st.ProjectDirty:
			return "The project folder has uncommitted changes."
		case !st.CanMerge:
			return "Only a worktree can be merged, into the branch the project folder has checked out."
		case st.Uncommitted > 0:
			return "Commit the changes before merging them."
		}
		return st.Branch + " has nothing " + st.Base + " doesn't."
	case ActDiscard:
		return "Only a session's own worktree can be discarded."
	}
	return ""
}

// start takes action: a form for those that need one or a confirmation,
// at once for a push.
func (v *View) start(o Options, action string) {
	if !v.can(action) {
		v.err = v.whyNot(action)
		return
	}
	switch action {
	case ActCommit:
		v.commit = &commitForm{open: true}
	case ActPush:
		branch := v.status.Branch
		v.run(o, "Pushing", func(cl *client.Client) error {
			_, err := cl.Push(o.Session.ID, v.base)
			return err
		}, func() { o.Notify("Pushed " + branch) })
	case ActPR:
		v.pr = &prForm{open: true}
	case ActMerge:
		v.merging = true
	case ActDiscard:
		v.discard = &discardForm{open: true, deleteBranch: true}
	}
}

// summary is how the branch stands against its base and its remote.
func (v *View) summary() string {
	st := v.status
	var parts []string
	if st.Base != "" && (st.Ahead > 0 || st.Behind > 0) {
		parts = append(parts, fmt.Sprintf("%d ahead, %d behind %s", st.Ahead, st.Behind, st.Base))
	}
	switch {
	case st.Remote == "":
	case st.Upstream == "" && st.Ahead > 0:
		parts = append(parts, "not pushed")
	case st.Unpushed > 0:
		parts = append(parts, fmt.Sprintf("%d to push", st.Unpushed))
	}
	if v.diff.Truncated {
		parts = append(parts, "more files than shown")
	}
	return strings.Join(parts, " · ")
}

// baseMenu picks what the changes are compared with.
func (v *View) baseMenu(c *ui.Context, st proto.ReviewStatus) {
	label := "last commit"
	if st.Base != "" {
		label = st.Base
	}
	b := widget.Trigger(ui.ButtonBase(c), false).Width(96).Shrink(0).Label("Compare with " + label).Justify(ui.Start)
	b.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Running(c, label, widget.RunStyle{}) })
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	v.baseMenuState.OpenBelow(b, "base", theme.ControlHeight)
	v.baseMenuState.Show(c, b, "base", func(done func()) {
		if widget.MenuItem(c, icon.Check, "Since the last commit", false) {
			v.setBase(noBase)
			done()
		}
		for _, base := range st.Bases {
			if widget.MenuItem(c, icon.Git, "Against "+base, false) {
				v.setBase(base)
				done()
			}
		}
	})
}

// noBase asks for the changes since the last commit. The daemon takes an
// empty base for the session's default, so the last commit is "HEAD".
const noBase = "HEAD"

func (v *View) commitDialog(c *ui.Context, o Options) {
	f := v.commit
	if f == nil {
		return
	}
	submit := false
	widget.Dialog(c, &f.open, 480, 14, func() {
		widget.DialogHeader(c, "Commit changes", fmt.Sprintf("Commits all %d changed %s in %s, new files included.",
			v.status.Uncommitted, textfmt.Plural(v.status.Uncommitted, "file", "files"), v.status.Branch), false)
		in := widget.Input(c, &f.message, "Commit message").FillWidth().AutoFocus()
		submit = in.Submitted()
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.open = false
			}
			if widget.PrimaryButton(c, icon.Check, "Commit", "").Clicked() {
				submit = true
			}
		})
	})
	if submit && strings.TrimSpace(f.message) != "" {
		msg := f.message
		f.open = false
		v.run(o, "Committing", func(cl *client.Client) error {
			_, err := cl.Commit(o.Session.ID, msg)
			return err
		}, func() { o.Notify("Committed: " + textfmt.FirstLine(msg, 60)) })
	}
	if !f.open {
		v.commit = nil
	}
}

func (v *View) prDialog(c *ui.Context, o Options) {
	f := v.pr
	if f == nil {
		return
	}
	submit := false
	st := v.status
	widget.Dialog(c, &f.open, 520, 14, func() {
		widget.DialogHeader(c, "Open a pull request",
			fmt.Sprintf("Pushes %s and asks GitHub to merge it into %s, with gh on this host.", st.Branch, st.Base), false)
		widget.Field(c, "Title", func() {
			widget.Input(c, &f.title, "From the commits when empty").FillWidth().AutoFocus()
		})
		widget.Field(c, "Description", func() {
			widget.Input(c, &f.body, "Optional").FillWidth()
		})
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.open = false
			}
			if widget.PrimaryButton(c, icon.Globe, "Open pull request", "").Clicked() {
				submit = true
			}
		})
	})
	if submit {
		p := proto.PRParams{Session: o.Session.ID, Base: st.Base, Title: f.title, Body: f.body}
		f.open = false
		var url string
		v.run(o, "Opening a pull request", func(cl *client.Client) error {
			pr, err := cl.PullRequest(p)
			url = pr.URL
			return err
		}, func() {
			v.lastPRURL = url
			o.Notify("Opened " + url)
			o.OpenURL(url)
		})
	}
	if !f.open {
		v.pr = nil
	}
}

func (v *View) mergeDialog(c *ui.Context, o Options) {
	if !v.merging {
		return
	}
	st := v.status
	msg := fmt.Sprintf("Merges the %d %s of %s into %s in the project folder. A merge that conflicts is undone.",
		st.Ahead, textfmt.Plural(st.Ahead, "commit", "commits"), st.Branch, st.Base)
	switch ui.AlertDialog(c, &v.merging, "Merge "+st.Branch+" into "+st.Base+"?", msg, "Cancel", "Merge") {
	case 1:
		v.merging = false
		v.run(o, "Merging", func(cl *client.Client) error {
			_, err := cl.Merge(o.Session.ID, v.base)
			return err
		}, func() { o.Notify("Merged " + st.Branch + " into " + st.Base) })
	case 0, ui.AlertDismissed:
		v.merging = false
	}
}

func (v *View) discardDialog(c *ui.Context, o Options) {
	f := v.discard
	if f == nil {
		return
	}
	submit := false
	st := v.status
	widget.Dialog(c, &f.open, 480, 14, func() {
		body := "Ends the sessions in this worktree and deletes its folder."
		if st.Uncommitted > 0 {
			body += fmt.Sprintf(" Its %d uncommitted %s are lost.", st.Uncommitted, textfmt.Plural(st.Uncommitted, "change", "changes"))
		}
		widget.DialogHeader(c, "Discard the worktree of "+st.Branch+"?", body, false)
		widget.Checkbox(c, &f.deleteBranch, "Also delete the branch "+st.Branch)
		widget.DialogFooter(c, "", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.open = false
			}
			if widget.DangerButton(c, "Discard").Clicked() {
				submit = true
			}
		})
	})
	if submit {
		del := f.deleteBranch
		f.open = false
		v.run(o, "Discarding", func(cl *client.Client) error {
			return cl.Discard(o.Session.ID, del)
		}, func() { o.Notify("Discarded the worktree of " + st.Branch) })
	}
	if !f.open {
		v.discard = nil
	}
}

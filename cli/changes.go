package cli

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/enowx/enowx/proto"
)

const changesUsage = `usage: enowx-app changes SESSION [ACTION] [--base B] [--host H]
  (none) | status                  its branch against its base: ahead, behind, uncommitted, unpushed
  diff [--files]                   what it changed, uncommitted included (--files: the list only)
  commit -m MESSAGE                commit every change in its folder
  push --yes                       push its branch
  pr [--title T] [--body B] --yes  push it and open a pull request into its base (gh)
  merge --yes                      merge its branch into its base, in the project folder
  discard [--delete-branch] --yes  end the sessions in its worktree and remove it (and its branch)
push, pr, merge and discard can't be undone: they need --yes, and from an agent the user says yes in the app`

// changes shows a session's changes against its base and acts on them.
func (r runner) changes(args []string) error {
	fs := flag.NewFlagSet("changes", flag.ContinueOnError)
	base := fs.String("base", "", "")
	msg := fs.String("m", "", "")
	title := fs.String("title", "", "")
	body := fs.String("body", "", "")
	files := fs.Bool("files", false, "")
	deleteBranch := fs.Bool("delete-branch", false, "")
	yes := fs.Bool("yes", false, "")
	s, rest, err := r.sessionAndFlags(fs, args, changesUsage)
	if err != nil {
		return err
	}
	action := "status"
	if len(rest) > 0 {
		action = rest[0]
	}
	if len(rest) > 1 {
		return errors.New(changesUsage)
	}
	ref := proto.ReviewRef{Session: s.ID, Base: *base}
	var st proto.ReviewStatus
	switch action {
	case "status":
		st, err = r.c.ReviewStatus(s.ID, *base)
	case "diff":
		return r.printDiff(s.ID, *base, *files)
	case "commit":
		if strings.TrimSpace(*msg) == "" {
			return errors.New("changes commit: give its message with -m")
		}
		st, err = r.c.Commit(s.ID, *msg)
	case "push":
		if err := needYes(*yes, "pushing "+s.ID+"'s branch"); err != nil {
			return err
		}
		err = r.guarded(proto.MethodReviewPush, ref, &st)
	case "pr":
		if err := needYes(*yes, "pushing "+s.ID+"'s branch and opening a pull request"); err != nil {
			return err
		}
		var pr proto.PullRequest
		if err := r.guarded(proto.MethodReviewPR, proto.PRParams{Session: s.ID, Base: *base, Title: *title, Body: *body}, &pr); err != nil {
			return err
		}
		fmt.Fprintln(r.out, pr.URL)
		return nil
	case "merge":
		if err := needYes(*yes, "merging "+s.ID+"'s branch"); err != nil {
			return err
		}
		err = r.guarded(proto.MethodReviewMerge, ref, &st)
	case "discard":
		if err := needYes(*yes, "discarding "+s.ID+"'s worktree"); err != nil {
			return err
		}
		if err := r.guarded(proto.MethodReviewDiscard, proto.DiscardParams{Session: s.ID, DeleteBranch: *deleteBranch}, nil); err != nil {
			return err
		}
		fmt.Fprintln(r.out, "Discarded.")
		return nil
	default:
		return errors.New(changesUsage)
	}
	if err != nil {
		return err
	}
	printReviewStatus(r, st)
	return nil
}

func printReviewStatus(r runner, st proto.ReviewStatus) {
	fmt.Fprintf(r.out, "branch %s", nonEmpty(st.Branch, "(detached)"))
	if st.Base != "" {
		fmt.Fprintf(r.out, " against %s: %d ahead, %d behind", st.Base, st.Ahead, st.Behind)
	}
	fmt.Fprintf(r.out, "\n%d uncommitted files", st.Uncommitted)
	if st.Upstream != "" {
		fmt.Fprintf(r.out, ", %d commits not pushed to %s", st.Unpushed, st.Upstream)
	}
	fmt.Fprintln(r.out)
	if st.Worktree {
		fmt.Fprintf(r.out, "worktree: %s\n", st.Dir)
	}
	if st.CanMerge {
		fmt.Fprintln(r.out, "can be merged into its base")
	} else if st.ProjectDirty {
		fmt.Fprintln(r.out, "the project folder has changes of its own: merging waits for them")
	}
}

// printDiff prints what session id changed, as a unified diff.
func (r runner) printDiff(id, base string, filesOnly bool) error {
	d, err := r.c.Diff(id, base)
	if err != nil {
		return err
	}
	if len(d.Files) == 0 {
		fmt.Fprintln(r.out, "No changes.")
		return nil
	}
	for _, f := range d.Files {
		if filesOnly {
			fmt.Fprintf(r.out, "%-8s +%d -%d  %s\n", f.Status, f.Added, f.Removed, f.Path)
			continue
		}
		fmt.Fprintf(r.out, "--- %s\n+++ %s (%s, +%d -%d)\n", nonEmpty(f.OldPath, f.Path), f.Path, f.Status, f.Added, f.Removed)
		if f.Binary {
			fmt.Fprintln(r.out, "(binary)")
		}
		for _, h := range f.Hunks {
			fmt.Fprintln(r.out, h.Header)
			for _, l := range h.Lines {
				fmt.Fprintf(r.out, "%c%s\n", l.Kind, l.Text)
			}
		}
		if f.Truncated {
			fmt.Fprintln(r.out, "(cut: too long)")
		}
	}
	if d.Truncated {
		fmt.Fprintln(r.out, "(more files than shown)")
	}
	return nil
}

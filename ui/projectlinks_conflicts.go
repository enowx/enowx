package ui

import (
	"bytes"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/projectlinks"
	"github.com/enowx/enowx/core/projectsync"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// What changed on both copies of a linked project since their last sync
// stays as each has it until the user picks: one copy's version, or both
// (the other's kept beside it, its name marked with its host's).

// Picks for a conflict.
const (
	keepSource = iota
	keepTarget
	keepBoth
)

// linkConflicts lists link l's files in conflict, each with its picks.
func (a *App) linkConflicts(c *ui.Context, f *linksForm, l proto.ProjectLinkState) {
	_, srcOn := a.reach(l.Source.Daemon)
	_, dstOn := a.reach(l.Target.Daemon)
	online := srcOn && dstOn && a.links.running[l.ID] == ""
	now := time.Now()
	ui.Column(c).FillWidth().Shrink(0).Gap(theme.S2).Padding(theme.S3, 0, 0, 0).BorderWidth(0.5, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Text(c, "Changed on both since the last sync: keep whose?").FontSize(theme.TextUI).FontWeight(500)
		for _, x := range l.Status.Conflicts {
			ui.Column(c.Key(x.Path)).FillWidth().Shrink(0).Gap(theme.S1).Children(func() {
				ui.Row(c.Key("file")).FillWidth().Gap(theme.S2).Children(func() {
					ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, x.Path, widget.RunStyle{Size: theme.TextLabel, Color: theme.Warning}) })
					ui.Text(c, "since "+textfmt.Age(x.Found, now)).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
				})
				widget.CardNote(c.Key("sides"), sideNote(l.Source.Host, x.Sides[0], x.Gone[0])+" · "+sideNote(l.Target.Host, x.Sides[1], x.Gone[1]), 0)
				ui.Row(c.Key("picks")).FillWidth().Gap(theme.S1).Wrap().Children(func() {
					if !x.Binary && !x.Gone[0] && !x.Gone[1] && widget.GhostButton(c, "Compare").Clicked() {
						a.openConflictDiff(f, l.ProjectLink, x)
					}
					for keep, label := range []string{"Keep " + l.Source.Host + "'s", "Keep " + l.Target.Host + "'s", "Keep both"} {
						if keep == keepBoth && (x.Gone[0] || x.Gone[1]) {
							continue
						}
						b := widget.Button(c.Key(keep), widget.Secondary, nil, label, "").Disabled(!online)
						if b.Clicked() && online {
							a.resolveConflict(f, l.ProjectLink, x, keep)
						}
					}
				})
			})
		}
		if !srcOn || !dstOn {
			widget.CardNote(c.Key("online"), "Both hosts must be online to settle them.", 0)
		}
	})
}

func sideNote(host string, f proto.SyncFile, gone bool) string {
	if gone {
		return host + ": deleted"
	}
	return fmt.Sprintf("%s: %s, %s", host, textfmt.Bytes(uint64(f.Size)), textfmt.Age(f.Modified, time.Now()))
}

// resolveConflict brings the version picked to both copies, then syncs
// the link, which settles the conflict.
func (a *App) resolveConflict(f *linksForm, l proto.ProjectLink, x proto.ProjectSyncConflict, keep int) {
	src, ok1 := a.reach(l.Source.Daemon)
	dst, ok2 := a.reach(l.Target.Daemon)
	if !ok1 || !ok2 {
		return
	}
	a.links.running[l.ID] = "resolving"
	clients := [2]*client.Client{src.cl, dst.cl}
	winner := projectsync.Side(keep)
	if keep == keepBoth {
		winner = projectlinks.Source
	}
	go func() {
		var err error
		if keep == keepBoth {
			_, err = dst.cl.KeepProjectCopy(proto.ProjectKeepCopyParams{Project: l.Target.Path, Path: x.Path, Suffix: l.Target.Host})
		}
		if err == nil {
			err = projectlinks.Sync{
				Link: l, Daemons: [2]projectlinks.Daemon{src.cl, dst.cl},
				Carry: func(from projectsync.Side, id string, size int64) error {
					return client.CopyTransfer(clients[from], clients[from.Other()], id, size, nil)
				},
			}.Bring([]projectsync.File{{Path: x.Path, From: winner, Delete: x.Gone[winner]}})
		}
		a.onMain(func() {
			delete(a.links.running, l.ID)
			if err != nil {
				f.err = client.Explain(err)
				return
			}
			f.diff = nil
			a.runLink(l, src, dst, nil)
		})
	}()
}

// conflictDiff compares both versions of a file in conflict.
type conflictDiff struct {
	link    proto.ProjectLink
	file    proto.ProjectSyncConflict
	lines   []diffLine
	err     string
	loading bool
}

type diffLine struct {
	op   byte // ' ', '-' (the source's), '+' (the copy's)
	text string
}

func (a *App) openConflictDiff(f *linksForm, l proto.ProjectLink, x proto.ProjectSyncConflict) {
	src, ok1 := a.reach(l.Source.Daemon)
	dst, ok2 := a.reach(l.Target.Daemon)
	if !ok1 || !ok2 {
		return
	}
	d := &conflictDiff{link: l, file: x, loading: true}
	f.diff = d
	go func() {
		before, err1 := src.cl.ProjectFile(l.Source.Path, x.Path)
		after, err2 := dst.cl.ProjectFile(l.Target.Path, x.Path)
		a.onMain(func() {
			d.loading = false
			switch {
			case err1 != nil:
				d.err = client.Explain(err1)
			case err2 != nil:
				d.err = client.Explain(err2)
			case !utf8.Valid(before) || !utf8.Valid(after) || bytes.IndexByte(before, 0) >= 0:
				d.err = "Not text: pick a version without comparing"
			default:
				d.lines = lineDiff(string(before), string(after))
				if d.lines == nil {
					d.err = "Too long to compare here"
				}
			}
		})
	}()
}

func (a *App) conflictDiffDialog(c *ui.Context, f *linksForm) {
	d := f.diff
	open := true
	widget.Sheet(c, &open, 720, func() {
		widget.SheetHead(c, &open, d.file.Path, "Lines only on "+d.link.Source.Host+"'s are red, only on "+d.link.Target.Host+"'s green.")
	}, func() {
		switch {
		case d.loading:
			widget.Spinner(c, 14)
		case d.err != "":
			widget.Notice(c, icon.Alert, theme.Muted, d.err, "", nil)
		default:
			widget.Scroll(c.Key("lines")).FillWidth().MaxHeight(440).Radius(theme.RadiusCard).Background(widget.CodeGround).Children(func() {
				ui.Column(c).FillWidth().Padding(theme.S2, 0).Children(func() {
					for i, l := range d.lines {
						col, bg := widget.CodeText, ui.Color{}
						switch l.op {
						case '-':
							col, bg = theme.Del, theme.Del.Alpha(0.1)
						case '+':
							col, bg = theme.Add, theme.Add.Alpha(0.1)
						}
						ui.Text(c.Key(i), string(l.op)+" "+strings.TrimRight(l.text, "\n")).FontSize(theme.TextLabel).
							Font(theme.MonoFont).TextColor(col).Background(bg).FillWidth().PaddingX(theme.S3)
					}
				})
			})
		}
	}, func() {
		ui.Spacer(c)
		if widget.GhostButton(c, "Back").Clicked() {
			f.diff = nil
		}
		for keep, label := range []string{"Keep " + d.link.Source.Host + "'s", "Keep " + d.link.Target.Host + "'s", "Keep both"} {
			if widget.Button(c.Key(keep), widget.Secondary, nil, label, "").Clicked() {
				a.resolveConflict(f, d.link, d.file, keep)
			}
		}
	})
	if !open {
		f.diff = nil
	}
}

// lineDiff is a's lines turned into b's, nil when too long to compare.
func lineDiff(a, b string) []diffLine {
	x, y := strings.SplitAfter(a, "\n"), strings.SplitAfter(b, "\n")
	if len(x)*len(y) > 4_000_000 {
		return nil
	}
	// lcs[i][j] is the longest common run of x[i:] and y[j:].
	lcs := make([][]int32, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []diffLine
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			out = append(out, diffLine{' ', x[i]})
			i, j = i+1, j+1
		case j < len(y) && (i == len(x) || lcs[i][j+1] >= lcs[i+1][j]):
			out = append(out, diffLine{'+', y[j]})
			j++
		default:
			out = append(out, diffLine{'-', x[i]})
			i++
		}
	}
	return out
}

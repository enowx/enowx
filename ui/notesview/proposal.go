package notesview

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// What an agent proposes shows above the page as the lines it changes,
// for the user to accept or reject; an accepted change can be undone.

// Proposal is a change an agent proposed, with the agent's name and the
// page whose agent it is (where a new page it proposes shows).
type Proposal struct {
	proto.NoteProposal
	Agent   string // its name
	AgentID string
	For     string
}

// diffLine is a line of a change: kept (0), removed (-1) or added (1).
type diffLine struct {
	op   int
	text string
}

// lineDiff is how b's lines come from a's: the lines kept, removed and
// added, by their longest common run.
func lineDiff(a, b string) []diffLine {
	x, y := splitLines(a), splitLines(b)
	n, m := len(x), len(y)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []diffLine
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case x[i] == y[j]:
			out = append(out, diffLine{0, x[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, diffLine{-1, x[i]})
			i++
		default:
			out = append(out, diffLine{1, y[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, diffLine{-1, x[i]})
	}
	for ; j < m; j++ {
		out = append(out, diffLine{1, y[j]})
	}
	return out
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// changed is the diff's changes with up to context kept lines around
// each; skipped kept lines show as one "…".
func changed(d []diffLine, context int) []diffLine {
	near := make([]bool, len(d))
	for i, l := range d {
		if l.op == 0 {
			continue
		}
		for k := max(0, i-context); k <= min(len(d)-1, i+context); k++ {
			near[k] = true
		}
	}
	var out []diffLine
	gap := false
	for i, l := range d {
		if near[i] {
			out = append(out, l)
			gap = false
		} else if !gap {
			out = append(out, diffLine{0, "…"})
			gap = true
		}
	}
	return out
}

// proposalCard shows proposal p against the page as it is now (body), and
// its Accept and Reject.
func (v *View) proposalCard(c *ui.Context, o Options, p Proposal, body string) {
	isNew := p.Note.ID == ""
	ui.Column(c).FillWidth().Gap(8).Padding(12).Radius(theme.RadiusSmall).Background(theme.Raised).Border(1, theme.AccentLine).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon.Agent(p.AgentID)).FontSize(13).TextColor(theme.Accent)
			what := p.Agent + " proposes changes to this page"
			if isNew {
				what = p.Agent + " proposes a new page: " + titleOf(p.Note.NoteInfo)
			}
			ui.Text(c, what).FontSize(theme.TextUI).TextColor(theme.TextStrong).Grow(1).Shrink(1)
			if widget.GhostButton(c, "Reject").Clicked() {
				o.Reject(p.ID, v.failed)
			}
			if widget.PrimaryButton(c, icon.Check, "Accept", "").Clicked() {
				v.accept(o, p, body)
			}
		})
		if !isNew && body != p.Before {
			ui.Text(c, "The page changed since: accepting replaces it with this.").FontSize(theme.TextLabel).TextColor(theme.Warning)
		}
		lines := changed(lineDiff(body, p.Note.Body), 2)
		if isNew {
			lines = lineDiff("", p.Note.Body)
		}
		ui.Scroll(c).FillWidth().MaxHeight(320).Children(func() {
			ui.Column(c).FillWidth().Radius(theme.RadiusSmall).Background(theme.Sunken).Padding(6, 0).Children(func() {
				for i, l := range lines {
					row := ui.Row(c.Key(i)).FillWidth().PaddingX(10).Gap(8)
					mark, col := " ", theme.TextSoft
					switch l.op {
					case 1:
						mark, col = "+", theme.Done
						row.Background(theme.Fade(theme.Done, 0.12))
					case -1:
						mark, col = "−", theme.Error
						row.Background(theme.Fade(theme.Error, 0.12))
					}
					row.Children(func() {
						ui.Text(c, mark).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(col).Width(10)
						t := ui.Text(c, orSpace(l.text)).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(col).Grow(1).Shrink(1)
						if l.op == -1 {
							t.Strikethrough()
						}
					})
				}
			})
		})
	})
}

func orSpace(s string) string {
	if s == "" {
		return " "
	}
	return s
}

// accept makes p's change, keeping what the page said for Undo.
func (v *View) accept(o Options, p Proposal, body string) {
	o.Accept(p.ID, func(info proto.NoteInfo, err error) {
		if err != nil {
			v.err = err.Error()
			return
		}
		v.undo = &undoAccept{id: info.ID, body: body, isNew: p.Note.ID == ""}
		if pg := v.page; pg != nil && pg.id == info.ID {
			v.reload(o, pg)
		}
	})
}

// undoAccept is what Undo puts back: the page's body before an accepted
// change, or, for a new page, deleting it.
type undoAccept struct {
	id    string
	body  string
	isNew bool
}

// undoBar offers to undo the change accepted last.
func (v *View) undoBar(c *ui.Context, o Options) {
	u := v.undo
	if u == nil {
		return
	}
	ui.Row(c).FillWidth().Gap(8).Padding(8, 12).Radius(theme.RadiusSmall).Background(theme.Raised).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon.Check).FontSize(12).TextColor(theme.Done)
		ui.Text(c, "Change accepted.").FontSize(theme.TextLabel).TextColor(theme.TextSoft).Grow(1)
		if widget.GhostButton(c, "Undo").Clicked() {
			v.undo = nil
			if u.isNew {
				o.Delete(u.id, v.failed)
				return
			}
			o.Load(u.id, func(n proto.Note, err error) {
				if err != nil {
					v.err = err.Error()
					return
				}
				n.Body = u.body
				o.Save(n, func(proto.NoteInfo, error) {
					if pg := v.page; pg != nil && pg.id == u.id {
						v.reload(o, pg)
					}
				})
			})
		}
		if widget.IconButton(c, icon.Close, "Dismiss", false).Clicked() {
			v.undo = nil
		}
	})
}

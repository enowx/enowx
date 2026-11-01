package ui

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Pipelines run agents one after another on one task (proto.Pipeline): the
// New pipeline dialog starts one in the project, and each shows over the
// project's sessions as a line of its stages, which open their sessions.

// pipelineForm is the New pipeline dialog. It belongs to the main thread.
type pipelineForm struct {
	open      bool
	project   proto.ProjectInfo
	task      string
	branch    string
	named     bool // the user typed a branch name: the task no longer names it
	base      string
	branches  []string // the project's, to start from
	stages    []proto.PipelineStage
	editing   int // the stage whose prompt shows, -1 for none
	busy      bool
	err       string
	shareName string // the stages shared with the project's team as (teamkitdefs.go)
}

// openPipeline opens the dialog for project p: plan, implement and review
// by the agent new sessions start with.
func (a *App) openPipeline(p proto.ProjectInfo) {
	f := &pipelineForm{open: true, project: p, stages: proto.DefaultPipeline(a.defaultAgent()), editing: -1}
	if !slices.ContainsFunc(a.agents, func(ag proto.AgentInfo) bool { return ag.ID == f.stages[0].Agent && ag.Command != "" }) {
		f.stages = proto.DefaultPipeline("claude")
	}
	a.piping = f
	go func() {
		detail, err := a.client.InspectProject(p.Path)
		a.onMain(func() {
			if err == nil {
				f.base, f.branches = detail.Branch, detail.Branches
			}
		})
	}()
}

var notBranchChar = regexp.MustCompile(`[^a-z0-9]+`)

// branchFor names a pipeline's branch after its task.
func branchFor(task string) string {
	words := strings.Fields(strings.ToLower(task))
	if len(words) > 5 {
		words = words[:5]
	}
	slug := strings.Trim(notBranchChar.ReplaceAllString(strings.Join(words, "-"), "-"), "-")
	if slug == "" {
		return ""
	}
	return "pipeline/" + slug
}

func (a *App) pipelineDialog(c *ui.Context) {
	f := a.piping
	if f == nil {
		return
	}
	if !f.open {
		a.piping = nil
		return
	}
	widget.Dialog(c, &f.open, 600, 14, func() {
		widget.DialogHeader(c, "New pipeline in “"+f.project.Name+"”",
			"Agents take the task one after another on a branch of its own: each starts from what the one before ended with, once it is done.", false)
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
		widget.Field(c, "Task", func() {
			prev := f.task
			ui.TextAreaBase(c, &f.task).FillWidth().MinHeight(70).MaxHeight(160).Padding(10, 12).FontSize(theme.TextUI).
				Placeholder("What should they do?").Label("Task").Radius(theme.RadiusControl).Background(theme.Sunken).
				Border(1, theme.Border).AutoFocus()
			if f.task != prev && !f.named {
				f.branch = branchFor(f.task)
			}
		})
		widget.Field(c, "Branch", func() {
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				prev := f.branch
				widget.Input(c, &f.branch, "pipeline/add-search").Grow(1).Font(theme.MonoFont)
				if f.branch != prev {
					f.named = true
				}
				if len(f.branches) > 0 {
					ui.Text(c, "from").FontSize(theme.TextLabel).TextColor(theme.Muted)
					ui.Box(c).Width(170).Children(func() { branchSelect(c, &f.base, f.branches) })
				}
			})
		})
		widget.Field(c, "Stages", func() {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				for i := range f.stages {
					a.pipelineStageRow(c.Key(i), f, i)
				}
				if len(f.stages) < 8 && widget.GhostButton(c, "Add a stage").Clicked() {
					last := f.stages[len(f.stages)-1]
					f.stages = append(f.stages, proto.PipelineStage{Name: "Stage", Agent: last.Agent, Prompt: "The task:\n\n{task}\n\nThe step before ended with:\n\n{previous}\n\n"})
					f.editing = len(f.stages) - 1
				}
			})
		})
		a.pipelineTeamRow(c, f)
		widget.DialogFooter(c, "Skip permission prompts follows New session: "+onOff(a.skipPermissions)+".", func() {
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.open = false
			}
			ok := strings.TrimSpace(f.task) != "" && strings.TrimSpace(f.branch) != "" && !f.busy
			if widget.PrimaryButton(c, icon.Play, "Start", "").Disabled(!ok).Clicked() && ok {
				a.startPipeline(f)
			}
		})
	})
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// pipelineStageRow is a stage: its name, its agent, and its prompt to
// open; stages after the first can go.
func (a *App) pipelineStageRow(c *ui.Context, f *pipelineForm, i int) {
	st := &f.stages[i]
	widget.Card(c).FillWidth().Gap(6).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, string(rune('1'+i))).FontSize(theme.TextLabel).TextColor(theme.Muted).Width(12)
			widget.Input(c, &st.Name, "Name").Width(130)
			sel := ui.SelectBase(c, &st.Agent)
			widget.Trigger(sel.Trigger, sel.Open()).Grow(1).Label("Agent").Children(func() {
				agentTile(c, st.Agent, 20)
				ui.Text(c, a.agentName(st.Agent)).FontSize(theme.TextUI).SingleLine().Shrink(1)
				ui.Spacer(c)
				ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.Muted)
			})
			sel.Popup(func(panel ui.Element) {
				widget.Popup(panel.Margin(6, 0, 0, 0).Width(280))
				ui.Scroll(c).FillWidth().MaxHeight(320).Children(func() {
					ui.Column(c).FillWidth().Children(func() {
						for _, ag := range a.agents {
							if ag.Command == "" || !ag.Installed {
								continue
							}
							item := sel.Item(ag.ID)
							widget.PopupItem(item, item.Highlighted()).Children(func() {
								agentTile(c, ag.ID, 20)
								ui.Text(c, ag.Name).FontSize(theme.TextUI)
							})
						}
					})
				})
			})
			label := "Prompt"
			if f.editing == i {
				label = "Hide prompt"
			}
			if widget.GhostButton(c, label).Clicked() {
				if f.editing == i {
					f.editing = -1
				} else {
					f.editing = i
				}
			}
			if i > 0 && widget.IconButton(c, icon.Close, "Remove this stage", false).Clicked() {
				f.stages = slices.Delete(f.stages, i, i+1)
				f.editing = -1
			}
		})
		if f.editing == i && i < len(f.stages) {
			ui.TextAreaBase(c, &f.stages[i].Prompt).FillWidth().MinHeight(90).MaxHeight(200).Padding(8, 10).
				FontSize(12).Label("Prompt").Radius(theme.RadiusControl).Background(theme.Sunken).Border(1, theme.Border)
			ui.Text(c, "{task} is the task, {previous} what the stage before ended with, {base} the branch it started from.").
				FontSize(11).TextColor(theme.Muted)
		}
	})
}

func (a *App) startPipeline(f *pipelineForm) {
	cl := a.client
	if cl == nil {
		return
	}
	f.busy, f.err = true, ""
	params := proto.PipelineStartParams{
		Project: f.project.Path, Task: strings.TrimSpace(f.task), Branch: strings.TrimSpace(f.branch), Base: f.base,
		Stages: slices.Clone(f.stages), SkipPermissions: a.skipPermissions,
	}
	go func() {
		pl, err := cl.StartPipeline(params)
		a.onMain(func() {
			f.busy = false
			if err != nil {
				f.err = client.Explain(err)
				return
			}
			a.upsertPipeline(pl)
			f.open = false
		})
	}()
}

// loadPipelines asks the daemon for its pipelines; older daemons have
// none.
func (a *App) loadPipelines(cl *client.Client, gen int) {
	list, err := cl.Pipelines()
	if err != nil {
		return
	}
	a.onMain(func() {
		if gen == a.gen {
			a.pipelines = list
		}
	})
}

func (a *App) upsertPipeline(p proto.Pipeline) {
	if i := slices.IndexFunc(a.pipelines, func(o proto.Pipeline) bool { return o.ID == p.ID }); i >= 0 {
		a.pipelines[i] = p
		return
	}
	a.pipelines = append(a.pipelines, p)
}

// pipelineEvent applies EventPipelineUpdated.
func (a *App) pipelineEvent(data json.RawMessage) func() {
	var p proto.Pipeline
	if json.Unmarshal(data, &p) != nil {
		return nil
	}
	return func() { a.upsertPipeline(p) }
}

// pipelineCards show project p's pipelines over its sessions: the task,
// then a chip per stage that opens its session.
func (a *App) pipelineCards(c *ui.Context, p proto.ProjectInfo) {
	for _, pl := range a.pipelines {
		if pl.Project != p.Path {
			continue
		}
		// Indented as sessions are, by padding: a margin would push it out.
		ui.Row(c.Key("pipeline:"+pl.ID)).FillWidth().Padding(2, 0, 2, 14).Children(func() {
			a.pipelineCard(c, pl)
		})
	}
}

// pipelineCard is a pipeline's task, then a chip per stage.
func (a *App) pipelineCard(c *ui.Context, pl proto.Pipeline) {
	ui.Column(c).Grow(1).MinWidth(0).Gap(5).Padding(7, 8).Radius(theme.RadiusControl).
		Background(theme.Raised).Border(1, theme.Border).Children(func() {
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			ui.Icon(c, icon.List).FontSize(12).TextColor(theme.Muted)
			ui.Text(c, pl.Task).FontSize(theme.TextLabel).SingleLine().Shrink(1).Grow(1)
			more := widget.IconButton(c, icon.More, "More for this pipeline", false)
			a.menu.OpenBelow(more, "pipeline:"+pl.ID, 28)
			a.menu.Show(c, more, "pipeline:"+pl.ID, func(done func()) { a.pipelineMenuItems(c, pl, done) })
		})
		ui.Row(c).FillWidth().Gap(4).Wrap().Children(func() {
			for i, st := range pl.Stages {
				if i > 0 {
					ui.Icon(c, icon.ChevronRight).FontSize(9).TextColor(theme.Idle)
				}
				a.stageChip(c.Key(i), st)
			}
		})
		if pl.Error != "" {
			ui.Text(c, pl.Error).FontSize(theme.TextLabel).TextColor(theme.Error)
		}
	})
}

// stageChip is a stage's name with its status; it opens its session.
func (a *App) stageChip(c *ui.Context, st proto.PipelineStageRun) {
	chip := ui.ButtonBase(c).Height(22).PaddingX(7).Gap(5).Radius(theme.RadiusSmall).Label(st.Name).Tooltip(st.Name + ": " + st.Status).
		Background(theme.Sunken)
	if st.Session != "" {
		chip.Cursor(ui.CursorPointer)
		if chip.Hovered() {
			chip.Background(theme.Hover)
		}
	}
	chip.Children(func() {
		switch st.Status {
		case proto.PipelineRunning:
			widget.Spinner(c, 9)
		case proto.PipelineDone:
			ui.Icon(c, icon.Check).FontSize(10).TextColor(theme.Done)
		case proto.PipelineWaiting:
			ui.Icon(c, icon.Alert).FontSize(10).TextColor(theme.Warning)
		case proto.PipelineFailed:
			ui.Icon(c, icon.Close).FontSize(10).TextColor(theme.Error)
		default:
			ui.Box(c).Size(7, 7).Radius(theme.RadiusTail).Border(1, theme.Idle)
		}
		ui.Text(c, st.Name).FontSize(theme.TextLabel).SingleLine().Shrink(1)
	})
	if chip.Clicked() && st.Session != "" {
		if s, ok := a.sessionByID(st.Session); ok {
			a.selectSession(s)
		}
	}
}

func (a *App) pipelineMenuItems(c *ui.Context, pl proto.Pipeline, done func()) {
	cl := a.client
	if cl == nil {
		return
	}
	running := pl.Status == proto.PipelineRunning || pl.Status == proto.PipelineWaiting
	if running && widget.MenuItem(c, icon.Stop, "Cancel the pipeline", false) {
		go func() {
			if err := cl.CancelPipeline(pl.ID); err != nil {
				a.onMain(func() { a.status = client.Explain(err) })
			}
		}()
		done()
	}
	if !running && widget.MenuItem(c, icon.Trash, "Remove from the list", false) {
		go func() {
			err := cl.RemovePipeline(pl.ID)
			a.onMain(func() {
				if err != nil {
					a.status = client.Explain(err)
					return
				}
				a.pipelines = slices.DeleteFunc(a.pipelines, func(o proto.Pipeline) bool { return o.ID == pl.ID })
			})
		}()
		done()
	}
}

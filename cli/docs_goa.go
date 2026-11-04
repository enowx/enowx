package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/core/projectdocs"
	"github.com/enowx/enowx/proto"
)

// Project Docs and GOD of Agents: tasks handed over with what the documents
// say, and a project's plan, progress and checks read beside the board. All of
// it runs here rather than in the daemon, so no protocol changes and a daemon
// of before serves it.

func init() {
	docsCommands["plan"] = docsCommand{"plan PROJECT [--json]            its tasks, what's ready, the command to hand each over (delegates nothing)", runner.docsPlan}
	docsCommands["progress"] = docsCommand{"progress PROJECT [--json]        its documents, gate, tasks, board tasks and criteria", runner.docsProgress}
	docsCommands["verify"] = docsCommand{"verify PROJECT --task ID [--json] a board task's result beside its criteria (ticks nothing)", runner.docsVerify}
}

// docsProject reads the project of note ref: the project's note or a document's.
func (r runner) docsProject(ref string) (*projectdocs.Project, error) {
	n, err := r.findNote(ref)
	if err != nil {
		return nil, err
	}
	list, err := r.notesSeen()
	if err != nil {
		return nil, err
	}
	return projectdocs.Load(list, n.ID, r.c.Note)
}

// withDocContext is goa delegate's --doc: the project's gate holds, the prompt
// carries what the documents say, and the title stays what was asked, as the
// prompt's first line is no longer that.
func (r runner) withDocContext(p *proto.GOADelegateParams, ref string) error {
	name, section, _ := strings.Cut(ref, "#")
	n, err := r.findNote(name)
	if err != nil {
		return err
	}
	project, err := r.docsProject(n.ID)
	if err != nil {
		return err
	}
	if blockers := project.Gate(); len(blockers) > 0 {
		return fmt.Errorf("goa delegate: %s isn't approved to build yet:\n- %s\nThe owner approves documents in the Notes page",
			project.Note.Title, strings.Join(blockerLines(blockers), "\n- "))
	}
	docRef := n.ID
	if section != "" {
		docRef += "#" + section
	}
	context, err := projectdocs.TaskContext(project, docRef, projectdocs.ContextLimit)
	if err != nil {
		return err
	}
	if p.Title == "" {
		p.Title = goa.TitleOf(p.Prompt)
	}
	p.Prompt = context + "\nThe task:\n\n" + p.Prompt
	return nil
}

func blockerLines(blockers []projectdocs.Blocker) []string {
	out := make([]string, 0, len(blockers))
	for _, b := range blockers {
		out = append(out, b.Doc+" "+b.Why)
	}
	return out
}

// docsBoardTask is a board task of a project.
type docsBoardTask struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Session string `json:"session,omitempty"`
	Host    string `json:"host,omitempty"`
	Doc     string `json:"doc"`
	Section string `json:"section,omitempty"`
	Result  string `json:"result,omitempty"`
}

// docsBoardTasks is the board's tasks that came from project.
func (r runner) docsBoardTasks(project *projectdocs.Project) ([]docsBoardTask, error) {
	var b proto.GOABoard
	if err := r.c.Call(proto.MethodGOABoard, nil, &b); err != nil {
		return nil, err
	}
	out := []docsBoardTask{}
	for _, t := range b.Tasks {
		id, doc, section, ok := projectdocs.DocRefOf(t.Prompt)
		if ok && id == project.Note.ID {
			out = append(out, docsBoardTask{ID: t.ID, Title: t.Title, Status: t.Status, Session: t.Session, Host: t.Host, Doc: doc, Section: section, Result: t.Result})
		}
	}
	return out, nil
}

// docsArgs reads a command's PROJECT and --json.
func docsArgs(name string, args []string) (project string, asJSON bool, err error) {
	fs := flagSet("docs " + name)
	j := fs.Bool("json", false, "")
	words, err := parseAll(fs, args)
	if err != nil || len(words) != 1 {
		return "", false, errors.New(docsUsage())
	}
	return words[0], *j, nil
}

// docsPlanned is a planned task, who has it and how to hand it over.
type docsPlanned struct {
	projectdocs.PlannedTask
	Board []docsBoardTask `json:"board,omitempty"`
	// Unknown are dependencies the plan doesn't hold.
	Unknown []string `json:"unknown,omitempty"`
	Ready   bool     `json:"ready"`
	Command string   `json:"command,omitempty"`
}

type docsPlanReport struct {
	Project  string `json:"project"`
	Title    string `json:"title"`
	Document string `json:"document,omitempty"`
	// Unfilled are the Work Breakdown's sections still as the template left
	// them: its tasks there may still be the template's examples.
	Unfilled []string      `json:"unfilled,omitempty"`
	Blockers []string      `json:"blockers,omitempty"`
	Tasks    []docsPlanned `json:"tasks"`
}

func (r runner) docsPlan(args []string) error {
	ref, asJSON, err := docsArgs("plan", args)
	if err != nil {
		return err
	}
	project, err := r.docsProject(ref)
	if err != nil {
		return err
	}
	board, err := r.docsBoardTasks(project)
	if err != nil {
		return err
	}
	report := docsPlanReport{Project: project.Note.ID, Title: project.Note.Title, Blockers: blockerLines(project.Gate()), Tasks: []docsPlanned{}}
	work := projectdocs.WorkBreakdown(project)
	if work != nil {
		report.Document, report.Unfilled = work.ID, work.Unfilled
	}
	report.Tasks = r.docsPlanned(project, board)
	if asJSON {
		return r.printJSON(report)
	}
	if work == nil {
		fmt.Fprintf(r.out, "%s has no Work Breakdown and Backlog document, so no tasks are planned.\n", report.Title)
		return nil
	}
	return r.printPlan(report)
}

// docsPlanned reads the plan beside the board. A task on the board that failed
// or stopped is free to hand over again; one that says done isn't, until
// someone has checked it and ticked its to-do.
func (r runner) docsPlanned(project *projectdocs.Project, board []docsBoardTask) []docsPlanned {
	tasks := projectdocs.PlanTasks(project)
	carried := map[string][]docsBoardTask{}
	for _, t := range board {
		for _, id := range projectdocs.TaskIDs(t.Title) {
			carried[id] = append(carried[id], t)
		}
	}
	busy := func(id string) bool {
		return slices.ContainsFunc(carried[id], func(t docsBoardTask) bool {
			return t.Status != proto.GOATaskFailed && t.Status != proto.GOATaskStopped
		})
	}
	known := map[string]bool{}
	for _, t := range tasks {
		known[t.ID] = true
	}
	ready := map[string]bool{}
	for _, t := range projectdocs.Ready(tasks, busy) {
		ready[t.ID] = true
	}
	out := make([]docsPlanned, 0, len(tasks))
	for _, t := range tasks {
		row := docsPlanned{PlannedTask: t, Board: carried[t.ID], Ready: ready[t.ID]}
		for _, dep := range t.Depends {
			if !known[dep] {
				row.Unknown = append(row.Unknown, dep)
			}
		}
		if row.Ready {
			row.Command = r.docsDelegateCommand(project, t)
		}
		out = append(out, row)
	}
	return out
}

// docsDelegateCommand is the command that hands t over; its title is what ties
// the board's task back to the plan, and it starts with the task's ID.
func (r runner) docsDelegateCommand(project *projectdocs.Project, t projectdocs.PlannedTask) string {
	work := projectdocs.WorkBreakdown(project)
	doc := work.ID
	if t.Section != "" {
		doc += "#" + t.Section
	}
	line := t.ID + ": " + t.Text
	prompt := fmt.Sprintf("Do only %s, not the other tasks around it. Once it is committed, tick that to-do with notes check on note %s.", line, work.ID)
	return fmt.Sprintf(`"$ENOWX_BIN" goa delegate --doc %s %s--title %s %s`, doc, r.docsWhere(project), singleQuoted(line), singleQuoted(prompt))
}

// docsWhere is the flags naming where the work is done: the folder the project
// is attached to, else one for whoever runs the command to fill in.
func (r runner) docsWhere(project *projectdocs.Project) string {
	for _, attached := range project.Note.Projects {
		if attached.Path == "" {
			continue
		}
		flags := "--project " + singleQuoted(attached.Path) + " "
		if info, err := r.c.Info(); err == nil && attached.Host != "" && attached.Host != info.ID && attached.HostName != "" {
			flags += "--host " + singleQuoted(attached.HostName) + " "
		}
		return flags
	}
	return "--project DIR "
}

// singleQuoted is s as one word of a shell, nothing in it expanded.
func singleQuoted(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (r runner) printPlan(report docsPlanReport) error {
	done := 0
	for _, t := range report.Tasks {
		if t.Done {
			done++
		}
	}
	fmt.Fprintf(r.out, "%s: %d of %d tasks done (document %s).\n", report.Title, done, len(report.Tasks), report.Document)
	if len(report.Unfilled) > 0 {
		fmt.Fprintf(r.out, "Its sections still to fill: %s. Tasks there may still be the template's examples.\n", strings.Join(report.Unfilled, ", "))
	}
	w := tabwriter.NewWriter(r.out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TASK\tSTATE\tSTAGE\tDEPENDS ON\tBOARD\tTEXT")
	for _, t := range report.Tasks {
		state := "waiting"
		switch {
		case t.Done:
			state = "done"
		case t.Ready:
			state = "ready"
		case slices.ContainsFunc(t.Board, func(b docsBoardTask) bool { return b.Status == proto.GOATaskDone }):
			state = "to check"
		}
		var on []string
		for _, b := range t.Board {
			on = append(on, b.ID+" "+b.Status)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", t.ID, state, nonEmpty(t.Stage, "-"), nonEmpty(strings.Join(t.Depends, ", "), "-"), nonEmpty(strings.Join(on, ", "), "-"), t.Text)
	}
	w.Flush()
	for _, t := range report.Tasks {
		if len(t.Unknown) > 0 {
			fmt.Fprintf(r.out, "%s depends on %s, which the plan doesn't hold: it waits until that's put right.\n", t.ID, strings.Join(t.Unknown, ", "))
		}
	}
	if len(report.Blockers) > 0 {
		fmt.Fprintf(r.out, "\nThe gate is closed, and goa delegate --doc refuses until the owner approves in the Notes page:\n- %s\n", strings.Join(report.Blockers, "\n- "))
	}
	heading := "\nReady, each with its command:\n"
	if len(report.Blockers) > 0 {
		heading = "\nReady once the gate opens, each with its command:\n"
	}
	listed := false
	for _, t := range report.Tasks {
		if t.Ready {
			if !listed {
				fmt.Fprint(r.out, heading)
				listed = true
			}
			fmt.Fprintf(r.out, "%s\n  %s\n", t.ID, t.Command)
		}
	}
	if !listed {
		fmt.Fprintln(r.out, "\nNothing is ready: every task is done, on the board, or waits on another.")
	}
	if slices.ContainsFunc(report.Tasks, func(t docsPlanned) bool { return strings.Contains(t.Command, "--project DIR ") }) {
		fmt.Fprintln(r.out, "\nDIR is the folder to work in: this project's note is attached to none.")
	}
	fmt.Fprintln(r.out, "\nThis handed nothing over.")
	return nil
}

type docsProgressDoc struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type docsProgressReport struct {
	Project      string            `json:"project"`
	Title        string            `json:"title"`
	Documents    []docsProgressDoc `json:"documents"`
	ByStatus     map[string]int    `json:"byStatus"`
	Blockers     []string          `json:"blockers,omitempty"`
	TasksPlanned int               `json:"tasksPlanned"`
	TasksDone    int               `json:"tasksDone"`
	Board        []docsBoardTask   `json:"board"`
	CriteriaAll  int               `json:"criteria"`
	CriteriaDone int               `json:"criteriaDone"`
}

func (r runner) docsProgress(args []string) error {
	ref, asJSON, err := docsArgs("progress", args)
	if err != nil {
		return err
	}
	project, err := r.docsProject(ref)
	if err != nil {
		return err
	}
	board, err := r.docsBoardTasks(project)
	if err != nil {
		return err
	}
	summary := project.Summarize()
	report := docsProgressReport{Project: project.Note.ID, Title: project.Note.Title, Documents: []docsProgressDoc{}, ByStatus: summary.ByStatus,
		Blockers: blockerLines(summary.Blockers), Board: board}
	for _, d := range project.Docs {
		report.Documents = append(report.Documents, docsProgressDoc{ID: d.ID, Title: d.Title, Status: nonEmpty(d.Meta.Status, projectdocs.StatusDraft)})
	}
	for _, t := range projectdocs.PlanTasks(project) {
		report.TasksPlanned++
		if t.Done {
			report.TasksDone++
		}
	}
	for _, c := range projectdocs.Criteria(project, "") {
		report.CriteriaAll++
		if c.Done {
			report.CriteriaDone++
		}
	}
	if asJSON {
		return r.printJSON(report)
	}
	fmt.Fprintf(r.out, "%s: %d documents\n", report.Title, len(report.Documents))
	for _, status := range projectdocs.Statuses {
		var titles []string
		for _, d := range report.Documents {
			if d.Status == status {
				titles = append(titles, d.Title)
			}
		}
		if len(titles) > 0 {
			fmt.Fprintf(r.out, "  %s (%d): %s\n", status, len(titles), strings.Join(titles, ", "))
		}
	}
	if len(report.Blockers) > 0 {
		fmt.Fprintf(r.out, "Gate closed; the owner approves in the Notes page:\n- %s\n", strings.Join(report.Blockers, "\n- "))
	} else {
		fmt.Fprintln(r.out, "Gate open.")
	}
	fmt.Fprintf(r.out, "Tasks planned: %d done of %d.\nAcceptance criteria: %d done of %d.\n", report.TasksDone, report.TasksPlanned, report.CriteriaDone, report.CriteriaAll)
	if len(board) == 0 {
		fmt.Fprintln(r.out, "No task of this project on the board.")
		return nil
	}
	fmt.Fprintln(r.out, "On the board:")
	for _, t := range board {
		fmt.Fprintf(r.out, "  %s %s: %s\n", t.ID, t.Status, t.Title)
	}
	return nil
}

type docsVerifyReport struct {
	Task      string `json:"task"`
	Title     string `json:"title,omitempty"`
	Status    string `json:"status,omitempty"`
	Result    string `json:"result,omitempty"`
	OfProject bool   `json:"ofProject"`
	// Why says why OfProject is false.
	Why      string                  `json:"why,omitempty"`
	Criteria []projectdocs.Criterion `json:"criteria,omitempty"`
	Advice   string                  `json:"advice,omitempty"`
}

// docsVerifyAdvice is what the checker is told: the to-do is the record, and a
// worker's saying it's done is no evidence.
const docsVerifyAdvice = `Tick a criterion only with evidence you have seen yourself, the worker's word not being that: "$ENOWX_BIN" notes check DOC --item TEXT. Name what remains. This command ticked nothing.`

func (r runner) docsVerify(args []string) error {
	fs := flagSet("docs verify")
	id := fs.String("task", "", "")
	asJSON := fs.Bool("json", false, "")
	words, err := parseAll(fs, args)
	if err != nil || len(words) != 1 || *id == "" {
		return errors.New(docsUsage())
	}
	project, err := r.docsProject(words[0])
	if err != nil {
		return err
	}
	var b proto.GOABoard
	if err := r.c.Call(proto.MethodGOABoard, nil, &b); err != nil {
		return err
	}
	t, err := docsFindTask(b.Tasks, *id)
	if err != nil {
		return err
	}
	report := docsVerifyReport{Task: t.ID, Title: t.Title, Status: t.Status, Result: t.Result}
	projectID, doc, section, ok := projectdocs.DocRefOf(t.Prompt)
	switch {
	case !ok:
		report.Why = fmt.Sprintf("task %s wasn't handed over with --doc: it names no document", t.ID)
	case projectID != project.Note.ID:
		report.Why = fmt.Sprintf("task %s isn't of %s: it came from note %s", t.ID, project.Note.Title, projectID)
	default:
		report.OfProject = true
		ref := doc
		if section != "" {
			ref += "#" + section
		}
		report.Criteria, report.Advice = projectdocs.Criteria(project, ref), docsVerifyAdvice
	}
	if *asJSON {
		return r.printJSON(report)
	}
	if !report.OfProject {
		fmt.Fprintln(r.out, report.Why+".")
		return nil
	}
	fmt.Fprintf(r.out, "Task %s (%s): %s\nIts result: %s\n", t.ID, t.Status, t.Title, nonEmpty(t.Result, "none yet"))
	if len(report.Criteria) == 0 {
		fmt.Fprintln(r.out, "This project has no acceptance criteria written for it.")
	} else {
		fmt.Fprintf(r.out, "Acceptance criteria, from %s (note %s):\n", report.Criteria[0].Title, report.Criteria[0].Doc)
	}
	for _, c := range report.Criteria {
		mark := " "
		if c.Done {
			mark = "x"
		}
		fmt.Fprintf(r.out, "- [%s] %s\n", mark, c.Text)
	}
	fmt.Fprintln(r.out, docsVerifyAdvice)
	return nil
}

// docsFindTask is the task id names, or the only one it starts.
func docsFindTask(tasks []proto.GOATask, id string) (proto.GOATask, error) {
	var found []proto.GOATask
	for _, t := range tasks {
		if t.ID == id {
			return t, nil
		}
		if strings.HasPrefix(t.ID, id) {
			found = append(found, t)
		}
	}
	switch len(found) {
	case 0:
		return proto.GOATask{}, fmt.Errorf("docs verify: no task %q on the board", id)
	case 1:
		return found[0], nil
	}
	return proto.GOATask{}, fmt.Errorf("docs verify: %q names several tasks", id)
}

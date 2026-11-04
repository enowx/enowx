package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/project"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/core/transcript"
	"github.com/enowx/enowx/proto"
)

// pipelines are the pipelines the daemon runs or ran (proto.Pipeline),
// kept in pipelines.json so that a restarted daemon goes on with them.
// Each running one has a goroutine of its own (runPipeline); list and
// stop are guarded by mu, and a pipeline changes only under it.
type pipelines struct {
	mu   sync.Mutex
	file string
	list []*proto.Pipeline
	stop map[string]chan struct{} // closed to cancel a running pipeline
}

// How a stage's agent is watched: how often, and, for agents whose hooks
// don't say when they are done, how long it must be quiet to count as done.
const (
	pipelinePoll  = time.Second
	pipelineQuiet = 15 * time.Second
	handoffMax    = 8000 // bytes of a stage's output handed to the next
	maxStages     = 8
)

func (ps *pipelines) load(file string) {
	ps.file, ps.stop = file, map[string]chan struct{}{}
	data, err := os.ReadFile(file)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &ps.list)
}

// saveLocked writes the pipelines; mu is held.
func (ps *pipelines) saveLocked() {
	data, err := json.MarshalIndent(ps.list, "", "  ")
	if err == nil && ps.file != "" {
		_ = writeFileAtomic(ps.file, data)
	}
}

func (ps *pipelines) find(id string) *proto.Pipeline {
	for _, p := range ps.list {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// change changes pipeline id under the lock, saves it and tells the apps.
func (s *Server) changePipeline(id string, fn func(p *proto.Pipeline)) {
	ps := &s.pipes
	ps.mu.Lock()
	p := ps.find(id)
	if p == nil {
		ps.mu.Unlock()
		return
	}
	fn(p)
	ps.saveLocked()
	snapshot := clonePipeline(p)
	ps.mu.Unlock()
	s.broadcast(proto.EventPipelineUpdated, snapshot)
}

func clonePipeline(p *proto.Pipeline) proto.Pipeline {
	c := *p
	c.Stages = slices.Clone(p.Stages)
	return c
}

func (s *Server) listPipelines() []proto.Pipeline {
	ps := &s.pipes
	ps.mu.Lock()
	defer ps.mu.Unlock()
	out := make([]proto.Pipeline, 0, len(ps.list))
	for _, p := range ps.list {
		out = append(out, clonePipeline(p))
	}
	return out
}

// startPipeline checks p and starts running it.
func (s *Server) startPipeline(p proto.PipelineStartParams) (proto.Pipeline, error) {
	task := strings.TrimSpace(p.Task)
	switch {
	case task == "":
		return proto.Pipeline{}, errors.New("pipeline: the task is empty")
	case len(p.Stages) == 0 || len(p.Stages) > maxStages:
		return proto.Pipeline{}, fmt.Errorf("pipeline: give 1 to %d stages", maxStages)
	case !git.ValidBranchName(p.Branch):
		return proto.Pipeline{}, fmt.Errorf("pipeline: %q isn't a branch name", p.Branch)
	}
	proj := project.Inspect(p.Project)
	if !proj.IsGit {
		return proto.Pipeline{}, fmt.Errorf("pipeline: %s is not a git repository", p.Project)
	}
	if slices.Contains(proj.Branches, p.Branch) {
		return proto.Pipeline{}, fmt.Errorf("pipeline: branch %s already exists", p.Branch)
	}
	pl := &proto.Pipeline{
		ID: session.NewID(), Project: proj.Path, Task: task, Branch: p.Branch, Base: p.Base,
		Created: time.Now(), Status: proto.PipelineRunning, Skip: p.SkipPermissions,
	}
	if pl.Base == "" {
		pl.Base = proj.Branch
	}
	for i, st := range p.Stages {
		if _, ok := agent.Find(st.Agent); !ok {
			return proto.Pipeline{}, fmt.Errorf("pipeline: stage %d: unknown agent %q", i+1, st.Agent)
		}
		if strings.TrimSpace(st.Name) == "" {
			st.Name = fmt.Sprintf("Stage %d", i+1)
		}
		pl.Stages = append(pl.Stages, proto.PipelineStageRun{PipelineStage: st, Status: proto.PipelinePending})
	}
	ps := &s.pipes
	ps.mu.Lock()
	ps.list = append(ps.list, pl)
	stop := make(chan struct{})
	ps.stop[pl.ID] = stop
	ps.saveLocked()
	out := clonePipeline(pl)
	ps.mu.Unlock()
	s.broadcast(proto.EventPipelineUpdated, out)
	go s.runPipeline(pl.ID, stop)
	return out, nil
}

// resumePipelines goes on with the pipelines an earlier daemon ran,
// watching the sessions it restored.
func (s *Server) resumePipelines() {
	ps := &s.pipes
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for _, p := range ps.list {
		if p.Status == proto.PipelineRunning || p.Status == proto.PipelineWaiting {
			stop := make(chan struct{})
			ps.stop[p.ID] = stop
			go s.runPipeline(p.ID, stop)
		}
	}
}

func (s *Server) cancelPipeline(id string) error {
	ps := &s.pipes
	ps.mu.Lock()
	stop, ok := ps.stop[id]
	delete(ps.stop, id)
	ps.mu.Unlock()
	if !ok {
		return fmt.Errorf("pipeline %s isn't running", id)
	}
	close(stop)
	s.changePipeline(id, func(p *proto.Pipeline) {
		p.Status = proto.PipelineCancelled
		for i := range p.Stages {
			if st := &p.Stages[i]; st.Status != proto.PipelineDone && st.Status != proto.PipelineFailed {
				st.Status = proto.PipelineCancelled
			}
		}
	})
	return nil
}

func (s *Server) removePipeline(id string) error {
	ps := &s.pipes
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if _, running := ps.stop[id]; running {
		return fmt.Errorf("pipeline %s is running: cancel it first", id)
	}
	n := len(ps.list)
	ps.list = slices.DeleteFunc(ps.list, func(p *proto.Pipeline) bool { return p.ID == id })
	if len(ps.list) == n {
		return fmt.Errorf("no pipeline %s", id)
	}
	ps.saveLocked()
	return nil
}

// runPipeline runs pipeline id's stages from the first not done, until
// they are, one fails, or stop closes. It owns the pipeline's progress.
func (s *Server) runPipeline(id string, stop <-chan struct{}) {
	defer func() {
		s.pipes.mu.Lock()
		delete(s.pipes.stop, id)
		s.pipes.mu.Unlock()
	}()
	for {
		s.pipes.mu.Lock()
		p := s.pipes.find(id)
		if p == nil || p.Status == proto.PipelineCancelled {
			s.pipes.mu.Unlock()
			return
		}
		i := slices.IndexFunc(p.Stages, func(st proto.PipelineStageRun) bool { return st.Status != proto.PipelineDone })
		pl := clonePipeline(p)
		s.pipes.mu.Unlock()
		if i < 0 {
			s.changePipeline(id, func(p *proto.Pipeline) { p.Status = proto.PipelineDone })
			return
		}
		output, err := s.runStage(pl, i, stop)
		select {
		case <-stop:
			return
		default:
		}
		if errors.Is(err, errStopping) {
			return // the next daemon goes on with it
		}
		if err != nil {
			s.changePipeline(id, func(p *proto.Pipeline) {
				p.Status, p.Error = proto.PipelineFailed, err.Error()
				p.Stages[i].Status, p.Stages[i].Ended = proto.PipelineFailed, time.Now()
			})
			return
		}
		s.changePipeline(id, func(p *proto.Pipeline) {
			p.Stages[i].Status, p.Stages[i].Ended, p.Stages[i].Output = proto.PipelineDone, time.Now(), output
		})
	}
}

// runStage runs stage i of pl, or follows its session when it started
// already, until its agent is done; it returns what the agent ended with.
func (s *Server) runStage(pl proto.Pipeline, i int, stop <-chan struct{}) (string, error) {
	st := pl.Stages[i]
	if st.Session == "" {
		previous := ""
		if i > 0 {
			previous = pl.Stages[i-1].Output
		}
		prompt := strings.NewReplacer("{task}", pl.Task, "{previous}", previous, "{base}", pl.Base).Replace(st.Prompt)
		params := proto.SessionCreateParams{
			Project: pl.Project, Agent: st.Agent, Name: st.Name + " · " + shortTask(pl.Task), Prompt: prompt,
			SkipPermissions: pl.Skip, BranchMode: proto.BranchExisting, Branch: pl.Branch,
		}
		if !git.BranchExists(pl.Project, pl.Branch) {
			params.BranchMode, params.Base = proto.BranchNew, pl.Base
		}
		sess, err := s.createSession(params)
		if err != nil {
			return "", err
		}
		st.Session = sess.ID()
		s.changePipeline(pl.ID, func(p *proto.Pipeline) {
			p.Stages[i].Session, p.Stages[i].Started, p.Stages[i].Status = sess.ID(), time.Now(), proto.PipelineRunning
		})
	}
	hooks := agent.ReportsItself(st.Agent)
	started := time.Now()
	quietSince := time.Time{}
	tick := time.NewTicker(pipelinePoll)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return "", errors.New("cancelled")
		case <-s.done:
			return "", errStopping
		case <-tick.C:
		}
		sess, err := s.sessions.Get(st.Session)
		if err != nil {
			return "", fmt.Errorf("the %s session was deleted", st.Name)
		}
		info := sess.Info()
		status := proto.PipelineRunning
		switch {
		case info.Exited && info.ExitCode != 0:
			return "", fmt.Errorf("%s's agent ended with code %d", st.Name, info.ExitCode)
		case info.Exited, info.State == proto.StateDone:
			return s.handoff(sess, st.Agent), nil
		case info.State == proto.StateWaiting:
			status = proto.PipelineWaiting
		case !hooks && !info.Active && !info.Queued && time.Since(started) > pipelineQuiet:
			if quietSince.IsZero() {
				quietSince = time.Now()
			} else if time.Since(quietSince) >= pipelineQuiet {
				return s.handoff(sess, st.Agent), nil
			}
		default:
			quietSince = time.Time{}
		}
		s.pipes.mu.Lock()
		p := s.pipes.find(pl.ID)
		changed := p != nil && p.Stages[i].Status != status
		s.pipes.mu.Unlock()
		if changed {
			s.changePipeline(pl.ID, func(p *proto.Pipeline) {
				p.Stages[i].Status = status
				p.Status = status
			})
		}
	}
}

// handoff is what a stage's agent ended with: its last answer, from its
// conversation where enowX reads it, else the end of its screen.
func (s *Server) handoff(sess *session.Session, agentID string) string {
	text := ""
	if transcript.Supports(agentID) {
		if tr, err := s.transcript(proto.TranscriptParams{ID: sess.ID(), Tail: 400}); err == nil {
			var answer []string
			for _, e := range tr.Entries {
				switch e.Kind {
				case proto.TranscriptUser:
					answer = nil
				case proto.TranscriptAssistant:
					answer = append(answer, e.Text)
				}
			}
			text = strings.Join(answer, "\n\n")
		}
	}
	if strings.TrimSpace(text) == "" {
		lines := strings.Split(strings.TrimRight(sess.Terminal().Text(), "\n "), "\n")
		text = strings.Join(lines[max(0, len(lines)-80):], "\n")
	}
	if len(text) > handoffMax {
		text = "…" + text[len(text)-handoffMax:]
	}
	return strings.TrimSpace(text)
}

// shortTask is the start of a task, to name sessions by.
func shortTask(task string) string {
	line, _, _ := strings.Cut(task, "\n")
	if r := []rune(line); len(r) > 40 {
		line = string(r[:40]) + "…"
	}
	return line
}

// errStopping ends a stage's watch as the daemon stops, leaving the
// pipeline as it was for the next daemon.
var errStopping = errors.New("the daemon is stopping")

func pipelinesFile(dir string) string { return filepath.Join(dir, "pipelines.json") }

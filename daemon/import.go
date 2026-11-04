package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/core/transcript"
	"github.com/enowx/enowx/core/transfer"
	"github.com/enowx/enowx/core/worktree"
	"github.com/enowx/enowx/proto"
)

// maxCarried bounds what a session's screen or conversation may weigh in
// an archive: both are read whole.
const maxCarried = 256 << 20

// unpack unpacks archive p.ID into p.Dest: the project's files, then each
// session, started again on its agent's conversation.
func (s *Server) unpack(p proto.TransferUnpackParams) (proto.TransferResult, error) {
	file, err := s.transferFile(p.ID)
	if err != nil {
		return proto.TransferResult{}, err
	}
	dest, err := expandHome(p.Dest)
	if err != nil {
		return proto.TransferResult{}, err
	}
	if entries, err := os.ReadDir(dest); err == nil && len(entries) > 0 {
		return proto.TransferResult{}, fmt.Errorf("%s already holds files: pick an empty or new folder", dest)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return proto.TransferResult{}, err
	}
	f, err := os.Open(file)
	if err != nil {
		return proto.TransferResult{}, fmt.Errorf("transfer: %w", err)
	}
	defer f.Close()

	var m transfer.Manifest
	in := importing{dest: dest, history: map[string][]byte{}, convo: map[string][]byte{}, dirs: map[string]string{}}
	err = transfer.Read(f, func(e transfer.Entry) error {
		switch {
		case e.Name == transfer.ManifestName:
			if err := json.NewDecoder(io.LimitReader(e, maxCarried)).Decode(&m); err != nil {
				return fmt.Errorf("transfer: manifest: %w", err)
			}
			if m.Version > transfer.Version {
				return errors.New("transfer: the archive is newer than this daemon: update it")
			}
			in.manifest = &m
			return nil
		case in.manifest == nil:
			return errors.New("transfer: the archive has no manifest")
		}
		if rel, ok := strings.CutPrefix(e.Name, transfer.ProjectPrefix+"/"); ok {
			return transfer.Extract(e, dest, rel)
		}
		rest, ok := strings.CutPrefix(e.Name, transfer.SessionPrefix+"/")
		if !ok {
			return nil
		}
		id, what, _ := strings.Cut(rest, "/")
		switch {
		case what == "history.vt":
			in.history[id], err = io.ReadAll(io.LimitReader(e, maxCarried))
			return err
		case what == "transcript.jsonl":
			in.convo[id], err = io.ReadAll(io.LimitReader(e, maxCarried))
			return err
		case strings.HasPrefix(what, "worktree/"):
			dir, err := s.importWorktree(&in, id)
			if err != nil {
				return err
			}
			return transfer.Extract(e, dir, strings.TrimPrefix(what, "worktree/"))
		}
		return nil
	})
	if err != nil {
		return proto.TransferResult{}, err
	}
	if in.manifest == nil {
		return proto.TransferResult{}, errors.New("transfer: the archive is empty")
	}
	in.prune()
	proj, err := s.projects.Add(dest)
	if err != nil {
		return proto.TransferResult{}, err
	}
	s.projectsChanged()

	res := proto.TransferResult{Project: proj}
	ids := map[string]string{} // the source's session ids → the new ones
	for _, ms := range m.Sessions {
		info, note, err := s.importSession(&in, ms, ids)
		if note != "" {
			res.Notes = append(res.Notes, note)
		}
		if err != nil {
			res.Notes = append(res.Notes, sessionLabel(ms)+" didn't come back: "+err.Error())
			continue
		}
		ids[ms.ID] = info.ID
		res.Sessions = append(res.Sessions, info)
	}
	_ = os.Remove(file)
	return res, nil
}

// importing is what unpacking keeps on the way.
type importing struct {
	manifest *transfer.Manifest
	dest     string
	history  map[string][]byte // by the source's session id
	convo    map[string][]byte
	dirs     map[string]string // worktrees made, by session id
	pruned   bool
}

// prune drops the worktrees the source's .git lists, which are paths of
// the source; the ones made here are added again after.
func (in *importing) prune() {
	if !in.pruned {
		in.pruned = true
		_, _ = git.Run(in.dest, "worktree", "prune")
	}
}

// importWorktree makes session id's worktree on its branch, once.
func (s *Server) importWorktree(in *importing, id string) (string, error) {
	if dir, ok := in.dirs[id]; ok {
		return dir, nil
	}
	var ms *transfer.Session
	for i := range in.manifest.Sessions {
		if in.manifest.Sessions[i].ID == id {
			ms = &in.manifest.Sessions[i]
		}
	}
	if ms == nil || !ms.Worktree || ms.Branch == "" {
		return "", fmt.Errorf("transfer: session %s has no worktree to carry", id)
	}
	in.prune()
	wt, err := worktree.Create(s.WorktreeRoot, proto.WorktreeCreateParams{Repo: in.dest, Name: worktree.FolderName(ms.Branch), Branch: ms.Branch})
	if err != nil {
		return "", err
	}
	in.dirs[id] = wt.Path
	return wt.Path, nil
}

// importSession starts ms again here: in its folder (the project's, or
// its worktree made again), its screen shown, its agent on its
// conversation when the archive carries it. The note says what didn't
// come along.
func (s *Server) importSession(in *importing, ms transfer.Session, ids map[string]string) (proto.SessionInfo, string, error) {
	dir := in.dest
	if ms.Worktree {
		d, err := s.importWorktree(in, ms.ID)
		if err != nil {
			return proto.SessionInfo{}, "", err
		}
		dir = d
	}
	note := ""
	convo := ms.Conversation
	if data, ok := in.convo[ms.ID]; ok && convo != "" {
		if err := s.placeConversation(ms, dir, data); err != nil {
			note, convo = sessionLabel(ms)+"'s conversation couldn't be placed: "+err.Error(), ""
		}
	} else if preset, ok := agent.Find(ms.Agent); ok {
		note = preset.Name + " in " + sessionLabel(ms) + " starts a new conversation: enowX carries Claude Code's only."
		convo = ""
	}
	meta := session.Meta{
		Name: ms.Name, Prompt: ms.Prompt, Project: in.dest, Agent: ms.Agent, Branch: ms.Branch, Worktree: ms.Worktree,
		SkipPermissions: ms.Skip, Budget: time.Duration(ms.Budget) * time.Second, Parent: ids[ms.Parent],
	}
	spec := session.Spec{ID: session.NewID(), Dir: dir, Created: ms.Created, History: in.history[ms.ID], Env: s.sessionEnv()}
	var command []string
	switch preset, isAgent := agent.Find(ms.Agent); {
	case isAgent:
		keepBypass(&meta)
		command = thenShell(preset.Argv(agentLaunch(preset, meta.SkipPermissions, "", false, chatFor{guide: s.guideFor(meta), files: s.agentFilesDir(spec.ID)})))
		if convo != "" {
			l := agentLaunch(preset, meta.SkipPermissions, "", true, chatFor{guide: s.guideFor(meta), files: s.agentFilesDir(spec.ID)})
			l.Resume = convo
			l.Hooks = append(l.Hooks, keptArgs(meta)...)
			command = thenShell(preset.Argv(l))
		}
		meta.Restart, meta.Relaunch = thenShell(preset.Argv(agentLaunch(preset, meta.SkipPermissions, "", true, chatFor{guide: s.guideFor(meta), files: s.agentFilesDir(spec.ID)}))), len(preset.Continue) > 0
	case ms.Agent == agent.Shell:
		meta.Relaunch = true
	default:
		// A custom command may not be safe to run again: it shows as it
		// ended.
		spec.Exited, spec.ExitCode, command = true, -1, ms.Command
	}
	spec.Command, spec.Meta = command, meta
	sess, err := s.sessions.Create(spec)
	if err != nil {
		return proto.SessionInfo{}, note, err
	}
	if spec.Exited {
		s.persist.update(sess, command)
	} else {
		s.began(sess, command, "")
	}
	return sess.Info(), note, nil
}

// placeConversation puts ms's conversation where its agent looks for it
// in dir, with the source's folder in it replaced by this one.
func (s *Server) placeConversation(ms transfer.Session, dir string, data []byte) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	folder := filepath.Join(transcript.ClaudeHome(home), "projects", transcript.ClaudeFolder(dir))
	if err := os.MkdirAll(folder, 0o700); err != nil {
		return err
	}
	data = transfer.RewritePaths(data, ms.Dir, dir)
	return os.WriteFile(filepath.Join(folder, filepath.Base(ms.Conversation)+".jsonl"), data, 0o600)
}

// sessionLabel names a session in notes.
func sessionLabel(ms transfer.Session) string {
	switch {
	case ms.Name != "":
		return "“" + ms.Name + "”"
	case ms.Prompt != "":
		return "“" + firstWords(ms.Prompt) + "”"
	}
	return "a session"
}

func firstWords(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if r := []rune(s); len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return s
}

// expandHome makes "~/x" a path of the home folder; the path must be
// absolute.
func expandHome(p string) (string, error) {
	p = strings.TrimSpace(p)
	if rest, ok := strings.CutPrefix(p, "~"); ok && (rest == "" || strings.HasPrefix(rest, "/")) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = home + rest
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%q isn't a full path: start it with / or ~/", p)
	}
	return filepath.Clean(p), nil
}

package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/projectsync"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/core/transfer"
	"github.com/enowx/enowx/proto"
)

// syncing is what applying a sync's archive keeps on the way.
type syncing struct {
	importing
	p     proto.ProjectSyncApplyParams
	local map[string]*session.Session // the project's sessions here, by key
	keys  map[string]string           // the archive's session ids → keys
	moved map[string]bool             // branches moved forward
	res   proto.ProjectSyncResult
	refs  bool // the branches are set
}

// syncApply applies archive p.ID, packed by syncPack on the other host,
// to the copy of the project here.
func (s *Server) syncApply(p proto.ProjectSyncApplyParams) (proto.ProjectSyncResult, error) {
	file, err := s.transferFile(p.ID)
	if err != nil {
		return proto.ProjectSyncResult{}, err
	}
	defer os.Remove(file)
	if info, err := os.Stat(p.Project); err != nil || !info.IsDir() {
		return proto.ProjectSyncResult{}, fmt.Errorf("sync: %s isn't a folder here", p.Project)
	}
	f, err := os.Open(file)
	if err != nil {
		return proto.ProjectSyncResult{}, fmt.Errorf("sync: %w", err)
	}
	defer f.Close()
	in := &syncing{
		importing: importing{dest: p.Project, history: map[string][]byte{}, convo: map[string][]byte{}, dirs: map[string]string{}, pruned: true},
		p:         p, local: map[string]*session.Session{}, keys: map[string]string{}, moved: map[string]bool{},
	}
	for _, sess := range s.projectSessions(p.Project) {
		in.local[sessionKey(sess)] = sess
	}
	var m transfer.Manifest
	err = transfer.Read(f, func(e transfer.Entry) error {
		switch {
		case e.Name == transfer.ManifestName:
			if err := json.NewDecoder(io.LimitReader(e, maxCarried)).Decode(&m); err != nil {
				return fmt.Errorf("sync: manifest: %w", err)
			}
			if m.Version > transfer.Version {
				return errors.New("sync: the archive is newer than this daemon: update it")
			}
			in.manifest = &m
			for _, ms := range m.Sessions {
				in.keys[ms.ID] = projectsync.Key(ms.Created, ms.Agent)
			}
			return nil
		case in.manifest == nil:
			return errors.New("sync: the archive has no manifest")
		case e.Name == bundleName:
			return s.fetchBundle(in, e)
		}
		in.setBranches()
		if rel, ok := strings.CutPrefix(e.Name, transfer.ProjectPrefix+"/"); ok {
			return transfer.Extract(e, p.Project, rel)
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
			dir := ""
			if local, ok := in.local[in.keys[id]]; ok {
				dir = local.Info().Dir
			} else if dir, err = s.importWorktree(&in.importing, id); err != nil {
				return err
			}
			return transfer.Extract(e, dir, strings.TrimPrefix(what, "worktree/"))
		}
		return nil
	})
	if err != nil {
		return in.res, err
	}
	if in.manifest == nil {
		return in.res, errors.New("sync: the archive is empty")
	}
	in.setBranches()
	deleteFiles(p.Project, p.Delete)
	for key, rels := range p.DeleteIn {
		if local, ok := in.local[key]; ok {
			deleteFiles(local.Info().Dir, rels)
		}
	}
	in.resetMoved()
	if _, err := s.projects.Add(p.Project); err == nil {
		s.projectsChanged()
	}
	ids := map[string]string{} // the archive's session ids → the ones here
	for _, ms := range m.Sessions {
		if parent, ok := in.local[ms.ParentKey]; ok && ms.Parent != "" {
			ids[ms.Parent] = parent.ID()
		}
		if local, ok := in.local[in.keys[ms.ID]]; ok {
			ids[ms.ID] = local.ID()
			if data, ok := in.convo[ms.ID]; ok {
				info, err := s.syncConversation(local, ms, data, in.history[ms.ID])
				if err != nil {
					in.res.Notes = append(in.res.Notes, sessionLabel(ms)+"'s conversation didn't come: "+err.Error())
					continue
				}
				in.res.Sessions = append(in.res.Sessions, info)
			}
			continue
		}
		info, note, err := s.importSession(&in.importing, ms, ids)
		if note != "" {
			in.res.Notes = append(in.res.Notes, note)
		}
		if err != nil {
			in.res.Notes = append(in.res.Notes, sessionLabel(ms)+" didn't come: "+err.Error())
			continue
		}
		ids[ms.ID] = info.ID
		in.res.Sessions = append(in.res.Sessions, info)
	}
	return in.res, nil
}

// fetchBundle takes the commits of the bundle e into the repository,
// under syncRefs, then sets the branches.
func (s *Server) fetchBundle(in *syncing, e transfer.Entry) error {
	tmp, err := os.CreateTemp("", "enowx-sync-*.bundle")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, e)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	if _, err := git.Run(in.dest, "fetch", "-q", "--no-tags", tmp.Name(), "+refs/heads/*:"+syncRefs+"*"); err != nil {
		return fmt.Errorf("sync: take the commits: %w", err)
	}
	in.setBranches()
	return nil
}

// setBranches sets the branches the sync brings, once: one only moves
// forward; one that went its own way here keeps the other's commit as a
// branch beside it.
func (in *syncing) setBranches() {
	if in.refs {
		return
	}
	in.refs = true
	defer func() {
		if left, err := branchCommits(in.dest, syncRefs); err == nil {
			for name := range left {
				_, _ = git.Run(in.dest, "update-ref", "-d", syncRefs+name)
			}
		}
	}()
	for _, name := range slices.Sorted(mapKeys(in.p.Branches)) {
		commit := in.p.Branches[name]
		if !git.ValidBranchName(name) {
			continue
		}
		if _, err := git.Run(in.dest, "cat-file", "-e", commit+"^{commit}"); err != nil {
			in.res.Notes = append(in.res.Notes, "Branch "+name+" didn't come: its commits are missing.")
			continue
		}
		cur, err := git.Run(in.dest, "rev-parse", "--verify", "-q", "refs/heads/"+name)
		switch {
		case err != nil || cur == "":
			_, err = git.Run(in.dest, "update-ref", "refs/heads/"+name, commit)
		case cur == commit || isAncestor(in.dest, commit, cur):
			continue // here already, or further on here
		case isAncestor(in.dest, cur, commit):
			if _, err = git.Run(in.dest, "update-ref", "refs/heads/"+name, commit, cur); err == nil {
				in.moved[name] = true
			}
		default:
			beside := name + "-from-" + branchWord(in.p.From)
			if _, err = git.Run(in.dest, "update-ref", "refs/heads/"+beside, commit); err == nil {
				in.res.Notes = append(in.res.Notes, fmt.Sprintf("%s went its own way on both hosts: %s's commits are on the branch %s, to merge.", name, in.p.From, beside))
			}
		}
		if err != nil {
			in.res.Notes = append(in.res.Notes, "Branch "+name+" didn't come: "+err.Error())
		}
	}
}

// resetMoved makes git's index follow the branches moved forward in the
// folders that have them checked out, whose files came with the sync:
// what isn't committed stays as changes.
func (in *syncing) resetMoved() {
	if len(in.moved) == 0 {
		return
	}
	dirs := []string{in.dest}
	for _, sess := range in.local {
		if info := sess.Info(); info.Worktree {
			dirs = append(dirs, info.Dir)
		}
	}
	for _, d := range in.dirs {
		dirs = append(dirs, d)
	}
	for _, dir := range dirs {
		if in.moved[git.CurrentBranch(dir)] {
			_, _ = git.Run(dir, "reset", "-q")
		}
	}
}

func isAncestor(dir, a, b string) bool {
	_, err := git.Run(dir, "merge-base", "--is-ancestor", a, b)
	return err == nil
}

var notBranchWord = regexp.MustCompile(`[^a-z0-9]+`)

// branchWord makes a host's name fit in a branch's.
func branchWord(name string) string {
	w := strings.Trim(notBranchWord.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if w == "" {
		return "other"
	}
	return w
}

func mapKeys[V any](m map[string]V) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// deleteFiles deletes the files rels of dir, and the folders they leave
// empty.
func deleteFiles(dir string, rels []string) {
	for _, rel := range rels {
		target, err := insideOf(dir, rel)
		if err != nil {
			continue
		}
		if os.Remove(target) != nil {
			continue
		}
		for d := filepath.Dir(target); d != filepath.Clean(dir) && strings.HasPrefix(d, dir); d = filepath.Dir(d) {
			if os.Remove(d) != nil {
				break
			}
		}
	}
}

// syncConversation brings the conversation the other host's copy of a
// session went on with to sess: its agent, stopped if running, starts
// again on it, the other's screen shown. It resumes that conversation by
// its id: the agent stopped may still write its own as it exits, which
// would then be the folder's latest.
func (s *Server) syncConversation(sess *session.Session, ms transfer.Session, data, history []byte) (proto.SessionInfo, error) {
	info, meta := sess.Info(), sess.Meta()
	running := !info.Exited
	_, convo := s.carriedSession(sess)
	if history == nil {
		history = sess.History()
	}
	// Stopped first: the agent would write over the conversation.
	s.procsLook()
	if _, err := s.sessions.Replace(session.Spec{
		ID: info.ID, Dir: info.Dir, Created: info.Created, History: history,
		Command: info.Command, Exited: true, Meta: meta,
	}); err != nil {
		return proto.SessionInfo{}, err
	}
	if running {
		s.reapSession(info.ID)
	}
	if convo != "" {
		// Where its agent keeps it here, which an account may have moved.
		target := filepath.Join(filepath.Dir(convo), ms.Conversation+".jsonl")
		if err := os.WriteFile(target, transfer.RewritePaths(data, ms.Dir, info.Dir), 0o600); err != nil {
			return proto.SessionInfo{}, err
		}
	} else if err := s.placeConversation(ms, info.Dir, data); err != nil {
		return proto.SessionInfo{}, err
	}
	s.transcripts.forget(info.ID) // its chat reads the conversation brought
	if !running || !meta.Relaunch {
		next, err := s.sessions.Get(info.ID)
		if err != nil {
			return proto.SessionInfo{}, err
		}
		return next.Info(), nil
	}
	next, err := s.resumeSession(info.ID, ms.Conversation)
	if err != nil {
		return proto.SessionInfo{}, err
	}
	return next.Info(), nil
}

package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/cloud/api"
	cloudclient "github.com/enowx/enowx/cloud/client"
	"github.com/enowx/enowx/core/project"
	"github.com/enowx/enowx/proto"
)

// A project's rules: Markdown every session in it reads first, in its
// guide, whatever its agent. The user's own project keeps them in the
// synced settings (settings/rules/), by the project's Git remote, so a
// clone on another of their hosts has them too; a team's project in the
// team's space (rules/<project>.md), the same for every member, edited by
// its members or only its admins.

// maxRules is the most a project's rules hold: they go in every session's
// guide.
const maxRules = proto.RulesMaxBytes

// rulesPlacesCache keeps each folder's ownRulesPlaces a while.
type rulesPlacesCache struct {
	mu sync.Mutex
	of map[string]ownRulesPlaces
}

// ownRulesFileOf is the file of rules kept under key.
func (s *Server) ownRulesFileOf(key string) string {
	h := sha256.Sum256([]byte(key))
	return filepath.Join(s.dataDir, "settings", "rules", hex.EncodeToString(h[:8])+".md")
}

// ownRulesPlaces are the files a project of the user's may keep its rules in.
type ownRulesPlaces struct {
	home    string   // where its rules go: by its remote, else its main folder
	earlier []string // where an earlier save may have left them, by a path as it was spelled
	above   []string // the rules of the folders it's in, nearest first, for a folder in no repository
	at      time.Time
}

// ownRulesPlacesOf finds folder's places; what Git says of it is kept a while,
// as a prompt's hook asks each time.
func (s *Server) ownRulesPlacesOf(folder string) ownRulesPlaces {
	st := &s.rulesPlaces
	st.mu.Lock()
	c, ok := st.of[folder]
	st.mu.Unlock()
	if ok && time.Since(c.at) < remoteFresh {
		return c
	}
	id, inRepo := project.Identity(folder)
	seen := map[string]bool{}
	var keys []string
	add := func(key string) {
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	if ref := s.remoteRef(folder); ref != "" {
		add(ref) // the same however the remote is written
	}
	add("path:" + id)
	add("path:" + project.Canonical(folder))
	add("path:" + folder)
	for _, wt := range project.Worktrees(id) {
		add("path:" + wt)
	}
	for _, known := range s.projects.Paths() {
		if project.Canonical(known) == id {
			add("path:" + known)
		}
	}
	places := ownRulesPlaces{home: s.ownRulesFileOf(keys[0]), at: time.Now()}
	for _, key := range keys[1:] {
		places.earlier = append(places.earlier, s.ownRulesFileOf(key))
	}
	if !inRepo {
		home, _ := os.UserHomeDir()
		for dir := filepath.Dir(id); dir != filepath.Dir(dir) && dir != home; dir = filepath.Dir(dir) {
			places.above = append(places.above, s.ownRulesFileOf("path:"+dir))
		}
	}
	st.mu.Lock()
	if st.of == nil {
		st.of = map[string]ownRulesPlaces{}
	}
	st.of[folder] = places
	st.mu.Unlock()
	return places
}

// found is the file holding the rules, "" when none does; inherited says it is
// a folder's above.
func (p ownRulesPlaces) found() (file string, inherited bool) {
	for _, f := range append([]string{p.home}, p.earlier...) {
		if exists(f) {
			return f, false
		}
	}
	for _, f := range p.above {
		if exists(f) {
			return f, true
		}
	}
	return "", false
}

// target is the file a save goes in: the one above
// when that is what the user was shown, else its home.
func (p ownRulesPlaces) target() string {
	if f, inherited := p.found(); inherited {
		return f
	}
	return p.home
}

// write saves rules in target, or clears them, and leaves no copy under an
// earlier spelling to come back.
func (p ownRulesPlaces) write(rules string) error {
	target := p.target()
	if rules == "" {
		os.Remove(target)
	} else if err := writeTeamFile(target, []byte(rules+"\n")); err != nil {
		return err
	}
	for _, f := range append([]string{p.home}, p.earlier...) {
		if f != target {
			os.Remove(f)
		}
	}
	return nil
}

// rulesHistoryKeys are the keys folder's versions are kept under: the first
// from now on, the others as earlier saves spelled it.
func rulesHistoryKeys(folder string) []string {
	id, _ := project.Identity(folder)
	keys := []string{"project:" + id}
	if folder != id {
		keys = append(keys, "project:"+folder)
	}
	return keys
}

// teamRulesFile is the file of team's project's rules, under the team's
// rules/ folder; an ID that isn't one addTeamProject makes (projects.json
// comes from the cloud) has none.
func (s *Server) teamRulesFile(team, project string) (string, error) {
	if !teamPattern.MatchString(team) || !teamProjectID.MatchString(project) {
		return "", fmt.Errorf("rules: no team project %q", project)
	}
	dir := filepath.Join(s.teamsDir(), team, "rules")
	file := filepath.Join(dir, project+".md")
	if rel, err := filepath.Rel(dir, file); err != nil || rel != project+".md" {
		return "", fmt.Errorf("rules: no team project %q", project)
	}
	return file, nil
}

// projectRules are the rules of the project at folder path.
func (s *Server) projectRules(path string) (proto.ProjectRules, error) {
	if path == "" {
		return proto.ProjectRules{}, errors.New("rules: name a project's folder")
	}
	if sp, tp, ok := s.teamOfFolder(path); ok {
		file, err := s.teamRulesFile(sp.Team, tp.ID)
		if err != nil {
			return proto.ProjectRules{}, err
		}
		data, _ := os.ReadFile(file)
		editors := tp.RulesBy
		if editors == "" {
			editors = proto.RulesByMembers
		}
		admin := sp.Role == api.RoleOwner || sp.Role == api.RoleAdmin
		return proto.ProjectRules{Path: path, Rules: string(data), Team: sp.Team, Project: tp.Name, Editors: editors,
			CanEdit: admin || sp.Role == api.RoleMember && editors == proto.RulesByMembers}, nil
	}
	file, _ := s.ownRulesPlacesOf(path).found()
	data, _ := os.ReadFile(file)
	return proto.ProjectRules{Path: path, Rules: string(data), CanEdit: true}, nil
}

// saveProjectRules writes a project's rules, as the user may.
func (s *Server) saveProjectRules(p proto.ProjectRules) (proto.ProjectRules, error) {
	cur, err := s.projectRules(p.Path)
	if err != nil {
		return proto.ProjectRules{}, err
	}
	rules := strings.TrimSpace(p.Rules)
	if rules == "" && strings.TrimSpace(cur.Rules) != "" && !p.Clear {
		return proto.ProjectRules{}, errors.New("rules: saving them empty would delete every rule: say so to clear them")
	}
	if len(rules) > maxRules {
		return proto.ProjectRules{}, fmt.Errorf("rules: %d KB at most, they go in every session's guide", maxRules>>10)
	}
	if err := secretsIn(map[string][]byte{"the rules": []byte(rules)}); err != nil {
		return proto.ProjectRules{}, errors.New("rules: they look like they hold a secret: agents read them, keep credentials in the vault")
	}
	if rules != strings.TrimSpace(cur.Rules) {
		s.keepRulesVersion(rulesHistoryKeys(p.Path)[0], cur.Rules)
	}
	if cur.Team == "" {
		if err := s.ownRulesPlacesOf(p.Path).write(rules); err != nil {
			return proto.ProjectRules{}, err
		}
		return s.projectRules(p.Path)
	}
	sp, tp, _ := s.teamOfFolder(p.Path)
	admin := sp.Role == api.RoleOwner || sp.Role == api.RoleAdmin
	if p.Editors != "" && p.Editors != cur.Editors {
		if !admin {
			return proto.ProjectRules{}, fmt.Errorf("rules: only %s's admins say who edits a project's rules", sp.Name)
		}
		if p.Editors != proto.RulesByMembers && p.Editors != proto.RulesByAdmins {
			return proto.ProjectRules{}, fmt.Errorf("rules: %q edit, or %q", proto.RulesByMembers, proto.RulesByAdmins)
		}
		list := s.readTeamProjects(sp.Team)
		if i := slices.IndexFunc(list, func(x storedTeamProject) bool { return x.ID == tp.ID }); i >= 0 {
			list[i].RulesBy = p.Editors
			if err := s.writeTeamProjects(sp.Team, list); err != nil {
				return proto.ProjectRules{}, err
			}
		}
		cur.CanEdit = admin
	}
	if rules != strings.TrimSpace(cur.Rules) {
		if !cur.CanEdit {
			return proto.ProjectRules{}, fmt.Errorf("rules: %s's rules are edited by its admins", tp.Name)
		}
		file, err := s.teamRulesFile(sp.Team, tp.ID)
		if err != nil {
			return proto.ProjectRules{}, err
		}
		if rules == "" {
			os.Remove(file)
		} else if err := writeTeamFile(file, []byte(rules+"\n")); err != nil {
			return proto.ProjectRules{}, err
		}
		go s.recordAudit(sp.Team, api.AuditParams{Action: api.AuditRulesChanged, Detail: tp.Name})
	}
	s.teamSync.kick()
	return s.projectRules(p.Path)
}

// recordAudit records an action in a team's audit log, best effort.
func (s *Server) recordAudit(team string, p api.AuditParams) {
	st, err := s.cloud.store.Get()
	if err != nil || !st.SignedIn() {
		return
	}
	c, err := cloudclient.New(st.URL, st.Token)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cloudTimeout)
	defer cancel()
	if err := c.RecordAudit(ctx, team, p); err != nil {
		s.logger().Warn("cloud: audit", "team", team, "err", err)
	}
}

// rulesGuide is a project's rules as a session's guide gives them.
func (s *Server) rulesGuide(project string) string {
	r, err := s.projectRules(project)
	if err != nil || strings.TrimSpace(r.Rules) == "" {
		return ""
	}
	whose := "the user's"
	if r.Team != "" {
		whose = "the team's"
	}
	rules := strings.TrimSpace(r.Rules)
	return "\n\n## This project's rules\n\nThese are " + whose + " rules for this project: follow them in all you do here.\n\n" + rulesVersionLine(rules) + "\n\n" + rules + "\n"
}

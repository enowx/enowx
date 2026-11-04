package daemon

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
)

// A team's kit (docs/cloud.md): skills, commands and subagents taken from
// a member's agents, kept in kit/ of the team's space and synced with it.
// A session in one of the team's projects is told of them in its guide,
// so any agent uses them.

// kitFolders are where each kind lives in kit/.
var kitFolders = map[string]string{proto.EnvSkill: "skills", proto.EnvCommand: "commands", proto.EnvSubagent: "subagents",
	proto.KitPipeline: "pipelines", proto.KitAutomation: "automations", proto.KitTemplate: "templates"}

// kitDefinitions are the kinds kept as a definition: kit/<kind>s/<name>.json.
var kitDefinitions = []string{proto.KitPipeline, proto.KitAutomation, proto.KitTemplate}

// kitFile is a definition as kept.
type kitFile struct {
	About string          `json:"about,omitempty"`
	Data  json.RawMessage `json:"data"`
}

var kitName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func (s *Server) kitDir(team string) string { return filepath.Join(s.teamsDir(), team, "kit") }

// teamKit is every team's kit.
func (s *Server) teamKit() []proto.TeamKitItem {
	out := []proto.TeamKitItem{}
	for _, sp := range s.spaces() {
		out = append(out, s.kitOf(sp.Team)...)
	}
	return out
}

func (s *Server) kitOf(team string) []proto.TeamKitItem {
	var out []proto.TeamKitItem
	dir := s.kitDir(team)
	for _, kind := range []string{proto.EnvSkill, proto.EnvCommand, proto.EnvSubagent} {
		entries, _ := os.ReadDir(filepath.Join(dir, kitFolders[kind]))
		for _, e := range entries {
			it := proto.TeamKitItem{Team: team, Kind: kind}
			p := filepath.Join(dir, kitFolders[kind], e.Name())
			switch {
			case kind == proto.EnvSkill && e.IsDir():
				it.Name, it.Path = e.Name(), p
				it.About = aboutOf(filepath.Join(p, "SKILL.md"))
			case kind != proto.EnvSkill && !e.IsDir() && strings.HasSuffix(e.Name(), ".md"):
				it.Name, it.Path = strings.TrimSuffix(e.Name(), ".md"), p
				it.About = aboutOf(p)
			default:
				continue
			}
			out = append(out, it)
		}
	}
	for _, kind := range kitDefinitions {
		entries, _ := os.ReadDir(filepath.Join(dir, kitFolders[kind]))
		for _, e := range entries {
			name, ok := strings.CutSuffix(e.Name(), ".json")
			if !ok || e.IsDir() {
				continue
			}
			p := filepath.Join(dir, kitFolders[kind], e.Name())
			var f kitFile
			if data, err := os.ReadFile(p); err != nil || json.Unmarshal(data, &f) != nil {
				continue
			}
			if kind == proto.KitAutomation {
				// As the kit keeps it, whatever a member's host wrote.
				var au proto.Automation
				if json.Unmarshal(f.Data, &au) != nil {
					continue
				}
				f.Data, _ = json.Marshal(kitAutomation(au))
			}
			out = append(out, proto.TeamKitItem{Team: team, Kind: kind, Name: name, About: f.About, Path: p, Data: f.Data})
		}
	}
	return out
}

// saveToKit keeps a definition in a team's kit, checked by its kind.
func (s *Server) saveToKit(p proto.TeamKitSaveParams) (proto.TeamKitItem, error) {
	sp, err := s.memberOf(p.Team)
	if err != nil {
		return proto.TeamKitItem{}, err
	}
	if !slices.Contains(kitDefinitions, p.Kind) {
		return proto.TeamKitItem{}, fmt.Errorf("teams: a kit keeps pipelines, automations and templates, not %q", p.Kind)
	}
	name := strings.TrimSpace(p.Name)
	if !kitName.MatchString(name) {
		return proto.TeamKitItem{}, fmt.Errorf("teams: name it with letters, digits, dots, dashes (%q)", name)
	}
	data := p.Data
	switch p.Kind {
	case proto.KitPipeline:
		var stages []proto.PipelineStage
		if json.Unmarshal(data, &stages) != nil || len(stages) == 0 {
			return proto.TeamKitItem{}, errors.New("teams: a pipeline is its stages, one at least")
		}
	case proto.KitAutomation:
		var au proto.Automation
		if json.Unmarshal(data, &au) != nil || strings.TrimSpace(au.Prompt) == "" || au.Agent == "" || au.Schedule == "" {
			return proto.TeamKitItem{}, errors.New("teams: an automation is an agent, a prompt and a schedule")
		}
		if !slices.ContainsFunc(s.readTeamProjects(sp.Team), func(x storedTeamProject) bool { return remoteRefOf(x.Repo) == au.Project }) {
			return proto.TeamKitItem{}, errors.New("teams: a team's automation runs in one of its projects: name it by the project's ref")
		}
		data, _ = json.Marshal(kitAutomation(au))
	case proto.KitTemplate:
		var t proto.KitTemplateData
		if json.Unmarshal(data, &t) != nil {
			return proto.TeamKitItem{}, errors.New("teams: a template is a repository's link")
		}
		repo, err := cleanRepo(t.Repo)
		if err != nil {
			return proto.TeamKitItem{}, err
		}
		data, _ = json.Marshal(proto.KitTemplateData{Repo: repo})
	}
	if err := secretsIn(map[string][]byte{name: data, "its description": []byte(p.About)}); err != nil {
		return proto.TeamKitItem{}, err
	}
	file, _ := json.Marshal(kitFile{About: strings.TrimSpace(p.About), Data: data})
	if err := writeTeamFile(filepath.Join(s.kitDir(sp.Team), kitFolders[p.Kind], name+".json"), file); err != nil {
		return proto.TeamKitItem{}, err
	}
	s.teamKitChanged()
	for _, it := range s.kitOf(sp.Team) {
		if it.Kind == p.Kind && it.Name == name {
			return it, nil
		}
	}
	return proto.TeamKitItem{}, errors.New("teams: the kit didn't take it")
}

// kitAutomation is automation au as a team's kit keeps it: what it does,
// never that it skips the agent's permissions, nor on: whoever sets one up
// turns it on, after reading its prompt.
func kitAutomation(au proto.Automation) proto.Automation {
	return proto.Automation{Name: au.Name, Project: au.Project, Agent: au.Agent, Prompt: au.Prompt, Schedule: au.Schedule, Branch: au.Branch}
}

// aboutOf is what a skill's or command's file says it is for: its
// description, or its first line.
func aboutOf(file string) string {
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	first := ""
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if d, ok := strings.CutPrefix(line, "description:"); ok {
			return strings.Trim(strings.TrimSpace(d), `"'`)
		}
		if first == "" && line != "" && line != "---" && !strings.Contains(line, ":") {
			first = strings.TrimLeft(line, "# ")
		}
	}
	return first
}

// memberOf is team's space when the host's account may change it.
func (s *Server) memberOf(team string) (proto.Space, error) {
	sp, ok := s.spaceByTeam(team)
	if !ok {
		return proto.Space{}, fmt.Errorf("spaces: team %s isn't on this host", team)
	}
	if sp.Role == api.RoleViewer {
		return sp, fmt.Errorf("teams: as a viewer of %s you can't change its kit", sp.Name)
	}
	return sp, nil
}

// addToKit copies the host's agent item p.Item into p.Team's kit.
func (s *Server) addToKit(p proto.TeamKitAddParams) (proto.TeamKitItem, error) {
	sp, err := s.memberOf(p.Team)
	if err != nil {
		return proto.TeamKitItem{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return proto.TeamKitItem{}, err
	}
	payload, err := envsync.Export(home, p.Item)
	if err != nil {
		return proto.TeamKitItem{}, err
	}
	kind, name := payload.Item.Kind, payload.Item.Name
	folder, ok := kitFolders[kind]
	if !ok {
		return proto.TeamKitItem{}, fmt.Errorf("teams: a kit takes skills, commands and subagents, not a %s", kind)
	}
	if !kitName.MatchString(name) {
		return proto.TeamKitItem{}, fmt.Errorf("teams: %q isn't a name a kit takes", name)
	}
	if len(payload.Item.Secrets) > 0 {
		return proto.TeamKitItem{}, fmt.Errorf("teams: %s isn't shared, it holds what looks secret: %s", name, strings.Join(payload.Item.Secrets, ", "))
	}
	texts := map[string][]byte{}
	for _, f := range payload.Files {
		texts[f.Path] = f.Data
	}
	if err := secretsIn(texts); err != nil {
		return proto.TeamKitItem{}, err
	}
	dst := filepath.Join(s.kitDir(sp.Team), folder)
	switch kind {
	case proto.EnvSkill:
		// Its files under its folder, as they are under the skill's own.
		root := ""
		for _, f := range payload.Files {
			if path.Base(f.Path) == "SKILL.md" {
				root = path.Dir(f.Path)
			}
		}
		if root == "" {
			return proto.TeamKitItem{}, fmt.Errorf("teams: %s has no SKILL.md", name)
		}
		dst = filepath.Join(dst, name)
		os.RemoveAll(dst)
		for _, f := range payload.Files {
			rel, ok := strings.CutPrefix(f.Path, root+"/")
			if !ok || strings.Contains(rel, "..") {
				continue
			}
			if err := writeTeamFile(filepath.Join(dst, filepath.FromSlash(rel)), f.Data); err != nil {
				return proto.TeamKitItem{}, err
			}
		}
	default:
		if len(payload.Files) != 1 {
			return proto.TeamKitItem{}, fmt.Errorf("teams: %s isn't one file", name)
		}
		dst = filepath.Join(dst, name+".md")
		if err := writeTeamFile(dst, payload.Files[0].Data); err != nil {
			return proto.TeamKitItem{}, err
		}
	}
	s.teamKitChanged()
	for _, it := range s.kitOf(sp.Team) {
		if it.Kind == kind && it.Name == name {
			return it, nil
		}
	}
	return proto.TeamKitItem{}, errors.New("teams: the kit didn't take it")
}

// removeFromKit takes an item out of a team's kit.
func (s *Server) removeFromKit(p proto.TeamKitRef) error {
	sp, err := s.memberOf(p.Team)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(s.kitOf(sp.Team), func(it proto.TeamKitItem) bool { return it.Kind == p.Kind && it.Name == p.Name })
	if i < 0 {
		return fmt.Errorf("teams: %s's kit has no %s %q", sp.Name, p.Kind, p.Name)
	}
	if err := os.RemoveAll(s.kitOf(sp.Team)[i].Path); err != nil {
		return err
	}
	s.teamKitChanged()
	return nil
}

func (s *Server) teamKitChanged() {
	s.broadcast(proto.EventTeamKitChanged, s.teamKit())
	s.teamSync.kick()
}

// teamOfFolder is the team whose project folder dir holds, by its Git
// remote: its space and the project.
func (s *Server) teamOfFolder(dir string) (proto.Space, storedTeamProject, bool) {
	if dir == "" {
		return proto.Space{}, storedTeamProject{}, false
	}
	spaces := s.spaces()
	if len(spaces) == 0 {
		return proto.Space{}, storedTeamProject{}, false
	}
	ref := s.remoteRef(dir)
	if ref == "" {
		return proto.Space{}, storedTeamProject{}, false
	}
	for _, sp := range spaces {
		for _, p := range s.readTeamProjects(sp.Team) {
			if remoteRefOf(p.Repo) == ref {
				return sp, p, true
			}
		}
	}
	return proto.Space{}, storedTeamProject{}, false
}

// teamGuide is what a session in a team's project is told of the team:
// whose project it is, and the team's kit.
func (s *Server) teamGuide(dir string) string {
	sp, p, ok := s.teamOfFolder(dir)
	if !ok {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n\n## Team %s\n\nThis project, %s, is team %s's: its notes and canvases are the team's (enowX notes and canvas commands show the team's here).\n", sp.Name, p.Name, sp.Name)
	kit := slices.DeleteFunc(s.kitOf(sp.Team), func(it proto.TeamKitItem) bool { return slices.Contains(kitDefinitions, it.Kind) })
	if len(kit) == 0 {
		return b.String()
	}
	fmt.Fprintf(&b, "The team shares these; use them as you would the user's own, reading each file when it fits the task:\n")
	for _, it := range kit {
		file := it.Path
		if it.Kind == proto.EnvSkill {
			file = filepath.Join(it.Path, "SKILL.md")
		}
		about := ""
		if it.About != "" {
			about = ": " + it.About
		}
		fmt.Fprintf(&b, "- %s %s%s (%s)\n", it.Kind, it.Name, about, file)
	}
	return b.String()
}

// projectGuide is what a session is told of its project beyond enowX's
// guide: the team whose project it is, its kit, and the project's rules.
func (s *Server) projectGuide(project string) string {
	if project == "" {
		return ""
	}
	part := s.teamGuide(project) + s.rulesGuide(project)
	if part == "" {
		return ""
	}
	return agent.ProjectPartMark + part
}

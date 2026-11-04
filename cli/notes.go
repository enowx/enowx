package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/enowx/enowx/proto"
)

const notesUsage = `usage: enowx-app notes COMMAND
  (none)                           the host's notes, as a tree: ID, then title; yours, then each team's
  show NOTE [--section S]          a note's Markdown (NOTE: its ID or title), or one section's
  outline NOTE                     a note's sections (headings, tabs, toggles…): ID, to-dos, title
  new TITLE [--parent NOTE] [--group G] [--team T] [--icon ICON | --icon-svg FILE|-] [--body TEXT|-]
                                   a new note, its ID printed; --body - reads stdin; --team makes it
                                   in a team's space (its name or ID), a page goes in its parent's
  write NOTE [--title T] [--icon ICON | --icon-svg FILE|-] [--body TEXT|- | --append TEXT]
                                   change a note: its body replaced, or added to
  write NOTE --section S [--body TEXT|- | --append TEXT]
                                   change one section only: its content under its heading
  check NOTE --item TEXT [--undo]  tick a to-do (by its text, or a part only it has)
  move NOTE [--parent NOTE|--top] [--before NOTE] | move NOTE --group G ("" for none)
  rm NOTE                          delete a note and the notes under it
  space NOTE --to TEAM|personal [--copy]
                                   move a top-level note, with the notes under it, to a team's space or your own
                                   (copied with --copy); checked for secrets first; out of a team only when its admins allow it
  ask NOTE PROMPT [--agent A] [--new] [--wait]
                                   ask the note's agent in the Notes page (it proposes changes)
  chat NOTE                        what was said with the note's agent, on any host
  agent NOTE [--use A]             the agent the note's page talks to, or pick it
  proposals | accept ID | reject ID
                                   changes agents proposed, waiting for the user
  history NOTE [VERSION]           a note's earlier versions, or one's Markdown
  restore NOTE VERSION             bring an earlier version back
  attach NOTE --project P [--project P2] [--host H] | detach NOTE (--project P | --all)
                                   a note's projects, of any host; "notes projects NOTE" lists them
  --project P [--host H]           the notes attached to a project ("." for this session's)
  groups                           the groups of notes, in order
  group new NAME [--icon ICON] | group rename FROM TO | group icon NAME ICON | group rm NAME
                                   a group's notes stay when it goes, in none
  import FILE|DIR | export [NOTE]  notes from other apps, and out as files (below)
ICON is an SVG icon, never an emoji: lib:name (lucide:rocket), "#rrggbb" after it
for a color ("enowx-app icons search WORD" finds names); --icon-svg takes the
user's own SVG, cleaned, its colors the text's; --icon "" removes a note's`

// notes reads and writes the host's notes, the Notes page's.
func (r runner) notes(args []string) error {
	if len(args) == 0 {
		return r.notesTree()
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "ask", "chat", "agent", "proposals", "accept", "reject":
		return r.noteAgent(cmd, rest) // flags of their own
	}
	switch cmd {
	case "import", "import-status", "import-cancel", "export":
		return r.notesIO(cmd, rest)
	case "attach", "detach", "projects":
		return r.noteProjects(cmd, rest)
	case "--project", "-project":
		fs := flag.NewFlagSet("notes", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		host := fs.String("host", "", "")
		spec, err := parseWithArg(fs, rest)
		if err != nil || spec == "" {
			return errors.New(attachUsage)
		}
		return r.notesOfProject(spec, *host)
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	parent := fs.String("parent", "", "")
	icon := fs.String("icon", "", "")
	iconSVG := fs.String("icon-svg", "", "")
	body := fs.String("body", "", "")
	appendText := fs.String("append", "", "")
	title := fs.String("title", "", "")
	before := fs.String("before", "", "")
	top := fs.Bool("top", false, "")
	group := fs.String("group", "", "")
	section := fs.String("section", "", "")
	item := fs.String("item", "", "")
	undo := fs.Bool("undo", false, "")
	noAttach := fs.Bool("no-attach", false, "")
	teamName := fs.String("team", "", "")
	toSpace := fs.String("to", "", "")
	copyIt := fs.Bool("copy", false, "")
	arg, err := parseWithArg(fs, rest)
	if err != nil {
		return errors.New(notesUsage)
	}
	if *body == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		*body = string(data)
	}
	if isSet(fs, "icon") || isSet(fs, "icon-svg") {
		v, err := iconValue(*icon, *iconSVG)
		if err != nil {
			return err
		}
		*icon = v
	}
	propose := os.Getenv(proto.EnvNotesPropose) != ""
	switch cmd {
	case "history", "restore":
		return r.noteHistory(cmd, rest)
	case "groups":
		return r.noteGroups()
	case "group":
		if propose {
			return errors.New("notes: here notes are only proposed: write or new, which the user accepts")
		}
		return r.noteGroup(rest)
	}
	switch cmd {
	case "rm", "move", "space":
		if propose {
			return errors.New("notes: here notes are only proposed: write or new, which the user accepts")
		}
	}
	switch cmd {
	case "help":
		fmt.Fprintln(r.out, notesUsage)
		fmt.Fprintln(r.out)
		fmt.Fprintln(r.out, notesIOUsage)
		return nil
	case "outline":
		return r.noteOutline(arg)
	case "check":
		return r.noteCheck(arg, *item, !*undo, propose)
	case "show":
		n, err := r.findNote(arg)
		if err != nil {
			return err
		}
		full, err := r.c.Note(n.ID)
		if err != nil {
			return err
		}
		if *section != "" {
			return r.showSection(full, *section)
		}
		fmt.Fprintf(r.out, "# %s\n", full.Title)
		if full.Icon != "" {
			fmt.Fprintf(r.out, "icon: %s\n", iconShown(full.Icon))
		}
		fmt.Fprintf(r.out, "\n%s", full.Body)
		if !strings.HasSuffix(full.Body, "\n") {
			fmt.Fprintln(r.out)
		}
		return nil
	case "new":
		if arg == "" {
			return errors.New(notesUsage)
		}
		if err := guardNewNote(*body); err != nil {
			return err
		}
		team, err := r.findSpace(*teamName)
		if err != nil {
			return err
		}
		if here, _ := sessionTeam(); here != "" {
			// In a team's project, a note is the team's.
			if *teamName != "" && team != here {
				return errors.New("notes: this session is in a team's project: its notes go to that team")
			}
			team = here
		}
		n := proto.Note{NoteInfo: proto.NoteInfo{Title: arg, Icon: *icon, Group: *group, Team: team}, Body: *body}
		if *parent != "" {
			p, err := r.findNote(*parent)
			if err != nil {
				return err
			}
			n.Parent = p.ID
		}
		if propose {
			return r.proposeNote(n)
		}
		info, err := r.c.SaveNote(n)
		if err != nil {
			return err
		}
		if !*noAttach && n.Parent == "" {
			r.attachToSession(info.ID) // made from a project's session, it belongs to it
		}
		fmt.Fprintln(r.out, info.ID)
		return nil
	case "write":
		n, err := r.findNote(arg)
		if err != nil {
			return err
		}
		full, err := r.c.Note(n.ID)
		if err != nil {
			return err
		}
		before := full.Body
		titled := full.Title
		set := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
		if set["title"] {
			full.Title = *title
		}
		if set["icon"] || set["icon-svg"] {
			full.Icon = *icon
		}
		switch {
		case set["section"]:
			if full.Body, err = sectionWritten(full.Body, *section, *body, *appendText, set["append"]); err != nil {
				return err
			}
		case set["body"]:
			full.Body = *body
		case set["append"]:
			if full.Body != "" && !strings.HasSuffix(full.Body, "\n") {
				full.Body += "\n"
			}
			full.Body += *appendText + "\n"
		}
		if propose {
			return r.proposeNote(full)
		}
		if proposed, err := r.guardNoteWrite(titled, before, full); proposed || err != nil {
			return err
		}
		_, err = r.c.SaveNote(full)
		return err
	case "move":
		n, err := r.findNote(arg)
		if err != nil {
			return err
		}
		if err := r.guardNoteRemove(n.ID); err != nil {
			return err
		}
		if isSet(fs, "group") {
			_, err := r.c.SetNoteGroup(n.ID, *group)
			return err
		}
		to := n.Parent
		switch {
		case *top:
			to = ""
		case *parent != "":
			p, err := r.findNote(*parent)
			if err != nil {
				return err
			}
			to = p.ID
		}
		b := ""
		if *before != "" {
			bn, err := r.findNote(*before)
			if err != nil {
				return err
			}
			b = bn.ID
		}
		return r.c.MoveNote(n.ID, to, b)
	case "rm":
		n, err := r.findNote(arg)
		if err != nil {
			return err
		}
		if err := r.guardNoteRemove(n.ID); err != nil {
			return err
		}
		return r.c.DeleteNote(n.ID)
	case "space":
		n, err := r.findNote(arg)
		if err != nil {
			return err
		}
		if err := r.guardNoteRemove(n.ID); err != nil {
			return err
		}
		if *toSpace == "" {
			return errors.New("usage: enowx-app notes space NOTE --to TEAM|personal [--copy]")
		}
		team, err := r.findSpace(*toSpace)
		if err != nil {
			return err
		}
		info, err := r.c.MoveNoteToSpace(n.ID, team, *copyIt)
		if err != nil {
			return err
		}
		fmt.Fprintln(r.out, info.ID)
		return nil
	}
	return errors.New(notesUsage)
}

// proposeNote proposes n for the user to accept in the Notes page.
func (r runner) proposeNote(n proto.Note) error {
	p, err := r.c.ProposeNote(n, os.Getenv("ENOWX_SESSION"))
	if err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Proposed (%s): the user sees it in Notes and accepts or rejects it.\n", p.ID)
	return nil
}

// notesTree prints the notes, each under its parent, by group: the
// user's own, then each team's space under its name.
func (r runner) notesTree() error {
	list, err := r.notesSeen()
	if err != nil {
		return err
	}
	groups := r.groupsSeen(list)
	if len(list) == 0 && len(groups) == 0 {
		fmt.Fprintln(r.out, "No notes yet: \"enowx-app notes new TITLE\" makes one.")
		return nil
	}
	spaces, _ := r.c.Spaces()
	r.spaceTree("", list, groups)
	for _, sp := range spaces {
		if slices.ContainsFunc(list, func(n proto.NoteInfo) bool { return n.Team == sp.Team }) || slices.ContainsFunc(groups, func(g proto.NoteGroup) bool { return g.Team == sp.Team }) {
			fmt.Fprintf(r.out, "\n== %s (team) ==\n", sp.Name)
			r.spaceTree(sp.Team, list, groups)
		}
	}
	return nil
}

// spaceTree prints the notes of team's space ("" the user's own).
func (r runner) spaceTree(team string, list []proto.NoteInfo, all []proto.NoteGroup) {
	children := map[string][]proto.NoteInfo{}
	for _, n := range list {
		if n.Team == team {
			children[n.Parent] = append(children[n.Parent], n)
		}
	}
	var groups []proto.NoteGroup
	for _, g := range all {
		if g.Team == team {
			groups = append(groups, g)
		}
	}
	var walk func(parent string, depth int, group string)
	walk = func(parent string, depth int, group string) {
		for _, n := range children[parent] {
			if depth == 0 && n.Group != group {
				continue
			}
			fmt.Fprintf(r.out, "%s  %s%s\n", n.ID, strings.Repeat("  ", depth), n.Title)
			walk(n.ID, depth+1, group)
		}
	}
	for _, g := range groups {
		fmt.Fprintf(r.out, "[%s]\n", g.Name)
		walk("", 0, g.Name)
	}
	if len(groups) > 0 {
		fmt.Fprintln(r.out, "[No group]")
	}
	walk("", 0, "")
}

// findSpace is the team named or IDed spec, "" (the user's own) for "".
func (r runner) findSpace(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.EqualFold(spec, "personal") {
		return "", nil
	}
	spaces, err := r.c.Spaces()
	if err != nil {
		return "", err
	}
	for _, sp := range spaces {
		if sp.Team == spec || strings.EqualFold(sp.Name, spec) {
			return sp.Team, nil
		}
	}
	return "", fmt.Errorf("notes: no team %q on this host (\"enowx-app team list\" lists yours; the host signs in with \"enowx-app cloud login\")", spec)
}

// noteGroups lists the groups of notes.
func (r runner) noteGroups() error {
	notes, err := r.notesSeen()
	if err != nil {
		return err
	}
	groups := r.groupsSeen(notes)
	if len(groups) == 0 {
		fmt.Fprintln(r.out, "No groups: \"enowx-app notes group new NAME\" makes one.")
	}
	for _, g := range groups {
		fmt.Fprintf(r.out, "%s", g.Name)
		if g.Icon != "" {
			fmt.Fprintf(r.out, "  (%s)", iconShown(g.Icon))
		}
		fmt.Fprintln(r.out)
	}
	return nil
}

// noteGroup makes, renames or deletes a group of notes.
func (r runner) noteGroup(args []string) error {
	usage := errors.New("usage: enowx-app notes group new NAME [--icon ICON] | rename FROM TO | icon NAME ICON | rm NAME")
	if len(args) < 2 {
		return usage
	}
	var err error
	switch args[0] {
	case "new":
		fs := flag.NewFlagSet("group", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		icon := fs.String("icon", "", "")
		iconSVG := fs.String("icon-svg", "", "")
		name, perr := parseWithArg(fs, args[1:])
		if perr != nil || name == "" {
			return usage
		}
		v, ierr := iconValue(*icon, *iconSVG)
		if ierr != nil {
			return ierr
		}
		_, err = r.c.SaveNoteGroup("", proto.NoteGroup{Name: name, Icon: v})
	case "rename":
		if len(args) != 3 {
			return usage
		}
		groups, gerr := r.c.NoteGroups()
		if gerr != nil {
			return gerr
		}
		i := slices.IndexFunc(groups, func(g proto.NoteGroup) bool { return g.Name == args[1] })
		if i < 0 {
			return fmt.Errorf("no group %q: \"enowx-app notes groups\" lists them", args[1])
		}
		g := groups[i]
		g.Name = args[2]
		_, err = r.c.SaveNoteGroup(args[1], g)
	case "icon":
		if len(args) != 3 {
			return usage
		}
		groups, gerr := r.c.NoteGroups()
		if gerr != nil {
			return gerr
		}
		i := slices.IndexFunc(groups, func(g proto.NoteGroup) bool { return g.Name == args[1] })
		if i < 0 {
			return fmt.Errorf("no group %q: \"enowx-app notes groups\" lists them", args[1])
		}
		v, ierr := iconValue(args[2], "")
		if ierr != nil {
			return ierr
		}
		g := groups[i]
		g.Icon = v
		_, err = r.c.SaveNoteGroup(g.Name, g)
	case "rm":
		_, err = r.c.DeleteNoteGroup("", args[1])
	default:
		return usage
	}
	return err
}

// isSet says flag name was given.
func isSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

// findNote is the note whose ID or title (any case) is ref.
func (r runner) findNote(ref string) (proto.NoteInfo, error) {
	if ref == "" {
		return proto.NoteInfo{}, errors.New(notesUsage)
	}
	list, err := r.notesSeen()
	if err != nil {
		return proto.NoteInfo{}, err
	}
	var found []proto.NoteInfo
	for _, n := range list {
		if n.ID == ref {
			return n, nil
		}
		if strings.EqualFold(n.Title, ref) {
			found = append(found, n)
		}
	}
	switch len(found) {
	case 0:
		return proto.NoteInfo{}, fmt.Errorf("no note %q: \"enowx-app notes\" lists them", ref)
	case 1:
		return found[0], nil
	}
	return proto.NoteInfo{}, fmt.Errorf("%d notes are titled %q: use the ID (\"enowx-app notes\")", len(found), ref)
}

// noteTools are the notes as MCP tools.
var noteTools = []mcpTool{
	{name: "notes_list", description: "The user's notes on this host (the Notes page in enowX), as a tree under their groups ([Group]): each note's ID and title.",
		args: func(map[string]any) []string { return nil }},
	{name: "notes_read", description: "A note's Markdown, or one section of it (a long note: notes_outline first, then the section needed).",
		props:    map[string]mcpProp{"note": {"string", "The note's ID or title"}, "section": {"string", "A section's #ID or title, as notes_outline lists them"}},
		required: []string{"note"},
		args: func(a map[string]any) []string {
			return withFlag([]string{"show", str(a, "note")}, a, "section", "section")
		}},
	{name: "notes_outline", description: "A note's sections (headings, tabs, toggles, cards…), each with its #ID and to-dos done, to read or write one alone.",
		props: map[string]mcpProp{"note": {"string", "The note's ID or title"}}, required: []string{"note"},
		args: func(a map[string]any) []string { return []string{"outline", str(a, "note")} }},
	{name: "notes_check", description: "Tick a to-do in a note (or untick it), by its text or a part of it only that to-do has.",
		props:    map[string]mcpProp{"note": {"string", "The note's ID or title"}, "item": {"string", "The to-do's text"}, "undo": {"boolean", "Untick it"}},
		required: []string{"note", "item"},
		args: func(a map[string]any) []string {
			args := withFlag([]string{"check", str(a, "note")}, a, "item", "item")
			if v, _ := a["undo"].(bool); v {
				args = append(args, "--undo")
			}
			return args
		}},
	{name: "notes_create", description: "A new note in the user's notes, in Markdown (task lists, headings, code…); answers its ID.",
		props: map[string]mcpProp{"title": {"string", "Its title"}, "body": {"string", "Its Markdown"}, "parent": {"string", "The note it goes under (ID or title)"},
			"group":    {"string", "The group it goes in, as notes_list shows them in [brackets]; a new name makes one"},
			"icon":     {"string", "An SVG icon, lib:name like lucide:rocket (icons_search finds names); never an emoji"},
			"icon_svg": {"string", "The user's own SVG code as its icon, instead of icon"}},
		required: []string{"title"},
		args: func(a map[string]any) []string {
			args := []string{"new", str(a, "title")}
			for _, k := range []string{"body", "parent", "group", "icon"} {
				args = withFlag(args, a, k, k)
			}
			args = withFlag(args, a, "icon_svg", "icon-svg")
			return args
		}},
	{name: "notes_write", description: "Change a note: replace its body, add to its end, or rename it; with section, change that section's content only (under its heading), leaving the rest as it is.",
		props: map[string]mcpProp{"note": {"string", "The note's ID or title"}, "body": {"string", "Its new Markdown, replacing all of it (or all of the section)"},
			"section": {"string", "A section's #ID or title (notes_outline): body and append then go to it alone"},
			"append":  {"string", "Markdown added at its end"}, "title": {"string", "A new title"},
			"icon":     {"string", "A new SVG icon, lib:name like lucide:rocket (icons_search finds names); never an emoji"},
			"icon_svg": {"string", "The user's own SVG code as its icon, instead of icon"}},
		required: []string{"note"},
		args: func(a map[string]any) []string {
			args := []string{"write", str(a, "note")}
			for _, k := range []string{"body", "append", "title", "icon", "section"} {
				args = withFlag(args, a, k, k)
			}
			args = withFlag(args, a, "icon_svg", "icon-svg")
			return args
		}},
	{name: "notes_delete", description: "Delete a note and the notes under it.",
		props: map[string]mcpProp{"note": {"string", "The note's ID or title"}}, required: []string{"note"},
		args: func(a map[string]any) []string { return []string{"rm", str(a, "note")} }},
}

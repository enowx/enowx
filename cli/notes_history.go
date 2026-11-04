package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/enowx/enowx/proto"
)

// noteHistory lists a note's earlier versions (history NOTE), shows one
// (history NOTE VERSION) or brings it back (restore NOTE VERSION).
func (r runner) noteHistory(cmd string, args []string) error {
	if len(args) == 0 || cmd == "restore" && len(args) != 2 {
		return errors.New("usage: enowx-app notes history NOTE [VERSION] | notes restore NOTE VERSION")
	}
	n, err := r.findNote(args[0])
	if err != nil {
		return err
	}
	switch {
	case cmd == "restore":
		if os.Getenv(proto.EnvNotesPropose) != "" {
			return errors.New("notes: here notes are only proposed: write the version's text with write, which the user accepts")
		}
		if proposed, err := r.guardNoteRestore(n.ID, args[1]); proposed || err != nil {
			return err
		}
		_, err := r.c.RestoreNote(n.ID, args[1])
		return err
	case len(args) == 2:
		v, err := r.c.NoteVersion(n.ID, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(r.out, "# %s\n", v.Title)
		if v.Icon != "" {
			fmt.Fprintf(r.out, "icon: %s\n", iconShown(v.Icon))
		}
		fmt.Fprintf(r.out, "\n%s\n", v.Body)
		return nil
	}
	list, err := r.c.NoteHistory(n.ID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Fprintln(r.out, "no earlier version yet")
	}
	for _, v := range list {
		fmt.Fprintf(r.out, "%s\t%s\t%s\t%d bytes\n", v.ID, v.Time.Local().Format("2006-01-02 15:04"), v.Title, v.Size)
	}
	return nil
}

// noteHistoryTools are a note's versions as MCP tools.
var noteHistoryTools = []mcpTool{
	{name: "notes_history", description: "A note's earlier versions, kept before each change from an agent or another host and every 10 minutes of writing: version ID, when, title, size; with version, that version's Markdown.",
		props: map[string]mcpProp{"note": {"string", "The note's ID or title"}, "version": {"string", "A version's ID, to read it"}}, required: []string{"note"},
		args: func(a map[string]any) []string {
			args := []string{"history", str(a, "note")}
			if v := str(a, "version"); v != "" {
				args = append(args, v)
			}
			return args
		}},
	{name: "notes_restore", description: "Bring a note's earlier version back (what it is now is kept as a version too).",
		props: map[string]mcpProp{"note": {"string", "The note's ID or title"}, "version": {"string", "The version's ID, from notes_history"}}, required: []string{"note", "version"},
		args: func(a map[string]any) []string { return []string{"restore", str(a, "note"), str(a, "version")} }},
}

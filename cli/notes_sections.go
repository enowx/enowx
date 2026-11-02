package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/enowx/enowx/core/noteblocks"
	"github.com/enowx/enowx/proto"
)

// A note read or changed a section at a time (core/noteblocks): its
// outline, one section shown or written, one to-do ticked.

// noteOutline prints note ref's sections: ID, to-dos done, title,
// indented by level.
func (r runner) noteOutline(ref string) error {
	n, err := r.findNote(ref)
	if err != nil {
		return err
	}
	full, err := r.c.Note(n.ID)
	if err != nil {
		return err
	}
	out := noteblocks.Outline(full.Body)
	if len(out) == 0 {
		fmt.Fprintln(r.out, "No sections: the note has no headings or titled blocks.")
		return nil
	}
	if done, total := noteblocks.Progress(full.Body); total > 0 {
		fmt.Fprintf(r.out, "%d/%d to-dos done\n", done, total)
	}
	for _, s := range out {
		todos := ""
		if s.Todos > 0 {
			todos = fmt.Sprintf(" (%d/%d)", s.Done, s.Todos)
		}
		kind := ""
		if !strings.HasPrefix(s.Kind, "h") {
			kind = "[" + s.Kind + "] "
		}
		fmt.Fprintf(r.out, "#%s  %s%s%s%s\n", s.ID, strings.Repeat("  ", max(s.Level-1, 0)), kind, s.Title, todos)
	}
	return nil
}

// showSection prints section name of note n.
func (r runner) showSection(n proto.Note, name string) error {
	s, err := noteblocks.Find(noteblocks.Outline(n.Body), name)
	if err != nil {
		return fmt.Errorf("%w (\"enowx-app notes outline %s\" lists them)", err, n.ID)
	}
	fmt.Fprint(r.out, noteblocks.Text(n.Body, s))
	return nil
}

// sectionWritten is body with section name's content replaced by text,
// or, adding, text added at its end.
func sectionWritten(body, name, text, more string, adding bool) (string, error) {
	s, err := noteblocks.Find(noteblocks.Outline(body), name)
	if err != nil {
		return "", err
	}
	if adding {
		cur := strings.TrimRight(noteblocks.Text(body, s), "\n")
		// The section's own content, without its heading or start.
		_, content, _ := strings.Cut(cur, "\n")
		if content != "" {
			content += "\n"
		}
		text = content + more
	}
	return noteblocks.Replace(body, s, text), nil
}

// noteCheck ticks (or unticks) a to-do of note ref.
func (r runner) noteCheck(ref, item string, done, propose bool) error {
	if item == "" {
		return errors.New("usage: enowx-app notes check NOTE --item TEXT [--undo]")
	}
	n, err := r.findNote(ref)
	if err != nil {
		return err
	}
	full, err := r.c.Note(n.ID)
	if err != nil {
		return err
	}
	body, line, err := noteblocks.Check(full.Body, item, done)
	if err != nil {
		return err
	}
	if body == full.Body {
		fmt.Fprintln(r.out, "Already so.")
		return nil
	}
	full.Body = body
	if propose {
		return r.proposeNote(full)
	}
	if _, err := r.c.SaveNote(full); err != nil {
		return err
	}
	_, text, _ := noteblocks.Todo(noteblocks.Lines(body)[line])
	state := "Ticked"
	if !done {
		state = "Unticked"
	} else if session := os.Getenv(EnvSession); session != "" {
		_ = r.c.Call(proto.MethodNoteTick, proto.NoteTick{Note: n.ID, Item: text, Session: session}, nil) // an older daemon has no record
	}
	fmt.Fprintf(r.out, "%s %q (line %d).\n", state, text, line+1)
	return nil
}

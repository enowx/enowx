package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

// noteAgent drives a note's agent in the Notes page from anywhere:
// ask NOTE PROMPT, chat NOTE (what was said), agent NOTE [--use A],
// proposals, accept ID, reject ID.
func (r runner) noteAgent(cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	use := fs.String("use", "", "")
	agentFlag := fs.String("agent", "", "")
	fresh := fs.Bool("new", false, "")
	wait := fs.Bool("wait", false, "")
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return errors.New(notesUsage)
		}
		if fs.NArg() == 0 {
			break
		}
		pos, args = append(pos, fs.Arg(0)), fs.Args()[1:]
	}
	propose := os.Getenv(proto.EnvNotesPropose) != ""
	switch cmd {
	case "proposals":
		list, err := r.c.NoteProposals()
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Fprintln(r.out, "no changes waiting")
		}
		for _, p := range list {
			what := "changes"
			if p.Note.ID == "" {
				what = "makes"
			}
			fmt.Fprintf(r.out, "%s\t%s %q\n", p.ID, what, p.Note.Title)
		}
		return nil
	case "accept", "reject":
		if len(pos) != 1 {
			return errors.New(notesUsage)
		}
		if propose {
			return errors.New("notes: the user accepts or rejects what you proposed, in the Notes page")
		}
		if cmd == "reject" {
			return r.c.RejectNoteProposal(pos[0])
		}
		if err := r.guardNoteAccept(pos[0]); err != nil {
			return err
		}
		_, err := r.c.AcceptNoteProposal(pos[0])
		return err
	}
	if len(pos) == 0 {
		return errors.New(notesUsage)
	}
	if propose {
		return errors.New("notes: here you are the note's agent")
	}
	n, err := r.findNote(pos[0])
	if err != nil {
		return err
	}
	switch cmd {
	case "agent":
		if *use == "" {
			agent := n.Agent
			if agent == "" {
				agent = "claude (by default)"
			}
			fmt.Fprintln(r.out, agent)
			return nil
		}
		_, err := r.c.SetNoteAgent(n.ID, *use)
		return err
	case "ask":
		prompt := strings.Join(pos[1:], " ")
		if prompt == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			prompt = string(data)
		}
		log, err := r.c.NoteChat(proto.NoteChatParams{ID: n.ID, Prompt: prompt, Agent: *agentFlag, New: *fresh})
		if err != nil {
			return err
		}
		fmt.Fprintf(r.out, "asked (session %s); the user sees it beside the note, and its changes as proposals\n", log.Session)
		if !*wait {
			return nil
		}
		time.Sleep(3 * time.Second) // it starts on the prompt, not on its last turn's end
		quiet := r
		quiet.out = io.Discard
		if err := quiet.wait([]string{log.Session}); err != nil {
			return err
		}
		return r.printNoteChat(n.ID, log.Session)
	case "chat":
		return r.printNoteChat(n.ID, "")
	}
	return errors.New(notesUsage)
}

// printNoteChat prints note id's conversations (only session's, when set).
func (r runner) printNoteChat(id, session string) error {
	log, err := r.c.NoteChatLog(id)
	if err != nil {
		return err
	}
	if len(log.Messages) == 0 {
		fmt.Fprintln(r.out, "no conversation yet")
	}
	for _, m := range log.Messages {
		if session != "" && m.Session != session {
			continue
		}
		who := "user"
		if m.Role == proto.TranscriptAssistant {
			who = m.Agent
		}
		fmt.Fprintf(r.out, "[%s %s on %s]\n%s\n\n", m.Time.Local().Format("2006-01-02 15:04"), who, m.Host, strings.TrimSpace(m.Text))
	}
	return nil
}

// noteAgentTools are a note's agent as MCP tools.
var noteAgentTools = []mcpTool{
	{name: "notes_ask", description: "Ask a note's agent in the user's Notes page something (it works on notes only and proposes its changes, which the user accepts); it continues its conversation, or starts one.",
		props: map[string]mcpProp{"note": {"string", "The note's ID or title"}, "prompt": {"string", "What to ask"},
			"agent": {"string", "Which agent (claude, codex…); the note's own by default"}, "wait": {"boolean", "Wait for its answer, and give it"}},
		required: []string{"note", "prompt"},
		args: func(a map[string]any) []string {
			return withFlag(withFlag([]string{"ask", str(a, "note"), str(a, "prompt")}, a, "agent", "agent"), a, "wait", "wait")
		}},
	{name: "notes_chat", description: "What was said with a note's agent, on any host: the user's messages and the agent's answers.",
		props: map[string]mcpProp{"note": {"string", "The note's ID or title"}}, required: []string{"note"},
		args: func(a map[string]any) []string { return []string{"chat", str(a, "note")} }},
	{name: "notes_agent", description: "Which agent a note's page talks to, or with use, pick it.",
		props: map[string]mcpProp{"note": {"string", "The note's ID or title"}, "use": {"string", "The agent to pick: claude, codex…"}}, required: []string{"note"},
		args: func(a map[string]any) []string { return withFlag([]string{"agent", str(a, "note")}, a, "use", "use") }},
	{name: "notes_proposals", description: "Changes to notes agents proposed, waiting for the user: ID and note.",
		args: func(map[string]any) []string { return []string{"proposals"} }},
	{name: "notes_accept", description: "Make a proposed change to a note, as the user's Accept does. Only when the user asked you to.",
		props: map[string]mcpProp{"id": {"string", "The proposal's ID"}}, required: []string{"id"},
		args: func(a map[string]any) []string { return []string{"accept", str(a, "id")} }},
	{name: "notes_reject", description: "Drop a proposed change to a note, as the user's Reject does. Only when the user asked you to.",
		props: map[string]mcpProp{"id": {"string", "The proposal's ID"}}, required: []string{"id"},
		args: func(a map[string]any) []string { return []string{"reject", str(a, "id")} }},
}

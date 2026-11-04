package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
)

// The Canvas page's agent from the command line: another agent (or a
// script) asks it for a change, which the user watches made, and reads
// what it was asked and said.

const canvasChatUsage = `the Canvas page's agent (as the chat beside the canvas):
  chat CANVAS --prompt TEXT [--agent A] [--design N]   ask it, starting it (with the canvas's agent, else A, else claude) if needed
  chat-history CANVAS              what it was asked (with the canvas's version then) and what it answered`

var chatCommands = []string{"chat", "chat-history"}

func (r runner) canvasChat(cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	prompt := fs.String("prompt", "", "")
	agentID := fs.String("agent", "", "")
	design := fs.String("design", "", "")
	pos, err := parseAll(fs, args)
	if err != nil || len(pos) == 0 {
		return errors.New(canvasChatUsage)
	}
	c, err := r.findCanvas(pos[0])
	if err != nil {
		return err
	}
	log, err := r.c.CanvasChatLog(c.ID)
	if err != nil {
		return err
	}
	if cmd == "chat-history" {
		return r.chatHistory(c, log)
	}
	text := strings.TrimSpace(*prompt + " " + strings.Join(pos[1:], " "))
	if text == "" {
		return errors.New(canvasChatUsage)
	}
	use := *agentID
	if use == "" {
		use = log.Agent
	}
	if use == "" {
		use = "claude"
	}
	msg := canvas.ChatContext(c, *design, "") + text
	s, ok, err := r.canvasSession(c.ID, use)
	if err != nil {
		return err
	}
	if ok {
		err = r.c.Input(s.ID, msg, true, true)
	} else {
		s, err = r.c.CreateSession(proto.SessionCreateParams{Agent: use, CanvasFor: c.ID, Prompt: msg, Cols: 120, Rows: 40})
	}
	if err != nil {
		return err
	}
	if _, err := r.c.CanvasChat(proto.CanvasChatParams{ID: c.ID, Text: text, Session: s.ID, Agent: use}); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Asked %s (session %s); the user watches it in the Canvas page. \"enowx-app wait %s\" waits for it, \"canvas chat-history %s\" reads it.\n", use, s.ID, s.ID, c.ID)
	return nil
}

// canvasSession is canvas id's agent session of agent, the newest still
// running.
func (r runner) canvasSession(id, agent string) (proto.SessionInfo, bool, error) {
	list, err := r.c.Sessions()
	if err != nil {
		return proto.SessionInfo{}, false, err
	}
	for i := len(list) - 1; i >= 0; i-- {
		if s := list[i]; s.CanvasFor == id && s.Agent == agent && !s.Exited && !strings.HasPrefix(s.Group, proto.UnslopGroup) {
			return s, true, nil
		}
	}
	return proto.SessionInfo{}, false, nil
}

func (r runner) chatHistory(c proto.Canvas, log proto.CanvasChatLog) error {
	if len(log.Messages) == 0 {
		fmt.Fprintf(r.out, "No messages yet to the agent of %s.\n", c.Title)
		return nil
	}
	agent := log.Agent
	if agent == "" {
		agent = "the agent"
	}
	fmt.Fprintf(r.out, "%s, with %s (%s mode):\n", c.Title, agent, log.Mode)
	answers := map[string][]string{} // a session's answers, in order
	for _, m := range log.Messages {
		if _, read := answers[m.Session]; read || m.Session == "" {
			continue
		}
		answers[m.Session] = r.answersOf(m.Session)
	}
	asked := map[string]int{}
	for _, m := range log.Messages {
		state := ""
		if m.State != "" {
			state = ", " + m.State
		}
		fmt.Fprintf(r.out, "\n#%d %s · at version %d%s\nYou: %s\n", m.N, m.Time.Local().Format("Jan 2 15:04"), m.Version, state, m.Text)
		for _, f := range m.Files {
			fmt.Fprintf(r.out, "  attached %s\n", f)
		}
		if list := answers[m.Session]; asked[m.Session] < len(list) {
			fmt.Fprintf(r.out, "Agent: %s\n", list[asked[m.Session]])
		}
		asked[m.Session]++
	}
	return nil
}

// answersOf is the last answer of each turn of session id's agent.
func (r runner) answersOf(id string) []string {
	t, err := r.c.Transcript(id, 0)
	if err != nil || !t.Supported {
		return nil
	}
	var out []string
	last, turn := "", false
	for _, e := range t.Entries {
		switch e.Kind {
		case proto.TranscriptUser:
			if turn {
				out = append(out, last)
			}
			last, turn = "", true
		case proto.TranscriptAssistant:
			last = e.Text
		}
	}
	if turn {
		out = append(out, last)
	}
	return out
}

var chatTools = []mcpTool{
	{name: "canvas_chat", description: "Ask the Canvas page's agent of a canvas for a change (it designs live, the user watching), starting it if needed.",
		props:    map[string]mcpProp{"canvas": canvasProp, "prompt": {"string", "What to ask"}, "agent": {"string", "The agent, when it isn't running yet (default: the canvas's, else claude)"}, "design": designProp},
		required: []string{"canvas", "prompt"},
		args: func(a map[string]any) []string {
			args := []string{"chat", str(a, "canvas")}
			for _, k := range []string{"prompt", "agent", "design"} {
				args = withFlag(args, a, k, k)
			}
			return args
		}},
	{name: "canvas_chat_history", description: "What the Canvas page's agent of a canvas was asked, at which canvas version, and what it answered.",
		props: map[string]mcpProp{"canvas": canvasProp}, required: []string{"canvas"},
		args: func(a map[string]any) []string { return []string{"chat-history", str(a, "canvas")} }},
}

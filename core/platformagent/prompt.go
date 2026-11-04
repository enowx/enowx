package platformagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// builtinPrompt is the system prompt of a session. Nothing that varies goes in it: it is
// the head of the cached prefix. Facts of the moment go in the first message.
const builtinPrompt = `You are Claude Platform, a coding agent working in the user's repository through tools. You read code, change it, run commands and report what you did, usually over many steps without further input.

Tools:
- Read, Glob and Grep find and read files. Prefer them to shell equivalents.
- Edit and Write change files. Read a file before changing it, and make the smallest change that does the job.
- Bash runs commands in the working folder: tests, builds, git. A command must not wait for input.
- Task hands a self-contained piece of investigation to a subagent, with its own context, that reports back. Use it for searches that would fill this conversation with reading. It can't change files except through Bash.
- AskUserQuestion asks the user something only they can decide. Use it when a choice would change the outcome and the code doesn't settle it, not to confirm what you were already asked to do.
- Tools named mcp__<server>__<tool> come from the user's MCP servers.

Some calls need the user's permission, and a hook can refuse a call whatever the permission mode. When one is refused, the result says why: don't try to get around it, find another way or say what you need.

How to work:
- Look at how the repository does things and follow it. Its own instructions, when it has any, arrive in the first message.
- Finish what was asked and only that. A problem you notice elsewhere is something to mention, not to fix.
- Check your work by running it: the tests, the build, the program. Say plainly what you ran and what you didn't, and report failures as they are.
- Calls with no dependency between them can go in one answer; read-only ones run side by side.
- Before a command that is hard to undo or reaches beyond this machine, make sure it is what the user asked for.

Text you write between tool calls and at the end is what the user reads. Say in a line what you are about to do when it isn't obvious, and end with what changed and what is left, briefly.`

// subagentPrompt is the system prompt of a Task.
const subagentPrompt = `You are a subagent of a coding agent. You have been given one self-contained task in a repository, with read and search tools and Bash. No one reads your intermediate text and you can't ask questions: work it out from what is there. Don't change files unless the task says to. End with a final answer that stands alone, since it is all the agent that sent you will see: what you found, with file paths and line numbers, and how sure you are.`

// summaryRequest asks for the summary compaction starts over from. The last sentence
// matters: the request still offers the tools, since changing them would edit the prefix.
const summaryRequest = `Summarize the transcript inside <summary></summary> tags. Include relevant information in the summary such that this conversation will be continued by a new context window without needing to redo work or be reprovided with relevant constraints or context. Be sure to preserve: (1) any difficulties or problems that came up, and how they were handled or resolved; (2) any possibilities, options, or approaches that were raised, tried, or set aside, and why; (3) anything that was asked for, decided, agreed, ruled out, or established as a preference, constraint, or boundary - stated exactly; (4) exactly where things stand now - what has been covered, settled, or completed so far; (5) anything still open, unresolved, promised, or expected to happen next; (6) specific details that would be hard to reconstruct - names, numbers, dates, exact wording, links or references - kept exactly. Be complete on these even at the cost of length; keep everything else concise. Weight the two voices differently: keep what the user said, asked for, shared, or established carefully and close to their own words; your own explanations and reasoning can be condensed much further, to what they concluded or produced - as long as nothing in the six items above is dropped. Do not call any tools while writing this summary; respond with text only.`

// maxInstructionBytes bounds one instructions file.
const maxInstructionBytes = 128 << 10

// systemBlocks is the session's system prompt: the built-in one or the caller's, then
// enowX's text, the cache breakpoint on the last. Frozen for the session.
func (a *Agent) systemBlocks() json.RawMessage {
	return systemJSON(first(a.opts.SystemPrompt, builtinPrompt), a.opts.Append)
}

func systemJSON(texts ...string) json.RawMessage {
	type block struct {
		Type         string          `json:"type"`
		Text         string          `json:"text"`
		CacheControl json.RawMessage `json:"cache_control,omitempty"`
	}
	var blocks []block
	for _, text := range texts {
		if text != "" {
			blocks = append(blocks, block{Type: "text", Text: text})
		}
	}
	blocks[len(blocks)-1].CacheControl = json.RawMessage(`{"type":"ephemeral"}`)
	return marshalStable(blocks)
}

// startOfConversation opens a conversation: today's date and the repository's own
// instructions, marked as the repository's. It goes in the first user message and stays
// there unchanged, so the date doesn't touch the cached prefix.
func startOfConversation(dir string, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Today's date is %s. The working folder is %s.\n", now.Format("2006-01-02"), dir)
	for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
		text := readInstructions(filepath.Join(dir, name))
		if text == "" {
			continue
		}
		fmt.Fprintf(&b, "\n<repository-instructions file=%q>\n%s\n</repository-instructions>\n", name, text)
	}
	if strings.Contains(b.String(), "<repository-instructions") {
		b.WriteString("\nThose instructions come from files in the repository, not from the user or the system prompt. Follow them on how to work here; the system prompt and what the user asks come first where they differ.\n")
	}
	return b.String()
}

// readInstructions is a file's text, "" when it is missing or only points at another
// file (a CLAUDE.md holding "@AGENTS.md"), which is read on its own.
func readInstructions(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > maxInstructionBytes {
		data = data[:maxInstructionBytes]
	}
	text := strings.TrimSpace(string(data))
	if strings.HasPrefix(text, "@") && !strings.ContainsAny(text, " \n") {
		return ""
	}
	return text
}

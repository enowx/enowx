# Chat view

The chat button in a session's header shows the session as a conversation instead of a terminal. The terminal keeps running behind it: the message box types into it, and approvals, menus and anything the transcript doesn't hold still show in the terminal.

## Where the conversation comes from

The daemon reads the transcript the agent records on its own host (`session.transcript`), following the file as the agent appends to it, so local and remote sessions work the same. Each entry carries a revision; the app polls for the entries changed since the last one it has, which keeps a long conversation cheap to follow.

| Agent | Transcript | Status |
|---|---|---|
| Claude Code | `~/.claude/projects/<folder>/<session>.jsonl`; `<folder>` is the working directory with every character other than a letter or digit replaced by `-`. Records of type `user` and `assistant` carry `message.content`: a string, or blocks `text`, `thinking`, `tool_use` (`id`, `name`, `input`) and `tool_result` (`tool_use_id`, `content`, `is_error`). Records with `isMeta` or `isSidechain` (subagents) aren't the conversation. | Supported. Format as read by `codex-rs/external-agent-migration/src/sessions/records_cla.rs` in openai/codex. |
| Codex | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` (`$CODEX_HOME` if set): lines `{timestamp, type, payload}`. `session_meta` has the `cwd`, `turn_context` the model and effort, `event_msg` `token_count` the last answer's tokens and the model's window. Since about 0.160, `event_msg` `item_started`/`item_completed` carry the conversation as Codex shows it (`UserMessage`, `AgentMessage`, `Reasoning`, `Plan`, `CommandExecution` with its exit code and output, `FileChange` with a unified diff per file, `McpToolCall`, `WebSearch`, `ImageView`), and those are read: the model's own calls there are scripts (`exec`) calling the real tools. Older rollouts are read from `response_item` (assistant `message`s, `reasoning` summaries, `function_call`/`custom_tool_call` with `apply_patch` and their outputs) and `user_message` events. | Supported; checked against Codex 0.164. The newest 200 rollouts are searched for a session's folder. |
| OpenCode, Gemini CLI, Kiro CLI | — | Terminal only: the Chat tab doesn't show for them. |

## Which file belongs to a session

A session's transcript is the newest file for its folder that changed after the session was created. A new file (after `/clear`, say) takes over; a file another session follows is left to it, so two Claude Code sessions in one folder each keep their own.

## What the chat shows

A long conversation stays light. The chat opens on about the last 300 entries, whole turns (`TranscriptParams.Tail`), and loads 300 more each time the user scrolls up to the oldest (`Before`); daemons before this answer with every entry, which still works. The turns are a virtual list (`ui.List` with `FollowEnd`), which builds only those in view; the poller wakes the app only with news, and every 5 seconds otherwise; Markdown is parsed once per text; and only turns that arrive while the chat shows animate in. The +/− in the composer counts the entries loaded.

Each message of the user's starts a turn. The agent's thoughts, tool calls and what it said on the way fold into one line, "Worked for 12s · 4 tools · 2 files edited", open while the turn is under way; its answer, the text it ended on, shows under it as Markdown (headings, nested and task lists, quotes, rules, tables, fenced code with a Copy button, inline code, bold, italics, strikethrough, links). Three or more tool calls in a row fold into one line that sums them up ("Read 3 files, searched twice, ran 1 command"), open while the turn is under way; a shell command counts by its program (cat and sed read, rg and find search, ls lists), past a leading cd, pwd or echo. A call that went well shows no mark; a failed one says so, an edit shows its +/−. An edit opens to its diff; other tools to their output. While the agent works, the last line says what it is at and for how long ("Editing `ui/goa_test.go`", "Working, 4m 12s"); otherwise it says whether it waits, ended or is done and how long the turn took. The composer floats over the frame's foot, the conversation scrolling under it; its row says the lines the edits added and removed, and the context used. Search and folding every step sit in the frame's corner.

Beside a wide conversation, **Your messages** lists the user's messages, the one read now lit, each a click away. It stays at the newest as messages come; scrolled up, it stays where it is left and new messages don't move it, until it is scrolled back to its end. The lit row doesn't move it. Each session keeps its own place, starting at the newest.

A message sent shows in the same frame, the composer emptied, without waiting for the host or the agent's file; the entry the agent records takes its row, with nothing doubled. While the agent is busy it says it waits for the agent. If it couldn't reach the session, or stays unrecorded for 45 seconds with the agent free, it says so and offers Retry and Edit, which puts it back in the composer with its files. A command (`/model`) shows no bubble: agents don't record those as messages.

The outline of the user's messages stands at the conversation's side when the column leaves room for it. A column running the frame's width (GOD of Agents') gives up that side; otherwise the outline folds to a button in the chat's corner.

## Changing the agent's settings

The composer shows the agent's model, permission mode and effort, as the transcript and the screen say them, and changes them the way the agent's user would (`core/agent/settings.go`):

| Setting | Claude Code | Codex (checked on 0.164) |
|---|---|---|
| Model | types `/model <name>`, with the names Claude Code records: Default, Opus 5.5, Fable 5.1, Sonnet 5.5, Haiku 5.5, Haiku 4.5, Sonnet 5, Opus 5, Fable 5, Opus 4.8 (the first ten of its `/model` menu on 2.1.308). Read back from `message.model`. | types `/model <slug>`: GPT-6-Luna, GPT-5.6-Terra, GPT-5.6-Luna (its `/model` menu). Read back from `turn_context`. |
| Mode | the daemon presses Shift+Tab until the screen says the mode (`session.mode`): auto, manual, accept edits, plan, bypass permissions, in that order (checked on 2.1.308). Bypass is in the cycle because enowX starts Claude Code with `--allow-dangerously-skip-permissions`, except as root. Read back from the screen, else from `permissionMode`. | Shift+Tab switches Default and Plan; the status line says "Plan mode" and nothing in Default. |
| Effort | types `/effort <level>`: low, medium, high, xhigh, max. Read back from `effort` on assistant records. | types `/model --reasoning=<level>`: low, medium, high, xhigh, max. Read back from `turn_context`. |

Other agents show no settings.

## Editing a message

A pencil beside one of the user's messages (shown while the pointer is over it, and not while the agent works) puts the message in the composer, over a line saying it is being edited, with an × to stop. Sending it asks the daemon to take the agent back to before that message the way its user would (`session.rewind`, `core/agent/rewind.go`), then sends it as any message goes. Files the agent changed since stay as they are.

| | Claude Code (2.1.309) | Codex (0.164) |
|---|---|---|
| Going back | Esc Esc opens Rewind, its current point selected at the bottom; Up until the message selected is the one edited (the list may hold commands the chat doesn't show), Enter, then "Restore conversation" in the menu that follows | Esc twice with its input empty browses the transcript at the last message; Left steps to earlier ones, Enter goes back |
| The input | the message comes back in it; Ctrl+C empties it | the same |
| The transcript | the same file: the message sent again names as its parent a record before the ones it replaces, so the reader drops those (`parentUuid`) | a new rollout, `<base>_<id>.jsonl`, whose `session_meta` keeps its base's start up to `history_base.end_byte_offset`; the reader reads that first |

Either way the chat starts over (`Transcript.Reset`) with the conversation as the agent has it now.

## Images and files

Pasting an image into the composer (Cmd+V, a screenshot copied, say) saves it as a PNG in `~/.enowx-app/screenshots` and attaches it to the message, shown as a small preview over it (files copied in Finder show by name), each with an × that drops it; clicking an image shows it large, fitted to the window, with Remove. The message waits for them to upload, then goes as a paste of its text and their paths, which Claude Code takes as images (`[Image #1]`). For a session on a server they are uploaded to its host first (`file.upload`), so the path is one the agent can open there: agents read an image from a path in the prompt. The terminal does the same with an image or files pasted into it; text pastes as text, even with a picture of it on the clipboard (a spreadsheet's cells).

## Slash commands

Typing `/` at the start of the composer opens a menu of the agent's commands: arrows move, Tab or Enter puts the command in the box, Escape closes it. The daemon reads the list once per folder and keeps it for ten minutes (`session.commands`, `core/agent/commands.go`). For Claude Code it is the `slash_commands` of the init record of `claude -p --output-format stream-json --verbose /cost`, run in the session's folder with `--no-session-persistence`, `--strict-mcp-config` and hooks off: `/cost` is answered locally, so nothing reaches a model, no hook runs and no session is saved; MCP prompts are left out. On 2.1.308 it lists 132 commands. Skills and commands on disk (the user's, the project's, the plugins') get their description from their front matter; built-ins have none. Codex can't be asked, so its list is the commands of its `/` menu on 0.164, with their descriptions, and the user's prompts in `~/.codex/prompts` as `/prompts:<name>`.

The context meter beside the send button is the tokens of Claude Code's last answer (input, cache reads and writes, output, from `message.usage`) over the model's window. Claude Code doesn't record the window, so it comes from the preset: 1M for Opus 5.5 (its status line shows 141k as 14%) and for models named `[1m]`. For a model whose window hasn't been checked, the meter shows the token count without a percentage.

## Past conversations

A project's menu has **History…** and the command palette **Past conversations**: what Claude Code and Codex talked about in the project's folder and its worktrees, newest first (`agent.history`, read on the project's host from the same files as the chat: `core/transcript/history.go`), each with its first message, branch, message count, model and age. **Resume** starts a session of that agent going on with it (`SessionCreateParams.Resume`: `claude --resume <id>`, `codex resume <id>`, the ID being the transcript's name for Claude Code and the rollout's `session_meta` ID for Codex), in the folder it ran in: the project's, or its branch's worktree.

The New session dialog's first field, **Session**, lists the same: the project's sessions (running, idle or hibernated) and its agents' past conversations, the latest first, to search by name or branch. Picking one goes on with it instead of starting a new session: **Open session** shows a running one, **Wake up** wakes a hibernated one, **Resume** resumes a past conversation where it ran; what the message box holds is sent to it (typed into a resumed conversation once its agent is back).

package agent

import (
	"slices"
	"strings"
)

// The Guide is built of features the user turns on or off (Settings →
// Features, core/features): a feature turned off is left out of what
// agents are told, and its MCP tools aren't served, so it costs no
// tokens. A lazy feature, when the user asks for it, is a line pointing
// at "enowx-app help ID", which prints the whole of it.

// Feature is something enowX gives agents, which the user can turn off.
type Feature struct {
	ID, Name string
	Group    string // where Settings lists it
	About    string // what it is, for the user
	Default  bool   // on unless the user turned it off
	Core     bool   // always on
	Lazy     bool   // a line (Summary) in a lazy guide, the rest on demand
	Requires []string
	Summary  string // one line for a lazy guide
	Guide    string // its part of the guide, "" for none
	// Tools are its MCP tools: names, or prefixes ending in "_".
	Tools []string
}

// Feature groups, in the order Settings lists them.
const (
	GroupCore    = "Core"
	GroupPreview = "Preview & debug"
	GroupNotes   = "Notes"
	GroupProject = "Project"
	GroupDesign  = "Design"
	GroupSecure  = "Security"
)

// FeatureGroups are the groups, in order.
var FeatureGroups = []string{GroupCore, GroupPreview, GroupNotes, GroupProject, GroupDesign, GroupSecure}

const guideIntro = `You are running inside an enowX session, possibly on a remote Linux server. The user watches this session from the enowX app on their own computer. enowX's tools are commands of "$ENOWX_BIN" (set in every session's environment): "$ENOWX_BIN" help lists them all, "$ENOWX_BIN" guide prints this again. Use them rather than asking the user to do it by hand.`

const guideMCP = `The same tools are MCP tools too, from "$ENOWX_BIN" mcp (stdio), screenshots as images, and its "enowx" tool runs any command above, for when MCP suits you better than the shell.`

const previewHead = "Previewing what you build:\n"

// Features are every feature, in the guide's order.
var Features = []Feature{
	{ID: "browser", Name: "Browser", Group: GroupPreview, Default: true,
		About:   "Dev servers the session starts show in the app's Browser, which agents can drive and screenshot.",
		Summary: "Browser: dev servers show in the user's Browser; \"$ENOWX_BIN\" browser open|snapshot|click|fill|screenshot drives it.",
		Guide: `- Web apps: start the dev server as usual (bind to localhost). enowX notices ports the session listens on, forwards them to the user's computer, and shows them in its Browser tool, with hot reload. Don't ask the user to open ports or use the server's IP.
` + `- Run long-running dev servers and apps in the background or so they keep running, and say which port or window to look at.
` + `- To check a web app yourself, drive the same Browser the user sees (it needs the enowX app open):
    "$ENOWX_BIN" browser open localhost:5173   # or a path of the dev server, or any URL
    "$ENOWX_BIN" browser snapshot              # the page as text, its controls numbered
    "$ENOWX_BIN" browser click 3               # a control's number, or a CSS selector
    "$ENOWX_BIN" browser fill 2 "ada@example.com"
    "$ENOWX_BIN" browser screenshot /tmp/page.png
    "$ENOWX_BIN" browser eval "document.title"   # any JavaScript in the page: DOM, storage, state
    "$ENOWX_BIN" browser back | forward | reload
    "$ENOWX_BIN" browser unslop                # what makes the page look AI-made, by pass (the project's, on localhost)
`,
		Tools: []string{"browser_"}},
	{ID: "ports", Name: "Ports & Run menu", Group: GroupPreview, Default: true,
		About:   "The ports a session listens on, and its project's commands.",
		Summary: "Ports & Run: \"$ENOWX_BIN\" ports, run.",
		Guide: `- The session's ports, and its project's commands (the Run menu), run in a tab of the session:
    "$ENOWX_BIN" ports                          # what it listens on; "ports preview 5173" shows one in the Browser
    "$ENOWX_BIN" run                            # the commands; "run 2" or "run dev" runs one, "read TAB" shows it` + "\n",
		Tools: []string{"ports_", "run_"}},
	{ID: "display", Name: "Display (desktop apps on Linux)", Group: GroupPreview,
		About:   "A virtual display for desktop apps on a Linux server, shown on the user's screen.",
		Summary: "Desktop apps on Linux: eval \"$(\"$ENOWX_BIN\" display)\" first, then run the app; it opens on the user's screen.",
		Guide: `- Desktop apps (MyGo, Tauri, Electron, Wails, GTK, Qt, Flutter desktop, ...) on Linux: the session may have no screen. Before running one, get a virtual display with
    eval "$("$ENOWX_BIN" display)"
  then run the app (for example the framework's dev command) in that same shell. enowX opens the app on the user's own screen, in a window of its own, as if it ran there: they see and use it live. Prefer this over Xvfb and screenshots, and tell the user the app's window is open. When the user closes that window, the app and everything started with that DISPLAY stop, dev server included: run the eval line and the app again if they ask for it.
- If the command fails because Xpra is missing, install it with the system package manager (xpra), then run it again; install the libraries the app needs (GTK, WebKitGTK, ...) the same way. When sudo asks for a password you don't have, give the user the exact install command to run in a terminal tab of this session (the + next to the session's tabs), and go on once they say it is done.
`},
	{ID: "traffic", Name: "Traffic (HTTP inspector)", Group: GroupPreview, Lazy: true,
		About:   "Catches the HTTP of what sessions run, to read, mock and change it.",
		Summary: "HTTP traffic: enowX can catch, mock and change what this session's programs send (\"$ENOWX_BIN\" help traffic).",
		Guide: `HTTP traffic: enowX catches the HTTP(S) what this session runs sends, once attached, to read, mock and change it, rather than adding logging to the code:
    eval "$("$ENOWX_BIN" traffic env)"         # this shell's programs go through it (curl, Node, Python, Go, Java...)
    "$ENOWX_BIN" traffic list --filter "domain:api.example.com status:>=400"
    "$ENOWX_BIN" traffic show 12               # headers and bodies, decoded, secrets masked
    "$ENOWX_BIN" traffic watch --until-match --filter "path:/api/login"
    "$ENOWX_BIN" traffic mock --method POST --path /api/pay --status 500   # saved in .enowx/traffic.json
    "$ENOWX_BIN" traffic fail --path /api/pay --how timeout                 # or forward --to, transform --res-set…
    "$ENOWX_BIN" traffic show 12 --as curl     # the request as code that sends it again
    "$ENOWX_BIN" traffic status                # recording, paused or stopped; "traffic resume" or "start" when nothing new shows
    "$ENOWX_BIN" traffic help                  # breakpoints, rules, send, HAR, settings, the CA, attaching browsers, Electron, docker, Android`,
		Tools: []string{"traffic_"}},
	{ID: "android", Name: "Android", Group: GroupPreview, Lazy: true,
		About:   "Emulators and phones: run the project's app, look at it, tap and type.",
		Summary: "Android: run and drive the app on an emulator or a phone (\"$ENOWX_BIN\" help android).",
		Guide: `Android: run and debug the app on an emulator or a phone, of this host or another the user connected:
    "$ENOWX_BIN" android status                # devices and virtual phones; "android setup" brings the SDK when missing
    "$ENOWX_BIN" android start Pixel_8         # a virtual phone, by its name in status
    "$ENOWX_BIN" android run emulator-5554     # the project's app (Flutter, Expo, React Native, Capacitor, NativePHP, Gradle) in a new tab
    "$ENOWX_BIN" android logcat emulator-5554 --package com.example.app --level W
  and use it, to check what you built yourself (-s SERIAL when more than one is online):
    "$ENOWX_BIN" android screenshot /tmp/screen.png   # look at it (read the PNG)
    "$ENOWX_BIN" android controls              # what is on screen, controls numbered
    "$ENOWX_BIN" android tap 3                 # a control by its number, or "tap X Y"; also longpress, swipe X1 Y1 X2 Y2
    "$ENOWX_BIN" android type --into 2 "ada@example.com"
    "$ENOWX_BIN" android key back              # home, recents, enter, delete…; on an Android TV, up, down, left, right, ok
    "$ENOWX_BIN" android catch                 # its HTTP shows in traffic list, its CA trusted`,
		Tools: []string{"android_"}},
	{ID: "agents", Name: "Multi-agent", Group: GroupCore, Default: true,
		About:   "Agents start, follow and talk to other agents in sessions of their own.",
		Summary: "Other agents: \"$ENOWX_BIN\" spawn, wait, read, send.",
		Guide: `Other agents: when work splits into parts that can go on at once, or the user asks for a second opinion, you can start other coding agents in sessions of their own, which the user sees in enowX too, and follow them:
    "$ENOWX_BIN" spawn --agent codex --branch fix/parser "Fix the parser's handling of ..."   # prints the session's ID
    "$ENOWX_BIN" wait ID          # until it is done or asks something
    "$ENOWX_BIN" read ID          # what its screen shows (also a tab's, as android run's)
    "$ENOWX_BIN" send ID "..."    # a message to it
    "$ENOWX_BIN" ls               # every session; "$ENOWX_BIN" agents lists the agents installed
--branch gives it a branch in a worktree of its own, so it doesn't touch your files; without it, it works in this project's folder. Tell the user what you started.
Done with sessions you spawned? "$ENOWX_BIN" hibernate ID... frees the host's memory and CPU (the user confirms in the app; "wake ID" brings one back on its conversation; --dry-run says what it would stop).`,
		Tools: []string{"session_spawn", "session_read", "session_send", "session_wait", "sessions_list"}},
	{ID: "modelapi", Name: "Agent configs", Group: GroupCore,
		About:   "Agents see their saved configs (another account, a model API such as a gateway, OpenRouter or a local server) and start sessions on one.",
		Summary: "Configs: \"$ENOWX_BIN\" configs lists each agent's; configs add AGENT NAME --url URL --key-cred CRED --model M saves a model API; spawn --config NAME starts on one.",
		Guide: `Configs: an agent's configs are what its sessions can run with besides the official one (the agent as installed, never changed): another account, or a model API (a gateway, OpenRouter, a local server) with its models, its key a credential of the vault. A session picks one as it starts; others keep theirs.
    "$ENOWX_BIN" configs                                   # every agent's configs
    "$ENOWX_BIN" configs add claude GLM --url https://openrouter.ai/api --key-cred openrouter --model z-ai/glm-5   # when the user asks
    "$ENOWX_BIN" configs edit claude GLM --model z-ai/glm-5.1 | rm claude GLM
    "$ENOWX_BIN" spawn --agent claude --config GLM "…"    # a session on it
Agents that can't take an API per session (enowx-app api lists them) still take one in their own settings: "$ENOWX_BIN" api set AGENT …, backed up first; api restore AGENT puts them back.`,
		Tools: []string{"agent_api_", "configs_"}},
	{ID: "clone", Name: "Clone from Git", Group: GroupProject, Default: true,
		About:   "Clone a repository as a new project, on any host.",
		Summary: "Projects: \"$ENOWX_BIN\" project clone URL --dir D, project add PATH, project list.",
		Guide:   `Projects: "$ENOWX_BIN" project clone URL --dir ~/code clones a repository on this host (with its git credentials) as a new enowX project, showing git's progress; "project add PATH" makes a folder one, "project list" lists them.`,
		Tools:   []string{"project_add", "project_clone", "project_clone_status", "projects_list"}},
	{ID: "autosync", Name: "Project copies & auto push", Group: GroupProject, Default: true, Lazy: true,
		About:   "Copies of a project on other hosts kept in sync, and work pushed to a branch of its own.",
		Summary: "Project copies on other hosts and auto push to its own branch (\"$ENOWX_BIN\" help autosync).",
		Guide:   ProjectLinksGuide,
		Tools:   []string{"project_autopush", "project_conflicts", "project_copy", "project_history", "project_links", "project_origin", "project_sync"}},
	{ID: "attach", Name: "Notes attached to projects", Group: GroupProject, Default: true, Requires: []string{"notes"},
		About:   "Plans and TODOs attached to projects on any host, and their context read-only.",
		Summary: "A project's plans are notes attached to it: \"$ENOWX_BIN\" notes --project . (\"$ENOWX_BIN\" help attach).",
		Guide:   AttachGuide,
		Tools:   []string{"notes_attach", "notes_detach", "notes_projects", "notes_for_project", "project_context", "project_read"}},
	{ID: "peek", Name: "Other sessions", Group: GroupProject, Default: true, Lazy: true,
		About:   "Agents read another session's conversation and project, on any host, when you ask.",
		Summary: "Other sessions, only when the user asks: \"$ENOWX_BIN\" sessions, \"$ENOWX_BIN\" peek SESSION --host H.",
		Guide:   `Other sessions: only when the user asks (to copy a feature from another project, say), read another enowX session, never on your own. "$ENOWX_BIN" sessions lists the sessions of every host the app reaches; "$ENOWX_BIN" peek SESSION --host H shows its last messages (--last N) and its project's folder; then "$ENOWX_BIN" project context P --host H and project read P FILE --host H show its code, read-only.`,
		Tools:   []string{"sessions_everywhere", "session_peek"}},
	{ID: "control", Name: "Control (sessions & settings, any host)", Group: GroupCore, Lazy: true,
		About:   "Agents control sessions on any host the app reaches (interrupt, stop, answer, switch model, review and merge, close) and change the app's, hosts' and projects' settings.",
		Summary: "Control of sessions and settings on any host: \"$ENOWX_BIN\" help control.",
		Guide:   ControlGuide,
		Tools: []string{"session_interrupt", "session_stop", "session_continue", "session_rename", "session_close", "session_chat",
			"session_ask", "session_answer", "session_switch", "session_changes", "session_commit", "session_push", "session_pr",
			"session_merge", "session_discard", "host_settings", "host_env", "app_settings", "project_settings", "control_log"}},
	{ID: "notes", Name: "Notes", Group: GroupNotes, Default: true, Core: true,
		About: "The user's notes, which agents read and write.",
		Guide: `The user's notes: their Notes page in enowX, pages of Markdown on this host. Read or write them when the user asks (a plan to keep, a checklist, what you found):
    "$ENOWX_BIN" notes                         # the notes, as a tree
    "$ENOWX_BIN" notes show "Release plan"     # one's Markdown, by title or ID
    "$ENOWX_BIN" notes new "Release plan" --body "- [ ] tag"   # also --parent NOTE, --group G, --body - for stdin
    "$ENOWX_BIN" notes write "Release plan" --append "- [ ] publish"   # or --body to replace it`,
		Tools: []string{"notes_list", "notes_read", "notes_create", "notes_write", "notes_delete", "notes_history", "notes_restore"}},
	{ID: "sync", Name: "Sync between hosts", Group: GroupCore, Default: true, Core: true,
		About: "Notes, canvases, settings and the vault the same on every host.",
		Guide: `Notes and the user's other enowX data are the same on every host (the app syncs them): "$ENOWX_BIN" sync status, sync now, sync conflicts, sync resolve ID --keep HOST|both, sync trash, sync restore ID.`,
		Tools: []string{"sync_"}},
	{ID: "notes-agent", Name: "Agent chat in Notes", Group: GroupNotes, Default: true,
		About: "Ask an agent about a note, beside it in the Notes page.",
		Guide: "A note's own agent in the Notes page (notes only, its changes proposed to the user): \"$ENOWX_BIN\" notes ask NOTE \"…\" [--wait] asks it, notes chat NOTE shows what was said, notes agent NOTE --use codex picks it; notes proposals lists what agents proposed, notes accept|reject ID settles one when the user asks.",
		Tools: []string{"notes_ask", "notes_chat", "notes_agent", "notes_proposals", "notes_accept", "notes_reject"}},
	{ID: "groups", Name: "Note groups", Group: GroupNotes, Default: true,
		About: "Notes in groups, like folders.",
		Guide: "Notes can be in groups, like folders: \"$ENOWX_BIN\" notes groups, notes new T --group G, notes move NOTE --group G."},
	{ID: "blocks", Name: "Structured notes", Group: GroupNotes, Default: true, Requires: []string{"notes"},
		About:   "Tabs, toggles, columns, cards in notes, and reading or writing a section at a time.",
		Summary: "Notes have layout blocks and sections (\"$ENOWX_BIN\" help blocks).",
		Guide:   BlocksGuide,
		Tools:   []string{"notes_outline", "notes_check"}},
	{ID: "icons", Name: "SVG icons", Group: GroupNotes, Default: true, Requires: []string{"notes"},
		About: "Notes' and groups' icons are SVG from icon libraries.",
		Guide: IconsGuide,
		Tools: []string{"icons_search"}},
	{ID: "notesio", Name: "Import & export notes", Group: GroupNotes, Requires: []string{"notes"},
		About:   "Notes from Notion, Obsidian, Evernote, Keep, HTML, and out as Markdown, HTML, PDF, a backup.",
		Summary: "Notes import/export: \"$ENOWX_BIN\" notes import FILE --dry-run, notes export --all --format zip.",
		Guide:   "Notes from and to other apps:\n" + NotesIOGuide,
		Tools:   []string{"notes_import", "notes_import_status", "notes_export"}},
	{ID: "canvas", Name: "Canvas", Group: GroupDesign, Requires: []string{"browser"},
		About: "Mockups, wireframes and diagrams agents design live.",
		Guide: `Canvas: when the user asks for a mockup, a wireframe or a diagram, design it on a canvas they watch build up live in enowX: "$ENOWX_BIN" canvas guide tells how (read it first); "$ENOWX_BIN" canvas list lists them.`,
		Tools: []string{"canvas_"}},
	{ID: "creds", Name: "Credential vault", Group: GroupSecure,
		About:   "Logins kept encrypted on the user's device, used on any host by name.",
		Summary: "Credentials: use the vault's logins by name, never their values (\"$ENOWX_BIN\" help creds).",
		Guide:   CredsGuide,
		Tools:   []string{"creds_"}},
	{ID: "notify", Name: "Notification sounds", Group: GroupCore, Default: true,
		About:   "Sounds in the app when agents finish, ask or fail; agents can play one.",
		Summary: "Sounds: \"$ENOWX_BIN\" notify sound done|input|error plays one in the user's app.",
		Guide:   NotifyGuide,
		Tools:   []string{"notify_"}},
	{ID: "hooks", Name: "Hooks", Group: GroupCore, Default: true, Lazy: true,
		About:   "Agents see the user's hooks and propose new ones, which run once the user allows them.",
		Summary: "Hooks: \"$ENOWX_BIN\" hooks lists what runs when something happens in a session; hooks add proposes one for the user to allow.",
		Guide: `Hooks: the user's hooks run when something happens in a session, on every agent (a command, context for you, a refusal, a changed command, a notification, a webhook). One may refuse what you're about to do, with a reason: take it as the user's rule, not an error to work around.
    "$ENOWX_BIN" hooks                         # what runs, when, where (host, project, session)
    "$ENOWX_BIN" hooks events                  # the events: tool.before, prompt.start, turn.done, file.changed, git.before, schedule, …
    "$ENOWX_BIN" hooks add --event tool.before --tool 'git push.*main' --do refuse --reason "Push to a branch" --why "…"
    "$ENOWX_BIN" hooks edit ID --off | rm ID | test ID | log
What you add, change or remove is proposed: the user allows it in Settings → Hooks, and it doesn't run before. --scope project keeps one in the project's .enowx/hooks.json, --scope session for this session alone.
`,
		Tools: []string{"hooks_"}},
	{ID: "accounts", Name: "Agent accounts", Group: GroupCore, Default: true, Lazy: true,
		About:   "Agents see the accounts they can run as, their limits, and switch one when the user asks.",
		Summary: "Accounts: \"$ENOWX_BIN\" accounts lists the agents' accounts, their limits and fallback; accounts use NAME switches a session's.",
		Guide: `Accounts: an agent may have several accounts on this host (Claude Code, Codex), used in an order the user set; at a usage limit the session may go on as the next one by itself (its conversation resumed, you told to go on), or the user is asked. When told you are now on another account, go on with the task as it was.
    "$ENOWX_BIN" accounts                      # accounts, their state (limit until when, credits, signed out), what they have left
    "$ENOWX_BIN" accounts use Work --session S # only when the user asks: S goes on as Work
    "$ENOWX_BIN" accounts priority claude Work default Spare | fallback claude --on|--ask|--off [--to codex] [--return] | log
`,
		Tools: []string{"accounts_"}},
	{ID: "decide", Name: "Decisions", Group: GroupProject,
		About:   "Agents know enowX's judge (Settings → Decisions): why a command was allowed or refused, a turn stopped as a loop, an output trimmed; and they can trim a long command's output themselves.",
		Summary: "Decisions: enowX's judge may allow or refuse your commands, stop a loop, trim long output; \"$ENOWX_BIN\" decide log says why.",
		Guide: `Decisions: enowX's judge may act on what you do, as the user set it (Settings → Decisions):
- a command refused "by enowX's judge": don't run it another way; tell the user what you wanted and why, and let them run it.
- your turn stopped because you seemed to loop: say what you tried and ask the user how to go on, rather than trying the same again.
- an output that ends with "[enowX kept N of M lines …]": the full output is in the file it names; read it when what was kept isn't enough.
    "$ENOWX_BIN" decide log --last 5            # the latest decisions and why
    "$ENOWX_BIN" decide trim -- go test ./...    # a long command's output cut to what matters, its exit code kept
    "$ENOWX_BIN" decide route --agent codex "PROMPT"   # a model and effort for a session you spawn (spawn --model --effort), when the user turned it on
`,
		Tools: []string{"decide_"}},
	{ID: "rag", Name: "RAG", Group: GroupProject,
		About:   "Agents search their project by meaning (code, docs, attached notes, memories) and keep memories in it, once the project is indexed (Settings → RAG).",
		Summary: "RAG: \"$ENOWX_BIN\" rag search QUERY finds this project's code, docs, notes and memories by meaning; rag remember TEXT keeps a decision.",
		Guide: `RAG: this project may be indexed by meaning. Before reading widely, or changing code you don't know, search it; then read the files it points at before relying on them, and say which file:line you used.
    "$ENOWX_BIN" rag search "where are invoices totalled"   # file:line, symbol, the part found; --scope code,docs,notes,memory
    "$ENOWX_BIN" rag remember "Invoices round half up: the tax office's rule"   # a decision or gotcha, for later
    "$ENOWX_BIN" rag status
A prompt may come with parts of the project already found ("Parts of this project that may bear on the prompt"): treat them as leads, not answers.
`,
		Tools: []string{"rag_"}},
	{ID: "status", Name: "Session status line", Group: GroupCore, Default: true,
		About:   "Agents say where they are in a line under their session.",
		Summary: "Say where you are at milestones: \"$ENOWX_BIN\" note self \"…\".",
		Guide: `Keeping the user in the loop: at milestones (a hypothesis confirmed, a fix in, tests running, blocked on something), say where you are in a line, which enowX shows under this session:
    "$ENOWX_BIN" note self "fix in for the login redirect; running the integration tests"`,
		Tools: []string{"session_note"}},
}

// FindFeature is feature id.
func FindFeature(id string) (Feature, bool) {
	i := slices.IndexFunc(Features, func(f Feature) bool { return f.ID == id })
	if i < 0 {
		return Feature{}, false
	}
	return Features[i], true
}

// DefaultFeatures are the features on for a user who changed none.
func DefaultFeatures() map[string]bool {
	out := map[string]bool{}
	for _, f := range Features {
		out[f.ID] = f.Default || f.Core
	}
	return out
}

// AllFeatures are every feature on.
func AllFeatures() map[string]bool {
	out := map[string]bool{}
	for _, f := range Features {
		out[f.ID] = true
	}
	return out
}

// GuideFor is the guide of the features on: each in full, or with lazy,
// the long ones as a line pointing at their help.
func GuideFor(on map[string]bool, lazy bool) string {
	var b strings.Builder
	b.WriteString(guideIntro)
	part := func(f Feature) string {
		if lazy && f.Lazy && f.Summary != "" {
			return f.Summary
		}
		return strings.TrimRight(f.Guide, "\n")
	}
	preview := []string{"browser", "ports", "display"}
	var bullets []string
	for _, id := range preview {
		if f, _ := FindFeature(id); on[id] && f.Guide != "" {
			bullets = append(bullets, part(f))
		}
	}
	if len(bullets) > 0 {
		b.WriteString("\n\n" + previewHead + strings.Join(bullets, "\n"))
	}
	for _, f := range Features {
		if slices.Contains(preview, f.ID) || !on[f.ID] && !f.Core || f.Guide == "" {
			continue
		}
		b.WriteString("\n\n" + part(f))
	}
	b.WriteString("\n\n" + guideMCP)
	return b.String()
}

// FeatureOfTool is the feature MCP tool name belongs to, "" for one of
// none (always served).
func FeatureOfTool(name string) string {
	for _, f := range Features {
		for _, t := range f.Tools {
			if t == name || strings.HasSuffix(t, "_") && strings.HasPrefix(name, t) {
				return f.ID
			}
		}
	}
	return ""
}

// GuideTokens is about how many tokens text costs an agent.
func GuideTokens(text string) int { return (len(text) + 3) / 4 }

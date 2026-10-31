package agent

// ControlGuide is the "control" feature's part of the guide: sessions and
// settings of any host, from a session (docs/cli.md, Control).
const ControlGuide = `Control: the session commands reach any host the user's enowX app reaches with --host H (its name, as "$ENOWX_BIN" sessions lists hosts); without it, this host. Other hosts need the app open; this host works without it.
    "$ENOWX_BIN" ls --host vps                         # its sessions; "projects --host vps", "project add PATH --host vps"
    "$ENOWX_BIN" spawn --host vps --project ~/app --agent codex "..."   # a session there (its folder on that host)
    "$ENOWX_BIN" send ID "..." --host vps; read ID; wait ID; chat ID     # prompt it, its screen, wait, its messages
    "$ENOWX_BIN" interrupt ID | stop ID | continue ID | rename ID NAME | hibernate ID | wake ID
    "$ENOWX_BIN" answer ID                              # what its agent asks; "answer ID allow|always|deny|N"
    "$ENOWX_BIN" switch ID --model M --effort E --mode M --account A
    "$ENOWX_BIN" changes ID [diff|commit -m MSG]        # its changes; push, pr, merge, discard need --yes
    "$ENOWX_BIN" close ID --yes                         # ends it for good
    "$ENOWX_BIN" settings [set KEY VALUE|env|app [set KEY VALUE]]   # the host's settings, the app's
    "$ENOWX_BIN" project settings P | project rules P   # a project's .enowx.toml and rules
--yes only when the user asked for it: closing, pushing, opening a pull request, merging and discarding can't be undone, and from a session the user also says yes in the app. A credential's value is never readable.`

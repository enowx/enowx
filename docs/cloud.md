# enowX Cloud

The cloud lets a team work in enowX together: its members, their roles, the notes, canvases, skills and templates they share, and the hosts and sessions others can open or watch. It is optional and off by default ([decision 2](decisions/0002-optional-cloud.md)): the app and daemons use it only once signed in, with **Sign in** at the rail's foot or in **Settings → Cloud**. The app signs in to enowX's, https://cloud.enowx.ai; a host can be pointed at another with `enowx-app cloud url`. Before signing in nothing reaches a cloud, and enowX works as it always has.

The server, `enowx-cloud`, is open source (`cmd/enowx-cloud`) and runs anywhere a container does. enowX runs one; a team can run its own.

## What it keeps, and what never

| Kept | How |
|---|---|
| Accounts: name, email, avatar, the GitHub account signed in with | in the clear |
| Devices: an app install or a host's daemon, its name, public key, last seen | in the clear |
| Teams, members, roles, invites, the team's settings | in the clear (an invite's token hashed) |
| The team's key, once per device of each member | sealed to the device's public key; with each epoch's key check, which tells the key from another without giving it |
| Notes, canvases, skills, subagents, commands, templates, presets of the team | sealed with the team's key; the cloud sees an item's kind, size, version, when it changed and which device wrote it |
| Hosts shared with the team: which, and which of their projects | names and addresses sealed with the team's key |
| An audit log: sign-ins, invites, roles, shared hosts entered, sessions started, watched and taken over, projects' rules changed; admins read it in the team's Activity tab (`enowx-app team audit`) | in the clear |

The cloud can't read what is sealed as long as it seals the team's key only to the devices it should: the members' hosts seal it to the devices and recovery keys the cloud lists for the team's accounts, and can't tell a device the cloud added on its own from a member's new laptop. A malicious or compromised cloud could add a device to a member's account and read what is shared from then on (see Keys, for what limits this).

Never, not even encrypted:

- credentials: the vault, agents' logins, model API keys, session environments, `.env` files, SSH keys, git and GitHub tokens;
- agents' conversations (a session is watched live, over the host's connection), and traffic caught by the Traffic tool;
- a member's personal space, and their own preferences (appearance, sounds, terminal font).

## Personal and team

Everything has one owner: the member (personal) or a team. Personal stays as it is, on the member's own devices and synced between them by the app. A team's items live in their own folder (`teams/<id>/` beside `notes/`), sync through the cloud, and stay with the team when their author leaves. Nothing becomes a team's without a clear act: made in the team's space, or moved or shared to it, after a confirmation that names who will see it and a scan for secrets. Copying a team's item into one's personal space is off unless the team's admins allow it.

On a host, the user's own notes and canvases stay in `notes/` and `canvas/`; each team's are in `teams/<id>/notes` and `teams/<id>/canvas`, with the team's name in `teams/<id>/team.json`. The host learns its account's teams from the cloud's answers it relays, and shows only the spaces of the account it is signed in as. Its notes, groups and canvases come in one list, each marked with its team (`team` in the daemon's protocol, `space.list` names the spaces); the account's menu at the rail's foot (Personal, then each team) picks which the Notes and Canvas pages show, and what is made there is that space's. A page goes in its parent's space, and a page is never moved under one of another space. A team's canvas belongs to the team's projects by their Git link, so its agent reads, and **Make code** builds in, each host's own copy.

A note (with the pages under it and their images) or a canvas goes to another space from its menu, **Move or copy to a space…** (`enowx-app notes space NOTE --to TEAM|personal [--copy]`, `canvas space`): moved, or copied leaving the original where it was. To a team's, as a member or above, after the dialog says who will see it and a scan for secrets, which refuses one that looks like it holds any. Out of a team's, only as a copy and only when the team's admins let members copy its items out (the team's Options tab, `copyOut`); a copy gets IDs of its own.

A team's space syncs through the cloud from every member's host signed in, every 30 seconds and soon after a change (`enowx-app team sync`, or the Notes page's sync button in a team's space, syncs now). Each file of its notes and canvases is an item, sealed on the host with the team's key: its ID a hash of its path and the team, its path sealed with its content, so the cloud learns neither. A sync takes what changed in the cloud since the last it saw and merges it with what changed on the host since then, as the user's own sync merges (a note's header field by field, its body line by line; when they don't merge the cloud's wins and the host's stays among the note's versions), then sends what changed on the host, one version after the last agreed, so a change made meanwhile elsewhere is never overwritten unseen. A file that looks like it holds a secret is not sent, and the space says which: one named as files of secrets are (`.env` files, `*.pem`, `*.key`, `*.p12`, `*.pfx`, `id_rsa` and its kin, `kubeconfig`, `.npmrc`, `credentials.json`…), or holding a private key, an API key or token (GitHub, AWS, npm, Hugging Face, a JSON Web Token, a kubeconfig's), a password in a connection string, or a secret set in a variable (`DB_PASSWORD=…`, unquoted, placeholders aside); binary files are looked through too. Agents' conversations with a note or canvas, their proposals and the versions kept for undo stay on the host. Viewers' hosts take the team's changes and send none. A host whose account left the team, or was removed, deletes its copy of the space at its next sync.

What a sync takes is checked on the host, not left to the cloud. A deletion carries a mark sealed with the team's key, naming the file, which the cloud can't make: one without it leaves the host's copy as it is (older versions deleted without a mark: a deleted file the host never had stays away). The cloud says which device wrote each item and, asked for the team, each member's role; from these a host takes the team's projects (`projects.json`) only from an admin's device, a project's rules from an admin's, or a member's when its members edit them, a chat message only as written by its author's device (deleted by them or an admin), a shared host's announcement only from that host, and the kit from members, never viewers. What isn't taken leaves the host's copy, and is not sent back as its change; while the members can't be read, those files wait for a later sync. A team's note or canvas never takes an ID another space here holds (the user's own, or another team's), and an edit of a note is saved in the space it was opened from, so a team's copy can't stand in for one elsewhere.

The first sync of a team on an admin's host starts the team's key, with its check, and seals it for every member's device; afterwards each host holding keys seals every epoch's it holds for the members' devices that lack it, a new member's or a new host, when it syncs, so they read what was written before a member's removal too.

A team's **projects** are Git links its admins add (Settings → Cloud, a team's Projects tab, or `enowx-app team project add TEAM LINK`), kept in the team's space (`projects.json`) and synced with it; a link with credentials in it is refused, each member cloning with their own login. A member's host finds its copy of one by its Git remote, among its projects and the folders of its sessions running, so having it already is enough; otherwise **Clone** (or `enowx-app team project clone TEAM PROJECT`) clones it there. The session list marks a project that is a team's, and a team's notes attach to the team's projects (the note's Attach dialog lists them), so they show under its copy on every member's host.

A team's **kit** is the skills, commands and subagents it shares (the team's Kit tab, `enowx-app team kit`). A member shares one of their agents' items from the host (one that looks like it holds a secret is refused); it lives in the team's space (`kit/`) and syncs with it. A session in one of the team's projects, found by its folder's Git remote, is told in its guide whose project it is and what the kit holds, each item with its file to read, so any agent uses them. Which features agents get stays each host's own setting (Settings → Features). The kit also keeps definitions: **pipelines** (their stages, shared from a team's project's pipeline dialog, which offers the team's to start from), **automations** of the team's projects (shared from Settings › Automations; **Use on** a host shows the whole prompt, the agent and when it runs, and once confirmed sets it up in that host's copy of the project: off, its agent asking before it acts, as the kit never keeps an automation on or skipping permissions), and **project templates** (a Git link a new project is cloned from; `enowx-app team kit template TEAM NAME LINK`). None holding what looks like a secret is kept.

A team's **chat** is the side panel's Team group (`enowx-app team chat TEAM`, `--send TEXT`; MCP `team_chat_read`, `team_chat_send`, for agents only when the user asks). Each message is a file of the team's space (`chat/`), sealed and synced with the rest of it, so the cloud holds ciphertext only (`chat` items); sending syncs at once, and an open chat takes what came every few seconds. Typing `@` offers what to mention, narrowed as more is typed: the team's members (who are notified), notes, canvases, projects, and the files of the selected session when it works in one of the team's projects; what a message mentions is lit in it and opens from it. Enter sends, Shift+Enter breaks the line. A message's author edits it (hover it, or Up in an empty composer) and deletes it, an admin deletes any: the file goes, and the cloud keeps no content of it, only its deletion's sealed mark, so the other devices drop it too. The members' hosts take a message, its edit or its deletion only from a device of its author (a deletion, of an admin too). One that looks like it holds a secret isn't sent. Viewers read the chat; the Team group counts what is unread, and a message from someone else notifies while the chat isn't in front: the team chat sound, a card in the window's top right corner (its sender, team and text, lit when it mentions the user; Reply answers it from the card, Enter sends; a click opens the chat; it goes after 8 s unless hovered or being answered, three at most), and a desktop banner while the window is in the background. Settings → Notifications → Team chat sets every message, mentions only, or off, and each team apart; mentions of the user and replies to their messages notify whatever it says; quiet hours silence its sound. Unread messages count on the chat's button, the Team group, and a chip in the title bar that opens the chat with the latest. Messages older than two minutes, which a sync brought, notify nothing. A member removed loses the space, the chat with it, and the team's key moves to a new epoch.

A project's **rules** are Markdown every session in it reads first, in its guide, whatever its agent (the project's menu, **Rules…**; `enowx-app project rules`; the MCP tools `project_rules` and `project_rules_set`). The user's own project keeps them in their synced settings, by the project's Git remote, so a clone on another of their hosts has them; a team's project in the team's space (`rules/`), the same for every member. A team's admins say who edits a project's rules, its members (the default) or only its admins; viewers only read them. Rules that look like they hold a secret are refused, and each change of a team's project's rules goes in the team's audit log. A team's project's rules file is found by the project's ID, which must be one the daemon makes (12 hex digits), and stays in the team's `rules/` folder.

A session in a team's project reads the team's notes and canvases, and a member's personal notes only when that member attaches them. Its daemon sets `ENOWX_TEAM` and `ENOWX_TEAM_PROJECT` in its environment, and its agent's notes and canvas commands (and their MCP tools) then list and read the team's items, the member's own notes only when attached to the project (with the pages under them), and make new notes in the team's space.

## Roles

| Role | Can |
|---|---|
| Owner | everything; transfers ownership, deletes the team |
| Admin | invites and removes members, changes roles below Owner, pairs and unpairs team hosts, sets the team's rules |
| Member | edits the team's items, works in its projects, opens sessions on its hosts, watches others' |
| Viewer | reads the team's items, watches sessions |

## Deleting a team

Its owner alone deletes a team, from its Options in Settings → Cloud → Teams: a dialog says what goes and deletes nothing until the team's name is typed exactly. The cloud deletes all it holds at once (its members, invites, keys and recovery copies, its items, so its chat, notes, canvases, projects list, kit and shared hosts' announcements, and its chat's files), and the owner's host drops its space as the cloud answers. The members' hosts drop theirs at their next sync, as for a team they left: its copy on disk goes (their own notes and canvases stay), a host shared with the team stops being shared (its listener closes when no other team has it) and the team's visitors are let go. For 90 days the cloud keeps who the team's members were, the team's ID and when, nothing else, and answers them `404` with `the team was deleted by its owner` rather than plain not found; their hosts then keep a notice (`space.gone`) until the user saw it, a card in the app's corner saying the team was deleted. Someone never in the team learns nothing of it.

## Signing in and joining

- **Members** sign in with GitHub, for now: the app starts a device code and opens the cloud's `/device` page with it, the browser signs in with GitHub (the account is made at the first sign-in), shows which device asks, and the member approves it; the app gets its token. The cloud keeps a browser session of its own for 12 hours (a cookie, hashed in the database), not GitHub's token. Registering an account of its own on the cloud comes later, with the cloud's public release; there is no sign-in by email link or code, and no password.
- **Invites** are a one-time link, or an email when SMTP is set, valid seven days, with the role given. Accepting needs signing in. The link opens a page of the cloud's: it says which team, the role and who invites, signs the visitor in with GitHub (making their account when they have none) and joins, with no app needed yet; it says where to download enowX (enowx.ai), whose sign-in with the same GitHub account finds the team. The token stays after the link's `#`, out of the server's logs. In the app, the link is pasted in Settings › Cloud › Join a team.
- **A default team**, when the server sets one (`ENOWX_CLOUD_DEFAULT_TEAM`), takes everyone who signs in, with `ENOWX_CLOUD_DEFAULT_ROLE` (a viewer when unset); not someone who left it or was removed from it. With `ENOWX_CLOUD_DEFAULT_TEAM_REPO` (a public GitHub repository, `owner/name`), the people GitHub lists as its contributors join as members, matched by their GitHub account's ID, and a viewer who becomes one is made a member at their next sign-in or listing of teams; the list is read with the server's OAuth app every ten minutes. A role an admin set stands: nobody is made less, nor raised over an admin's choice.
- **Roles are changed** by admins from a member's role in the team's Members tab (Admin, Member, Viewer); the owner also hands the team over there, after a confirmation.
- **A host's daemon** keeps the host's sign-in: the cloud's address, the device's X25519 key and its token, in `~/.enowx-app/cloud/` (`0600`), so every host, this computer or a server, is a device of the account, and the token never leaves it. The apps and the command line ask the cloud through it (`cloud.request` in the daemon's protocol). With no address set, nothing reaches a cloud.
- **A host's daemon** signs in with a device code: `enowx-app cloud login` prints a code, the member approves it in the app or the browser, and the daemon gets a token of its own, revocable from the app.

Tokens are random, kept hashed on the server, one per device; signing out or removing a device revokes it.

## Keys

- Every device makes an X25519 key pair; the private key never leaves it (the system's keychain in the app, a `0600` file on a host).
- A team has a random 256-bit key per epoch. Each member device gets it sealed to its public key: X25519 with an ephemeral key, HKDF-SHA256, AES-256-GCM (`cloud/seal`).
- Items are sealed with the team's key of the current epoch (AES-256-GCM), their ID and version as additional data, so the server can't swap one for another.
- The admin's device starting an epoch sends its key's check (HMAC-SHA256 of a fixed label, keyed with the key) once; the cloud gives every device the checks with its copies. A device uses a copy only when it opens and matches its epoch's check: a member who sealed another key to a new device first is found out, the device drops that copy (`DELETE /v1/teams/{team}/keys/{epoch}`) and waits, and a holder of the real key seals it again. Epochs started by older versions have no check, and their copies are taken as they open.
- A new device of a member gets the team's keys from any device holding them (another of their own, or a teammate's), which seals them every epoch's key it holds; the cloud refuses a key for a device that isn't a member's, and sealing for another account's device needs a member or above; a copy stays until its device drops it. A **recovery key**, which the member keeps, gets them too, for when every device is lost: made on a host (Settings › Cloud, Devices; `enowx-app cloud recovery new`) and shown once, never kept on the host, its public half set in the cloud as a device of the account that the hosts seal the teams' keys to; given to a new device (`cloud recovery use KEY`), it opens those copies and seals them to it. Making a new one replaces it, and needs the asking device's own key, not only its token: the cloud seals a challenge to the device, which opens it.
- Removing a member starts a new epoch: new items use a key they never had. What they could read before, they may have kept; nothing undoes that.
- The limit of this design: the cloud says which devices and recovery keys the team's accounts have, and the hosts seal the team's key to them. A malicious or compromised cloud could add a device (or a recovery key) to a member's account, be sealed the keys at a teammate's next sync, and read what the team shares from then on. What limits it: a host seals only to devices of accounts the team lists as members; key checks keep a wrong key from being passed off; setting a recovery key needs a device's own key, so a stolen token alone doesn't; and the team's devices show in the team's dialog (admins see their names and when they were last seen), where a device nobody knows stands out.

## Hosts and sessions

Two kinds of hosts serve a team: a **team host**, paired to the team by an admin, which stays when members come and go, and a member's **personal host shared** with the team, project by project, until they take it back. Members reach them as today, over SSH or the tailnet; the cloud only says which hosts and projects a team has, and the daemon checks a member's token and role before letting them in. Sessions in a team's project show to the team (status, watching) unless marked private; taking over one needs its owner's approval. A session runs with the member's own agent account and git identity, lent from their vault for that session. An agent reads another session only when the user asks it to (`enowx-app peek`, docs/cli.md): its last messages and project, live from its host, never kept in the cloud; the user's own hosts, and the team's shared ones within what the member sees there.

A member reaches another's host over Tailscale, the host's node shared with their Tailscale account (no relay of enowX's). The host's daemon serves them on its team listener, apart from its own socket: a connection there gets nothing, not even events, until it hands a ticket the cloud gave the member for that host (`team.auth`): before it, a connection has 10 seconds, frames of 64 KiB at most and one request (anything but its ticket, or a ticket refused, closes it), and the listener takes a few connections and tickets a minute from one address; the host redeems the ticket with its own token, so no member's token ever leaves their device, and serves the visitor as their role allows. It asks the cloud again every 2 minutes, and before a visitor starts a session, whether each is still the team's member and in which role: a member removed is let go, one made a viewer keeps what a viewer may. A visitor sees no more of the host than the shared projects: not its home, name or process, not how a session was started or what the vault lent it, and a session they don't see is as missing as one that isn't there. The host's own app lists who is connected (`host.visitors`). Its user shares a host from a team's Hosts tab (or `enowx-app team share TEAM PROJECT…`), picking which of its projects the team sees: the team listener then opens on the host's Tailscale address (port 7178), and the host is announced in the team's space (`hosts/<device>.json`, sealed and synced like its other items). The members' apps list it in the rail, under TEAM (`enowx-app team hosts`); opening it gets a ticket from the cloud through the member's own host, connects, and shows the shared projects and their sessions. A visitor's lists hold the shared projects alone, and their files read only within them, a link that leads out of a shared project refused. Taking a project back closes visitors' views of its sessions and ends their control of them; sharing nothing more with a team, or leaving it, closes its visitors' connections. A visitor **watches** a shared session live, its terminal read-only: what they type and their window's size reach it only once they **take over**, which they ask from the watching view (one ask a session at a time, a few a minute); the host's user is asked (Let them type, or Not now), sees in the session's header who watches it and who has control, and takes it back any time; a visitor who leaves gives back what they took over. A member (not a viewer) also **starts sessions of their own** in a shared project, from its page, once the host's user lets them: **Members start sessions here** in the team's Hosts tab, off until turned on (`allowSessions` of `host.share.set`). Such a session runs as the host's OS user, so an agent there can do what that user can, the daemon's socket included: turn it on only for people trusted with the machine. It is Claude Code or Codex, run as an agent account of theirs on the host (`Team · @login` in its Agent accounts, kept by their cloud account, signed in by them the first time), its home their own with nothing linked from the host's user's (no sign-in, settings, skills or conversations); with their git identity and a GitHub CLI sign-in of their own (`GH_CONFIG_DIR` in that account's home); and none of the host's credentials: not the variables its user set for sessions, not its model API, nothing the visitor passes, no project setup script; the vault lends nothing to it. The session is theirs to type into, and shows as theirs in the host's list. The host's user keeps a session of a shared project from the team from its menu (**Keep from the team**): it leaves the team's lists, watchers' views close, and a takeover of it ends. A member's agent asked to read another session (`enowx-app peek`) reaches the team's shared hosts too, seeing only what the member sees there, and each read goes in the team's log (`session.read`).

## API

HTTPS and JSON under `/v1`, a bearer token per device. Like the daemon's protocol, it only grows: an endpoint, field or value once released stays, and `GET /v1/info` says what a server can do, so app, daemon and cloud update independently.

| Endpoint | Since | What |
|---|---|---|
| `GET /healthz` | 0 | the server is up |
| `GET /v1/info` | 0 | the server's version, API version, sign-in methods |
| `GET /v1/me` | 0 | the account and device the token is (`Me`) |
| `GET /v1/devices` | 0 | the account's devices, the asker's marked `current` |
| `DELETE /v1/devices/{id}` | 0 | signs one of the account's devices out: its token, and the team keys sealed to it, go |
| `POST /v1/signout` | 0 | signs the asking device out |
| `POST /v1/auth/device` | 0 | a device starts signing in with a code: its name, kind and public key; gets `deviceCode` (kept) and `userCode` (shown, `XXXX-XXXX`), valid 10 minutes |
| `POST /v1/auth/device/token` | 0 | the device asks for its token every `interval` seconds: `authorization_pending` until approved, then its token, once; `expired_token` after |
| `GET /v1/auth/device/pending/{code}` | 0 | a member signed in sees what waits behind a user code, before approving |
| `POST /v1/auth/device/approve` | 0 | a member signs the device showing a user code in to their account |
| `GET /v1/auth/github?next=` | 0 | page: signs the browser in with GitHub, then goes to `next`, a path of the cloud's |
| `GET /v1/auth/github/callback` | 0 | page: GitHub sending the browser back |
| `GET /device?code=` | 0 | page: what waits behind a code, to approve (`verifyUrlComplete` of a device code); signs in first |
| `POST /v1/teams` | 0 | makes a team, the asker its owner |
| `GET /v1/teams` | 0 | the asker's teams, with their role in each |
| `GET /v1/teams/{team}` | 0 | a team with its members; 404 to anyone not a member |
| `PATCH /v1/teams/{team}` | 0 | renames it, or lets members copy its items out (`copyOut`); admins |
| `DELETE /v1/teams/{team}` | 0 | deletes it and all it holds; its owner (Deleting a team) |
| `PATCH /v1/teams/{team}/members/{account}` | 0 | changes a member's role (admins, below owner); the owner hands the team over by making another member owner, and becomes an admin |
| `DELETE /v1/teams/{team}/members/{account}` | 0 | removes a member (admins), or leaves (the member; the owner hands the team over first); the team's keys sealed to their devices go and its key moves to a new epoch |
| `POST /v1/teams/{team}/invites` | 0 | invites as admin, member or viewer, valid seven days; its token (`enxi_…`) and link once, to the admin |
| `GET /v1/teams/{team}/invites` | 0 | the invites still open, without their tokens; admins |
| `DELETE /v1/teams/{team}/invites/{invite}` | 0 | withdraws one; admins |
| `POST /v1/invites/accept` | 0 | joins the invite's team with its role: the token, or the link as pasted; works once |
| `POST /v1/invites/peek` | 0 | without a token: what an invite is for, before accepting it: the team's name, the role, who invites, when it expires |
| `GET /invite` | 0 | a page: an invite's link: reads its token, signs in with GitHub and joins (`POST /invite/accept`) |
| `GET /v1/teams/{team}/keys` | 0 | the asking device's copies of the team's keys, sealed to it, the epochs' key checks (`checks`), and the epoch new items use |
| `GET /v1/teams/{team}/devices` | 0 | the members' devices, their public keys and the epochs whose key each holds: who to seal the key for |
| `PUT /v1/teams/{team}/keys/{epoch}` | 0 | copies of an epoch's key sealed to members' devices, from a device that holds it; an epoch nobody holds yet, the current one, is started by an admin's device, which keeps a copy for itself and sends the key's check (`check`); another account's device is sealed for by a member or above. A copy once kept stays until its device drops it |
| `DELETE /v1/teams/{team}/keys/{epoch}` | 0 | drops the asking device's own copy of an epoch's key, one that doesn't open or match its check, for a holder of the real key to seal again |
| `GET /v1/teams/{team}/items?after=` | 0 | the team's items changed after a sequence number, oldest first, 200 at most (`more` when others wait), and the number to ask after next; members and viewers |
| `PUT /v1/teams/{team}/items/{id}` | 0 | writes an item at its next version, sealed with the team's current key, or deletes it, keeping only the deletion's sealed mark (1 KiB at most); a version taken already gets 409 and the item as it is; members and above |
| `POST /v1/teams/{team}/audit` | 0 | records an action a device saw in the team's audit log: `rules.changed`, and on a shared host `session.started`, `session.watched`, `session.taken_over` (members and above) |
| `GET /v1/teams/{team}/audit?before=` | 0 | the team's log, the latest first, 100 a page: its own entries and its members' devices signing in and out (admins) |
| `POST /v1/recovery` | 0 | sets the account's recovery key: its public half, a device with no token, in place of the one before |
| `GET /v1/teams/{team}/recovery` | 0 | the team's keys sealed to the asker's recovery key |
| `POST /v1/teams/{team}/recovery` | 0 | keeps an epoch's key sealed to the asking device, when the account's recovery key holds that epoch's |
| `POST /v1/teams/{team}/tickets` | 0 | a one-time ticket (`enxt_…`, two minutes) into a host device of a member of the team, for the asking member's device to hand that host |
| `POST /v1/tickets/redeem` | 0 | the ticket's host redeems it, once: the visitor's account, team and role; recorded as `host.entered` |

Every endpoint but `/healthz` and `/v1/info` needs a device's token (`Authorization: Bearer enxc_…`), given when the device signs in; a request without one, or with one signed out, gets 401. Errors are `{"error": "…"}`. Sign-in, teams, invites, keys, items and hosts come with the phases below; each is recorded here when it lands.

## Running a server

```
docker build -f cmd/enowx-cloud/Dockerfile -t enowx-cloud .
docker run -p 8080:8080 -e ENOWX_CLOUD_URL=https://cloud.example.com -e ENOWX_CLOUD_DATABASE_URL=postgres://… enowx-cloud
```

| Variable | What |
|---|---|
| `ENOWX_CLOUD_URL` | the address members reach it at (required in production: links and sign-in callbacks use it) |
| `ENOWX_CLOUD_ADDR` | where it listens, `:8080` by default |
| `ENOWX_CLOUD_DATABASE_URL` | a Postgres URL (`postgres://user:pass@host:5432/db`), required by the image; unset, a server built without `-tags nosqlite` keeps SQLite in its folder |
| `ENOWX_CLOUD_DATABASE_SCHEMA` | the Postgres schema its tables go in, made when missing: `enowx_cloud` by default, so it can share a database with other apps |
| `ENOWX_CLOUD_DATA` | its folder: SQLite's `cloud.db` when no Postgres is set (not in the image, built without SQLite) |
| `ENOWX_CLOUD_GITHUB_CLIENT_ID`, `ENOWX_CLOUD_GITHUB_CLIENT_SECRET` | the GitHub OAuth app members sign in with; its callback is `<ENOWX_CLOUD_URL>/v1/auth/github/callback` |
| `ENOWX_CLOUD_DEFAULT_TEAM_REPO` | a public GitHub repository, `owner/name`, whose contributors join the default team as members; optional |
| `ENOWX_CLOUD_R2_BUCKET` | a Cloudflare R2 bucket keeping teams' files (a chat's images, sealed); unset, they're kept in the data folder's `blobs` |
| `ENOWX_CLOUD_R2_ACCOUNT_ID`, `ENOWX_CLOUD_R2_ACCESS_KEY_ID`, `ENOWX_CLOUD_R2_SECRET_ACCESS_KEY` | the bucket's account and an R2 API token's keys |
| `ENOWX_CLOUD_DEFAULT_TEAM`, `ENOWX_CLOUD_DEFAULT_ROLE` | a team's ID everyone joins as they sign in, and the role they get (`viewer` by default); optional |
| `ENOWX_CLOUD_SMTP_URL` | `smtp://user:pass@host:587`, to send invites by email; optional |

It speaks plain HTTP: put it behind a proxy that terminates TLS (Coolify's, Caddy, nginx).

## Phases

0. Foundation: this document, the server's frame and database (Postgres, or SQLite with none), its image.
1. Accounts and teams: sign-in, device codes, teams, invites, roles, keys; Settings → Cloud; `enowx-app cloud` and `team` commands.
2. The team's space: picked in the account's menu at the rail's foot, team notes and canvases synced through the cloud, moving and sharing to a team, skills, templates and presets.
3. Hosts and sessions: team hosts and shared hosts, members checked by daemons, watching and taking over sessions, the audit log.
4. Used by the enowX team every day, before it opens to others.

# enowX UI, the community library

People share designs from their canvases to `ui.enowx.ai`, where anyone can browse and try them. The website lives in its own repository, `enowx/enowx-ui`, and holds no logic: every decision on who may see or do what is made by the cloud, in `cloud/library`.

## Parts

| Where | What |
|---|---|
| `core/designshare` | A design's bundle (`enowx-design/1`), its checks, its draft prompt, drafts on a host. The host and the cloud run the same checks |
| `core/designshare/thumb` | A design's picture as a WebP. Apart, so the cloud doesn't carry an image encoder |
| `proto/library.go`, `daemon/library.go`, `daemon/rpc_library.go` | `library.prepare`, `saveDraft`, `deleteDraft`, `publish`, `status`, `withdraw` |
| `proto/library_reuse.go`, `daemon/library_reuse.go`, `cli/library_reuse.go` | `library.useInfo`, `open`, `reference`, `skill`, `newer`; `enowx-app library open`, `ref`, `skill` |
| `cloud/library/reuse.go`, `core/designshare/reuse.go`, `skill.go`, `link.go` | Code, copies, skills, `enowx://` links |
| `cmd/enowx/links.go`, `ui/librarylink.go`, `ui/canvasview/source.go` | Receiving a link, the dialog it opens, what a copied canvas says |
| `ui/canvasview/library.go` | The entry in a design's menu, the form, the status dialog |
| `proto/library_browse.go`, `daemon/library_browse.go` | `library.search`, `item`, `prompt`, `picture`, `askPublish`, all through the cloud the user set |
| `ui/communityview` | The browse page, reached from the Canvas page's list |
| `cli/library.go`, `cli/library_mcp.go` | `enowx-app library` and its MCP tools, for agents |
| `cloud/library`, `cloud/store/library*.go`, `cloud/api/library*.go` | The API under `/v1/library`, its data, the web sign-in, moderation, comments, notifications, numbers, announcing picks |

## Off unless set up

The library is off unless the cloud has `ENOWX_CLOUD_LIBRARY=on`. `GET /v1/info` then says `features: ["library"]` and the website's address in `libraryUrl`. The app offers sharing only when the cloud it was set to answers so: a team's own cloud is unaffected, and nothing reaches any other service.

| Variable | What |
|---|---|
| `ENOWX_CLOUD_LIBRARY` | `on` turns it on. Over an R2 bucket it needs Postgres |
| `ENOWX_CLOUD_LIBRARY_PREFIX` | Where its objects are in the store, `library` by default. A staging cloud sharing a bucket uses its own |
| `ENOWX_CLOUD_LIBRARY_PUBLIC_URL` | The website's address, for credits and links |
| `ENOWX_CLOUD_LIBRARY_RETURN_URL` | Where a sign-in sends its code (`https://ui.enowx.ai/auth/callback`). Only from here, never from a request |
| `ENOWX_CLOUD_LIBRARY_WEB_SECRET` | What the website's server proves itself with. Without it the website can't sign anyone in |
| `ENOWX_CLOUD_LIBRARY_ADMIN_TEAM` | The team whose owner and admins moderate |
| `ENOWX_CLOUD_LIBRARY_DISCORD_CHANNEL` | The channel picks are said in, with `ENOWX_CLOUD_DISCORD_BOT_TOKEN`. Unset, nothing is sent to Discord |
| `ENOWX_CLOUD_LIBRARY_EMBED_URL`, `_EMBED_MODEL`, `_EMBED_KEY` | Searching by meaning: an OpenAI-compatible endpoint (`…/v1`), its model, its key if any. Off while the address is unset |

The cloud now imports `core/autopush`, `core/canvas` and `core/designshare`: a deploy that watches paths has to watch those and `proto/` too.

## Sharing the bucket with teams' files

The library keeps its objects in the store the cloud already uses for teams' files (`ENOWX_CLOUD_R2_*`), under its prefix. `cloud/library` only ever holds that store wrapped by `blobstore.Prefixed`, which checks a key before adding the prefix, so no key resolves outside it. Teams' files, under `teams/`, are never read, changed or counted by it: the library's limits are its own (`library_profiles.used_bytes`, `library_usage`).

**The website holds no bucket token.** R2's lasting API tokens are per bucket, and this one holds teams' files. Only R2's temporary credentials can be limited to a prefix, and they last at most 7 days. So the website gets objects from the cloud, with its secret (`GET items/{id}/versions/{v}/{part}`, live versions only, nobody else), and keeps them in memory (`UI_OBJECT_CACHE_MB`, 128 by default). A version's objects never change, so a kept one costs the cloud nothing; whether it may still show is asked of the item each time.

What this costs: each object not yet kept passes through the cloud once, as a read from R2 and a copy in its memory, at most 16 at once. R2's egress is free and reads the same as before; what's new is CPU and memory on the cloud, and bytes between the two apps. Point `CLOUD_INTERNAL_URL` at the cloud's address on the servers' own network, or those bytes go out through the proxy and back.

## Publishing

1. A design's menu says **Share to Community Library** when the cloud has one. Signed out, it asks to sign in.
2. `library.prepare` checks the design may leave its space (a team's only when its admins allow copying out), packs it, puts known CDNs' scripts inside it (a preview loads nothing else), checks it and makes its picture, as a draft in `library/` in the host's folder.
3. The form says what becomes public and what stays private. Sending waits for the prompt's approval and the rights statement, and warnings wait to be accepted.
4. `library.publish` checks once more, then creates the item, uploads, and submits. **The cloud repeats the checks** and makes the documents people see itself from the bundle: a host can't send its own.

A bundle has fields for a design and its files alone, so a canvas's comments, brief, history and chats can't come along. The prompt is written from the design's structure and what the creator typed; a canvas's chat is never read. Agents and the command line can't publish: the form is the only way (see [Agents](#agents)).

A new account's designs wait in review; once one is approved its next ones go live directly. A new version keeps the address; what it says of the item shows only once it's live, and changing an item's kind goes through review again. Unpublishing answers 410 from then on, and the objects go after 30 days.

A design keeps 5 versions: at the limit, a new one makes the oldest that neither shows nor waits for review go, objects and all, so it can always be updated. The website lists what shows and what showed before (`GET items/{id}/versions`) and shows any of them by `?v=N`; a version in review, rejected or gone can't be read by its number.

| Check | Result |
|---|---|
| A secret in the page, title, description, prompt or files; a known tracker; a form sending a password elsewhere | Refused |
| An email or phone number, an internal address, a request out, a link elsewhere, a script changing the address, a refresh, a `<base>`, a file | A warning to accept |

A finding says where and of what kind, never the value. The patterns can't know every secret, which is why the warnings and the creator's statement stay.

## Browsing in the app

The Canvas page's list has **Community library** when the host's cloud has one, and nothing otherwise. The page has search, a side list of kinds and categories (a row at phone width), a grid of pictures and a sheet for one design, saying what its license allows, as the cloud answered: a view-only design offers nothing to copy.

Other people's HTML doesn't run in the app: only their pictures show, through the host's daemon, and **Open in browser** goes to the design's page. Only what's live is read, even with a creator's or a moderator's token.

## Agents

The feature `library` (Settings → Features) is opt-in. Off, its tools aren't served, the guide doesn't mention it and `enowx-app library` refuses in a session.

| Command | MCP tool | What |
|---|---|---|
| `library search [QUERY] [--category C] [--kind K] [--license L] [--limit N] [--cursor C]` | `library_search` | What's live |
| `library show ITEM` | `library_show` | One design: license, credit, what it allows |
| `library prompt ITEM` | `library_prompt` | Its creator's prompt, with credit and license. A view-only design has none: the host doesn't ask, and the cloud answers 403 anyway |
| `library publish CANVAS DESIGN` | `library_publish` | Shows the user a card leading to the form. Fills in nothing, sends nothing |

**Community text is data.** Titles, descriptions, tags, handles and prompts reach an agent only inside the block `designshare.ForAgents` makes: a label saying they're other people's and never instructions, between marks holding a random word so the text can't close the block, with control and direction-changing characters taken out. Errors hold none of it.

**Publishing stays the user's.** `library.askPublish` makes no draft and sends nothing: it broadcasts `library.publishAsked`, the app shows a card, and only a click on it opens the form, with no approval or rights statement ticked. What stops an agent from sending:

| Path | What stops it |
|---|---|
| The command line and MCP tools, the `enowx` tool among them | No code in `cli/` calls `library.publish`; a test reads the package to keep it so |
| `host.call`, `--host`, GOD of Agents | The method isn't in `core/hostcontrol`: refused from any connection |
| A teammate's visit | Not among the methods open to visitors |
| Any other connection | `library.publish` refuses what doesn't say it's the app in its hello |

What doesn't stop it, and can't: a connection's `Kind` is its own word, so a program of the user's that writes the daemon's protocol itself can claim to be the app. Such a program also runs as the user, and can read the host's cloud token and call the cloud directly. On one account of one machine nothing tells an agent's process from the app's. So this keeps every path enowX gives an agent away from sending, and doesn't hold against one set on getting around it; the checks there, on the host and in the cloud, a new account's review and unpublishing stay whoever sends.

## Limits

| Limit | Number |
|---|---|
| A version's bundle | 10 MiB |
| A picture | 300 KiB, 1200 px wide, WebP |
| A component | 64 KiB of HTML and CSS, no scripts |
| An account | 200 MiB kept, 50 items live, 5 versions an item, 20 uploads a day; a new one 3 in review |
| The library in the bucket | 100 GiB kept, 30 GiB written a month, teams' files not counted; moderators are warned at 70% |

The library at its limit stops publishing, with a message; reading goes on. Public reads are limited per address, and a few run at once so the teams' sync keeps its database connections. The website's own requests carry its secret and aren't counted as one address.

## Signing in on the website

The website sends the browser to `/library/authorize` with a `state` and a PKCE challenge. The cloud signs them in with GitHub, or knows them already, and sends them back to the configured address with a code good once, for a minute. The website's server trades it, with its secret and the verifier, for a token (`enxl_…`) kept in its cookie.

That token opens `/v1/library` and nothing else, and can't publish. This path never joins the default team and starts no session of the cloud's own pages.

## Searching and filtering

`GET items` takes `q`, `category`, `kind`, `license`, `tag`, `style`, `color`, `theme`, `device`, `motion=1` and `sort` (`relevant`, `new`, `popular`, `featured`). Every value reaches the database as a parameter; one that isn't known finds nothing. A search ranks by how well it matches unless another sort is asked for.

**Words.** On Postgres, a `tsvector` the database keeps itself (title first, then tags, then description, English stemming) with a GIN index; all words have to match, the last as a prefix. It needs Postgres 12 or later. SQLite, for tests and small clouds, uses LIKE with the same meaning and order.

**What the server reads at submit, from the bundle and not from the host** (`designshare.LookOf`): the theme by the page's ground, the device by its width (600 or less is a phone; a phone version counts too), and up to three dominant colors, kept per version in `library_version_colors`. Nothing is drawn: a page's ground counts most, then backgrounds, then the rest, following CSS variables and utility classes. Scripts aren't read, so a design that paints itself from a script reads as what its page says. Style is the creator's pick in the form.

**Load.** Searches share the few places of public reads, so teams' sync keeps its connections, hold a connection 3 seconds at most, and are limited to 60 a minute per address. Behind the website the browser's address is counted, which its secret vouches for.

**By meaning.** Off unless an embedder is set, and on Postgres only with pgvector: found at the start, turned on if the database role may, and otherwise the library runs without it and says so in the log. `GET /v1/info` adds `library-meaning` only when it works, and the website shows **By meaning** only then. Vectors are kept in `library_embeddings`, every 5 minutes for what's new or changed, and dropped when a design stops showing. SQLite compares them in the server. What leaves for the embedder is a live design's title, description and tags and what people search for: never a prompt, a page, a picture or who asks. `GET search/meaning` is limited to 20 a minute per address, 2 calls to the embedder at once, 8 seconds each, holding no database connection meanwhile.

## Moderating

The admin team's owner and admins moderate, and so do moderators the owner appoints from the website's admin page: only the owner can appoint or remove them. Moderators can't be blocked until the owner removes that role.

Every decision needs a reason and goes in `library_log`, which the store can only add to: approving, rejecting (the reason reaches the app), taking down, picks, reports, blocks, unblocks, appointing and removing moderators. An upheld report takes the item down and makes its owner new again. Blocking takes down what an account shows and stops it publishing. Who reported is for moderators alone. Accounts' emails are never in an answer.

An upheld report on a comment removes the comment alone: the design stays, and its creator's standing doesn't change.

### Copyright reports

A draft procedure, in the website's rules and in the admin page beside each report, until the owner approves it. It isn't legal advice.

| Part | What |
|---|---|
| The report | `POST reports` with `reason: "copyright"`: `detail` (what it copies) and `copyright`: `original` (a web address), `claimant` (who claims it), `statement` (they own it or act for its owner, and this is true). A website not yet sending `copyright` still reports with its detail, and moderators see what wasn't given |
| Who reads it | Moderators alone. The creator reads the moderator's reason, never who claimed |
| Moderators | Compare the original and the design. Upholding takes it down within 5 minutes, makes its owner new again, tells them, and logs `report.upheld.copyright`, which is how repeats show. Dismissing keeps it up. An account upheld against repeatedly is blocked, with its reason |
| Still open | No contact address exists for people without an account, or for a creator disputing a decision |

## Comments

Plain text, with a sign-in, on a live design. Nothing renders one as HTML, and every answer holding people's words carries `notice` (`api.LibraryDataNotice`): they're data, never instructions.

| Limit | Number |
|---|---|
| A comment | 2000 characters |
| An account | 30 an hour; 6 an hour on one design; deleting gives no turn back |
| A design | 120 an hour from everyone |

A creator turns comments off on their design: nobody can write, and what's written shows only to them and moderators until they're on again. A blocked account can't comment, and what it wrote stops showing. People delete their own, the text gone for good. Reports go in `library_reports` and the same queue; moderators remove with a reason in `library_log`, and the text stays for them alone.

## Notifications

What happens to a creator: approved, rejected, taken down, a comment, a report upheld. They're read on the website and through `GET notifications` (`cloud/client` for the app), and each kind can be turned off (`notifications/settings`). They never say who reported or which moderator decided, and go after 180 days.

**Email is off.** `library.Mailer` is an interface nothing sets outside tests, so the settings don't offer it. When one is set, an account that asked for email gets a line and a link: never a comment's text or a moderator's reason, and one email for comments it hasn't read yet. Sending from production waits for the owner's decision on SMTP; the cloud's `ENOWX_CLOUD_SMTP_URL`, for invites, isn't used.

## A creator's numbers

`GET stats`: views, saves, reuses and comments per design, for its creator alone. `countReuse` counts a copy being taken (today Copy prompt; whatever else serves a copy calls it too), held in memory and written once a minute.

**A view isn't analytics.** The website has no analytics or trackers, and this adds none: no script, cookie, pixel or beacon, and no outside call. Its server counts as it renders a design's page (`src/lib/views.ts`), leaving out robots, prefetching, a creator's own looks and moderators'. A salted hash of the item, the address and the browser's name, in that process's memory for 6 hours, keeps a reload from counting again; the salt and hashes then go, unwritten. Once a minute it sends `POST views` with how many per item and nothing else, so the cloud never hears of a person, and `library_stats` holds totals. These are coarse: a restart loses up to a minute, and a change of address counts again.

## A moving design's video

At publish, `library.publish` has the app render a design that moves, unless the form says not to, with `canvas.video` and `preview`: 6 seconds at most, 720 pixels wide, smaller than an export, left in no folder of the user's. It needs a serving app, a web view and ffmpeg: without them, or if it comes out too big, the design goes without one and nothing else changes.

It's part `video`: 4 MiB, 15 seconds, only on a design that moves. The cloud reads the file's boxes (`designshare.CheckVideo`) and refuses anything but an MP4 of video tracks alone, so a preview can't carry sound. Like a picture it comes from the host: moderators see it in review, and a trusted account's isn't reviewed first.

The website serves it at `/v/<id>/<version>`, with the headers of `/t/*`, no cookie, and byte ranges for Safari. A card shows its picture until a mouse rests on it, muted; a design's page has a Video button with the picture first. Nothing is fetched until then, and asking for less motion stops the hover.

## Announcing picks

An admin says a pick on Discord from the admin page's Picks tab, with `DecisionAnnounce`. It's off unless a channel is set, for the owner and admins (not moderators they appoint), only for a live pick, once (`announced_at`), with its reason in the log. A failure is logged and can be tried again. A title is escaped and mentions are off, so it can neither ping anyone nor make a link. Which channel, and the bot's permission to post in it, wait for the owner.

## Creators

A profile is made as an account first uses the library. Its name and picture are the account's; what else shows is the creator's choice, and off until chosen: GitHub is hidden unless turned on, numbers show unless turned off. A handle is 3 to 30 small letters, digits and single hyphens, changes once in 30 days, and can't be a reserved word or another account's GitHub login. Links and the contact are `http(s)` only, with no name, password or email in them, and a bio or skill holding an email is refused. An account's email is in no answer, an export included.

**Open for work** is a flag, kinds of work, skills, a time zone and a contact link: a link and nothing else, as the site has no messages or forms. It needs that link, and the list (`GET creators?open=1`, by `kind`, `skill`, `zone`) also needs a live design, so an account made only to post a link has nothing to list it by. Turning it off, like hiding, goes through even past the rate limit.

**Hiding.** A hidden or blocked profile answers just as one that doesn't exist, to all but its creator and moderators, and so do its items list and public collections. Its designs leave every list (browse, feed, others' public collections, the open for work list) but **stay at their addresses, with their credit**, as links and copies may need it under their license: the item says `unlisted`, and the website then keeps search engines off and links to no profile. Unpublishing is how a design goes away. Those who follow get it all back when the creator shows again. A collection's owner, and someone who liked a design, still see it in their own lists.

**The contributor badge** isn't the creator's to set. `github_contributors` holds GitHub IDs the Cloud found to have contributed: a merged pull request in the Contributor role's repository (its signed webhook or its sign-in, see `docs/cloud.md`) or GitHub's list for the default team's repository, when the Cloud reads that anyway. No new call goes to GitHub or Discord for it. Someone who contributed earlier gets it at their next merge, Connect, or reading of the list.

**Likes, collections, follows.** A like is `library_saves`, and `items.saves` counts them; one's own design can't be liked, nor oneself followed. What someone likes, who they follow, and a private collection answer their owner alone, moderators not excepted; others get only counts, and a private collection answers 404. All take a device or a web session, and a blocked account can only remove. The store's functions are plain to call, among them `LibraryFollowers` and `LibrarySavers`.

| Limit | Number |
|---|---|
| Per account, a minute | 60 likes, 30 follows, 60 collection changes, 20 profile changes, 2 exports, 240 reads of one's own lists (in memory: a restart forgets them) |
| Per account, in all | 100 collections, 500 designs in one, 2000 follows, 5000 likes |

## Taking data out, and deleting it

`GET me/export` answers a file of what the library keeps of the asker: profile with its settings, designs with their versions and prompts, likes, collections, follows, reports they made, and decisions about them without who decided. A design's files aren't in it: the app that published them has them.

`DELETE me` takes the handle, typed, and deletes the profile, likes, collections, follows both ways and web sessions. Each design is unpublished and emptied, its address answering 410 for good, and its objects go by the sweep's purge, made due at once and run then, the rest on the next hourly sweep; that includes a design a moderator took down. **It touches no Cloud account, device, team or team file**, and signing in again starts an empty profile.

Kept on purpose: `library_log`'s rows with the account's ID, as it's append-only, plus an `account.deleted` row; reports they made, for moderators, no longer naming them (those about them close); a blocked account's row, emptied, so that deleting isn't a way out of a block. Those who moderate give that up first. A new table holding an account's data has to join `DeleteLibraryAccount` and `LibraryExport` in `cloud/store/library_erase.go`.

**Similar designs.** At submit the server hashes the picture (a 64-bit perceptual hash, pure Go). The queue lists, with the design under review, up to 5 other designs within 10 bits, taken-down ones too. This is advice to check by hand: it refuses, holds and logs nothing, and a trusted account's lookalike still goes live without passing the queue. A near-blank picture has no hash, and the queue says it couldn't be compared.

## API

| Access | Endpoints under `/v1/library` |
|---|---|
| Anyone, limited per address | `GET items`, `GET items/{id}`, `GET items/{id}/versions`, `GET items/{id}/prompt` (403 when view only), `GET items/{id}/comments`, `GET items/{id}/versions/{v}/pictures/{thumb,thumb-phone}` (a live version's), `GET creators?open=1`, `GET creators/{handle}`, `GET creators/{handle}/items`, `GET creators/{handle}/collections`, `GET collections/{id}`, `GET search/meaning` |
| The website's secret | `GET items/{id}/versions/{v}/{doc,doc-phone,thumb,thumb-phone,video}` (a live version's), `POST items/{id}/escaped`, `POST views`, `POST auth/exchange` |
| A device | `POST items`, `PUT items/{id}/versions/{v}/{source,thumb,thumb-phone,video}`, `POST items/{id}/versions/{v}/submit`, `GET mine` |
| A device or a web session | `DELETE items/{id}`, `GET me`, `POST reports`, `POST auth/signout`, `POST items/{id}/comments`, `DELETE items/{id}/comments/{comment}`, `PUT items/{id}/comments-off`, `GET notifications`, `POST notifications/read`, `GET`/`PUT notifications/settings`, `GET stats` |
| Moderators | `POST admin/comments/{id}/remove`, `GET admin/picks`, `GET admin/queue`, `GET admin/items/{id}/versions/{v}/{doc,doc-phone,thumb,thumb-phone,video}`, `POST admin/items/{id}/decisions`, `POST admin/reports/{id}/decisions`, `POST admin/accounts/{id}/block`, `.../unblock`, `GET admin/log`, `GET admin/usage` |
| Anyone, by license and kind, limited per address | `GET items/{id}/code`, `GET items/{id}/skill` |
| A device, by license and kind | `GET items/{id}/source`, `GET items/{id}/reference` |
| The website's secret | `GET items/{id}/versions/{v}/{doc,doc-phone,thumb,thumb-phone}` (a live version's), `POST items/{id}/escaped`, `POST auth/exchange` |
| A device | `POST items`, `PUT items/{id}/versions/{v}/{source,thumb,thumb-phone}`, `POST items/{id}/versions/{v}/submit`, `GET mine` |
| A device or a web session | `DELETE items/{id}`, `GET me`, `PUT me`, `DELETE me`, `GET me/export`, `POST reports`, `POST auth/signout`, `GET saves`, `GET/PUT/DELETE saves/{id}`, `PUT/DELETE creators/{handle}/follow`, `GET following`, `GET feed`, `GET/POST collections`, `PUT/DELETE collections/{id}`, `PUT/DELETE collections/{id}/items/{item}` |
| Moderators | `GET admin/queue`, `GET admin/items/{id}/versions/{v}/{doc,doc-phone,thumb,thumb-phone}`, `POST admin/items/{id}/decisions`, `POST admin/reports/{id}/decisions`, `POST admin/accounts/{id}/block`, `.../unblock`, `GET admin/log`, `GET admin/usage` |
| The owner | `GET admin/accounts?q=`, `GET admin/moderators`, `POST admin/moderators`, `DELETE admin/moderators/{id}` |

Objects are in the store as `<prefix>/items/<id>/v<n>/{source,doc,doc-phone,thumb,thumb-phone,video}`, `source` and `doc` gzipped.

## Taking a design

| What | Component | Full page | View only |
|---|---|---|---|
| Try the preview | yes | yes | yes |
| Copy prompt (`prompt`) | yes | yes | 403 |
| Use component (`code`) | yes | 404 | 403 |
| Reference or skill (`reference`, `skill`) | 404 | yes | 403 |
| Open in enowX (`source`) | yes | yes | 403 |

The API is what refuses, not only the buttons. Each of these takes `?version=N` and answers by **that version's** license and kind: a license change applies from its version on, so an earlier version keeps what it allowed, and one that was view only stays so.

- **Use component** answers the HTML and the CSS apart (split by the parser `core/canvas` uses), each opening with the notice, and the files they name, which don't come along as their rights may not be the design's. A title can't close the comment it's in.
- **Open in enowX** makes a new canvas with the one design and its files, a `Source` (item, version, creator, license, address) and the notice in its page. It stays as it is when the original changes or goes; the canvas says when a newer version is out. It needs the host signed in to its cloud.
- **Reference** keeps a full page's picture in a canvas's files and one credit line beside its brief (`Canvas.References`, apart from `Brief`, since builds ask for a brief by its being there). Nothing goes in the designs.
- **Skill** is a `SKILL.md`: credit and license on top, the label, then colors, fonts and spacing matched value by value from the CSS, and the prompt in a fence it can't close. No code. Its description holds no word of the creator's, as agents load that line by themselves. It's installed through `core/envsync`, from the app's dialog alone: `library.skill` refuses to install for any connection but the app, as `library.publish` does. `library skill ITEM --install` and the `library_skill` tool's `install` only call `library.askSkill`, which keeps nothing and shows the user a card leading to that dialog: what's kept is loaded by agents from then on, so an agent told to by a page it read mustn't be able to keep it. Without `--install` the command only prints it.

**It's other people's data.** Everything an agent or a command gets from the library says so (`designshare.OthersData`), and `canvas show` and `canvas brief` say it first for a copy or a reference.

### Links

The website opens `enowx://library/{open,reference,skill}?item=ID&version=N`, and shows "Nothing opened?" either way, since the system doesn't say. Any page on the web can open such a link, so it only ever opens a dialog: it shows what the chosen host's cloud says of the item, never what the link says, and nothing happens until the user picks where and says so. Links on anything but this form are ignored.

`urlSchemes` in `cmd/enowx/mygo.json` registers the scheme in what the installers write: the bundle's `Info.plist` on macOS, the `.desktop` entry on Linux, the registry on Windows. An install that updates itself never runs an installer again, so a packaged app registers it at start when it isn't. macOS sends links to the running app by Apple Events. Windows and Linux start a new process, which hands the link to the first by MyGo's single-instance lock and exits; a launch without a link goes on as before, so enowX can still run twice. An app with `ENOWX_HOME` set (`make dev`, tests) neither holds the lock nor registers.

## What this phase leaves out

Remixes, copying one component out of a page, and the pgvector path of meaning search, which has never run: the test database lacks the extension.

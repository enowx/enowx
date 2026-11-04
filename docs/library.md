# enowX UI, the community library

People share designs from their canvases to `ui.enowx.ai`, where anyone can browse and try them. The website lives in its own repository, `enowx/enowx-ui`, and holds no logic: every decision on who may see or do what is made by the cloud, in `cloud/library`.

## Parts

| Where | What |
|---|---|
| `core/designshare` | A design's bundle (`enowx-design/1`), its checks, its draft prompt, drafts on a host. The host and the cloud run the same checks |
| `core/designshare/thumb` | A design's picture as a WebP. Apart, so the cloud doesn't carry an image encoder |
| `proto/library.go`, `daemon/library.go`, `daemon/rpc_library.go` | `library.prepare`, `saveDraft`, `deleteDraft`, `publish`, `status`, `withdraw` |
| `ui/canvasview/library.go` | The entry in a design's menu, the form, the status dialog |
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

A bundle has fields for a design and its files alone, so a canvas's comments, brief, history and chats can't come along. The prompt is written from the design's structure and what the creator typed; a canvas's chat is never read. Agents and the command line can't publish: the form is the only way.

A new account's designs wait in review; once one is approved its next ones go live directly. A new version keeps the address; what it says of the item shows only once it's live, and changing an item's kind goes through review again. Unpublishing answers 410 from then on, and the objects go after 30 days.

| Check | Result |
|---|---|
| A secret in the page, title, description, prompt or files; a known tracker; a form sending a password elsewhere | Refused |
| An email or phone number, an internal address, a request out, a link elsewhere, a script changing the address, a refresh, a `<base>`, a file | A warning to accept |

A finding says where and of what kind, never the value. The patterns can't know every secret, which is why the warnings and the creator's statement stay.

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

## API

| Access | Endpoints under `/v1/library` |
|---|---|
| Anyone, limited per address | `GET items`, `GET items/{id}`, `GET items/{id}/versions`, `GET items/{id}/prompt` (403 when view only), `GET items/{id}/comments` |
| The website's secret | `GET items/{id}/versions/{v}/{doc,doc-phone,thumb,thumb-phone,video}` (a live version's), `POST items/{id}/escaped`, `POST views`, `POST auth/exchange` |
| A device | `POST items`, `PUT items/{id}/versions/{v}/{source,thumb,thumb-phone,video}`, `POST items/{id}/versions/{v}/submit`, `GET mine` |
| A device or a web session | `DELETE items/{id}`, `GET me`, `POST reports`, `POST auth/signout`, `POST items/{id}/comments`, `DELETE items/{id}/comments/{comment}`, `PUT items/{id}/comments-off`, `GET notifications`, `POST notifications/read`, `GET`/`PUT notifications/settings`, `GET stats` |
| Moderators | `POST admin/comments/{id}/remove`, `GET admin/picks`, `GET admin/queue`, `GET admin/items/{id}/versions/{v}/{doc,doc-phone,thumb,thumb-phone,video}`, `POST admin/items/{id}/decisions`, `POST admin/reports/{id}/decisions`, `POST admin/accounts/{id}/block`, `.../unblock`, `GET admin/log`, `GET admin/usage` |
| The owner | `GET admin/accounts?q=`, `GET admin/moderators`, `POST admin/moderators`, `DELETE admin/moderators/{id}` |

Objects are in the store as `<prefix>/items/<id>/v<n>/{source,doc,doc-phone,thumb,thumb-phone,video}`, `source` and `doc` gzipped.

## What this phase leaves out

Copying a component's code, Open in enowX, references and skills (`code`, `source` and `skill` have no endpoints yet, though an item's `actions` already say what its license allows), full-text and meaning search, profiles, saves, and exporting or deleting a profile.

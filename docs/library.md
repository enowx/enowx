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
| `cloud/library`, `cloud/store/library*.go`, `cloud/api/library.go` | The API under `/v1/library`, its data, the web sign-in, moderation |

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

A design keeps 5 versions: at the limit, a new one makes the oldest that neither shows nor waits for review go, objects and all, so it can always be updated. The website lists what shows and what showed before (`GET items/{id}/versions`) and shows any of them by `?v=N`; a version in review, rejected or gone can't be read by its number.

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

## API

| Access | Endpoints under `/v1/library` |
|---|---|
| Anyone, limited per address | `GET items`, `GET items/{id}`, `GET items/{id}/versions`, `GET items/{id}/prompt` (403 when view only) |
| The website's secret | `GET items/{id}/versions/{v}/{doc,doc-phone,thumb,thumb-phone}` (a live version's), `POST items/{id}/escaped`, `POST auth/exchange` |
| A device | `POST items`, `PUT items/{id}/versions/{v}/{source,thumb,thumb-phone}`, `POST items/{id}/versions/{v}/submit`, `GET mine` |
| A device or a web session | `DELETE items/{id}`, `GET me`, `POST reports`, `POST auth/signout` |
| Moderators | `GET admin/queue`, `GET admin/items/{id}/versions/{v}/{doc,doc-phone,thumb,thumb-phone}`, `POST admin/items/{id}/decisions`, `POST admin/reports/{id}/decisions`, `POST admin/accounts/{id}/block`, `.../unblock`, `GET admin/log`, `GET admin/usage` |
| Anyone, by license and kind, limited per address | `GET items/{id}/code`, `GET items/{id}/skill` |
| A device, by license and kind | `GET items/{id}/source`, `GET items/{id}/reference` |
| The owner | `GET admin/accounts?q=`, `GET admin/moderators`, `POST admin/moderators`, `DELETE admin/moderators/{id}` |

Objects are in the store as `<prefix>/items/<id>/v<n>/{source,doc,doc-phone,thumb,thumb-phone}`, `source` and `doc` gzipped.

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
- **Skill** is a `SKILL.md`: credit and license on top, the label, then colors, fonts and spacing matched value by value from the CSS, and the prompt in a fence it can't close. No code. Its description holds no word of the creator's, as agents load that line by themselves. It's installed through `core/envsync`: from the app's dialog, or by `library skill --install --agent NAME`; without `--install` the command only prints it.

**It's other people's data.** Everything an agent or a command gets from the library says so (`designshare.OthersData`), and `canvas show` and `canvas brief` say it first for a copy or a reference.

### Links

The website opens `enowx://library/{open,reference,skill}?item=ID&version=N`, and shows "Nothing opened?" either way, since the system doesn't say. Any page on the web can open such a link, so it only ever opens a dialog: it shows what the chosen host's cloud says of the item, never what the link says, and nothing happens until the user picks where and says so. Links on anything but this form are ignored.

`urlSchemes` in `cmd/enowx/mygo.json` registers the scheme in what the installers write: the bundle's `Info.plist` on macOS, the `.desktop` entry on Linux, the registry on Windows. An install that updates itself never runs an installer again, so a packaged app registers it at start when it isn't. macOS sends links to the running app by Apple Events. Windows and Linux start a new process, which hands the link to the first by MyGo's single-instance lock and exits; a launch without a link goes on as before, so enowX can still run twice. An app with `ENOWX_HOME` set (`make dev`, tests) neither holds the lock nor registers.

## What this phase leaves out

Copying one component out of a design, remixes, full-text and meaning search, profiles, saves, and exporting or deleting a profile.

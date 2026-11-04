# enowX UI, the community library

People share designs from their canvases to `ui.enowx.ai`, where anyone can browse and try them. The website lives in its own repository, `enowx/enowx-ui`, and holds no logic: every decision on who may see or do what is made by the cloud, in `cloud/library`.

## Parts

| Where | What |
|---|---|
| `core/designshare` | A design's bundle (`enowx-design/1`), its checks, its draft prompt, drafts on a host. The host and the cloud run the same checks |
| `core/designshare/thumb` | A design's picture as a WebP. Apart, so the cloud doesn't carry an image encoder |
| `proto/library.go`, `daemon/library.go`, `daemon/rpc_library.go` | `library.prepare`, `saveDraft`, `deleteDraft`, `publish`, `status`, `withdraw` |
| `ui/canvasview/library.go` | The entry in a design's menu, the form, the status dialog |
| `cloud/library`, `cloud/store/library*.go`, `cloud/api/library.go` | The API under `/v1/library`, its data, the web sign-in, moderation |

## Off unless set up

The library is off unless the cloud has `ENOWX_CLOUD_LIBRARY_R2_BUCKET`, which needs Postgres. `GET /v1/info` then says `features: ["library"]` and the website's address in `libraryUrl`. The app offers sharing only when the cloud it was set to answers so: a team's own cloud is unaffected, and nothing reaches any other service.

| Variable | What |
|---|---|
| `ENOWX_CLOUD_LIBRARY_R2_BUCKET`, `_R2_ACCOUNT_ID`, `_R2_ACCESS_KEY_ID`, `_R2_SECRET_ACCESS_KEY` | The bucket, apart from teams' files, and a token that can write it |
| `ENOWX_CLOUD_LIBRARY_PUBLIC_URL` | The website's address, for credits and links |
| `ENOWX_CLOUD_LIBRARY_RETURN_URL` | Where a sign-in sends its code (`https://ui.enowx.ai/auth/callback`). Only from here, never from a request |
| `ENOWX_CLOUD_LIBRARY_WEB_SECRET` | What the website's server proves itself with. Without it the website can't sign anyone in |
| `ENOWX_CLOUD_LIBRARY_ADMIN_TEAM` | The team whose owner and admins moderate |
| `ENOWX_CLOUD_LIBRARY_DIR` | For developing: a folder instead of a bucket |

The cloud now imports `core/autopush`, `core/canvas` and `core/designshare`: a deploy that watches paths has to watch those and `proto/` too.

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
| The bucket | 100 GiB kept, 30 GiB written a month; moderators are warned at 70% |

A full bucket stops publishing, with a message; reading goes on. Public reads are limited per address, and a few run at once so the teams' sync keeps its database connections. The website's own requests carry its secret and aren't counted as one address.

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
| The website's secret | `POST items/{id}/escaped`, `POST auth/exchange` |
| A device | `POST items`, `PUT items/{id}/versions/{v}/{source,thumb,thumb-phone}`, `POST items/{id}/versions/{v}/submit`, `GET mine` |
| A device or a web session | `DELETE items/{id}`, `GET me`, `POST reports`, `POST auth/signout` |
| Moderators | `GET admin/queue`, `GET admin/items/{id}/versions/{v}/{doc,doc-phone,thumb,thumb-phone}`, `POST admin/items/{id}/decisions`, `POST admin/reports/{id}/decisions`, `POST admin/accounts/{id}/block`, `.../unblock`, `GET admin/log`, `GET admin/usage` |
| The owner | `GET admin/accounts?q=`, `GET admin/moderators`, `POST admin/moderators`, `DELETE admin/moderators/{id}` |

Objects are in the bucket as `items/<id>/v<n>/{source,doc,doc-phone,thumb,thumb-phone}`, `source` and `doc` gzipped.

## What this phase leaves out

Copying a component's code, Open in enowX, references and skills (`code`, `source` and `skill` have no endpoints yet, though an item's `actions` already say what its license allows), full-text and meaning search, profiles, saves, and exporting or deleting a profile.

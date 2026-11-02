# Traffic: see, mock and change HTTP

The side panel's **Traffic** tool shows the HTTP that what a session runs sends and receives, on its host, live: requests and responses with their headers and bodies, WebSocket, server-sent events and gRPC messages, over TLS too. Rules answer requests from mocks, forward them, change them, slow or fail them, or pause them for the user to edit. Agents read and drive the same from the command line and over MCP. It is enowX's own code (`core/intercept`), written from the protocols.

## How it works

Each host's daemon runs an interceptor the first time something asks for it (`daemon/traffic.go`): an HTTP and SOCKS5 proxy on loopback, on the same port across restarts. It terminates TLS with a certificate authority of the host's own, made once in `~/.enowx-app/intercept/` (`ca.pem`; its key, `ca-key.pem`, never leaves the host), and makes a certificate per server name on the fly. HTTP/1.1, HTTP/2 (negotiated, or with prior knowledge), WebSocket and gRPC go through; anything else inside a tunnel is passed on as it is and shown as a tunnel with its sizes.

Nothing is caught until it is attached. Each thing attached is a **source**, with a loopback port of its own (clients that can't send proxy credentials, as Android's proxy setting, Chromium and Java, come there) and credentials for the main port (`http://source:token@127.0.0.1:port`). Every exchange carries its source and its session. Detaching a source stops recording it; its port stays and passes traffic on unrecorded, so programs pointed at it keep working. Targets come back on their ports when the daemon starts again.

Upstream, certificates are checked, except for loopback (development servers sign their own) and the hosts Settings names. An upstream proxy can be set.

## What can be attached (Intercept)

| Target | What it does |
|---|---|
| **New terminal** | A new tab of the session whose shell has the environment below: what is run there is caught. |
| **An open tab** | Types the environment into the tab's shell (`export …`), so what it runs next is caught. |
| **This session's new tabs** | Tabs opened from then on, and what the Run menu and Run app start. The agent's own traffic only when asked (its API calls carry its keys, masked in the list), once it starts again. |
| **The Browser tool** | A reverse proxy in front of the session's dev server: what previews load from it, the HMR WebSocket included, the Host header kept as the page sent it (`traffic.reverse`). Previews opened from then on. Other sites a page calls aren't caught yet: the web view has no proxy setting. |
| **Through Traffic** (Ports tool) | The same for every port of the session opened or forwarded from then on. |
| **A browser** | Chrome, Chromium, Edge, Brave or Firefox found on the host, on a fresh profile in `~/.enowx-app/intercept/profiles/`: Chromium-based ones with `--proxy-server` and `--ignore-certificate-errors-spki-list` for the CA's key only, Firefox with a profile whose proxy is set and, with NSS's `certutil`, whose certificates hold the CA. Your own profiles aren't touched. On a server without a screen, it opens on the session's display. |
| **An Electron app** | Its executable started with the same Chromium flags and the environment. |
| **Docker containers** | A tab whose `docker` is a wrapper: `docker run` and `docker create` get the proxy (`host.docker.internal`, which the source also listens for on the `docker0` bridge on Linux) and the CA mounted in, with the variables below pointing at it. |
| **An Android device** | From the Android tool: see [android.md](android.md#catching-a-devices-traffic). |
| **Anything else** | A source of its own, its address to give a program by hand, and the CA to copy. |

## The environment a caught shell gets

`intercept.Env`: the proxy in `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY` and their lowercase forms, `NODE_USE_ENV_PROXY=1` (Node 24 reads the proxy variables), and the CA where each language looks: `NODE_EXTRA_CA_CERTS`, `DENO_CERT`, `REQUESTS_CA_BUNDLE`, `CURL_CA_BUNDLE`, `GIT_SSL_CAINFO`, `PIP_CERT`, `AWS_CA_BUNDLE`, `CARGO_HTTP_CAINFO`, `HEX_CACERTS_PATH`, and `SSL_CERT_FILE` on Unix (Go, OpenSSL, Ruby, Python's `ssl`), which replaces the system's roots, so it names a bundle of the host's roots with the CA. Where a JDK's `keytool` is found, `JAVA_TOOL_OPTIONS` gives Java the proxy and a trust store of the JDK's roots with the CA. `enowx-app traffic env` prints the same as `export` lines (`--off` undoes them).

curl, Python's `requests`, Node's `fetch`, Go's `net/http` and Java are tested through it. Go never sends `localhost` or `127.0.0.1` through a proxy: calls to a local server go straight to it.

A session's checks run with its traffic caught when it is, so the project's mocks answer its tests.

## The tool

The side panel holds its lists; what they open shows in a tab of the session, in the center, with the width.

- **Requests:** the list, live, this session's or **All** of the host's, filtered (below). Picking one opens it in a tab of the session, with its own Request, Response, Messages and Timing: the request and the response with their headers and body by kind (JSON pretty, HTML, XML, JavaScript and CSS highlighted, images, forms as fields, other binaries as hex), the WebSocket, SSE or gRPC messages, and the timing (connected, TLS, sent, first byte, done), TLS and the rule that matched. **Mock this**, a breakpoint on requests like it, **Edit and send**, **Send to the agent** (secrets masked), pin, **Show secrets**, **Copy as** curl, fetch, Python, Go, HTTPie or PowerShell, copy the URL or the body, export it.
- **Recording, paused, stopped:** the toolbar says which (a blue dot records, amber is paused, grey stopped) with the exchanges not recorded since. **Pause** (⌘⌥P, again to resume) stops recording while traffic still flows through the proxy; its menu pauses everything, only this session's, only browsers', or only what the filter matches, and can pause the rules too (else mocks, forwards and failures keep running). **Stop** (⌘⌥S, again to start) catches nothing at all: what is attached stays pointed at the proxy, which passes its traffic on untouched, without rules, so nothing fails. Pausing or stopping lets requests held at a breakpoint go on (or drops them, as Settings say), and none is held while unrecorded. The list marks each pause and stop where it was ("— paused 14:30–14:42 · 38 not recorded —"). Settings choose whether rules keep running while paused, and can pause on its own after some minutes of recording or some exchanges; those preferences are the user's on every host (`settings/traffic.json`, synced), while whether a host records now is its own.
- **Paused:** exchanges a breakpoint holds show on top, each to resume, change (status, headers, body) then resume, or drop. Its session shows as waiting for you meanwhile; a pause ends by itself after 5 minutes.
- **Rules:** the project's rules, first match wins: each on or off, edited in a form, moved up, deleted, with its hits.
- **Intercept:** what is attached, and what can be.
- **Compose:** a request written by hand, or from one caught, sent from the host through the rules or around them, the last ones kept.
- **Settings** (the gear): certificates not checked, an upstream proxy, names pointed at other addresses (as a hosts file), client certificates for servers that ask, how much the capture keeps, being told when an app's request fails or is answered 5xx, and making the CA anew (what trusted the old one must be attached again).
- The **…** menu saves the filter with the project (in `.enowx/traffic.json`) and picks a saved one, exports what the list shows as HAR, and imports a HAR file of this device. The bin beside the filter clears the list. The globe lists the domains seen, with their counts, to hide or show each.

### Filtering

The same language in the app, the command line and agents' tools (`intercept.ParseFilter`); conditions all hold, `-` negates one, other words search the URL, headers and bodies:

| Condition | Matches |
|---|---|
| `domain:example.com` | the host and its subdomains |
| `host:example.com`, `host:*.example.com` | that host; its subdomains |
| `path:/api/*`, `url:REGEXP`, `query:page=2` | the path (a prefix, or a glob), the URL, a query parameter |
| `method:POST`, `status:4xx`, `status:>=500`, `status:200-299` | method, status |
| `type:json\|html\|image\|ws\|grpc\|sse\|graphql\|tunnel` | the body's kind, or the exchange's |
| `header:content-type=json`, `header:x-trace` | a header's value contains, or the header is there |
| `body:"invalid token"`, `graphql:op=GetUser` | a body contains (decoded); a GraphQL operation |
| `size:>1mb`, `time:>500ms`, `since:10m` | size, duration, how recent |
| `source:ID`, `session:ID`, `is:mocked\|error\|paused\|pending\|pinned` | where from, and state |

## Rules

A project's rules live in its folder, `.enowx/traffic.json`, to commit and share; a source outside a project uses the host's (`~/.enowx-app/intercept/rules.json`). A rule matches on methods, the host (`*.example.com`, `.example.com`), the path (prefix, exact, glob or regular expression), query parameters, headers, the body (text or regular expression), JSON fields (`user.id = 7`), the source or the session, WebSocket upgrades; then:

- **Mock:** a status, headers and a body, typed or a file of the project, only the first N times if asked; for a WebSocket, messages sent once connected, or what the client sends echoed.
- **Forward** to another origin, the path kept, the Host header kept or not.
- **Transform** the request (headers set or removed) and the response (headers, the status, text replaced, JSON fields set or removed).
- **Fail:** answer 503, close or reset the connection, or never answer.
- **Pause** the request, the response, or both, at a breakpoint.
- With any of them, a **delay** and a **throttle** (bytes a second).

## For agents

- `enowx-app traffic list|show|watch|mock|breakpoint|forward|transform|fail|pass|rules|paused|resume|send|clear|export|import|attach|attached|detach|env|status|settings|ca|pin|filters`, and `pause [--source S]... [--filter F] [--rules keep|pause] [--drop]`, `resume` (no ID: record again), `stop`, `start`, `prefs`; `status` says whether it records, and `watch` says when recording is paused rather than wait in silence ([cli.md](cli.md#traffic)): everything the tool does, this session's traffic by default, secrets always masked.
- `enowx-app mcp` serves the same as MCP tools (`traffic_list`, `traffic_show`, `traffic_watch`, `traffic_mock`, `traffic_breakpoint`, `traffic_rules`, `traffic_rule_remove`, `traffic_send`, `traffic_export`, `traffic_attach`, `traffic_clear`, `traffic_pause`, `traffic_resume_recording`, `traffic_stop`, `traffic_start`, `traffic_status`), with the Browser's (`browser_*`), Android's (`android_*`) and the sessions' ([cli.md](cli.md)). **Intercept → enowX's tools for the agents** adds it to Claude Code, Codex, OpenCode and Gemini CLI on the host. Every agent is also told about the commands in its guide ([agents.md](agents.md#what-agents-are-told)).

## Secrets and limits

- `Authorization`, `Cookie`, `Set-Cookie`, headers, query parameters and JSON or form fields named like tokens, secrets, passwords and keys are masked in the list, in what goes to agents and in exports; the tool's **Show secrets** reveals them for one exchange.
- The capture lives in the daemon: the last 2,000 exchanges and 256 MB of bodies in memory by default (Settings), a body kept up to 16 MB, those past a megabyte in files of `~/.enowx-app/intercept/bodies/`; pinned ones stay when the rest is cleared. It goes with the daemon.
- The proxy listens on loopback only, and sources' credentials keep other users of a shared server out of the main port.

## Protocol

`traffic.status`, `traffic.ca`, `traffic.attach`, `traffic.detach`, `traffic.attached`, `traffic.env`, `traffic.list`, `traffic.get`, `traffic.body`, `traffic.messages`, `traffic.pin`, `traffic.clear`, `traffic.rules`, `traffic.rules.set`, `traffic.paused`, `traffic.resume`, `traffic.send`, `traffic.export` (a HAR read with `transfer.read`), `traffic.import`, `traffic.reverse`, `traffic.tunnel`, `traffic.mcp`, `traffic.settings`, `traffic.settings.set`, `traffic.capture`, `traffic.capture.set`, `traffic.capture.prefs`, `traffic.capture.prefs.set`; events `traffic.exchange` (a summary, a few times a second for a busy exchange), `traffic.paused`, `traffic.attached`, `traffic.capture` (a change of state at once, counts not recorded gathered) (`proto/traffic.go`, `proto/traffic_capture.go`).

## Not yet

- Sites a page in the Browser tool calls other than its dev server: MyGo's web view has no proxy setting.
- Apps that don't read the system's proxy or trust (Flutter's Dart HTTP client, apps pinning certificates): they need a debug hook of their own.
- WebKitGTK windows on a server's display (Tauri on Linux) read the proxy variables but not the CA bundle.

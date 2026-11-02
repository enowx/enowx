# Credentials

The **Credentials** page (More › Credentials) keeps the user's logins to platforms (GitHub and other git hosts, cloud providers, edge and hosting, databases, registries, AI APIs, SSH keys, bots, or any key, login, env file or certificate) in one vault. Sessions on any host use them without logging in again on each machine, and agents use them by name without seeing a value.

## The vault

The vault belongs to this device. The first visit asks for a master password, twice, to make it. Its credentials are sealed with AES-256-GCM under a random master key, and the master key is sealed under a key derived from the master password with scrypt (`core/vault`, `<enowX dir>/vault/vault.json`). There is no way back in without the password.

- **Remember on this device** keeps the master key in the system's keychain (macOS Keychain, Secret Service on Linux, Credential Manager on Windows), so the vault opens without typing the password. Otherwise the password unlocks it each time.
- The vault locks after 15 minutes unused: the key leaves memory. The lock button in the page's header locks it at once.
- **Change master password** re-seals the master key; the credentials and the hosts' copies stay as they are.

Hosts keep a copy of the ciphertext and an index without values (`vault/index.json`: names, platforms, accounts, the variables each sets, statuses, the last four characters of a value). The app gives each host it connects to, shown or watched, the newer copy, and takes a host's when this device has none (`vault.get`, `vault.put`, `vault.changed`). Nothing a host keeps opens without the key, which never leaves the device.

## Credentials

**Add credential** picks a platform, then asks for its values (a token, a key and its ID, a username and password, a connection string, a service account's JSON, a private key) and:

- a name, used in commands (`gh-work`), and the account it logs in as;
- when it expires, to be reminded a week before (a notification once a day, and a notice on the page);
- **May be used without asking on**: the hosts it goes to without asking; **Only these hosts** refuses every other one without asking;
- **In every session**: its variables are in each new session of the hosts it may be used on;
- **Only for projects**: folders (or their names) whose sessions may have it, as a work account for work projects.

Each row shows the platform's mark, the end of its value, whether it works (**Test** asks the platform: GitHub's user, Cloudflare's token check, OpenAI's models…; platforms without such a check say so), where it may be used and when it expires. Its menu makes it the platform's default (picked when none is named), revokes it (no host may use it until allowed again) or removes it. Editing values rotates it.

**Import from HOST** finds the logins the connected host's tools keep and copies those picked into the vault: `gh` (its `hosts.yml`, or `gh auth token`), `~/.git-credentials`, `~/.netrc`, `~/.aws/credentials` with their regions, `wrangler`, `~/.docker/config.json`, `.npmrc`, `.pypirc`, cargo, Fly, doctl, hcloud, Supabase, Vercel, Railway, Netlify, and SSH keys without a passphrase (`vault.scan`). The tools keep theirs. A platform whose own command line logs in (`gh auth login`, `wrangler login`, `aws configure`, `docker login`, …) offers **Log in with** it in a new session of the host, to import afterwards.

A new credential's form starts with the quick ways, no command line needed: **Already on <host>** lists the logins of that platform the connected host's tools keep (the same as Import finds), each to **Use**; **Open the token page** opens the page where the platform's tokens are made, and a token copied there afterwards (one that looks like the platform's: `ghp_…`, `sk-ant-…`, …, or a long single word for the others) is offered to **Use it**; then signing in in the browser where the platform has it; and last **Log in with <its command line> instead**, which runs it in a session. A new credential takes a free name (`github-2`) when its platform's is taken.

**Sign in in the browser** (GitHub): no token to make; the form asks GitHub for a code, opens its page, and once the user types the code there, signed in, the token fills in to add. GitHub's device flow takes an OAuth app of the user's own (github.com/settings/developers, New OAuth App, Enable Device Flow): enowX has none to sign in through, so the form asks for its client ID once and keeps it (it isn't secret). The token goes from GitHub to the app alone.

## Lending

A host asks the app for a credential when one of its sessions needs it (`vault.ask` → `vault.request` → `vault.reply`). The app answers at once on a host the credential may be used on; otherwise it asks the user (a notification when the window isn't in front): **Allow once**, **Always on HOST** (the host is added to the credential's), or **Refuse**. A locked vault asks for the master password first. Adding or removing a credential for an agent asks the user too.

What the app lends goes to that session alone:

- **Variables.** Each platform's credential sets the variables its tools read (`GITHUB_TOKEN`/`GH_TOKEN`, `AWS_ACCESS_KEY_ID`…, `CLOUDFLARE_API_TOKEN`, `DATABASE_URL`, `OPENAI_API_KEY`…; `creds platforms` lists them). A service account's JSON or a certificate goes in a file of a folder only the user reads, its variable holding the path, removed with the session. A session gets the credentials New session picked, those **In every session**, and those `creds env` lends it; its tabs get its own.
- **git** asks enowX as its credential helper (`GIT_CONFIG_*` in the session names `enowx creds git-helper`) for the server it pushes to or pulls from: GitHub, GitLab, Bitbucket, a Gitea's server. A server the vault has no login for goes on to git's other helpers.
- **docker** asks it as `docker-credential-enowx` for the registries the vault has logins to (Docker Hub, GHCR, GitLab's), the user's own docker settings kept for the rest.
- **ssh** talks to an agent of the session's (`SSH_AUTH_SOCK`) that lists the vault's SSH keys and has the app sign: a private key never reaches the host. (Not on Windows yet.)

The daemon keeps what it was lent in memory for the session, masks it in Traffic wherever it shows (headers, URLs, bodies, base64 included) and in what is read of any session's screen or conversation (`session read`, the chat view), and forgets it with the session. Values reach a session as variables and files, never typed into its shell, so its history doesn't hold them, and the daemon logs none. Each use goes in the host's record, `vault/audit.jsonl`, without values: which credential, for what, by which session, allowed or refused; the page shows the connected host's last uses, `creds audit` prints them. An agent's model API can take its key from the vault (**From the vault** in Settings' model API form).

## Keys typed for one host

A judge's key, RAG's embedder's and a PostgreSQL URL can be typed for one host alone (`rag-keys.json`, `decide-keys.json`), which never sync. The daemon reads those files once, when it starts: a key typed in the app counts at once, one copied into a file by hand only after the daemon restarts. **Keep them in the vault**, in Settings → RAG and Decisions, moves them (`host.keysToVault`): the host hands each to the app, which asks the user to keep it; only once the vault lends the same value back do the settings name the credential and the typed copy go. If anything fails, that key stays as it was, and the reason is shown. The settings' fields are the ones hosts of older versions already read (`keyCred`, `postgresCred`, a judge's `cred`). Other hosts then read it from the vault, the app asking the user the first time each uses it (**May be used without asking on** starts with the host it came from alone), and only while the app is open or the daemon holds it. Agents can't call this through control.

## For agents

`enowx-app creds` (and the MCP tools `creds_*`) uses the vault by name; no command prints a value:

| Command | What it does |
|---|---|
| `creds [list] [--json]` | What the vault holds, without values. |
| `creds use [NAME...] -- COMMAND` | Runs COMMAND with the credentials' variables (no NAME: the session's); what it prints has their values masked. |
| `creds env NAME...` | Lends credentials to this session: programs it starts afterwards, and `creds use` without a name, get them. |
| `creds add NAME --platform P [--account A] [--value TEXT\|-] [--field KEY=VALUE]` | Keeps a new credential; the user confirms. |
| `creds test NAME`, `creds rm NAME` | Checks one with its platform; removes one (the user confirms). |
| `creds login PLATFORM` | Opens the page on adding the platform's, for the user. |
| `creds audit [--limit N]`, `creds platforms [QUERY]` | The host's record of uses; the platforms and the variables each sets. |

The guide tells agents never to print, echo or write a value, and to have the user add a secret in the app rather than paste it into the chat.

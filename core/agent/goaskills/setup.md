# Setting enowX up

Do what the user asks of this, one area at a time. Read a setting before changing it, say what you changed, and stop for anything below marked **the user**. Never ask for a secret in the chat, nor write one in a command: a key is either the name of a credential in the vault, or typed in the app.

Settings live per host unless said otherwise: name which host, and if several match what the user said ("the VPS"), ask which ("$ENOWX_BIN" sessions lists the hosts reached).

## Credentials (every key below)
    "$ENOWX_BIN" creds                       # what the vault holds: names, no values
    "$ENOWX_BIN" creds login PLATFORM        # asks the user to log in, in the app
**The user:** making the vault, unlocking it, adding a key. Each host asks them the first time it borrows one.

## RAG: finding a project's code by meaning
    "$ENOWX_BIN" rag settings                # the embedder and store, whether a key is set
    "$ENOWX_BIN" rag set embedder=voyage model=voyage-4-lite key_cred=NAME
    "$ENOWX_BIN" rag status --project P      # on or off, files, chunks, tokens
    "$ENOWX_BIN" rag project on agreed --project P  # only once the user agreed: its files go to the embedder
    "$ENOWX_BIN" rag plan --project P        # what an index would send, before it does
    "$ENOWX_BIN" rag index --project P
1. Ask which embedder (Voyage, an OpenAI-compatible server, a local TEI). 2. **The user:** the key, in the vault, or typed in Settings → RAG on that host. 3. Set it, check `rag settings` says a key is there. 4. **The user:** agreeing to one project's files going to that embedder, each project on its own. 5. Turn it on; it indexes by itself as files change. PostgreSQL's URL holds a password: **the user** types it in Settings → RAG. A key copied by hand into a host's file needs its daemon restarted, so don't: use the app.

## Decisions: the judge
    "$ENOWX_BIN" decide settings             # uses, judges, their order
    "$ENOWX_BIN" decide status               # the judges found on the host
    "$ENOWX_BIN" decide test                 # asks each a fixed question
    "$ENOWX_BIN" decide use approve on --sure 0.9 --calls 0
Uses: state, approve, trim, loop, route and others `decide settings` lists. Calls 0 is no limit. Agents' one-shot commands and local servers are found with no key; **the user** types a model API's key in Settings → Decisions. Turning `approve` on lets a model allow commands: say so before you do.

## Agents, accounts, configs
    "$ENOWX_BIN" agents --all                # which are installed
    "$ENOWX_BIN" configs                     # each agent's configs, and how each takes one or why it can't
    "$ENOWX_BIN" configs add claude NAME --url URL --key-cred CRED --model M
    "$ENOWX_BIN" accounts                    # accounts, limits, fallback
    "$ENOWX_BIN" accounts priority claude A B | fallback claude --on
**The user:** signing in to another account, from the app. Don't install an agent unasked.

## Projects, hosts, sync
    "$ENOWX_BIN" project list | add PATH | clone URL --dir DIR
    "$ENOWX_BIN" project autopush            # see "help autosync"
    "$ENOWX_BIN" sync status | now | conflicts
A host is added and connected in the app, by **the user** (Hosts): there's no command yet.

## Features, hooks, notifications, rules
    "$ENOWX_BIN" features                    # what agents get, per scope
    "$ENOWX_BIN" hooks                       # hooks add proposes one, for the user to allow
    "$ENOWX_BIN" notify status | set --on --volume 60 --quiet 22:00-07:00
    "$ENOWX_BIN" goa rules                   # your rules; only add to them
    "$ENOWX_BIN" settings                    # New session's defaults and other app settings, where the app is open

## What you can't change: send the user there
Appearance and themes; the floating control; terminal fonts; adding, connecting or removing a host; Discord; the keyboard shortcuts; the cloud's sign-in; a team's members. Say where it is in the app, and that you can't do it.

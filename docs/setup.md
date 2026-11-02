# Setting up a server like this device

**Set up…** on the connected server's card (Hosts page), or **Set up this host** in the command palette, brings a server to what this device has, so its agents work as they do here.

## What it brings

| Step | How | Done when |
|---|---|---|
| System packages | git, curl, unzip, a compiler, with the server's package manager (apt-get, dnf, zypper, pacman, apk; bash too on Alpine, whose agents' installers need it), through sudo (doas where there is no sudo) unless enowX runs as root | git, curl, unzip and make are there |
| mise | `curl https://mise.run \| sh`, its shims put on PATH in `~/.profile` (which the daemon's login shell reads), `~/.bashrc`, `~/.zshrc` | it is there |
| Toolchains | `mise use -g <tool>@<version>` for Node, Bun, Deno, pnpm, Python, Go, Rust, Java at this device's versions | the server has the same to the minor, or a newer one (macOS's own Python isn't worth going back to) |
| Git user | `git config --global user.name/email` | they match |
| GitHub CLI | gh through mise, then `gh auth login --web`: a code to enter in a browser. **No token leaves this device.** | gh is there / `gh auth status` passes |
| Agents | each agent's own installer | its command is there |
| Agent sign-in (opt-in, in `enowx.toml`) | the agent's sign-in, a code to enter in a browser | — |
| Xpra (when this device has it) | as **Install Xpra** does (`core/display`) | it is there |

Skills, subagents, commands, MCP servers and settings go with **Sync environment** ([sync.md](sync.md)), which the page links to; projects with **Copy to another host** ([transfer.md](transfer.md)).

## How

1. The app scans this device (`core/setup`, in the app) and the server (`setup.scan`, on its daemon): OS, package manager, the programs above with their versions, git's user, gh's sign-in. Programs are looked for on the daemon's PATH, taken from a login shell, so version managers count.
2. This device's scan makes the **manifest**, which **Save enowx.toml…** writes and **Load enowx.toml…** reads (to set up servers alike, or keep it in a repo).
3. The plan lists each step with what the server has; those it lacks are picked. **Show commands** is the dry run: every step's shell, before anything runs.
4. **Set up** runs the steps picked as one script in a session of the server, whose terminal shows them and takes what they ask (a sudo password, a sign-in). Each step runs in a subshell that checks before it changes anything, so running again is safe; a failed step doesn't stop the next. Each records its state in `~/.enowx-app/setup/status`, which the page reads every 2 seconds to mark it running, done or failed.

## Distributions

Checked by `scripts/test-distros.sh`, which runs each in a container (docker): the daemon starts, a session's terminal works, the system packages and git steps install what they say, and a second plan has nothing left to do. On Ubuntu 24.04, Debian 12, Fedora 41, Arch Linux (amd64; emulated on arm64 machines, where pacman's download sandbox is turned off for the test), Alpine 3.21 and openSUSE Tumbleweed.

Before installing the daemon, the app readies the server (`hostconn/prepare.sh`), as root where needed (root, else sudo or doas without a password; it says what to run otherwise): CA certificates where there are none (minimal images and containers), which terminals need to download their library over HTTPS; and on musl systems (Alpine) the glibc compatibility layer (`gcompat`, `libstdc++`), since that library is built for glibc.

## Drift

Connecting to a server scans both ends in the background; its card then says how many steps it lacks ("Set up · 3 to bring"), when this device got a new toolchain or agent since, say.

## Tailscale inside the app

By default enowX reaches tailnet servers through the `tailscale` command the device has (`core/tailnet`). A host's "Through enowX's own tailnet" option instead dials its SSH over a tailnet device of the app's own (`transport/owntailnet`), so the server is reached without Tailscale on this machine: it signs in once from a browser and keeps its state in `~/.enowx-app/tsnet`. `ENOWX_TAILNET_CONTROL` points it at a self-hosted coordination server (Headscale) instead of Tailscale's. It is part of the app only; the server daemon doesn't carry it.

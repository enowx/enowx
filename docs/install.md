# Getting started

## Install

Download the installers from [GitHub Releases](https://github.com/enowdev/enowx/releases/latest):

- **macOS:** the `.dmg`, then drag enowX to Applications. It isn't notarized yet, so macOS blocks the first launch: right-click it → Open, or `xattr -dr com.apple.quarantine /Applications/enowX.app`.
- **Windows:** the `Setup` `.exe`. It isn't signed yet: in SmartScreen's warning, More info → Run anyway. The daemon, which keeps sessions running when the window is closed, runs as a copy of the app in `%USERPROFILE%\.enowx-app\daemon\`, so installing over the app works while it runs. Before 0.2.1 it ran as the app itself: installing over one of those says "Error opening file for writing" until enowX is ended in Task Manager (`taskkill /F /IM enowX.exe`), then Retry.
- **Linux:** one line installs it for the user in `~/.local`, where it updates itself (or use the `.deb`):

```sh
curl -fsSL https://github.com/enowdev/enowx/releases/latest/download/install.sh | sh
```

Or build it yourself. **From source** (Go 1.27+ and git):

```sh
make build
./bin/enowx
```

**Installers** (`make package`, see `scripts/package.sh`): every platform's installers in `dist/<version>/`, built on this machine and published nowhere:

| Platform | Installer | Installs to |
|---|---|---|
| macOS | `enowX <version>.dmg` (universal): drag enowX to Applications. Signed ad hoc unless `cmd/enowx/mygo.json` names a Developer ID, so Gatekeeper asks the first time (right-click → Open). | `/Applications/enowX.app` |
| Linux | `enowx_<version>_<arch>.deb` (`sudo apt install ./enowx_….deb`), or `enowx-<version>-linux-<arch>.tar.gz` with `install-linux-<arch>.sh`, which installs for the user in `~/.local`, where the app updates itself | `/opt/enowx` (deb) |
| Windows | `enowX Setup <version> <arch>.exe` | the user's programs |

Settings → App → **Command line** links the `enowx-app` command (see [cli.md](cli.md)) into `~/.local/bin`.

## First session

1. Open a folder (⌘O / Ctrl+O, or the folder button over the sessions). It becomes a project.
2. **New** (⌘N): pick an agent (those installed come first; others can be installed from there), a branch (the project folder, a new branch in a worktree of its own, or an existing branch), and a first prompt.
3. The session shows as a terminal, and for Claude Code and Codex as a chat too. The side panel has its changes, files, checks, ports and a browser for what it serves.
4. Review the changes (Changes tool): comment lines to send back, commit, push, open a pull request, merge.

More: [agents.md](agents.md), [chat-view.md](chat-view.md), [review.md](review.md), [side-panel.md](side-panel.md), [pipelines.md](pipelines.md).

## A server

1. Hosts → **Add VPS** with `user@host` (a key from `~/.ssh` or a password kept in the keychain), or **Create a server…** at Hetzner, DigitalOcean or Vultr with your own token ([vps.md](vps.md)).
2. Connecting installs enowX's daemon there and offers to run it as a user of its own rather than root. Sessions keep running on the server while the app is closed.
3. **Set up…** brings the server what this device has: toolchains, git, gh, agents, their sign-in ([setup.md](setup.md)); **Sync** brings skills, MCP servers and instructions ([sync.md](sync.md)).
4. Ports the server's sessions listen on are forwarded to this device and shown in the Browser tool; desktop apps run on a display of their own ([preview.md](preview.md)).

Supported servers: Linux on amd64 or arm64; Ubuntu, Debian, Fedora, Arch, Alpine and openSUSE are checked ([setup.md](setup.md#distributions)).

# The daemon's lifetime

Each host runs one daemon (`enowxd`, or `enowx daemon` on a desktop), which owns its sessions. The app connects to it and can come and go.

## On this device

Quitting the app leaves the daemon running: agents keep working, automations run, and the next launch finds every session as it was. To have them stop:

- **Quit and stop all agents**: in the app menu on macOS (⌥⌘Q) and in the command palette everywhere.
- **Settings → Stop this device's agents when enowX quits**, off by default.
- **Stop all agents** in the command palette stops the connected host's agents without quitting.

Stopping an agent ends its program the way hibernation does (`daemon.stopAgents`): the session stays, showing what it showed, and its agent comes back on its conversation when the session is shown. Plain terminals keep running.

## On a server

Connecting installs the daemon at `~/.enowx-app/bin/enowxd` and starts it through `enowxd bridge`. Where the server runs systemd, it starts as the user's service (`~/.config/systemd/user/enowxd.service`, `core/userservice`; a daemon of another state folder, `ENOWX_HOME` set elsewhere, gets a unit of its own, `enowxd-<hash>.service`, and never replaces the user's), and the app turns on lingering for that user while preparing the server (`hostconn/linger.sh`, as root, or with sudo or doas without a password). The daemon then starts again after a reboot without anyone connecting, its sessions come back, and automations keep running.

Where there is no systemd (Alpine, containers) or lingering couldn't be turned on, the daemon runs as a process as before: it lives until the server reboots, and the next connection starts it. Why a service didn't start is written to `~/.enowx-app/enowxd.log`.

`scripts/test-service.sh` checks it in an Ubuntu container booted with systemd.

## When the app freezes

The app's main thread is watched: asked to answer every second, it leaves a report in `watchdog/` under the data folder when it takes over 0.7 seconds (at most once a minute: lag) or 3 seconds (a stall). A report holds every goroutine's stack, whether a frame was being built, and what was on screen as of its last answer (the page, session, agent and style, never what's in them). The newest 30 stay. `watchdog/app.log` and `watchdog/daemon.log` get a line every 5 minutes with the Go heap, its objects, goroutines and slow frames, so growth over hours shows. A process crashed or killed from outside leaves no report.

## What sessions leave behind

Closing a session hangs up its terminal, which ends its program and foreground, but an agent's servers and helpers in process groups of their own don't hear it. So the daemon writes down each session's process tree every 15 seconds and just before a stop (`session-procs.json`), and on a close, a hibernation or a switch of account asks what's still there to end, then forces it 3 seconds later. Each process goes by its PID and start time together, so a PID used again is left alone, and only processes on that list are ever stopped: never one no session started, the daemon itself or what started it.

A process a session left and nobody looks after (its session closed or ended, or it moved out from under the session) shows in Resource usage under *Left running*, with Stop. What a daemon that crashed left behind ends when the next one starts.

On Windows none of this runs yet: enowX can't tell which process belongs to which session there, so only the console's own closing ends them. Environment tags (`reap`) still apply on Linux, where they also catch what left a tree before it was seen.

## Its folder

enowX keeps its state in `~/.enowx-app` (`transport.HomeName`), on this device and on servers; `ENOWX_HOME` names another (`make dev` uses `~/.enowx-dev`). Its command line is `enowx-app`, linked at `~/.local/bin/enowx-app`. Before, both were `~/.enowx` and `enowx`, which another app uses too: the first start of a newer app or daemon stops the daemon there and moves enowX's own files across (hosts, projects, sessions and their history, worktrees, settings), repairing git's links to the worktrees, and leaves the rest of `~/.enowx` alone.

## Far and flaky connections

What makes a far server feel close, built in rather than through mosh (which serves one shell over UDP, and needs `mosh-server` and an open port on the server):

- **Keepalives.** Each SSH connection sends one every 15 seconds (`transport/ssh.go`), so routers and NATs keep it while idle; three unanswered in a row close it, so a connection that died with the network is noticed at once.
- **Connecting again by itself.** When a host's connection drops, the app tries again after 1 second and longer each time, up to 30, without asking anything, until the host answers or the user goes elsewhere; the view comes back as it was, the sessions having kept running.
- **Resizes once settled.** A window growing or zooming resizes its sessions once the size held for 120 ms, so their programs redraw once.

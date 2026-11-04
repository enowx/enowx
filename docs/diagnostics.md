# Diagnostics and logs

What explains a problem later, where it is, how to read it, and how much disk it may take. Logs hold names, IDs, sizes and errors, never secrets, tokens, prompts, or what a session or a canvas holds: only strings, errors, numbers, booleans and durations are written (anything else is written as `?`), and whatever looks like a secret is masked before it reaches the file (`core/diag/scrub.go`) and again when a bundle reads it. One exception to the first pass: a crash's stack is written directly by the Go runtime, so the bundle's pass is what masks it.

## Getting them

- **In the app:** command palette → **Save diagnostics for a bug report**. It writes a zip to Downloads: this device's logs and, for a remote host, that host's (`daemon.diagnostics`).
- **On any host:** `enowx-app diagnostics` prints the recent connection events of its app and daemon, merged by time. `--all` shows the end of both logs, `--lines N` how many, and `--bundle FILE` writes the zip. It reads the files itself, so it works with the daemon down.

A bundle holds the logs below and the five newest freeze reports, 6 MiB of each at most (the end). It doesn't hold the whole outputs in the temporary folder: those are commands' output.

## The files

In the data folder (`~/.enowx-app`, `~/.enowx-dev` in development):

| File | Written by | Holds | Limit |
|---|---|---|---|
| `app.log` | the app | see below | 5 MiB, then rotated |
| `enowxd.log` | the daemon | see below, and its logger's lines | 5 MiB, then rotated |
| `app.log.1` … `.3`, `enowxd.log.1` … `.3` | rotation | older lines; `.1` is the most recent | 3 copies; 30 days; 24 MiB together |
| `watchdog/app.log`, `watchdog/daemon.log` | the watchdog | memory every 5 minutes, slow frames, marks | 1 MiB, then its older half goes |
| `watchdog/<app\|daemon>-<lag\|stall>-<time>.txt` | the watchdog | stacks and what was on screen, at a freeze | 30 files; 30 days; 64 MiB together; the newest stays |
| `enowx-trim/<time>-<pid>.log`, in the system's temporary folder | `decide trim` | the whole output of a command an agent ran, for it to read | 7 days; 100 MiB together; the newest and those under an hour old stay |

A daemon running as a service used to log only to the service manager's journal, leaving `enowxd.log` stale. It now writes both.

Lines read `TIME EVENT key=value…`, with the same connection's `conn=` on both sides.

### Events

| Event | On | Says |
|---|---|---|
| `app.start`, `daemon.start` | each | version, build, pid; `lastRun=clean\|unclean\|first` (unclean: it crashed or was killed) |
| `exit.clean` | each | it ended as it should |
| crash stack | each | a panic's stack, written as the program dies |
| `conn.connecting`, `conn.connected`, `conn.failed`, `conn.retry`, `conn.reconnected`, `host.switched` | app | host, connection, how long it took, the try and its wait, the error |
| `conn.open`, `conn.closed` | both | `conn`, how long it lasted, `by=client\|daemon\|connection`, why. A command's (`enowx-app …`) are logged only if they fail |
| `proto.too_large` | daemon | a frame refused: which, and its size ([protocol.md](protocol.md)) |
| `proto.parts` | daemon | a frame sent as parts, at most once in 10 s per kind |
| `proto.panic` | daemon | the method a panic happened in, just before the crash stack |
| `rpc.slow` | both | a call over 2 s, or 4 MiB either way: method, time, sizes; a method at most once in 30 s |
| `page.change` | app | from which page to which |
| `canvas.load` | app | which canvas, its designs, its HTML's size, how long |
| `webview.created`, `webview.destroyed`, `webview.render` | app | the Canvas page's web view; a hidden one for a render (at most once in 10 s) |
| `frame.slow` | app | a frame over 250 ms, with what was on screen, at most once in 30 s |
| `logs.swept` | both | files and bytes removed |
| `log.dropped` | both | lines lost because the writer fell behind |

In `watchdog/*.log`, `mark memory jump` says the Go heap grew by 256 MiB and by half within 15 s, with the last events and what was on screen.

## Cleanup

Rotation copies a log's end to `.1` and empties the file in place: others hold it open to append (the daemon's own output, when the app started it). A log older versions let grow past the limit is cut down this way when the program next starts, its last 5 MiB kept as `.1`.

`diag.Sweep` runs when the app or the daemon starts and every 6 hours, on the limits above. **Clean cache**'s **Logs** kind (`enowx-app cache`) offers what else can go, with what it would free and the usual confirmation: rotated copies past the most recent, those outputs at least an hour old, and freeze reports older than the age chosen.

Whatever runs, the current logs, each one's `.1`, and the newest freeze report and output stay. Only exact names are removed (`NAME.2`, `NAME.3`) or the one shape each writer gives its files, in the one folder it writes; links, other names and other folders are left alone (`core/diag/clean.go`).

A bundle holds the logs while it reads them (`diagnostics.hold` in the data folder, ignored after 10 minutes should it die): nothing rotates or is removed meanwhile, in any process.

Logging never blocks the caller or the UI: lines go through a queue to a goroutine that owns the file, and are counted if the queue is full.

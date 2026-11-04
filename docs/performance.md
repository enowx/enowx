# Performance: what the app and the daemon hold and use

The owner's target is the app and the daemon each under 100 MB at idle. This says what they cost, by area, what has been done, and the path that is left, each number marked by where it comes from. Figures here were measured on Linux; macOS's are the owner's to take with `enowx-app profile`, and none are claimed for it.

## Measuring

`enowx-app profile [--cpu SECONDS] [--json] [--out DIR]` measures the running daemon and the app attached to it, changing nothing and restarting nothing (`daemon.profile`). It also goes in a diagnostics bundle as `profile.txt`. It says:

- **memory by the system's count.** On Linux: RSS, PSS, private dirty, private clean, shared clean, anonymous, swap, peak RSS, and the mappings by file. On macOS: RSS, and physical footprint with its regions, read from `vmmap --summary`, which stops the process for as long as it reads. Elsewhere: RSS.
- **memory by Go's count.** Live heap, what spans hold, what is free but still held, what has been returned, stacks, and `sys`.
- **which code holds the heap, and which allocates the most**, sampled, and the goroutines by code.
- **what the program keeps**: terminals, chat views, conversations, pictures, sessions.
- **what sessions' programs take**, by program with how many of each, MCP servers among them.
- **CPU** over `--cpu N` seconds, and with `--out` pprof files for `go tool pprof`.

`scripts/profile-scenario.sh BIN HOME` runs the same use on an isolated app and daemon on Xvfb and prints a line after each part. It starts them with `ENOWX_DEV=1`, whose driver (`ui/devdriver.go`) walks the pages without drawing anything off screen.

### Reading the numbers

- **RSS overstates what a program costs.** It counts clean file pages (code and libraries) that the system can drop at will, and counts shared ones in full. On Linux an empty window's RSS is 105 MB, of which 16 MB is its own dirty memory. **Private dirty is the nearest thing Linux has to macOS's footprint**, which also leaves out clean file pages, so that is the column to compare with the target. They aren't the same: footprint also counts compressed memory and GPU surfaces.
- **`sys` in Go's numbers only rises, and it says little.** It is the address space Go ever took; memory given back stays counted in it (`released`). What Go holds from the system is `sys` minus `released`. A `sys` of 5,509 MB says that the heap once reached that far, not that it was still in use.
- **`__TEXT` of 411 MB is address space, mostly clean.** Only pages touched are resident, and clean pages don't count in the footprint. The Linux builds measured here don't embed the servers' daemons, as a release does; that adds address space, and how much is resident on a Mac wasn't measured.
- **CPU here is pessimistic for the app.** Under Xvfb it draws in software (`present` took 70 to 200 ms a frame, allocating about 11 MB each), which a GPU does for far less. Frame counts and the daemon's CPU carry over; the app's percentages don't.
- **Host and load.** The host runs other sessions, with a load between 3 and 100 during these runs. Memory held up; CPU was compared only between runs at the same time.

## The floor

Measured on Linux x86-64 under Xvfb with llvmpipe, 25 to 60 s after starting, in MB. PSS varies with what else shares a binary, so it is the least comparable column.

| What | RSS | PSS | Private dirty | Clean own | Shared clean |
|---|---:|---:|---:|---:|---:|
| Go program that sleeps | 1.9 | 1.9 | 0.7 | 1.2 | 0 |
| Empty MyGo window | 104.7 | 68.2 | 16.2 | 38.7 | 45.8 |
| v0.1.0 app, first public release, no sessions | 141.5 | 66.6 | 26.0 | 1.7 | 105.9 |
| v0.6.2 app, shell with no sessions or pages | 197 | 125 | 64 | 30 | 99 |
| This branch's app, the same | 190 | 129 | 56 | 36 | 94 |
| v0.1.0 `enowxd`, no sessions | 13.5 | 12.0 | 2.1 | 9.9 | 1.5 |
| This branch's `enowxd`, no sessions | 39.5 | 37.7 | 12.2 | 25.5 | 1.8 |
| v0.1.0 `enowx daemon`, no sessions | 39.6 | 23.6 | 8.8 | 0.3 | 30.6 |
| v0.6.2 `enowx daemon`, no sessions | 69 | 36 | 19 | 1 | 48 |
| This branch's `enowx daemon`, the same | 65 | 40 | 16 | 1 | 48 |

The empty window's RSS is mostly libraries: 49 MB of libLLVM and 9 MB of the Mesa driver, of which 6.5 MB is dirty. A Mac has neither. **So neither program is at its floor because of MyGo or Go: by private dirty, both are already well under 100 MB until they hold something.**

Per thing, on this branch:

| What | App, private dirty | Daemon, private dirty |
|---|---:|---:|
| An idle session | about 0.05 MB (no terminal until it shows) | 0.55 MB |
| A session's full scrollback (10 MiB of its text) | about 10 to 13 MB while its terminal is attached, 0 otherwise | about 13 MB, outside Go, for as long as the session lasts |
| A small canvas (3 designs, 20 KB each) | 2.6 MB once made | none measurable |

Before lazy terminals, a full scrollback cost the app the same 10 to 13 MB for every session on the connected host, shown or not.

## Where it goes, by area

Same use each time: 3 agents with a stand-in program and a 30 MB conversation each, 20 terminal sessions, 5 of them with a full scrollback, 5 canvases of 6 designs with about 2,000 elements each, all pages walked, 3 chats and 6 terminals shown, then idle on a canvas for 3 minutes. Private dirty in MB.

| Area | Who holds it | v0.6.2 | This branch | Left, and what would move it |
|---|---|---:|---:|---|
| Terminals | app, native | 10 to 13 per session attached, all of them | Those shown, and the 3 last shown | A smaller scrollback, or fewer kept |
| Terminals | daemon, native | About 13 per full scrollback | The same | **The main cost left in the daemon:** a smaller scrollback |
| A canvas's layers | app, Go heap | 100 to 150 at 2,000 elements (`widget.Running`, `layerRow`) | About 10: the first 200, the rest on asking | - |
| Conversations | daemon, Go heap | About 28 per 30 MB of conversation shown, or of **any agent at work** | Only those a chat shows, for 2 minutes after | - |
| Canvas pictures | app, Go heap | Up to 16 per picture, all kept for the open canvas | 64 at most | By construction and a unit test. Not measured in a real run: no picture was kept in these runs, as `canvas.render` didn't return one under Xvfb |
| Go's slack | both, Go heap | About the live heap again, plus what a burst left | Half the live heap again, returned within 30 s when over 32 MiB | A hard limit, at a cost in CPU |
| Libraries and code | both, clean | 50 to 130 of RSS, none of private dirty | The same | A separate daemon binary on desktops: 27 of RSS, none of footprint |
| Package start-up | both, Go heap | A few MB (`iconlib`, `chroma`, regexps compiled at start) | The same | Not worth it against the rest |

What sessions' programs take comes on top, and is usually the largest part of a host's memory: see [MCP servers](#mcp-servers).

## What was done, with numbers

Each row is a commit's effect, measured with the scenario above on builds of the commits in order, in the same conditions. Runs differ by 5 to 10 MB from noise alone, so a smaller change isn't evidence. "Canvas page" is right after opening that page; "3 min" is idle afterwards.

| Step | Commit | App, canvas page | App, 3 min | Daemon, 3 min | Other measure |
|---|---|---:|---:|---:|---|
| v0.6.2 with `profile` | `a9b707b` | 289 | 351 | 173 | |
| Terminals attach when shown, and go when not | `9ee584b` | 296 | 366 | 167 | App with 5 full scrollbacks, none shown: 126 → 65 |
| A long page's layers on asking | `052c8dd` | 121 | 200 | 168 | Go heap on that page: 200 → 48 MB |
| Conversations let go with no chat asking | `fdb5d36` | 127 | 170 | 171 | |
| Memory returned, limit on the heap | `92fb3b6` | 126 | 164 | 178 | No clear change by itself: see below |
| Restart plan: one look, and only when behind; watches; ports; own usage; conversation found without reading it | `a778438` to `1b5ae63` | 116 | 162 | 137 | Daemon's CPU with 20 idle sessions, see below |
| No frame for looks that change nothing | `de6c1c6` | 115 | 160 | 169 | Frames, see below |
| GOGC 50 | `95c3681` | 100 | 137 | 146 | CPU the same: app 74.1% and 73.2%, daemon 5.3% and 5.2% |
| Open canvas asks without frames | `4bee637` | | | | App's CPU on an idle canvas page, 5 small canvases: 59.3% → 14.2% |
| A chat polls only while on screen | `abcf5fe` | | | | Conversations held in the daemon, 4 minutes after 3 chats left the screen: 2 → 0, and with that the daemon at 3 min: 145 → 99 |
| This branch, by `scripts/profile-scenario.sh` | | 101 | 168 | 99 | Daemon: Go heap 12 MB, of which conversations none |

The daemon's 137 in one row and 169 in the next is noise of the kind above, from when a collection last ran: 62 MB had just been returned in the first. The last row's app has its canvas open with 2,000-element designs, its web view in the process, and software drawing's buffers.

**Idle CPU**, % of a core, the two builds running at the same moment, under software drawing:

| | v0.6.2 | This branch |
|---|---:|---:|
| App, empty, 30 s | 16.0 | 6.7 |
| Daemon, empty, 30 s | 1.7 | 0.9 |
| App, 20 idle sessions, 60 s | 19.0 | 9.8 |
| Frames in those 60 s | 41 | 20 |
| Daemon, 20 idle sessions, 60 s | 8.0 | 2.7 |
| App, Canvas page open, 5 small canvases, 20 s | 65.9 | 14.2 |
| Daemon, the same | 4.7 | 2.9 |
| App's private dirty, Canvas page open | 112 | 83 |

### Causes worth knowing

- **Every app asked each host for a restart plan every 30 s**, daemon behind or not, and a plan looked at every process **once per terminal session**: half of what an idle daemon spent on Linux, and on macOS a `ps -A` per session. That burst every 30 s fits the bursty CPU the owner sees, though the Mac itself wasn't measured.
- **Saving a session read its whole conversation.** Every 20 s while a screen changed, `saveHistories` asked for the conversation's file through the chat's reader, which read it in and counted as a chat asking. So every agent at work held all its conversation in the daemon for good. That is a likely part of the daemon's heap on the Mac (median 113 MB), not measured there.
- **Every session was attached on connecting.** The app parsed all that every session printed, hidden or not.
- **An open canvas woke the window several times a second** to ask its page two things, and each asking and each answer drew a frame.
- **A chat polled for as long as its session stayed in chat mode**, on screen or not, every 700 ms each, which also kept its conversation in the daemon.
- **The app listed every process of the machine** to learn its own RSS.

The stand-in agents here don't print, so what agents at work cost isn't in these runs: the owner's 6-hour heap (median 377 MB, high 816) wasn't reproduced, only its likely causes, and the fixture's canvases are one case of them.

## Returning memory to the system, GOMEMLIMIT and GC

`core/memtune`, in the app and the daemon, leaves `GOMEMLIMIT` and `GOGC` to the environment when they're set. Otherwise:

- **GOGC 50.** Saves 20 to 25 MB in each program at this use, with no CPU change that shows over the noise. GC went from about 0.12% of CPU to about 0.2%. The saving grows with the heap.
- **GOMEMLIMIT 1 GiB.** Well above an ordinary heap, so it does nothing in ordinary use, and none of these runs reached it. It is for a burst such as the one behind a `sys` of 5,509 MB. Past it the runtime spends up to half the CPU collecting, so a heap that truly needs more runs slower rather than failing. A tighter one, 256 MB for instance, is the "hard limit" trade-off below.
- **Returning free memory.** Every 30 s, if over 32 MiB is free and held, `debug.FreeOSMemory`. In these runs that changed nothing clear, as continuous drawing allocated it again at once; `TestFreeHeldFallsAfterABurst` shows 128 MiB going back after a burst. Its worth is after a burst on a window at rest, which the owner's Mac will show better than this host.

## Evaluated

| Idea | Result |
|---|---|
| Lazy loading of pages | **Built where it costs.** Pages are drawn, not kept, so walking them moved the heap by little, other than Canvas. What was loaded regardless of being shown was the terminals, now lazy. Package start-up is a few MB, not worth changing |
| Unloading what is out of sight | **Built:** terminals after 3 minutes beyond the 3 last shown; the Canvas page's web view and pictures 2 minutes after leaving. **Not built:** one web view at a time. The Browser tool's and file tabs' web views stay per session until the session closes, and weren't measured |
| Image LRU with a size limit | **Built:** canvas pictures, 64 MB. Team chat images already had a limit |
| Scrollback on disk | **App: built in effect**, as its terminals only exist when shown. **Daemon: not built**, see trade-offs |
| Events over polling | **Built for what polled for nothing:** the plan, watches, ports, the app's own usage, frames for looks, canvas asking. Left: the daemon's look at each session's screen (a session's activity every 150 ms at most, its prompts every 3 s, its model every 5 s), for which programs give no event, and and the daemon's whole CPU with 20 idle sessions is now 2.7% of a core on Linux |
| One process sampler | **Built:** what only follows processes reuses the last look under 10 s old, a plan looks once, and the app looks only at itself. Left: `goa_watch` reads every process's environment per GOD of Agents session every 5 s |
| Incremental transcripts | **Reading was already incremental.** The bug was reading at all, now fixed. Left: a chat's poll walks all entries of its conversation, harmless next to the rest. **Large message buffers aren't bounded yet**: a frame up to 256 MiB is read whole. Canvas pages and pictures need that, so a bound means streaming them |
| Watchers of other hosts | **Built:** every 15 s focused, every minute without resource use unfocused, at once on coming back. Not measured, as these runs had one host, and no test covers it |
| Auto-hibernate default | **Checked, not changed:** on, after 30 idle minutes, for agents done with their turn (`defaultHibernateAfter`). It frees the agent and its MCP servers. The session's scrollback stays in the daemon |
| Shared or lazy MCP servers | **Not built**, see below |

The other side of this, by another agent on `wip/syncmem`: `sync.conflicts`'s size and polling, `session.list`'s repeats, and reconnect backoff.

## Path to under 100 MB

By private dirty on Linux, which stands in for footprint.

**App: around 100 MB at rest with what it held still kept, and under it fresh.** Fresh it holds 56; the heaviest use above, 168 with a canvas of 2,000-element designs open, three terminals kept, two of them with full scrollback, a web view in the process, and software drawing's buffers (about 15 MB of the heap and much of its garbage). Six minutes after leaving that page, at rest: **117**, with 67 in the Go heap's mapping and 26 of anonymous memory, mostly the terminals kept. About 13 of the heap is software drawing's, which a GPU doesn't need, so a Mac may well be under 100 there, though that is an estimate. Keeping one terminal rather than three takes off up to 26 more.

**Daemon: under 100 MB until scrollback passes it, and only a smaller scrollback gets it there with many sessions.** 99 in the use above, 65 of it those 5 full scrollbacks. Fresh 16; each idle session adds 0.55; each full scrollback about 13. At 30 full sessions that is about 16 + 390, whatever else is done. That is the evidence of a floor at today's scrollback, and the reason for the first trade-off.

### Trade-offs for the owner

These change behaviour or carry risk, so they aren't built.

| Step | Saves | Consequence |
|---|---|---|
| **Scrollback of 10 → 2 MiB** (`session.Scrollback`) | Measured in the daemon: 5 full sessions took 65 MB at 10 MiB and 10 MB at 2 MiB, so 11 MB per full session. 30 full sessions: from about 406 to about 76 MB | **The one with the most effect, and the only one that gets a many-session daemon under 100 MB.** A terminal keeps about a fifth as much history, so a long build's log is cut. Agents' chats aren't affected, being read from their transcripts |
| Older scrollback kept on disk and read on scrolling up | The same, without losing history | Needs the terminal library to page rows in, which it doesn't offer; or replaying on attach, which costs the time and memory while attached. A larger piece of work |
| Keep 1 terminal instead of 3 (`panesKept`) | Up to 26 MB in the app | Going back to a recent session takes attaching: a "Connecting…" flash and the time to replay its scrollback |
| Free an idle or hibernated session's terminal | Its 13 MB or so | Needs saving and bringing back its screen exactly, which restarts do only approximately. Risk of a session looking wrong |
| A hard `GOMEMLIMIT` such as 256 MB | Bounds Go's part | Past it, up to half the CPU goes on GC; a large canvas or a burst would feel slow. Not recommended above the plain limit until the Mac's numbers ask for it |
| A separate `enowxd` for desktops' daemon | 27 MB of RSS measured here, none of footprint | The app ships two binaries, or one more file to update. Only worth it if RSS itself is the target |
| Streaming canvas pages and pictures | Bounds what a 256 MiB frame takes | A protocol change, kept compatible |

### MCP servers

These are per session and not the app's or the daemon's, but on a busy host they dwarf both. On this host, read from `ps` and `smaps_rollup`: 50 MCP-like processes with 2,550 MB of RSS, one `firebase mcp` at 143.5 MB private dirty and one `coolify` at 43.3, each for each session using them, and `enowxd mcp`, enowX's own, at 7.9 private dirty.

- **Sharing one per host**, through a proxy each session talks to, would save nearly all of that when many sessions run the same ones. But a server may depend on the session's folder, environment or account, may hold state per connection, and not all can serve several clients; each would have to be proven safe or kept apart. Sessions could lose access in ways that are hard to see.
- **Starting on first use, stopping when idle**, again through a proxy, would save a server until it is used, and a tool list has to be known before that, so it would be cached. A first call pays the start, some seconds for `firebase`.
- **Hibernating already frees them**, 30 minutes idle by default, with no change in behaviour. A shorter default is the cheap step: it costs a wake when going back to a session.

The profile lists what sessions' programs take, so the owner can see what these would save on the Mac first.

## Only the owner can measure

- **Footprint on the Mac**: run `enowx-app profile --cpu 30` before and after updating, at the same kind of use, and again after hours. Check that `vmmap` answers there, as that path has never run (`pages_darwin.go`), and how much of `__TEXT` is resident.
- **GPU memory and frames' real cost.** No GPU was involved here.
- **Hours of use.** The longest run here was about 15 minutes. The Mac's growth over 6 hours and its 5,509 MB spike weren't reproduced.
- **Real agents at work.** Here they were stand-ins.

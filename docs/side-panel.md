# Side panel

The panel right of the workspace holds tools about the selected session. The panel button at the right of a session's header, or `⌘⇧D`, shows it; the same button at the right of its top row folds it. Its tools sit in groups, two rows: the top row picks a group, **Code** (Git, Files, Checks, Index: the project searched by meaning, [rag.md](rag.md)), **Preview** (Browser, Ports, Android) or **Debug** (Traffic, Activity), and **Team** (its chat, its projects with where each is on the host and the sessions working in them, and its members with who is on the host, [cloud.md](cloud.md)) while the host is in a team, and the row under it the group's tool, its name lit; files opened from Files follow Code's tools, each in a tab. A group comes back on the tool it showed last. The panel keeps its tool, each group's last, and its width (drag its left edge) between launches. Every tool reads through the session's daemon, so it works the same for a session on a server.

## Git

What the session changed against its base, and committing, pushing, opening a pull request, merging and discarding it: see [review.md](review.md).

## Checks

The commands that tell whether the session's work holds up, run in its folder (`core/checks`, `checks.*`): **Run all**, or one from its row; each shows whether it passed, how long it took, and its output's end when unfolded (the last 256 KB are kept). They run one after the other, with `CI=true` so test runners neither watch nor ask, give up after 15 minutes, and **Stop** ends them with every process they started.

Without settings, the checks come from the project's files: its Makefile's `lint`, `typecheck`, `build` and `test` targets; else `go vet`, `go build` and `go test` for a Go module, the same scripts of `package.json` run with the package manager its lockfile names, `cargo build` and `cargo test`, or `pytest`. **Edit** sets them for the project instead, kept by the daemon in `~/.enowx-app/checks.json`, so the project's own files stay untouched.

## Files

An opened text file's toolbar offers **Copy text**, which copies its source exactly, including line endings and trailing newlines. Markdown and HTML copy their source even while previewed. **Copy the path** remains a separate action. Both copy icons briefly change to a check after copying, then return to their original icon. For files too large to show in full, **Copy displayed text** copies only the loaded part; images and other binary files have no text action. Both local and server sessions copy the content already read through the daemon.

The tree of the session's folder (`core/files`, `file.list`, `file.read`): folders first, unfolded one at a time, and read again every 4 seconds while the tab shows, since the agent keeps changing them. Files are marked as git sees them: `M` modified, `A` added, `U` untracked, `D` deleted, `R` renamed; a folder holding changes is marked `M`, and what git ignores is dimmed. Clicking a file opens it in a tab of its own, after the tools in the panel's top row, the tree staying as it was: a Markdown file rendered (headings, lists, tables, code blocks, as the chat renders the agent's answers), with **Raw** in its bar for the text; an HTML file as code, with **Page** in its bar to show it as a web page (below); a PDF in that web view, which renders it; another file's text, numbered and colored as code by its language, every language the lexers know, HTML with its CSS and JavaScript included (plain text stays plain), its first 1 MB; an image (PNG, JPEG, GIF, WebP, BMP or SVG, up to 8 MB) shows as a picture with its size, scaled down to the panel; another binary file says so. It reads again every 4 seconds, the daemon sending nothing when the file hasn't changed since, with its path to copy and a button to open it in the editor. A file open already is shown, not opened twice; `×` closes its tab. Tabs belong to their session, since the panel follows the selected one; the files Activity lists open the same way. The Files tool stays in the session's folder.

Right-click an entry in Files for its menu, enowX's own: **New file** and **New folder** in its folder (a new file opens), **Add files**, **Rename**, **Duplicate** (`notes copy.md`, a folder with all it holds), **Copy path** and **Copy relative path**, **Properties** (kind, size, a folder's count of files and folders, where it is, when it changed, permissions, owner, git's status) and **Delete**, confirmed first, for good: there is no trash on a server. The space below the rows has the menu of the project folder. They go through the daemon (`file.change`, `file.stat`), so they work the same on a server; none replaces what exists or reaches out of the session's folder.

Files and folders of this device come into the session's folder from Files: right-click a folder for **Add files to …** (a file's menu adds to the folder it is in, and the space below the rows to the project folder), or use the toolbar's button for the project folder. enowX's own picker (`ui/localpicker`) browses this device: its box picks a file or a folder, a folder's name opens it, with Desktop, Downloads, Documents and Pictures a click away. They are copied through the daemon (`file.upload` with `into`), so on a server they land on the server, a folder with all it holds; a name taken gets a number (`logo-2.png`), nothing is overwritten, and nothing goes outside the session's folder. The folder unfolds to show them. A daemon too old to put files in a folder says so (`placed` missing) rather than putting them with the uploads.

**Go to file** (⌘P, or the command palette) finds a file of the selected session's folder by a few letters of its path, in order, anywhere in it (`file.find`, `core/files/find.go`): the files git tracks or would add (what it ignores left out) in a repository, else every file outside folders of dependencies and builds (`node_modules`, `.venv`, `target`, …). Matches in the file's name, at the start of words and in a row rank first. Enter or a click opens it in a tab.

An HTML file's **Page** shows it in a web view of the app, reloaded when the file changes. The web view loads it from a server of the app's on localhost, which reads the file, and what the page loads beside it (styles, scripts, images, fonts, whatever their bytes or size, up to 64 MB), from the session's host through its daemon (`file.bytes`, 4 MB a part). A page of a server's session shows as one of this device does. Each folder pages show from is served at `/<token>/<site>/`: the token, made at random as the app starts, keeps other pages on this device from reading it, and a site serves only what is under its folder. Daemons before `file.bytes` give what `file.read` does: text up to 1 MB, and images. Where the device has no web view (Linux without WebKitGTK), there is no **Page**.

A path a session's program prints opens here too, with `⌘`+click on it in the terminal (Ctrl+click elsewhere): relative to the session's folder, from `~/`, or absolute anywhere on the host (`file.read` with `abs`), such as a screenshot an agent saved in `/tmp`; a `:line:col` after it is dropped. A `http://localhost:<port>` link opens in the Browser tool as one of the session's ports; other URLs open in the system browser.


A text or code file opened in a tab can be edited, on any host, in normal and GOD of Agents modes: line numbers, colors up to 256 KB of text, undo and redo, copy and paste, Tab and Shift+Tab, indents kept on a new line, find and replace (⌘F), go to line (Ctrl+G), and comment out (⌘/). ⌘S saves through the daemon (`file.write`): a whole new file moved over the old, with its mode and line endings. A change not saved shows as a yellow dot on the tab, and closing the tab or quitting asks first.

When the file changes on disk (an agent's work, say) it reloads, unless there are changes here: then a bar offers Compare, Keep mine and Take theirs, and a save never writes over what it hasn't seen. Binary files, read-only ones, files over 16 MB and files with a line over 8 KB open in the read-only long view, the bar saying why. Markdown and HTML still open as Preview and Page; Raw and Code are where they're edited.

## Dropping files on a server's session

Files dropped on a terminal paste their paths, which an agent takes as files to read (an image to look at, say). On this machine that is all. On a server those paths would name nothing, so the app uploads the files first (`file.upload`, in 4 MB parts, through the daemon's connection) and pastes their paths on the server instead. They go to `~/.enowx-app/uploads/<session>/` there, out of the session's folder so its git status stays clean, a name taken being numbered (`shot-2.png`), and they are deleted with the session. The app does so for any paste that is only paths of files on this machine, as copying files in Finder and pasting them gives, up to 200 MB at once; any other paste goes as it is.

## Open in editor

A session's folder opens in VS Code, Cursor or Zed from its menu (right-click, or the header's More) and the command palette, and the file shown in Files opens from its bar in the editor last used. They open by the editor's URL scheme: `vscode://file/<path>`, `cursor://…`, `zed://file/<path>` on this machine; on a server, through the editor's own SSH remote, `vscode://vscode-remote/ssh-remote+<user@host><path>` (Cursor alike) and `zed://ssh/<user@host><path>`, so the files stay on the server. The editor needs its remote set up for that host (Remote-SSH for VS Code and Cursor); the server URLs aren't checked against a server yet.

## Activity

What the agent did, read from the transcript the chat view reads (so Claude Code for now; other agents say they aren't read yet). On top, the totals: turns, tool calls, edits and failed calls. Then the files the agent edited, with the lines each edit added and removed summed per file; clicking one shows it in Files. Then the turns, newest first: the prompt, when, how long the agent worked, and what it ran; unfolding a turn lists its tool calls with what each worked on, failed ones in red.

## Ports

The TCP ports the session's processes listen on (`session.ports`): a dev server the agent started, say, with the program and its PID, and whether only the host reaches it or every address. On this machine, **Open** opens it in the browser. On a server, **Forward** listens on this machine's loopback, on the same port when it is free, and carries each connection to the server's port through the SSH connection (`direct-tcpip`, `transport.Forward`); **Open** then opens the local address. Forwards last until stopped, until their session ends, or until the app disconnects from the server; a dev server restarting keeps its forward, connections failing while it is down.

When a session opens a port while the app is connected, a card at the bottom right says so (`node is listening on :5173 in …`) with **Open**, or **Forward & open** for a server, and **Ports** to see the tab; it goes when taken, dismissed or the port closes. The daemon looks every 5 seconds while a client is connected and tells clients what changed (`ports.changed`, with the ports opened); its first look only learns what is open, so ports open before aren't offered. **Pop up when a session opens a port**, at the bottom of the Ports tool, turns the cards off (or on) for every session; ports always previewed and displays' windows still open.

The daemon finds a session's processes by name: each session's program runs under a small `/bin/sh` named `enowx-session-<id>` (`core/session`), whose arguments every platform shows, where recent macOS hides other processes' environment. Ports come from `lsof` on macOS and `/proc/net/tcp` on Linux; Windows shows none yet.

## Traffic

The HTTP the session's programs send, caught by its host's interceptor: the list with its filter, an exchange's headers, bodies and messages in a tab of the session, mocks and breakpoints, rules, what is attached, Compose, and its settings. See [traffic.md](traffic.md).

## Android

The devices of the session's host and of this machine: start an emulator, connect a phone, set Android up, run the session's app on a device (across hosts through the ADB bridge), catch its traffic, and open its **Screen** and **Logcat** in tabs of the session, to use its screen or read its log. See [android.md](android.md).

# Preview

The **Browser** (`⌘⇧P`, or **Preview** on a port's card and in the Ports tool), in a tab of its own or docked in the side panel, shows what the selected session's dev server serves, in a web view of this device: WKWebView on macOS, WebView2 on Windows, WebKitGTK on Linux. The plan behind it, and what comes next, is in [plans/preview.md](plans/preview.md).

## How it works

- The tool asks the session's daemon for the ports its processes listen on (`session.ports`, every 2 seconds while it shows) and opens the first, or the one it showed last; several ports give a picker.
- On this machine the page loads `http://localhost:<port>`. On a server, the app forwards the port here first, on the same port when it is free, through the SSH connection (see Ports in [side-panel.md](side-panel.md)): the page renders here, and only HTTP and WebSocket traffic crosses, so hot module reload works as it does locally.
- Each session has one web view, made the first time it shows a page and kept while the session lasts: switching tools or sessions keeps the page, its scroll and its state. Where it was (port and path) is remembered between launches.
- When the port closes, as a dev server restarting does, the tool says so and reloads the page when the port is back.
- Until the session runs a dev server, the tool shows https://enowx.ai, its address left blank as a new tab's; the first dev server that starts takes its place. A site the user opened stays, the dev server waiting in the picker.

## The toolbar

Back, forward, reload; the address, as in a browser: a path (`/settings`) of the dev server shown, a host (`google.com` gets https, `localhost:3000` http) or a URL, opened even before the session has a dev server (another site loads from this device, not the session's host; a dev server starting meanwhile waits in the port picker); the page's width (Fit, or a laptop's, a tablet's or a phone's, centered); a screenshot of the page, typed into the session's prompt as a file for its agent to look at (uploaded to a server's session first); DevTools; and **Open in browser**.

## In the side panel or as a page

The page shows in one of two places, which the button at the right of the Browser's bar switches at any time, for every session:

- **Open as a page** (the default, as before): the page has a tab of its own in the center, and the side panel's Browser lists the session's pages, each opening there.
- **Show in the side panel:** the page fills the side panel's Browser, next to the session's terminal or chat, so the agent and the page show at once. The panel keeps its width rules (dragged between 280 and 1100, 480 until dragged); its bar keeps back, forward, reload, the address and the way out on the first row, and the port, the page's width, picking, the screenshot, the canvas and Unslop on a second, so a narrow panel loses none of them.

The choice is kept with the app's state (`browserInPanel`), and whatever opens a page opens it there: a preview from a port's card or the Ports tool, Traffic, and an agent's `enowx browser open` (which only marks the Browser when it isn't in front). Moving it doesn't reload the page: it is the same web view, only placed elsewhere in the window, so its scroll, form and state stay. A session showing the Browser as its view (below) keeps it there; docking from there moves it to the panel. GOD of Agents' sessions behave the same.

## As the session's view

**Browser** in a session's Terminal · Chat · Browser toggle shows the preview in the session's own place, for a wide window or a whole day of it; the side panel's Browser then says where it is, as one page shows in one place.

## Always preview a port

**Always preview :5173 in this project** on a port's card makes that port of the project load in the Browser by itself when a session shown opens it, without the card; a mark on the Browser says so, and the user opens it. It is kept per host and project; unticking it on a card ends it. When the user is at something else (a page, or another tool in the side panel such as the team's chat), the page loads in the Browser behind and its button gets a dot until looked at: nothing takes the panel from them.

## Hot reload on a server

Nothing to set up for most dev servers: they and their file watchers run in the session, next to the files the agent edits. When the local port can't match the remote one (it was taken here), set the HMR client port (Vite `server.hmr.clientPort`, webpack `client.webSocketURL`). HTTPS dev servers pass through; a self-signed certificate needs the browser. OAuth redirects work, as the origin is `localhost`.

## Desktop apps built with web technology

Tauri (`tauri dev`), Electron (`electron-vite dev`) and Wails (`wails dev`) serve their frontend from a dev server: the Browser previews it with hot reload. The page runs without the native bridge (`window.__TAURI__`, Electron's preload APIs); guard or mock it in development.

## Desktop apps with windows, on a Linux server

**Run with a display** in the New session dialog (offered where the host can: Linux with [Xpra](https://xpra.org); the dialog says what a host lacks) gives the session a virtual display of its own (`core/display`): the daemon starts `xpra start :N --bind-tcp=127.0.0.1:<port> --html=on` and sets `DISPLAY=:N` for the session, so whatever it runs — `tauri dev`, `npm run electron:dev`, `flutter run -d linux`, `cargo run`, a GTK or Qt app — opens its windows there. The display's HTML5 client is a port of the session, `:<port> display`: the Browser shows it, windows as windows, keyboard and pointer going back and the clipboard following. Hot reload is the framework's own, on the server. The display ends with its session, and a session continued after a daemon restart gets a new one.

The app shows in a window of its own on this device (`ui/appwindow`), as if it ran here: enowX opens it when the display starts and shows it with the app's first window, hides it while the app has none (it quit, or restarts on a change), titles it as the app titles its window, and resizing it resizes the app. It is a native window with a web view loading the display's HTML5 client, with Xpra's menu, desktop and window frames hidden and the app's main window filling it; dialogs and other windows of the app show over it. Nothing needs installing here, so it works on macOS, Windows and Linux alike. It stays sharp: the app tells the daemon its screen's scale on connecting (`display.scale`), displays started then draw their apps at it (`GDK_SCALE`, `QT_SCALE_FACTOR`), and the window zooms so each of their pixels is one of the screen's; Xpra keeps a minimum quality of 80 and no video encodings, so changes don't blur. The clipboard isn't polled on clicks, which in WebKit offers a Paste button that takes the click; pasting still works. Closing it quits the app, as on a desktop: the display stops (`session.displayStop`) with everything that runs on it, the app and the dev server or build that started it included (`enowx display` marks what runs after it with `ENOWX_DISPLAY`), and what Xpra started there (its D-Bus: `DISPLAY` without a session), while the session's agent or shell goes on with what it runs for itself. Disconnecting or quitting enowX only closes the window; the app goes on on its host, and the Ports tool's window button opens it again.

A session started without one gets one when it runs `eval "$("$ENOWX_BIN" display)"` (`session.display`): the agents enowX starts know to do this before running a desktop app (see [agents.md](agents.md#what-agents-are-told)), so a plain session works too.

Where Xpra is missing, the New session dialog shows the install command for the server's package manager and **Install Xpra** runs it in a session, as agents are installed; an agent may also install it itself. Every frame crosses the network, so latency shows; most servers have no GPU, so 3D is slow; audio is off.

## When the server's SSH doesn't forward ports

Forwards go through the SSH connection (`direct-tcpip`). When the server's SSH refuses (`AllowTcpForwarding no`), they go through the daemon instead (`tunnel.open`): bytes travel in frames of the daemon's own connection, at most 256 KB unacknowledged each way, so a large download can't hold terminals up, and the daemon reaches only ports the session's processes (or its display) listen on. A daemon from before tunnels leaves SSH's reason as the error.

## Without a web view

A Linux machine without WebKitGTK has none: the tool says so and offers to open the page in the browser.

## Picking an element

The Browser tool's target button (design mode) lets the user point at an element of the page, outlined as the pointer moves, and click it; the page doesn't get that click. The session's agent then gets, pasted into its prompt without being sent, what the element is (the page's address, a CSS selector, its HTML up to 4,000 characters, its main computed styles) and a picture of it, cut from a capture of the page with a little room around, saved as a PNG (uploaded first for a server's session, like screenshots). The user then says what to change. Escape in the page, or the button again, stops picking (`ui/browserview/pick.go`; checked in WebKit on macOS).

## Driving it from the command line

Agents check what they build in the same Browser the user sees, through `enowx-app browser` (the agent guide tells them how):

| Command | Does |
|---|---|
| `browser open URL` | Loads a page in the session's Browser: a URL, a host, or a path of its dev server. A mark on the Browser tells the user when it isn't in front: it never opens or moves for the agent. |
| `browser snapshot` | Prints the page's title, address and text, then its controls, numbered: `[2] button "Join"`. |
| `browser click REF` | Clicks a control, by its number from the last snapshot or a CSS selector, and waits for what it loads. |
| `browser fill REF TEXT` | Types into a field as the page sees typing (React's inputs included); a select takes an option's value or text. |
| `browser screenshot FILE` | Saves a PNG of the page, on the session's host. |

`--session S` names another session than the one it runs in. The daemon hands each action to the app connected to the host (`browser.act`, `browser.request`), so the app has to be open; the page is the Browser's, and stays for the user to look at.

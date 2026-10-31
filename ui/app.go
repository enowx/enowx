// Package ui is the enowX desktop interface, drawn with MyGo's native UI.
package ui

import (
	"encoding/json"
	"io"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/projectsync"
	"github.com/enowx/enowx/core/tailnet"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/activityview"
	"github.com/enowx/enowx/ui/androidview"
	"github.com/enowx/enowx/ui/appwindow"
	"github.com/enowx/enowx/ui/browserview"
	"github.com/enowx/enowx/ui/canvasview"
	"github.com/enowx/enowx/ui/chatview"
	"github.com/enowx/enowx/ui/checkview"
	"github.com/enowx/enowx/ui/creditview"
	"github.com/enowx/enowx/ui/diffview"
	"github.com/enowx/enowx/ui/fileview"
	"github.com/enowx/enowx/ui/folderpicker"
	"github.com/enowx/enowx/ui/libraryview"
	"github.com/enowx/enowx/ui/localpicker"
	"github.com/enowx/enowx/ui/notesview"
	"github.com/enowx/enowx/ui/portview"
	"github.com/enowx/enowx/ui/syncview"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/trafficview"
	"github.com/enowx/enowx/ui/usageview"
	"github.com/enowx/enowx/ui/widget"
)

// App is the state of the main window. Its fields are read and written on
// the main thread only; other goroutines go through win.Update.
type App struct {
	win   *mygo.Window
	theme *ui.Theme
	// onMain runs a function on the main thread: the window's Update.
	onMain func(func())
	// stopOnQuit says the app quits through "Quit and stop all agents".
	stopOnQuit bool
	opts       Options

	// The connection. gen counts connections, so callbacks of a previous
	// one (events, attaches, progress) are ignored after a switch.
	client          *client.Client
	gen             int
	host            hosts.Host
	connecting      string // progress while connecting; "" once connected
	connErr         string // why the last connection failed
	connFail        error  // the same, to tell what kind of failure it was
	notice          string // a.status is news, not a problem, while it equals this
	trust           *trustPrompt
	runAs           *runAsPrompt
	menu            widget.ContextMenu
	passphrase      *passphrasePrompt
	signIn          *signInPrompt
	sync            *syncview.Dialog
	appWindows      map[string]*appwindow.Window // desktop apps shown in their windows, by session ID
	runCommands     map[string]*commandList      // project commands, by session ID, for its Run menu
	commandForm     *commandForm
	shortcutsOpen   bool                             // Settings › App lists the shortcuts
	winW            float32                          // the window's width, last frame
	settingsSection string                           // the Settings section shown
	hostSet         *hostSettings                    // Settings' sections' state for the connected host
	shells          *hostShells                      // the connected host's shells
	cloudSet        *cloudSettings                   // Settings' Cloud section, for the connected host
	cloudSigningIn  bool                             // the cloud's sign-in dialog shows
	spaceList       *hostSpaces                      // the connected host's teams' spaces (spaces.go)
	goneList        *hostSpacesGone                  // the connected host's notices of teams deleted (spacesgone.go)
	confirming      *confirmation                    // a destructive action asking first (confirm.go)
	teamProjectList *hostTeamProjects                // the teams' projects (teamprojects.go)
	spaceMoving     *spaceMoveForm                   // a note or canvas going to another space (spacemove.go)
	teamKitList     *hostTeamKit                     // the teams' kits (teamkit.go)
	rulesEditing    *rulesForm                       // a project's rules being edited (rules.go)
	teamHostList    *hostTeamHosts                   // the host's share and its teams' hosts (teamhosts.go)
	visit           *teamVisit                       // a team's host visited (teamhosts.go)
	presence        *hostPresence                    // members visiting the connected host (takeover.go)
	here            *teamsHere                       // who of the user's teams is here (teamhere.go)
	unslopping      bool                             // a canvas's design being copied for Unslop (canvas_unslop.go)
	unslopErr       string                           // why the last unslop didn\'t start
	unslopRuns      map[string]proto.CanvasUnslopRun // by canvas, as the daemon told
	unslopPick      unslopChoice                     // the passes to unslop with (canvas_unslop_replay.go)
	replay          unslopReplay                     // a redesign played back
	buildRuns       map[string]proto.CanvasBuildRun  // UI builds, by canvas (canvas_build.go)
	buildStarting   bool                             // a build being started
	buildErr        string                           // why the last build didn't start
	// Browsers' unslops, by session: starting, and as the daemon told.
	browserUnslopping map[string]bool
	browserUnslopErr  map[string]string
	unslopComparing   *unslopCompare
	renaming          *renameForm // a session being renamed (session_rename.go)
	browserUnslops    map[string]proto.CanvasUnslopRun
	teamChats         *hostChats      // the teams' chats (teamchat.go)
	discord           discordState    // Discord Rich Presence (discord.go)
	fonts             []ui.FontFamily // this device's monospaced fonts, for Settings' Terminal section
	fontsLoading      bool            // the fonts are being read
	fontsLoaded       bool            // they were read, even when this device has none
	fontQuery         string          // the font search, in Settings' Terminal section
	notes             notesview.View  // the Notes page
	noteList          *hostNotes      // the connected host's notes
	clone             *cloneForm      // the Clone from Git dialog
	noteImport        *noteImportForm
	noteExport        *noteExportForm
	catalogs          noteCatalogs    // every host's projects, for notes attached to them
	attaching         *noteAttachForm // the dialog attaching a note to projects
	// saveAs asks where to save a file named name, "" for nowhere; nil
	// asks with the system's dialog (tests answer themselves).
	saveAs     func(name string) (string, error)
	canvases   canvasview.View // the Canvas page
	canvasList *hostCanvases   // the connected host's canvases
	canvasChat *hostCanvasChat // the open canvas's conversation with its agent
	// notesModel and notesEffort are what a note's agent starts on, by agent.
	notesModel, notesEffort map[string]string
	agentAPIs               *agentAPIState // Settings' Model APIs
	// tray is the app's icon in the menu bar or notification area, nil
	// where there is none; trayPanel its open panel. quitting: the app is
	// quitting, so closing the window doesn't keep it in the tray.
	tray               *mygo.Tray
	trayPanel          *mygo.Window
	trayTitle, trayTip string
	quitting           bool
	library            *libraryview.View // the Settings page's library, for libraryHost
	libraryHost        string
	setup              *setupPage                // the Setup page, for the host it was opened on
	setupPolled        time.Time                 // when the setup's status was last read
	drift              map[string]int            // steps a host lacks to be what this device is, by host ID
	watches            map[string]*hostWatch     // the hosts not shown, by ID
	dsync              dataSync                  // the user's data synced between hosts
	feats              *hostFeatures             // the connected host's feature switches
	noteChatsRead      noteChats                 // notes' conversations with their agents
	featAsks           []featureAsk              // agents asking to turn a feature on
	ntf                notifyState               // the sounds played (notifysound.go)
	chooseRunAs        string                    // the host whose next connection asks which user to run as
	connectLink        string                    // a link the server gave while signing in: a Tailscale SSH check
	openSession        string                    // a session to show once the host connected lists it
	openProject        string                    // a project to show once the host connected shows it
	copying            *copyForm                 // the Copy to another host dialog
	cloningTo          *cloneToForm              // the Clone on another host dialog
	projectSyncing     *projectSyncForm          // the Sync with another host dialog
	syncPairs          *projectsync.Store        // projects synced between hosts, and their base
	links              autoSync                  // links between hosts' copies of projects (autosync.go)
	notionRunning      map[string]bool           // Notion imports made again now, by notionSync ID
	linksForm          *linksForm                // the Sync & push dialog of a project
	vlt                *vaultApp                 // the credential vault (vault.go)
	piping             *pipelineForm             // the New pipeline dialog
	creating           *vpsForm                  // the Create a server dialog
	quick              quickOpen                 // the Go to file dialog
	issues             issuePicker               // the New session dialog's issues
	unread             map[string]bool           // sessions whose agent finished or asked unseen
	history            historyDialog             // the Past conversations dialog
	compare            compareDialog             // a fan-out's sessions, to keep one
	accts              accountsState             // Settings → Agent accounts
	localPick          localpicker.Picker        // picks this device's files to add to a session
	autos              automationsState          // Settings → Automations
	hooks              hooksState                // Settings → Hooks
	ragSet             ragSettingsState          // Settings → RAG (rag.go)
	ragPanel           ragPanelState             // the side panel's Index (rag_panel.go)
	ragOffer           *proto.RAGRememberParams  // a session's decision offered as a memory
	decideSet          decideState               // Settings → Decisions (decide.go)
	decision           *proto.Decision           // the judge stopped an agent or refused its command (decide_uses.go)
	chatToasts         []*chatToast              // team chat messages, in the corner (teamchatnotify.go)
	acctAsk            *proto.AccountFallback    // an account at its limit, asking to switch (accounts_switch.go)
	browserNews        bool                      // the Browser opened a page while the user was at something else
	deletingServer     *hosts.Host               // the server whose deletion is asked about
	clearingHost       *hosts.Host               // the host whose synced data's clearing is asked about
	picked             map[string]bool           // sessions picked with ⌘ or ⇧ clicks, for Hibernate
	hibernating        *hibernateForm            // hibernating sessions that would lose work, asked about
	pipelines          []proto.Pipeline          // the connected host's
	tabs               map[string]string         // the tab each session shows in front, by session ID
	installs           map[string]func()         // install sessions, by session ID: what to check once each ends
	chats              map[string]*chatview.Chat // by session ID
	diffs              map[string]*diffview.View // by session ID
	fileViews          map[string]*fileview.View // by session ID
	// fileTabs are the files open in the side panel, by session ID; docs
	// show them, by session ID and path.
	fileTabs        map[string][]string
	docs            map[string]*fileview.Doc
	pageViews       map[string]*pageView      // file tabs showing their page, by session and path
	portViews       map[string]*portview.View // by session ID
	trafficView     *trafficview.View         // the Traffic tool, for connection trafficGen
	trafficGen      int
	androidView     *androidview.View // the Android tool, for connection androidGen
	androidGen      int
	local           localDaemon                   // this machine's daemon, beside a server's
	webCapture      sync.Map                      // top session id → its previews go through Traffic
	trafficNotified time.Time                     // the last failed request told
	browsers        map[string]*browserview.View  // by session ID
	activityViews   map[string]*activityview.View // by session ID
	checkViews      map[string]*checkview.View    // by session ID
	// forwards are the connected server's ports forwarded here, by port.
	forwards map[int]*forward
	// sshRefused: the server's SSH refused to forward a port; forwards go
	// through the daemon. Read off the main thread.
	sshRefused atomic.Bool
	// portOffers are ports sessions just opened, offered to open.
	portOffers []portOffer
	// conflicts are the conflict radar's last look, by project path.
	conflicts  map[string][]proto.Conflict
	budget     *budgetForm // setting a session's budget
	agentLimit *agentsForm // setting the host's limit of agents at once
	usage      usageview.Meter
	usageShown bool          // the Resource usage page was shown last frame
	tok        tokensState   // token usage: the Tokens tab, sessions' badges (tokens.go)
	blurred    bool          // the window isn't the focused one
	winCtl     windowControl // what the window buttons act on, when not win (tests)
	// retrying counts the tries to connect again to retryHost after its
	// connection dropped; 0 when not retrying.
	retrying  int
	retryHost string
	// titleLeftW and titleRightW are how wide the title bar's sides were
	// last frame, which they keep.
	titleLeftW, titleRightW float32
	credits                 creditview.Tracker
	updates                 updater
	hostList                []hosts.Host
	passwords               map[string]string // SSH passwords when there is no keychain
	// keychainRead keeps the passwords read from the keychain this run,
	// so its prompt comes once, not at each connection or watch. Guarded
	// by keychainMu, which is held while reading.
	keychainMu   sync.Mutex
	keychainRead map[string]string
	peers        []tailnet.Peer // the tailnet's devices, if Tailscale runs
	peersErr     string         // why there are none
	peersLoaded  bool           // the tailnet was asked at least once
	peersAt      time.Time      // when the tailnet's devices were asked for, for the hosts menu
	tailUser     string         // SSH user for tailnet devices

	agents     []proto.AgentInfo
	daemonInfo proto.DaemonInfo // the connected daemon, as it described itself
	projects   []proto.ProjectInfo
	sessions   []proto.SessionInfo
	panes      map[string]*pane
	// removed remembers killed sessions, so a late update can't revive them.
	removed map[string]bool

	project   string          // selected project path
	collapsed map[string]bool // projects whose sessions are hidden
	notesOpen map[string]bool // projects whose attached notes are unfolded
	// foldedGroups are the groups of projects whose projects are hidden.
	foldedGroups map[string]bool
	groupForm    *groupForm
	selected     string // selected session ID
	maximized    string // session shown alone, if any
	page         string // a rail view shown instead of the sessions, or ""
	filter       string // the session list's filter
	// accentInput is the Appearance page's custom accent as typed.
	accentInput string
	systemDark  bool   // effective native appearance, owned by the UI thread
	status      string // last error, shown in the status bar

	skipPermissions bool               // New session's default
	deleting        *proto.SessionInfo // asking to confirm a delete
	removing        *proto.ProjectInfo // asking to confirm removing a project
	state           appState           // remembered between launches
	savedState      string
	restoredView    bool // the remembered project and session were reopened

	newSession newSessionForm
	palette    palette
	addProject folderpicker.Picker
	// hostFile chooses a file of the connected host for a tool; pickedFile
	// gets it.
	hostFile       folderpicker.Picker
	pickedFile     func(path string)
	addHost        addHostForm
	hostsPageShown bool // the Hosts page asked the tailnet since it opened

	// floating is the radial panel the global shortcut summons;
	// floatingShown says it is up, floatingFresh that it must bloom out of
	// the hub rather than appear.
	floating      *mygo.Window
	floatingShown bool
	floatingFresh bool
	// floatingShownAt is when the panel last showed, to ignore the blur of
	// its showing.
	floatingShownAt time.Time
}

// Options configures the app.
type Options struct {
	// Hosts is the device's list of servers.
	Hosts *hosts.Store
	// Start is the host to connect to first.
	Start hosts.Host
	// DaemonCommand starts this machine's daemon.
	DaemonCommand []string
	// OpenProject, if set, is opened as a project once connected.
	OpenProject string
	// StateFile keeps what the app reopens at the next launch.
	StateFile string
	// ResumeHost starts on the host shown when the app last quit, if it is
	// still saved, instead of Start.
	ResumeHost bool
}

// pane shows one attached session.
// termPadX and termPadY are the room around a terminal's text.
const termPadX, termPadY = 10, 8

type pane struct {
	term   *terminal.Terminal
	stream *client.Stream
}

// LocalHost is this machine as a host.
func LocalHost() hosts.Host {
	name := "This PC"
	if runtime.GOOS == "darwin" {
		name = "This Mac"
	}
	return hosts.Host{ID: hosts.LocalID, Name: name}
}

// New returns the app.
func New(opts Options) *App {
	a := &App{
		opts:            opts,
		panes:           map[string]*pane{},
		removed:         map[string]bool{},
		collapsed:       map[string]bool{},
		state:           loadState(opts.StateFile),
		skipPermissions: true,
		systemDark:      true,
	}
	theme.Apply(a.state.palette(a.lightAppearance()))
	a.theme = theme.Window()
	if a.state.SkipPermissions != nil {
		a.skipPermissions = *a.state.SkipPermissions
	}
	if opts.ResumeHost && opts.Hosts != nil && a.state.Host != "" && a.state.Host != hosts.LocalID {
		if h, ok := opts.Hosts.Get(a.state.Host); ok {
			a.opts.Start = h
		}
	}
	return a
}

// Open shows the main window and connects to the start host. Call it from
// mygo.App.WhenReady.
func (a *App) Open() *mygo.Window {
	a.startAppearance()
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:           "enowX",
		Width:           1280,
		Height:          800,
		MinWidth:        minWindowWidth,
		MinHeight:       minWindowHeight,
		BackgroundColor: theme.Hex(theme.Background),
		// The view draws the title bar: the command bar sits in it.
		TitleBarStyle:  mygo.TitleBarHidden,
		TitleBarHeight: titleBarHeight,
		// The view draws the window's buttons, the same on every system
		// (windowButtons); the window keeps its shadow and resizing edges.
		HideWindowButtons: true,
		Content:           ui.View(a.view),
	})
	a.onMain = a.win.Update
	stopAppearance := mygo.Theme.OnUpdated(func() { a.systemAppearanceChanged(mygo.Theme.IsDark()) })
	a.watchQuit()
	a.usage.Every = a.usageEvery
	a.win.OnFocus(func() { a.blurred = false; a.usage.Poke() })
	a.win.OnBlur(func() { a.blurred = true })
	setAppMenu(a)
	a.win.OnClosed(func() {
		stopAppearance()
		a.disconnect()
		a.stopWatches()
	})
	a.reloadHosts()
	go a.loadPeers()
	a.connect(a.opts.Start)
	a.startUpdates()
	a.startAutoSync()
	go func() {
		for range time.Tick(time.Minute) {
			a.onMain(a.vaultTick)
			a.onMain(a.notionSyncTick)
		}
	}()
	a.setUpTray()
	a.setUpFloating()
	return a.win
}

// handleEvent applies a daemon event of connection gen. It runs on the
// client's read goroutine and hands the change to the main thread.
func (a *App) handleEvent(gen int, ev proto.Event) {
	a.syncEvent(ev)
	a.notifyEvent(ev, a.host.Name)
	a.hooksEvent(ev)
	a.ragEvent(ev)
	a.decideEvent(ev)
	a.tokensEvent(ev)
	a.accountEvent(ev)
	a.cloudEvent(ev)
	var apply func()
	switch ev.Name {
	case proto.EventSessionUpdated:
		var info proto.SessionInfo
		if json.Unmarshal(ev.Data, &info) != nil {
			return
		}
		apply = func() {
			i := slices.IndexFunc(a.sessions, func(s proto.SessionInfo) bool { return s.ID == info.ID })
			var old proto.SessionInfo
			if i >= 0 {
				old = a.sessions[i]
			}
			a.upsert(info)
			a.installEnded(info)
			if i >= 0 {
				a.alert(old, info)
				a.markUnread(old, info)
				a.soundFor(old, info, true)
			}
		}
	case proto.EventBrowserRequest:
		var req proto.BrowserRequest
		if json.Unmarshal(ev.Data, &req) != nil {
			return
		}
		apply = func() { a.driveBrowser(req) }
	case proto.EventVaultRequest:
		var req proto.VaultRequest
		if json.Unmarshal(ev.Data, &req) != nil {
			return
		}
		apply = func() { a.vaultEvent(a.client, a.host, req) }
	case proto.EventSessionRemoved:
		var ref proto.SessionRef
		if json.Unmarshal(ev.Data, &ref) != nil {
			return
		}
		apply = func() { a.remove(ref.ID) }
	case proto.EventProjectsChanged:
		var list []proto.ProjectInfo
		if json.Unmarshal(ev.Data, &list) != nil {
			return
		}
		apply = func() { a.setProjects(list) }
	case proto.EventFeaturesChanged:
		apply = a.loadFeatures
	case proto.EventFeaturesRequested:
		var req proto.FeatureRequest
		if json.Unmarshal(ev.Data, &req) != nil {
			return
		}
		cl, host := a.client, a.host.Name
		apply = func() { a.featureRequested(cl, host, req) }
	case proto.EventNoteChat:
		if apply = a.noteChatEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventNotesChanged:
		if apply = a.notesEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventProjectClone:
		if apply = a.cloneEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventNoteImport:
		if apply = a.noteImportEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventCanvasChat:
		if apply = a.canvasChatEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventCanvasUnslop:
		if apply = a.unslopEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventBrowserUnslop:
		if apply = a.browserUnslopEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventCanvasBuild:
		if apply = a.buildEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventCanvasesChanged, proto.EventCanvasChanged, proto.EventCanvasRender:
		if apply = a.canvasEvent(ev); apply == nil {
			return
		}
	case proto.EventTeamChat:
		if apply = a.teamChatEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventTeamChatGone:
		if apply = a.teamChatGoneEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventHostVisitors, proto.EventTakeoverAsked:
		if apply = a.presenceEvent(ev); apply == nil {
			return
		}
	case proto.EventTeamHostsChanged:
		if apply = a.teamHostsEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventTeamKitChanged:
		if apply = a.teamKitEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventTeamProjectsChanged:
		if apply = a.teamProjectsEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventSpacesChanged:
		if apply = a.spacesEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventSpacesGone:
		if apply = a.spacesGoneEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventNoteGroupsChanged:
		if apply = a.groupsEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventNoteProposalsChanged:
		if apply = a.proposalsEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventChecksChanged:
		if apply = a.checksEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventPipelineUpdated:
		if apply = a.pipelineEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventPortsChanged:
		if apply = a.portsEvent(ev.Data); apply == nil {
			return
		}
	case proto.EventTrafficExchange, proto.EventTrafficPaused, proto.EventTrafficAttached, proto.EventTrafficCapture:
		if apply = a.trafficEvent(ev); apply == nil {
			return
		}
	default:
		if apply = a.linkEvent(ev, func() (*client.Client, string) { return a.client, a.daemonInfo.ID }); apply == nil {
			return
		}
	}
	a.onMain(func() {
		if gen == a.gen {
			apply()
		}
	})
}

// refresh loads the daemon's state; it runs off the main thread.
// screenScale is the screen's scale, for a host's displays; tests, with no
// window's loop to ask, replace it.
var screenScale = appwindow.ScreenScale

func (a *App) refresh(c *client.Client, gen int, openProject string) {
	if openProject != "" {
		if _, err := c.AddProject(openProject); err != nil {
			a.onMain(func() { a.status = err.Error() })
		}
	}
	info, _ := c.Info() // an older daemon leaves what it doesn't know zero
	if info.Display {
		_ = c.SetDisplayScale(screenScale())
	}
	agents, errA := c.Agents()
	projects, errP := c.Projects()
	sessions, errS := c.Sessions()
	a.onMain(func() {
		if gen != a.gen {
			return
		}
		for _, err := range []error{errA, errP, errS} {
			if err != nil {
				a.status = err.Error()
				return
			}
		}
		a.agents, a.daemonInfo = agents, info
		a.setProjects(projects)
		for _, info := range sessions {
			a.upsert(info)
		}
		if id := a.openSession; id != "" {
			a.openSession = ""
			if s, ok := a.sessionByID(id); ok {
				a.selectSession(s) // picked from another host's list
			}
		}
		a.restoreView()
		a.syncSoon()
		a.loadNotify()
	})
	a.loadPipelines(c, gen)
}

func (a *App) setProjects(list []proto.ProjectInfo) {
	a.projects = list
	if p := a.openProject; p != "" && slices.ContainsFunc(list, func(i proto.ProjectInfo) bool { return i.Path == p }) {
		a.project, a.openProject = p, "" // just copied here
	}
	if a.project == "" || !slices.ContainsFunc(list, func(p proto.ProjectInfo) bool { return p.Path == a.project }) {
		a.project = ""
		if len(list) > 0 {
			a.project = list[0].Path
		}
	}
}

// upsert records a session and attaches to it once.
func (a *App) upsert(info proto.SessionInfo) {
	if a.removed[info.ID] {
		return
	}
	if i := slices.IndexFunc(a.sessions, func(s proto.SessionInfo) bool { return s.ID == info.ID }); i >= 0 {
		// A program that starts again (continued, or a queued agent's)
		// runs in a new terminal of the daemon: the old stream shows the
		// last one, so attach anew.
		if old := a.sessions[i]; (old.Exited || old.Queued) && !info.Exited && !info.Queued {
			if p := a.panes[info.ID]; p != nil {
				p.close()
			}
			delete(a.panes, info.ID)
		}
		a.sessions[i] = info
	} else {
		a.sessions = append(a.sessions, info)
	}
	if _, ok := a.panes[info.ID]; !ok && a.client != nil {
		a.panes[info.ID] = nil // attaching
		go a.attach(a.client, a.gen, info.ID)
	}
	a.updateTray()
}

func (a *App) remove(id string) {
	a.removed[id] = true
	a.sessions = slices.DeleteFunc(a.sessions, func(s proto.SessionInfo) bool { return s.ID == id })
	a.updateTray()
	if p := a.panes[id]; p != nil {
		p.close()
	}
	delete(a.panes, id)
	delete(a.tabs, id)
	for parent, tab := range a.tabs {
		if tab == id {
			delete(a.tabs, parent)
		}
	}
	delete(a.chats, id)
	delete(a.state.Chats, id)
	delete(a.diffs, id)
	delete(a.fileViews, id)
	for _, p := range a.fileTabs[id] {
		delete(a.docs, id+"\x00"+p)
	}
	delete(a.fileTabs, id)
	delete(a.portViews, id)
	a.stopForwards(id)
	a.closeAppWindows(id)
	if b := a.browsers[id]; b != nil {
		b.Close()
		delete(a.browsers, id)
	}
	delete(a.state.Previews, id)
	delete(a.state.BrowserViews, id)
	delete(a.state.TrafficViews, id)
	delete(a.activityViews, id)
	delete(a.checkViews, id)
	a.forgetInLayouts(id)
	if a.selected == id {
		a.selected = ""
	}
	if a.maximized == id {
		a.maximized = ""
	}
}

// attach runs off the main thread: it waits on the daemon, then builds the
// pane's terminal on the main thread.
func (a *App) attach(c *client.Client, gen int, id string) {
	st, err := c.Attach(id)
	if err != nil {
		a.onMain(func() {
			if gen != a.gen {
				return
			}
			delete(a.panes, id)
			if strings.Contains(err.Error(), "not found") {
				a.remove(id) // killed before we attached
				return
			}
			a.status = err.Error()
		})
		return
	}
	// The terminal reads the stream from its own goroutine as soon as it
	// exists; resizes wait until it is assigned.
	ready := make(chan struct{})
	var term *terminal.Terminal
	st.OnResize = func(cols, rows int) {
		<-ready
		if term != nil {
			term.Resize(cols, rows)
		}
	}
	a.onMain(func() {
		defer close(ready)
		if _, wanted := a.panes[id]; !wanted || gen != a.gen {
			st.Close()
			return
		}
		var conn io.ReadWriteCloser = st
		if c.Remote() {
			// Files dropped on it go to the server first.
			conn = &uploadConn{
				Stream: st,
				upload: func(path string) (string, error) { return c.Upload(id, path) },
				report: func(msg string, failed bool) {
					a.onMain(func() {
						if failed {
							a.status = msg
						} else {
							a.notify(msg)
						}
					})
				},
			}
		}
		conn = &settledResize{ReadWriteCloser: conn, resize: st.ResizePixels}
		t, err := terminal.New(terminal.Options{
			Conn:         conn,
			ResizeByConn: true,
			Theme:        theme.Terminal(),
			DarkTheme:    theme.Terminal(),
			Font:         a.termFont(),
			OnZoom:       a.zoomTerms,
			Padding:      terminal.Padding{X: termPadX, Y: termPadY},
			OnClear: func(terminal.Clear) bool {
				go st.Clear()
				return true
			},
			OpenLink: func(link string) bool { return a.openLink(id, link) },
			// An image or files pasted reach the program as their paths
			// on its host, as agents take images.
			OnPaste: func(d transfer.Data) bool {
				s, ok := a.sessionByID(id)
				return ok && a.pasteFiles(s, d, func(paths string) {
					if term != nil {
						term.Paste(paths)
					}
				})
			},
		})
		if err != nil {
			st.Close()
			delete(a.panes, id)
			a.status = err.Error()
			return
		}
		term = t
		a.panes[id] = &pane{term: t, stream: st}
	})
}

func (p *pane) close() {
	if p == nil {
		return
	}
	p.term.Close()
	p.stream.Close()
}

func (a *App) selectSession(s proto.SessionInfo) {
	if s.Parent != "" {
		// A tab: its session, with it in front.
		a.showTab(s.Parent, s.ID)
		s = a.parentOf(s)
	}
	a.selected = s.ID
	a.page = ""
	delete(a.unread, s.ID)
	a.updateBadge()
	if s.Hibernated {
		a.continueSession(s) // shown, it wakes on its conversation
	}
	if a.showsChat(s.ID) {
		a.chatOf(s.ID).FocusComposer()
	}
	if s.Project != "" {
		a.project = s.Project
		delete(a.collapsed, s.Project)
	}
	if a.maximized != "" {
		a.maximized = s.ID
	}
}

// askDelete asks before deleting a session.
func (a *App) askDelete(s proto.SessionInfo) { a.deleting = &s }

func (a *App) killSession(id string) {
	c := a.client
	if c == nil {
		return
	}
	go func() {
		if err := c.KillSession(id); err != nil {
			a.onMain(func() { a.status = err.Error() })
		}
	}()
}

// openFolder opens the folder picker on the connected host.
func (a *App) openFolder() {
	if a.client == nil {
		return
	}
	a.addProject.Open(a.client, a.onMain)
}

// addProjectDialog is the folder picker; the folder picked becomes a
// project.
func (a *App) addProjectDialog(c *ui.Context) {
	a.addProject.View(c, a.host.Name, func(path string) {
		if cl := a.client; cl != nil {
			go a.addProjectPath(cl, path)
		}
	})
}

// pickHostFile asks for a file of the connected host, for pick.
func (a *App) pickHostFile(title, about string, pick func(path string)) {
	if a.client == nil {
		return
	}
	a.pickedFile = pick
	a.hostFile.OpenFile(a.client, a.onMain, title, about)
}

func (a *App) hostFileDialog(c *ui.Context) {
	a.hostFile.View(c, a.host.Name, func(path string) {
		if a.pickedFile != nil {
			a.pickedFile(path)
		}
	})
}

// addProjectPath runs off the main thread.
func (a *App) addProjectPath(c *client.Client, path string) {
	info, err := c.AddProject(path)
	a.onMain(func() {
		if err != nil {
			a.status = err.Error()
			return
		}
		a.status = ""
		a.project = info.Path
		a.selected = ""
		delete(a.collapsed, info.Path)
	})
}

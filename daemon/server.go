// Package daemon serves the enowX protocol: it owns sessions, projects and
// worktrees and streams terminal frames to every attached client.
package daemon

import (
	"encoding/json"
	"errors"
	"github.com/enowx/enowx/core/accounts"
	"github.com/enowx/enowx/core/checks"
	"github.com/enowx/enowx/core/commands"
	"github.com/enowx/enowx/core/credits"
	"github.com/enowx/enowx/core/display"
	"github.com/enowx/enowx/core/intercept"
	"github.com/enowx/enowx/core/vault"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/enowx/enowx/cloud/hostauth"
	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/core/decide"
	"github.com/enowx/enowx/core/features"
	"github.com/enowx/enowx/core/notes"
	"github.com/enowx/enowx/core/notify"
	"github.com/enowx/enowx/core/project"
	"github.com/enowx/enowx/core/projectlinks"
	"github.com/enowx/enowx/core/projectsync"
	"github.com/enowx/enowx/core/rag"
	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/core/usersync"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/version"
)

// Version is the daemon's version, reported in Hello and daemon.info.
var Version = version.String

// Server serves connections. One Server runs per user per machine.
type Server struct {
	exitsLogged sync.Map // session IDs whose end is in the log
	pipes       pipelines
	autos       automations
	hooks       *hookEngine // the user's hooks (hooks.go)
	// WorktreeRoot is where worktrees are created.
	WorktreeRoot string
	// Hash is the SHA-256 of the executable the daemon started from
	// (DaemonInfo.Hash).
	Hash string
	// HookBin is the program agents' hooks run to report their state
	// ("<HookBin> hook <state>"); empty leaves agents without hooks.
	HookBin string
	// Log receives diagnostics; nil discards them.
	Log *slog.Logger

	sessions          *session.Manager
	projects          *project.Store
	notes             *notes.Store
	userdata          *usersync.Folder // the user's data synced between hosts
	features          *features.Store  // which features the user has on
	featAsks          featureAsks      // agents asking to turn one on
	notify            *notify.Store    // how the apps notify, in the synced settings
	syncPeers         syncPeers
	catalogMu         sync.Mutex     // one catalog written at a time
	catalogs          sync.WaitGroup // catalogs being written
	clonePrefsMu      sync.Mutex     // one settings/clone.json written at a time
	dataDir           string
	clones            clones
	noteIO            noteImports
	accounts          *accounts.Store // agents' other accounts
	browsers          browsers        // apps doing enowx browser's actions
	persist           *persistence
	started           time.Time
	done              chan struct{}
	closeOnce         sync.Once
	canvases          *canvas.Store
	teams             teamSpaces       // the teams' notes and canvases (spaces.go)
	teamSync          teamSyncer       // their syncs with the cloud (teamsync.go)
	teamProjectsState teamProjectState // the teams\' projects found here (teamprojects.go)
	visitors          visitorList      // the teams\' members connected (visitors.go)
	share             hostSharing      // what the host shares, its team listener (hostshare.go)
	takeover          takeovers        // sessions visitors took over (takeover.go)
	canvasState       canvasState      // paused canvases, renders waiting
	unslopRuns        unslopRuns       // canvases' redesigns under way (canvas_unslop.go)
	stories           unslopStories    // their steps (canvas_unslop_steps.go)
	acctWatch         *accountWatch    // accounts' limits and their log (accounts_fallback.go)
	accountPrefs      *accounts.Prefs  // how agents' accounts are used, synced
	ragSettings       *rag.Settings    // RAG's settings and memories, synced (rag.go)
	rag               *rag.Engine
	ragRun            ragState
	judge             *decide.Engine // decisions (decide.go)
	decideRun         decideState
	tok               tokenState   // token usage (tokens.go)
	browserUnslops    unslopRuns   // Browsers' pages redesigned, by session (browser_unslop.go)
	canvasBuilds      buildRuns    // canvases' UI builds under way (canvas_build.go)
	shares            canvasShares // canvases shared by a link (canvas_share.go)
	vault             vaultState   // apps lending credentials, and what they lent (vault.go)

	transcripts transcripts
	statsState  stats
	credits     credits.Checker
	// commandCache keeps the agents' slash commands (see commands).
	commandCache commandCache
	checkStore   *checks.Store
	vars         sessionVars
	commandStore *commands.Store
	runner       checks.Runner
	sched        scheduler
	displays     displays
	syncHash     projectsync.Hasher // files' hashes, for syncs
	traf         traffic            // the HTTP interceptor (traffic.go)
	android      androidState       // Android devices (android.go)
	hibernates   hibernations       // sessions to hibernate once done (hibernate.go)
	daemonID     string             // DaemonInfo.ID
	links        *projectlinks.Store
	cloud        *cloudLink     // the host's sign-in to a team cloud
	pushes       autopushes     // auto pushes running (autopush.go)
	projWatches  projectWatches // folders watched (projectwatch.go)
	appReqs      appRequests    // what apps were asked (apprequest.go)

	mu    sync.Mutex
	conns map[*conn]struct{}
}

// NewServer returns a server keeping its state (projects, worktrees,
// sessions) under dir. Sessions saved by a previous daemon come back.
func NewServer(dir string) (*Server, error) { return newServer(dir, "") }

// newServer is NewServer with the program agents' hooks run (HookBin),
// set before saved sessions come back so they get it too.
func newServer(dir, hookBin string) (*Server, error) {
	forgetParentAgent()
	projects, err := project.Open(filepath.Join(dir, "projects.json"))
	if err != nil {
		return nil, err
	}
	noteStore := notes.Open(filepath.Join(dir, "notes"))
	// Icons of before SVG ones (emoji) move to theirs, once.
	if n, err := noteStore.MigrateIcons(); err != nil || n > 0 {
		slog.Default().Info("notes: icons moved to SVG", "notes", n, "err", err)
	}
	persist, err := openPersistence(dir)
	if err != nil {
		return nil, err
	}
	checkStore, err := checks.OpenStore(filepath.Join(dir, "checks.json"))
	if err != nil {
		return nil, err
	}
	commandStore, err := commands.OpenStore(filepath.Join(dir, "commands.json"))
	if err != nil {
		return nil, err
	}
	accountStore, err := accounts.Open(dir)
	if err != nil {
		return nil, err
	}
	s := &Server{
		WorktreeRoot: filepath.Join(dir, "worktrees"),
		HookBin:      hookBin,
		hooks:        newHookEngine(dir),
		acctWatch:    newAccountWatch(dir),
		accountPrefs: accounts.OpenPrefs(dir),
		sessions:     session.NewManager(),
		projects:     projects,
		notes:        noteStore,
		userdata:     usersync.OpenFolder(dir),
		features:     features.Open(dir),
		notify:       notify.Open(dir),
		dataDir:      dir,
		canvases:     canvas.Open(filepath.Join(dir, "canvas")),
		persist:      persist,
		checkStore:   checkStore,
		commandStore: commandStore,
		accounts:     accountStore,
		daemonID:     loadDaemonID(dir),
		links:        projectlinks.Open(dir),
		cloud:        &cloudLink{store: hostauth.Open(filepath.Join(dir, "cloud"))},
		started:      time.Now(),
		conns:        map[*conn]struct{}{},
		done:         make(chan struct{}),
	}
	s.vars.load(s.sessionEnvFile())
	s.userdata.Name, _ = os.Hostname()
	s.userdata.EmptyTrash(time.Now())
	s.publishSoon(s.projects.List())
	s.vault.audit = vault.OpenAudit(filepath.Join(dir, "vault", "audit.jsonl"))
	intercept.KnownSecrets(s.lentSecrets)
	s.loadShell()
	s.newRAG(dir)
	s.newDecide(dir)
	s.newTokens(dir)
	s.credits.Homes = func() []credits.Home { // each account's plan too
		var out []credits.Home
		for _, a := range s.accounts.List() {
			if a.Member == "" && a.Cred == "" {
				out = append(out, credits.Home{ID: a.ID, Agent: a.Agent, Name: a.Name, Path: s.accounts.Home(a.ID)})
			}
		}
		return out
	}
	s.sessions.OnUpdate = func(info proto.SessionInfo) {
		if info.Exited {
			// When and how each program ended, to tell later what ended it
			// (143 is SIGTERM from outside, say).
			if _, logged := s.exitsLogged.LoadOrStore(info.ID, true); !logged {
				s.logger().Info("session ended", "id", info.ID, "agent", info.Agent, "code", info.ExitCode)
			}
		} else {
			s.exitsLogged.Delete(info.ID)
		}
		s.sched.poke()
		if info.State == proto.StateDone || info.Exited {
			s.pokeTokens() // a turn ended: its tokens are recorded
		}
		if info.NotesFor != "" && (info.State == proto.StateDone || info.Exited) {
			go s.keepNoteChat(info.NotesFor, info.ID) // a turn ended: kept with the note
		}
		if sess, err := s.sessions.Get(info.ID); err == nil {
			s.persist.update(sess, nil)
		}
		s.broadcast(proto.EventSessionUpdated, info)
		s.watchSession(info)      // hooks seen from outside
		s.watchLimits(info)       // accounts at their limit
		s.sessionFolderSeen(info) // a team's project, maybe
	}
	s.sessions.OnPrompt = s.promptTyped // hooks on prompts, seen from outside
	if err := s.sched.open(settingsFile(dir)); err != nil {
		return nil, err
	}
	s.runner.OnChange = func(id string) { s.broadcast(proto.EventChecksChanged, proto.SessionRef{ID: id}) }
	// A session's checks run with its traffic caught, when it is, so its
	// project's mocks answer its tests.
	s.runner.Env = func(session string) []string { return s.trafficEnvFor(session, "") }
	s.restore()
	s.pipes.load(pipelinesFile(dir))
	s.resumePipelines()
	s.autos.load(automationsFile(dir))
	s.sweepTransfers()
	s.sweepFetches()
	go s.saveHistories(s.done)
	go s.teamSyncLoop(s.done)
	go s.startSharing() // a host shared before: its listener again
	go s.watchPorts(s.done)
	go s.runScheduler(s.done)
	go s.runAutomations(s.done)
	go s.watchGit(s.done)
	go s.runSchedules(s.done)
	go s.goBackLoop(s.done)
	go s.watchLimitsLoop(s.done)
	go s.runAutopushes(s.done)
	go s.sweepNoteAssets(s.done)
	go s.runTokens(s.done)
	return s, nil
}

// Serve accepts connections until l is closed.
func (s *Server) Serve(l net.Listener) error {
	for {
		nc, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.ServeConn(nc)
	}
}

// ServeConn serves one connection until it closes.
func (s *Server) ServeConn(nc net.Conn) { s.serveConn(nc, false) }

// serveConn serves one connection, a visitor's when remote.
func (s *Server) serveConn(nc net.Conn, remote bool) {
	c := newConn(s, nc)
	c.remote = remote
	s.mu.Lock()
	s.conns[c] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
		s.browsers.drop(c)
		s.projWatches.drop(c)
		s.appReqs.drop(c)
		s.vault.drop(c)
		s.visitorGone(c)
	}()
	if err := c.serve(); err != nil {
		s.logger().Debug("connection closed", "err", err)
	}
}

// Close saves every session for the next daemon, then hangs them up.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.stopSharing()
		s.clones.stopAll() // half-made clones go with them
		s.noteIO.stopAll()
		s.shares.mu.Lock()
		s.stopShares()
		s.shares.mu.Unlock()
		s.projWatches.stopAll()
		s.persist.stopping()
		var tags []tag
		for _, sess := range s.sessions.List() {
			s.persist.saveHistory(sess)
			tags = append(tags, tag{key: session.EnvSession, value: sess.ID()})
		}
		s.sessions.CloseAll()
		s.runner.StopAll()
		s.stopTraffic()
		s.android.logs.Close()
		s.vaultReleaseAll()
		tags = append(tags, s.displays.stopAll()...)
		// Sessions come back with the next daemon, and start what they
		// run again: nothing of theirs should run twice.
		reapAll(tags)
		s.catalogs.Wait()
	})
}

// deleteSession hangs up a session and forgets it for good.
func (s *Server) deleteSession(id string) error {
	if err := s.sessions.Kill(id); err != nil {
		return err
	}
	// What it started outside its terminal goes too: no leftovers.
	reap(tag{key: session.EnvSession, value: id})
	// Its tabs go with it.
	for _, tab := range s.sessions.List() {
		if tab.Meta().Parent == id && s.deleteSession(tab.ID()) == nil {
			s.broadcast(proto.EventSessionRemoved, proto.SessionRef{ID: tab.ID()})
		}
	}
	s.persist.remove(id)
	s.transcripts.forget(id)
	s.runner.Forget(id)
	s.unqueue(id)
	s.displays.stop(id)
	s.vaultRelease(id)
	_ = os.RemoveAll(s.uploadDir(id))
	return nil
}

func (s *Server) info() proto.DaemonInfo {
	host, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	info := proto.DaemonInfo{
		Version:  Version,
		Build:    version.Build(),
		PID:      os.Getpid(),
		Started:  s.started,
		Sessions: len(s.sessions.List()),
		Hostname: host,
		Home:     home,
		Hash:     s.Hash,
		ID:       s.daemonID,
	}
	info.Display, info.DisplayNote = display.Available()
	if !info.Display {
		info.DisplayInstall = display.InstallCommand()
	}
	return info
}

func (s *Server) broadcast(name string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	ev, err := json.Marshal(proto.Event{Name: name, Data: raw})
	if err != nil {
		return
	}
	s.mu.Lock()
	conns := make([]*conn, 0, len(s.conns))
	for c := range s.conns {
		if c.greeted.Load() && !c.remote { // a visitor gets no event unasked
			conns = append(conns, c)
		}
	}
	s.mu.Unlock()
	for _, c := range conns {
		c.send(proto.Frame{Type: proto.TypeEvent, Payload: ev})
	}
}

func (s *Server) logger() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.New(slog.DiscardHandler)
}

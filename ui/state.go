package ui

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// appState is what the app remembers between launches, in StateFile: the
// host it showed, and per host the project and session that were open.
type appState struct {
	Host             string            `json:"host,omitempty"`
	Projects         map[string]string `json:"projects,omitempty"` // host ID → project path
	Sessions         map[string]string `json:"sessions,omitempty"` // host ID → session ID
	SkipPermissions  *bool             `json:"skipPermissions,omitempty"`
	Chats            map[string]bool   `json:"chats,omitempty"`            // session IDs shown as a chat
	NotesAgents      map[string]string `json:"notesAgents,omitempty"`      // note ID → the agent its Notes page talks to
	CanvasGridHidden map[string]bool   `json:"canvasGridHidden,omitempty"` // canvases whose guides are hidden, on this device
	CanvasChatWidth  float32           `json:"canvasChatWidth,omitempty"`  // the Canvas page's agent panel, as the user left it
	NotesView        map[string]string `json:"notesView,omitempty"`        // "<note ID> <what>" → how its page was left: tabs, toggles, outline
	CloneDirs        map[string]string `json:"cloneDirs,omitempty"`        // host ID → the folder cloned into last
	Theme            string            `json:"theme,omitempty"`            // palette ID
	ThemeMode        string            `json:"themeMode,omitempty"`        // dark (default), light, or system
	// BackgroundImage is this device's image behind the panels, "" for the
	// theme's own; BackgroundOpacity how opaque the panels over it are, 0 for
	// the default; BackgroundBlur its blur's sigma, nil for the default.
	BackgroundImage   string             `json:"backgroundImage,omitempty"`
	BackgroundOpacity float64            `json:"backgroundOpacity,omitempty"`
	BackgroundBlur    *float64           `json:"backgroundBlur,omitempty"`
	Accent            string             `json:"accent,omitempty"`       // #RRGGBB replacing the palette's accent
	TermFont          string             `json:"termFont,omitempty"`     // the terminals' font family; the system's monospaced font when empty
	TermFontSize      float32            `json:"termFontSize,omitempty"` // the terminals' text size; 13 when 0
	RailWide          bool               `json:"railWide,omitempty"`     // the rail shows labels
	Space             string             `json:"space,omitempty"`        // the team whose space Notes and Canvas show, "" the user's own
	Layouts           map[string]layout  `json:"layouts,omitempty"`      // "<host ID> <project path>" → layout
	Panel             sidePanel          `json:"panel,omitempty"`
	ChatRead          map[string]string  `json:"chatRead,omitempty"` // the last message read of each team's chat
	Discord           discordPrefs       `json:"discord,omitempty"`
	Updates           updatePrefs        `json:"updates,omitempty"`
	Editor            string             `json:"editor,omitempty"`       // the editor last opened (editor.id)
	Previews          map[string]preview `json:"previews,omitempty"`     // session ID → where its browser was
	BrowserViews      map[string]bool    `json:"browserViews,omitempty"` // session IDs showing the Browser as their view
	TrafficViews      map[string]bool    `json:"trafficViews,omitempty"` // session IDs showing Traffic as their view
	AutoPreview       map[string][]int   `json:"autoPreview,omitempty"`  // "<host ID> <project>" → ports previewed when opened
	QuietPorts        bool               `json:"quietPorts,omitempty"`   // no card when a session opens a port
	CloseQuits        bool               `json:"closeQuits,omitempty"`   // closing the window quits, rather than keeping the app in the tray
	// What New session starts with (Settings): an agent, the project
	// folder rather than a new branch, a time budget in minutes.
	DefaultAgent  string `json:"defaultAgent,omitempty"`
	ProjectFolder bool   `json:"projectFolder,omitempty"`
	DefaultBudget int    `json:"defaultBudget,omitempty"`
	NotifyOff     bool   `json:"notifyOff,omitempty"` // no desktop notifications
	// VaultRemember keeps the vault's key in the system's keychain.
	VaultRemember bool `json:"vaultRemember,omitempty"`
	// OAuthClients are the user's OAuth apps, by platform, for signing
	// in in the browser (not secret).
	OAuthClients map[string]string `json:"oauthClients,omitempty"`
	// NotionSyncs are imports from Notion made again on their own.
	NotionSyncs []notionSync `json:"notionSyncs,omitempty"`
	// StopAgentsOnQuit stops this device's agents when the app quits; they
	// keep working without it otherwise.
	StopAgentsOnQuit bool `json:"stopAgentsOnQuit,omitempty"`
	// FloatingControl keeps the radial panel's global shortcut bound, to
	// summon it over any app.
	FloatingControl bool `json:"floatingControl,omitempty"`
	// FloatingFoldOnBlur folds the panel away when the user goes
	// elsewhere; FloatingAtPointer shows it where the cursor is. Pointers
	// so that an app which never saved them keeps both on.
	FloatingFoldOnBlur *bool `json:"floatingFoldOnBlur,omitempty"`
	FloatingAtPointer  *bool `json:"floatingAtPointer,omitempty"`
	// Floating appearance stays independent of the app and terminal theme.
	FloatingAccent       string `json:"floatingAccent,omitempty"`
	FloatingTint         bool   `json:"floatingTint,omitempty"`
	FloatingCompact      bool   `json:"floatingCompact,omitempty"`
	FloatingReduceMotion bool   `json:"floatingReduceMotion,omitempty"`
	// SessionsByProject lists the sessions under their projects, rather
	// than by state.
	SessionsByProject bool `json:"sessionsByProject,omitempty"`
	// HideSessions folds the sidebar away, giving the session the room.
	HideSessions bool `json:"hideSessions,omitempty"`
	// GOA shows GOD of Agents mode (goa.go).
	GOA bool `json:"goa,omitempty"`
}

// palette is the palette the user picked, with their accent.
func (st appState) palette(light bool) theme.Palette {
	var accent ui.Color
	if st.Accent != "" {
		accent = ui.Hex(st.Accent)
	}
	p := theme.LookupAppearance(st.Theme, accent, light)
	if st.BackgroundImage != "" {
		p = theme.WithBackdrop(p, st.backgroundOpacity())
	}
	return p
}

// backgroundOpacity is how opaque the panels over the user's image are.
func (st appState) backgroundOpacity() float64 {
	if st.BackgroundOpacity > 0 {
		return st.BackgroundOpacity
	}
	return backgroundDefaultOpacity
}

// backgroundBlur is the sigma of the blur over the user's image.
func (st appState) backgroundBlur() float64 {
	if st.BackgroundBlur != nil {
		return *st.BackgroundBlur
	}
	return backgroundDefaultBlur
}

func loadState(path string) appState {
	var st appState
	if path == "" {
		return st
	}
	data, err := os.ReadFile(path)
	if err != nil {
		st.RailWide = true // who opens the app the first time sees the list whole
		return st
	}
	_ = json.Unmarshal(data, &st)
	return st
}

// saveState writes the state when it changed. It runs on the main thread
// at the end of each frame, so every way of switching host, project or
// session is remembered without each one saying so.
func (a *App) saveState() {
	if a.opts.StateFile == "" || a.host.ID == "" {
		return
	}
	st := a.state
	st.Host = a.host.ID
	if st.Projects == nil {
		st.Projects, st.Sessions = map[string]string{}, map[string]string{}
	}
	if a.client != nil && a.restoredView {
		st.Projects[a.host.ID] = a.project
		st.Sessions[a.host.ID] = a.selected
	}
	skip := a.skipPermissions
	st.SkipPermissions = &skip
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil || string(data) == a.savedState {
		return
	}
	if err := os.MkdirAll(filepath.Dir(a.opts.StateFile), 0o700); err != nil {
		return
	}
	tmp := a.opts.StateFile + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil && os.Rename(tmp, a.opts.StateFile) == nil {
		a.state, a.savedState = st, string(data)
	}
}

// showRestored shows s again as the view was: not a user's pick, so it leaves
// the page and the keyboard as they are, and a hibernated session asleep.
func (a *App) showRestored(s proto.SessionInfo) {
	if s.Parent != "" {
		a.showTab(s.Parent, s.ID)
		s = a.parentOf(s)
	}
	a.selected = s.ID
	if s.Project != "" {
		a.project = s.Project
	}
}

// restoreView reopens the project and session that were open on this host,
// once its state has loaded.
func (a *App) restoreView() {
	if a.restoredView {
		return
	}
	a.restoredView = true
	if p := a.state.Projects[a.host.ID]; p != "" {
		for _, proj := range a.projects {
			if proj.Path == p {
				a.project = p
			}
		}
	}
	if id := a.state.Sessions[a.host.ID]; id != "" {
		for _, s := range a.sessions {
			if s.ID == id {
				a.showRestored(s)
			}
		}
	}
}

// Package icon holds the app's SVG icons, drawn in one stroke style.
package icon

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

//go:embed svg/*.svg
var files embed.FS

func load(name string) *ui.SVG {
	data, err := files.ReadFile("svg/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}

// The icons.
var (
	Plus         = load("plus")
	Monitor      = load("monitor")
	EnterKey     = load("corner-down-left")
	Languages    = load("languages")
	PanelRight   = load("panel-right")
	ArrowLeft    = load("arrow-left")
	ArrowRight   = load("arrow-right")
	Browser      = load("browser")
	AppWindow    = load("app-window")
	Play         = load("play")
	Moon         = load("moon") // hibernating
	Settings     = load("settings")
	Target       = load("target")
	Camera       = load("camera")
	Code         = load("code")
	External     = load("external")
	ChevronDown  = load("chevron-down")
	ChevronRight = load("chevron-right")
	ChevronsUD   = load("chevrons-up-down")
	Git          = load("git")
	Folder       = load("folder")
	FolderPlus   = load("folder-plus")
	Server       = load("server")
	Laptop       = load("laptop")
	Maximize     = load("maximize")
	Minus        = load("minus")
	FitWidth     = load("fit-width")
	Restore      = load("restore")
	Terminal     = load("terminal")
	Check        = load("check")
	Close        = load("x")
	Tailscale    = load("tailscale")
	Discord      = load("discord") // its logo, for what labels Discord itself
	Trash        = load("trash")
	Sync         = load("sync")
	Lock         = load("lock")
	Key          = load("key")
	Account      = load("account") // a person in a circle: the cloud account
	ArrowUp      = load("arrow-up")
	Home         = load("home")
	Chat         = load("chat")
	Usage        = load("usage")
	Split        = load("split")
	More         = load("more")
	Search       = load("search")
	Alert        = load("alert")
	Palette      = load("palette")
	Sidebar      = load("sidebar")
	Credits      = load("credits")
	// Logo is enowX's mascot, traced to one shape so it takes any color.
	Logo     = load("logo")
	Collapse = load("collapse")
	// The chat's: tools, thoughts, the composer.
	Copy   = load("copy")
	Brain  = load("brain")
	File   = load("file")
	Pencil = load("pencil")
	// A team chat's message: answer it, react to it.
	Reply     = load("reply")
	SmilePlus = load("smile-plus")
	// A canvas's Unslop.
	Wand    = load("wand")
	Traffic = load("traffic")
	Android = load("android")
	Send    = load("send")
	// A device's buttons, under its screen.
	Power      = load("power")
	ArrowDown  = load("arrow-down")
	VolumeUp   = load("volume-up")
	VolumeDown = load("volume-down")
	Globe      = load("globe")
	Eye        = load("eye") // someone watching
	EyeOff     = load("eye-off")
	AtSign     = load("at-sign") // a mention
	Users      = load("users")   // a team's members
	Stop       = load("stop")
	Paperclip  = load("paperclip")
	Slash      = load("slash")
	CPU        = load("cpu")
	Shield     = load("shield")
	Gauge      = load("gauge")
	Bot        = load("agentbot")
	Sparkle    = load("sparkle") // the agent's own words, in the chat's timeline
	List       = load("list")
	ListChecks = load("list-checks")
	Notebook   = load("notebook")
	// A note's layout blocks.
	Info       = load("info")
	Lightbulb  = load("lightbulb")
	Link       = load("link")
	Image      = load("image")
	Columns    = load("columns")
	LayoutGrid = load("layout-grid")
	Tabs       = load("tabs")
	ListTree   = load("list-tree")
	Frame      = load("frame")
	Grip       = load("grip")
	// The Canvas page's: its tools and actions.
	Canvas      = load("canvas")
	Square      = load("square")
	Circle      = load("circle")
	Diamond     = load("diamond")
	Type        = load("type")
	Sticky      = load("sticky")
	ArrowLink   = load("arrow-link")
	Undo        = load("undo")
	Redo        = load("redo")
	Pause       = load("pause")
	History     = load("history")
	Comment     = load("comment")
	Download    = load("download")
	Layers      = load("layers")
	Star        = load("star")
	Award       = load("award")
	Grid        = load("grid")
	ChevronLeft = load("chevron-left")
	ChevronUp   = load("chevron-up")
	Floating    = load("floating")
	agents      = map[string]*ui.SVG{
		"claude":      load("claude"),
		"codex":       load("codex"),
		"opencode":    load("opencode"),
		"gemini":      load("gemini"),
		"kiro":        load("kiro"),
		"omp":         load("omp"),
		"antigravity": load("antigravity"),
		"cline":       load("cline"),
		"hermes":      load("hermes"),
		"shell":       Terminal,
		// Claude Code with its teams of agents.
		"claude-agent-teams": load("claude"),
		"claude-platform":    load("claude"),
		"custom":             Plus,
	}
)

// Agent returns the icon of an agent ID: its logo where enowX has one,
// else a bot ("" and unknown commands get the terminal's).
func Agent(id string) *ui.SVG {
	if s, ok := agents[id]; ok {
		return s
	}
	if id == "" {
		return Terminal
	}
	return Bot
}

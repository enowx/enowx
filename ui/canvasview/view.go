// Package canvasview is the Canvas page: the connected host's canvases
// (mockups, wireframes, diagrams, boards), each shown as its frames, which
// the user edits, comments on and exports, and watches agents design live.
// The app gives it the canvases and how to change them through Options.
package canvasview

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/unslopview"
	"github.com/enowx/enowx/ui/widget"
)

// Options are what the page shows and does: the host's canvases, and
// calls to it, whose done funcs run on the main thread.
type Options struct {
	Host string
	// Space is the name of the team whose space Canvases are, "" for the
	// user's own; Team its ID.
	Space, Team string
	// Spaces are the spaces the page may switch to, the user's own first;
	// PickSpace switches to one.
	Spaces    []widget.Space
	PickSpace func(team string)
	Canvases  []proto.CanvasInfo
	// GridHidden says whether this device hides the guides of canvas id;
	// SetGridHidden keeps it.
	GridHidden    func(id string) bool
	SetGridHidden func(id string, hidden bool)
	Loaded        bool
	Err           string
	Load          func(id string, done func(proto.Canvas, error))
	Create        func(p proto.CanvasCreateParams, done func(proto.Canvas, error))
	Delete        func(id string, done func(error))
	Edit          func(e proto.CanvasEdit, done func(proto.Canvas, error))
	History       func(id string, done func(proto.CanvasHistory, error))
	Version       func(id string, n int, done func(proto.Canvas, error))
	Undo          func(id string, done func(proto.Canvas, error))
	Redo          func(id string, done func(proto.Canvas, error))
	Restore       func(id string, n int, done func(proto.Canvas, error))
	Comments      func(id string, done func([]proto.CanvasComment, error))
	Comment       func(p proto.CanvasCommentParams, done func([]proto.CanvasComment, error))
	Control       func(id, action string, done func(error))
	// Render renders a frame (or, for html without one, the canvas) as
	// format: png, pdf, svg or html.
	Render func(id, frame, format string, done func([]byte, error))
	// PictureCached gives done the picture kept on this device as key, nil for
	// none; PictureKeep keeps one. Both leave the main thread; done comes back to it.
	PictureCached func(key string, done func([]byte))
	PictureKeep   func(key string, data []byte)
	// SaveFile asks where to save data, named name, and saves it.
	SaveFile func(name string, data []byte)
	// Projects are the host's projects, to attach a canvas to; MakeCode
	// has an agent turn canvas c into code in project.
	Projects []proto.ProjectInfo
	MakeCode func(c proto.Canvas, project string)
	// MoveSpace opens moving canvas c to another space (the user's own or
	// a team's); nil while there is none.
	MoveSpace func(c proto.CanvasInfo)
	// Share makes canvas id's read-only link, or stops it (nil: no
	// sharing).
	Share func(id string, stop bool, done func(proto.CanvasShare, error))
	// Confirm does do once the user agrees (title, what is lost, the
	// action); nil does it at once.
	Confirm func(title, msg, action string, do func())
	// AgentName names the agent of a session.
	AgentName func(session string) string
	// Web shows a mockup's HTML: nil where this device has no web view.
	Web *Web
	// CanAsk offers Ask an agent, whose panel the app draws beside the
	// page while AssistantOpen; AskAgent puts a draft in its composer,
	// SendAgent sends it a message.
	CanAsk    bool
	AskAgent  func(draft string)
	SendAgent func(text string)
	// Unslop has the canvas's agent redesign design frame (a mockup's)
	// where it looks AI-made, on a copy; nil, it isn't offered. Unslopping
	// says one is on its way.
	Unslop     func(frame string)
	Unslopping bool
	// UnslopChoose offers which passes to run before unslopping frame;
	// UnslopReplay plays a redesign of frame back from step k.
	UnslopChoose func(frame string)
	UnslopReplay func(frame string, k int)
	// UnslopErr is why the last unslop didn't start, shown over the design
	// until UnslopDismiss.
	UnslopErr     string
	UnslopDismiss func()
	// Build starts a UI build on the canvas shown (image, a screenshot to
	// recreate, uploaded first); Building while it starts, BuildErr why
	// it didn't, BuildRun the build under way.
	Build    func(p proto.CanvasBuildParams, image []byte)
	Building bool
	BuildErr string
	BuildRun *proto.CanvasBuildRun
	// BuildDismiss clears BuildErr.
	BuildDismiss func()
	// PickDirection picks design frame of a build, with what to keep and
	// change, for its agent to refine.
	PickDirection func(cv proto.Canvas, frame, keep, change string)
	// ListOutside: the app draws the canvases' list in its own sidebar
	// (DrawList), so the page leaves it out.
	ListOutside bool
	// RenderWidth renders a design reflowed at width (a phone's).
	RenderWidth func(id, frame string, width int, done func([]byte, error))
	// OpenReviewer shows the session of an unslop's reviewer.
	OpenReviewer func(id string)
	// UnslopCompare opens a done run's before and after, of canvas id.
	UnslopCompare func(id string, run proto.CanvasUnslopRun)
	// UnslopRun is the shown canvas's redesign under way, nil when none:
	// shown over its design as it goes (unslop.go). AgentState says what
	// a region's agent is at (AgentWorking, …); OpenSession shows one.
	UnslopRun   *proto.CanvasUnslopRun
	AgentState  func(session string) string
	OpenSession func(session string)
	// Viewing is the region agent shown in the agent panel, "" none.
	Viewing string
}

// designingFor is how long after an agent's change it counts as designing.
const designingFor = 8 * time.Second

// Modes of the stage.
const (
	modeDesign   = 0 // select, move, resize, edit
	modeInteract = 1 // the mockup works as the real thing
)

// View is the Canvas page's state. It belongs to the main thread.
type View struct {
	selected string
	canvas   *proto.Canvas // the canvas open, as last read
	loading  bool
	err      string
	frame    string // the frame shown: its ID
	mode     int
	// preview shows a mockup frame at a device's width, 0 its own.
	preview int
	scale   float32       // the stage's zoom, as the last frame drew it
	cam     camera        // where the design shows on the stage (camera.go)
	comp    *componentSel // the mockup's component picked (component.go)
	sel     string        // the element selected
	// last is the latest change an agent made, highlighted a moment.
	last   proto.CanvasChange
	lastAt time.Time
	panel  string // the right panel: "layers", "comments", "versions", "grid"
	// panelBack is the panel the agent's made room for, or the one last hidden:
	// what shows again; panelSet: the page has had its first panel.
	panelBack string
	panelSet  bool
	comments  []proto.CanvasComment
	history   *proto.CanvasHistory
	compare   *compareState
	drag      *dragState
	editing   *textEdit
	connect   string // a shape to draw an arrow from, picked first
	draft     string // a comment being written
	creating  *createForm
	briefing  *briefForm
	building  *buildForm
	// buildClosed is the build the user closed when done.
	buildClosed string
	deleting    *proto.CanvasInfo
	renaming    *string
	sharing     *shareState // the Share link dialog, open
	menu        widget.ContextMenu
	filter      string
	htmlEdit    *htmlEdit
	web         *Web
	// assistant shows the agent's panel beside the page.
	assistant bool
	hud       unslopview.Panel // a redesign's panel (unslop.go)
	opened    int              // openings, so an earlier one's load is let go
	// grid is the Grid panel's fields; hideGrid hides the guides.
	gridF    *gridForm
	hideGrid bool
	// width is the page's, as the last frame laid it out: what shows
	// beside the stage depends on the room.
	width float32
	// listOpen shows the canvases' list in a narrow page, where it hides.
	listOpen bool
	// view is the designs' view: one at a time, all as a grid, or a few
	// side by side (compare, frame IDs).
	view   int
	cmpSel map[string]bool
	cmp    designCompare
	// listInSidebar: the list draws in the app's sidebar (DrawList);
	// listOutside: it does, so the page has no list of its own.
	listInSidebar, listOutside bool
	thumbs                     map[string]*thumb
	thumbsBusy                 int // pictures asked for, not back yet
	descr                      *describeForm
	// pendingFrame is a design to show once the canvas opening loaded.
	pendingFrame string
	pendingAt    time.Time
}

// Views of the designs.
const (
	viewDesign  = 0
	viewGrid    = 1
	viewCompare = 2
)

// WebOf is the view's web view of mockups, made by make the first time;
// nil where make gives none (no web view on this device).
func (v *View) WebOf(make func() *Web) *Web {
	if v.web == nil && make != nil {
		v.web = make()
	}
	return v.web
}

// Web is the view's web view of mockups, nil before WebOf made one.
func (v *View) Web() *Web { return v.web }

// AssistantOpen says the agent's panel shows.
func (v *View) AssistantOpen() bool { return v.assistant && v.canvas != nil }

// OpenAssistant shows the agent's panel, the side panel making room.
func (v *View) OpenAssistant() {
	v.assistant = true
	if v.panel != "" {
		v.panelBack = v.panel
	}
	v.panel = ""
}

// TogglePanel shows or hides a panel beside the stage: "layers",
// "comments", "versions" or "grid".
func (v *View) TogglePanel(o Options, id string) { v.togglePanel(o, id) }

// SetView shows the designs one at a time (0), as a grid (1), or those
// picked side by side (2).
func (v *View) SetView(view int) { v.view = view }

// NewCanvas opens the New canvas dialog.
func (v *View) NewCanvas() { v.creating = &createForm{open: true} }

// CloseAssistant hides the agent's panel.
func (v *View) CloseAssistant() {
	v.assistant = false
	if v.panel == "" {
		v.panel = v.panelBack
	}
}

// Selection is the design shown (its frame's ID) and the element
// selected, "" for none: what the agent is told the user looks at.
func (v *View) Selection() (frame, element string) { return v.frame, v.sel }

// Canvas is the canvas open as last read, nil for none.
func (v *View) Canvas() *proto.Canvas { return v.canvas }

// Selected is the canvas open, "" for none.
func (v *View) Selected() string { return v.selected }

// Open opens canvas id, as asked from elsewhere (an agent's canvas open).
func (v *View) Open(o Options, id string) {
	if v.selected == id && v.canvas != nil {
		return
	}
	v.selected, v.canvas, v.sel, v.frame, v.history, v.compare = id, nil, "", "", nil, nil
	v.hideGrid = o.GridHidden != nil && o.GridHidden(id)
	v.loading = true
	v.opened++
	opened := v.opened
	o.Load(id, func(c proto.Canvas, err error) {
		if v.opened != opened {
			return // an earlier opening's, older than what came since
		}
		v.loading = false
		if v.selected != id {
			return
		}
		if err != nil {
			v.err = err.Error()
			return
		}
		v.show(o, c)
		if v.pendingFrame != "" {
			if i, err := canvas.FrameIndex(c, v.pendingFrame); err == nil {
				v.frame = c.Frames[i].ID
			}
			v.pendingFrame = ""
		}
		v.loadComments(o)
	})
}

// show takes c as the canvas open.
func (v *View) show(o Options, c proto.Canvas) {
	old := v.canvas
	v.canvas = &c
	if old != nil && old.ID != c.ID {
		v.thumbs = nil // another canvas's pictures: the disk keeps them
	}
	if !slices.ContainsFunc(c.Frames, func(f proto.CanvasFrame) bool { return f.ID == v.frame }) && len(c.Frames) > 0 {
		v.frame = c.Frames[0].ID
	}
	if o.Web != nil {
		o.Web.show(c, v.frameOf(), old)
	}
}

// pendingFor is how long a design asked for may still come and be shown.
const pendingFor = 10 * time.Second

// Showing is the canvas open and the design shown.
func (v *View) Showing() (id, frame string) { return v.selected, v.frame }

// Apply takes a change the host told: the canvas open as it is now, and
// the element an agent changed, highlighted.
func (v *View) Apply(o Options, ch proto.CanvasChange) {
	if ch.Canvas.ID != v.selected {
		return
	}
	if ch.Session != "" {
		v.last, v.lastAt = ch, time.Now()
		// The view stays where the user put it; the strip marks where the
		// agent works (working).
	}
	old := v.canvas
	c := ch.Canvas
	v.canvas = &c
	if !slices.ContainsFunc(c.Frames, func(f proto.CanvasFrame) bool { return f.ID == v.frame }) && len(c.Frames) > 0 {
		v.frame = c.Frames[0].ID
	}
	// A design asked for before the change that made it came.
	if v.pendingFrame != "" {
		if time.Since(v.pendingAt) > pendingFor {
			v.pendingFrame = "" // long after the click: it would be a move of its own
		} else if i, err := canvas.FrameIndex(c, v.pendingFrame); err == nil {
			v.frame, v.sel, v.pendingFrame = c.Frames[i].ID, "", ""
		}
	}
	if o.Web != nil {
		o.Web.apply(c, v.frameOf(), old, ch)
	}
	if ch.Op == "comment" {
		v.loadComments(o)
	}
	if v.history != nil {
		v.loadHistory(o)
	}
}

func (v *View) loadComments(o Options) {
	id := v.selected
	o.Comments(id, func(list []proto.CanvasComment, err error) {
		if err == nil && v.selected == id {
			v.comments = list
		}
	})
}

func (v *View) loadHistory(o Options) {
	id := v.selected
	o.History(id, func(h proto.CanvasHistory, err error) {
		if err == nil && v.selected == id {
			v.history = &h
		}
	})
}

// frameOf is the frame shown.
func (v *View) frameOf() proto.CanvasFrame {
	if v.canvas == nil {
		return proto.CanvasFrame{}
	}
	for _, f := range v.canvas.Frames {
		if f.ID == v.frame {
			return f
		}
	}
	if len(v.canvas.Frames) > 0 {
		return v.canvas.Frames[0]
	}
	return proto.CanvasFrame{}
}

// designing says an agent is designing the canvas open: it changed it
// moments ago.
func (v *View) designing(now time.Time) bool {
	return v.canvas != nil && v.canvas.Agent != "" && now.Sub(v.canvas.AgentAt) < designingFor
}

// edit makes e to the canvas open, the canvas as the host has it after.
func (v *View) edit(o Options, e proto.CanvasEdit) {
	if v.canvas == nil {
		return
	}
	e.ID = v.canvas.ID
	if e.Frame == "" {
		e.Frame = v.frame
	}
	o.Edit(e, func(c proto.Canvas, err error) {
		if err != nil {
			v.err = err.Error()
			return
		}
		if v.canvas != nil && c.ID == v.canvas.ID && c.Version >= v.canvas.Version {
			v.show(o, c)
		}
	})
}

// Draw builds the page: the canvases beside the one open.
func (v *View) Draw(c *ui.Context, o Options) {
	if v.selected == "" && len(o.Canvases) > 0 && v.creating == nil {
		v.Open(o, o.Canvases[0].ID)
	}
	v.listOutside = o.ListOutside
	if !v.panelSet {
		// Layers show until the user says otherwise, as the frame's right side.
		v.panelSet = true
		if v.panel == "" && !v.assistant {
			v.panel = "layers"
		}
	}
	row := ui.Row(c).Grow(1).FillWidth().FillHeight().MinHeight(0).TextColor(theme.Text)
	if w := row.Bounds().W; w > 0 {
		v.width = w
	}
	row.Children(func() {
		if v.listShown() && !o.ListOutside {
			v.sidebar(c, o)
		}
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
			switch {
			case v.canvas == nil && v.loading:
				ui.Text(c, "Opening…").FontSize(theme.TextLabel).TextColor(theme.Muted).Padding(theme.S5)
			case v.canvas == nil:
				v.empty(c, o)
			default:
				v.page(c, o)
			}
		})
	})
	v.createDialog(c, o)
	v.briefDialog(c, o)
	v.buildDialog(c, o)
	v.deleteDialog(c, o)
	v.renameDialog(c, o)
	v.shareDialog(c, o)
	v.describeDialog(c, o)
}

// empty is the page without a canvas open.
func (v *View) empty(c *ui.Context, o Options) {
	ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
		widget.EmptyState(c, icon.Canvas, "No canvas open",
			"Mockups, wireframes and diagrams, which you and agents design together: ask an agent for one, or make one here.",
			func() {
				if widget.Button(c, widget.Primary, nil, "New canvas", "").Clicked() {
					v.creating = &createForm{open: true, kind: 0}
				}
			})
		if o.Err != "" {
			ui.Text(c, o.Err).FontSize(theme.TextUI).TextColor(theme.Error)
		}
	})
}

// Room the page needs for each thing beside the stage, so the stage keeps
// a useful width at any window size.
const (
	listRoom    = 1000 // the canvases' list shows from this wide on
	stripRoom   = 480  // the designs' strip, from this much room for the stage on
	labelRoom   = 1060 // the header's buttons keep their words from this wide on
	viewRoom    = 700  // its view switch keeps its words from this wide on
	actionsRoom = 560  // the header's actions stand in a row from this wide on
	toolsRoom   = 360  // the stage's tools stand in a row from this wide on
	headingRoom = 64   // the least of the header left to the canvas's title
	listWidth   = 248
	panelWidth  = 272
)

// listShown says the canvases' list shows: in a page wide enough, or when
// asked for.
func (v *View) listShown() bool { return v.width == 0 || v.width >= listRoom || v.listOpen }

// DrawList draws the canvases' list where the app puts it (its sidebar),
// filling the room it is given.
func (v *View) DrawList(c *ui.Context, o Options) {
	v.listInSidebar = true
	defer func() { v.listInSidebar = false }()
	v.sidebar(c, o)
}

// listColumn is the list's column: beside the page, or filling the app's
// sidebar.
func (v *View) listColumn(c *ui.Context) ui.Element {
	if v.listInSidebar {
		return ui.Column(c.Key("canvas-list")).FillWidth().Grow(1).MinHeight(0)
	}
	return ui.Column(c.Key("canvas-list")).Width(listWidth).Shrink(0).FillHeight().BorderWidth(0, theme.Rule, 0, 0).BorderColor(theme.Border)
}

// pageRoom is about how wide the frame's page is, its own list left out.
func (v *View) pageRoom() float32 {
	if v.listShown() && !v.listOutside {
		return v.width - listWidth
	}
	return v.width
}

// stageRoom is about how wide the stage's side of the page is.
func (v *View) stageRoom() float32 {
	if v.width == 0 {
		return 1200
	}
	room := v.pageRoom()
	if v.panel != "" {
		room -= panelWidth
	}
	return room
}

// compact says the header's buttons show as icons alone.
func (v *View) compact() bool { return v.width > 0 && v.pageRoom() < labelRoom }

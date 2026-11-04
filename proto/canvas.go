package proto

import "time"

// Canvases: mockups of UIs (HTML and CSS), wireframes, diagrams and free
// boards on a host, which the user and agents design together. An agent's
// changes reach every app as it makes them, an element at a time.
const (
	MethodCanvasList    = "canvas.list"    // → []CanvasInfo
	MethodCanvasGet     = "canvas.get"     // CanvasRef → Canvas
	MethodCanvasCreate  = "canvas.create"  // CanvasCreateParams → Canvas
	MethodCanvasDelete  = "canvas.delete"  // CanvasRef
	MethodCanvasEdit    = "canvas.edit"    // CanvasEdit → Canvas
	MethodCanvasHistory = "canvas.history" // CanvasRef → CanvasHistory
	MethodCanvasVersion = "canvas.version" // CanvasVersionRef → Canvas (as it was then)
	MethodCanvasUndo    = "canvas.undo"    // CanvasRef → Canvas
	MethodCanvasRedo    = "canvas.redo"    // CanvasRef → Canvas
	MethodCanvasRestore = "canvas.restore" // CanvasVersionRef → Canvas (a new version, as it was then)
	// MethodCanvasComments and MethodCanvasComment read and change the
	// comments on a canvas's elements.
	MethodCanvasComments = "canvas.comments" // CanvasRef → []CanvasComment
	MethodCanvasComment  = "canvas.comment"  // CanvasCommentParams → []CanvasComment
	// MethodCanvasControl pauses an agent designing a canvas, lets it go
	// on, or stops it.
	MethodCanvasControl  = "canvas.control"  // CanvasControl → CanvasInfo
	MethodCanvasAsset    = "canvas.asset"    // CanvasAsset → CanvasAsset (its Name, Data left out)
	MethodCanvasAssetGet = "canvas.assetGet" // CanvasAsset (ID, Name) → CanvasAsset with its Data
	// MethodCanvasRender renders a frame as a PNG or a PDF: a frame of
	// shapes alone by the daemon, one with HTML by an app connected to the
	// host (EventCanvasRender), which answers with MethodCanvasRenderReply.
	MethodCanvasRender      = "canvas.render"      // CanvasRenderParams → CanvasRendered
	MethodCanvasRenderReply = "canvas.renderReply" // CanvasRenderReply
	// MethodCanvasServe tells the daemon this connection renders frames but serves no
	// Browsers: an app only watching this host, showing another (the event's sent to
	// it when no app here serves Browsers).
	MethodCanvasServe = "canvas.serve"
)

// Canvas events.
const (
	// EventCanvasChanged carries a CanvasChange: a canvas after a change.
	EventCanvasChanged = "canvas.changed"
	// EventCanvasesChanged carries the full []CanvasInfo after a canvas
	// came, went or was renamed.
	EventCanvasesChanged = "canvas.listed"
	// EventCanvasRender asks the serving app to render: a CanvasRenderRequest.
	EventCanvasRender = "canvas.render"
)

// Kinds of canvas.
const (
	CanvasMockup    = "mockup"    // a UI in HTML and CSS
	CanvasWireframe = "wireframe" // a UI in boxes and text
	CanvasDiagram   = "diagram"   // a flow or an architecture
	CanvasBoard     = "board"     // shapes, text and images, free
)

// CanvasInfo is a canvas without its frames.
type CanvasInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
	// Group is the group it is in, as a note's (NoteInfo.Group).
	Group string `json:"group,omitempty"`
	// Projects are the project folders it belongs to.
	Projects []string  `json:"projects,omitempty"`
	Version  int       `json:"version"` // the version shown, of History
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
	// Agent is the session of the agent that changed it last, at
	// AgentAt: an agent designing it now when that was moments ago.
	Agent   string    `json:"agent,omitempty"`
	AgentAt time.Time `json:"agentAt,omitzero"`
	// Paused: the user paused agents designing it, whose changes are
	// refused until resumed. The daemon keeps it while it runs.
	Paused bool `json:"paused,omitempty"`
	// Winner is the design (a frame's ID) the user picked of them all.
	Winner string `json:"winner,omitempty"`
	// Team is the team whose space it is in, "" for the user's own.
	Team string `json:"team,omitempty"`
}

// Canvas is a canvas with its frames: pages, or the same page at a
// device's sizes.
type Canvas struct {
	CanvasInfo
	Frames []CanvasFrame `json:"frames"`
	// Brief is what its designs are for (CanvasOpBrief sets it).
	Brief *DesignBrief `json:"brief,omitempty"`
}

// CanvasFrame is a design of a canvas, Width×Height: a page, a variant of
// one, or a device's size of it. It holds a mockup's HTML (its elements
// with id attributes, which agents change one at a time) and shapes above
// it. Designs are numbered from 1 in the canvas's order ("design 3").
type CanvasFrame struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Background string `json:"background,omitempty"` // #rrggbb
	// Description says in a line what the design tries.
	Description string `json:"description,omitempty"`
	// Page and Variant group designs: page Home, Login…; variant A, B…
	Page     string `json:"page,omitempty"`
	Variant  string `json:"variant,omitempty"`
	Favorite bool   `json:"favorite,omitempty"`
	// Grid is the design's layout grid: its columns, rows, gaps and named
	// areas, shown as guides, which shapes snap to and CSS comes from.
	Grid *CanvasGrid `json:"grid,omitempty"`
	// Direction is the design's direction card, when it is one of a
	// build's directions (CanvasOpDirection sets it).
	Direction *DesignDirection `json:"direction,omitempty"`
	HTML      string           `json:"html,omitempty"`
	Elements  []CanvasElement  `json:"elements,omitempty"`
}

// Shapes of a frame's elements.
const (
	ShapeRect    = "rect"
	ShapeEllipse = "ellipse"
	ShapeDiamond = "diamond"
	ShapeText    = "text"
	ShapeSticky  = "sticky" // a note on a board
	ShapeLine    = "line"
	ShapeArrow   = "arrow"
	ShapeImage   = "image"
)

// CanvasElement is a shape of a frame at X, Y, W×H. A line or an arrow
// goes from element From to element To, or from (X, Y) to (X+W, Y+H)
// without them. Src is an image's asset (CanvasAsset.Name) or URL.
type CanvasElement struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	W           float64 `json:"w"`
	H           float64 `json:"h"`
	Text        string  `json:"text,omitempty"`
	Fill        string  `json:"fill,omitempty"`   // #rrggbb
	Stroke      string  `json:"stroke,omitempty"` // #rrggbb
	StrokeWidth float64 `json:"strokeWidth,omitempty"`
	Color       string  `json:"color,omitempty"` // the text's
	FontSize    float64 `json:"fontSize,omitempty"`
	Bold        bool    `json:"bold,omitempty"`
	Align       string  `json:"align,omitempty"` // left, center, right
	Radius      float64 `json:"radius,omitempty"`
	From        string  `json:"from,omitempty"`
	To          string  `json:"to,omitempty"`
	Src         string  `json:"src,omitempty"`
}

// CanvasRef names a canvas.
type CanvasRef struct {
	ID string `json:"id"`
}

// CanvasCreateParams makes a canvas of Kind, with Frames ("" sizes take
// the kind's) or one frame of the kind's.
type CanvasCreateParams struct {
	Title    string        `json:"title"`
	Kind     string        `json:"kind,omitempty"`
	Group    string        `json:"group,omitempty"`
	Projects []string      `json:"projects,omitempty"`
	Frames   []CanvasFrame `json:"frames,omitempty"`
	// Team makes it in that team's space, "" in the user's own.
	Team string `json:"team,omitempty"`
}

// Edits of a canvas (CanvasEdit.Op).
const (
	CanvasOpMeta        = "meta"         // Title, Kind, Group, Projects (those set)
	CanvasOpFrameAdd    = "frame.add"    // FrameInfo
	CanvasOpFrameSet    = "frame.set"    // FrameInfo: Frame's name, size, background
	CanvasOpFrameRemove = "frame.remove" // Frame
	// CanvasOpFrameMeta changes design Frame's Name, Description, Page,
	// Variant or Favorite (those set).
	CanvasOpFrameMeta = "frame.meta"
	CanvasOpFrameMove = "frame.move" // Frame before Before ("" for last)
	// CanvasOpFrameCopy adds a copy of design Frame after it, a variant
	// of it, named Name (or after it).
	CanvasOpFrameCopy = "frame.copy"
	CanvasOpPick      = "pick"            // Frame the winner ("" none)
	CanvasOpBrief     = "brief"           // Brief: what the canvas's designs are for (nil takes it away)
	CanvasOpDirection = "frame.direction" // Direction: Frame's direction card (nil takes it away)
	CanvasOpFrameGrid = "frame.grid"      // Grid: Frame's layout grid (nil takes it away)
	CanvasOpHTMLSet   = "html.set"        // HTML: Frame's whole
	// CanvasOpHTMLPatch puts HTML in place of Frame's element with id
	// Target, or, without one, at the end of element Into ("" for the
	// page's body).
	CanvasOpHTMLPatch  = "html.patch"
	CanvasOpHTMLRemove = "html.remove" // Target
	CanvasOpHTMLText   = "html.text"   // Target's text, as HTML
	CanvasOpElementSet = "el.set"      // Element: added, or put in place of the one with its ID
	CanvasOpElementDel = "el.remove"   // Target
)

// CanvasEdit is a change to canvas ID's frame Frame (its ID or name, ""
// for the first), made by the agent of Session, or by the user ("").
type CanvasEdit struct {
	ID        string         `json:"id"`
	Op        string         `json:"op"`
	Frame     string         `json:"frame,omitempty"`
	Target    string         `json:"target,omitempty"`
	Into      string         `json:"into,omitempty"`
	HTML      string         `json:"html,omitempty"`
	Element   *CanvasElement `json:"element,omitempty"`
	FrameInfo *CanvasFrame   `json:"frameInfo,omitempty"`
	Title     *string        `json:"title,omitempty"`
	Kind      *string        `json:"kind,omitempty"`
	Group     *string        `json:"group,omitempty"`
	Projects  *[]string      `json:"projects,omitempty"`
	// What frame.meta and frame.copy set of a design.
	Name        *string          `json:"name,omitempty"`
	Description *string          `json:"description,omitempty"`
	Page        *string          `json:"page,omitempty"`
	Variant     *string          `json:"variant,omitempty"`
	Favorite    *bool            `json:"favorite,omitempty"`
	Before      string           `json:"before,omitempty"`
	Grid        *CanvasGrid      `json:"grid,omitempty"`
	Session     string           `json:"session,omitempty"`
	Brief       *DesignBrief     `json:"brief,omitempty"`
	Direction   *DesignDirection `json:"direction,omitempty"`
}

// CanvasChange is a canvas after a change: what changed (Op, in Frame,
// element Element) and whose (Session, "" for the user's).
type CanvasChange struct {
	Canvas  Canvas `json:"canvas"`
	Op      string `json:"op,omitempty"`
	Frame   string `json:"frame,omitempty"`
	Element string `json:"element,omitempty"`
	Session string `json:"session,omitempty"`
	// Frames are the designs to compare (CanvasCompare).
	Frames []string `json:"frames,omitempty"`
}

// CanvasHistory is a canvas's versions, the oldest first; Head is the
// one shown, those after it what Redo brings back.
type CanvasHistory struct {
	Head     int             `json:"head"`
	Versions []CanvasVersion `json:"versions"`
}

// CanvasVersion is a version: what made it.
type CanvasVersion struct {
	N       int       `json:"n"`
	Time    time.Time `json:"time"`
	Op      string    `json:"op,omitempty"`
	Frame   string    `json:"frame,omitempty"`
	Element string    `json:"element,omitempty"`
	Session string    `json:"session,omitempty"`
	// Label names the version, as a redesign's pass that ended at it.
	Label string `json:"label,omitempty"`
}

// CanvasVersionRef names version N of canvas ID.
type CanvasVersionRef struct {
	ID string `json:"id"`
	N  int    `json:"n"`
}

// CanvasComment is a comment on a canvas, on element Element of frame
// Frame (an HTML id or a shape's), or the whole frame.
type CanvasComment struct {
	ID       string    `json:"id"`
	Frame    string    `json:"frame,omitempty"`
	Element  string    `json:"element,omitempty"`
	Text     string    `json:"text"`
	Author   string    `json:"author,omitempty"` // "" for the user; an agent's session
	Created  time.Time `json:"created"`
	Resolved bool      `json:"resolved,omitempty"`
}

// Comment changes (CanvasCommentParams.Op).
const (
	CanvasCommentAdd     = "add"
	CanvasCommentResolve = "resolve"
	CanvasCommentReopen  = "reopen"
	CanvasCommentDelete  = "delete"
)

// CanvasCommentParams adds Comment to canvas ID, or resolves, reopens or
// deletes the one with its ID.
type CanvasCommentParams struct {
	ID      string        `json:"id"`
	Op      string        `json:"op"`
	Comment CanvasComment `json:"comment"`
}

// Agents and the user designing a canvas (CanvasControl.Action).
const (
	CanvasPause  = "pause"
	CanvasResume = "resume"
	CanvasStop   = "stop" // interrupts the agent designing, and pauses
	// CanvasOpen tells the apps of the canvas, at design Frame when given;
	// CanvasCompare of designs Frames side by side. The user goes there if
	// they choose: an app never moves them for it.
	CanvasOpen    = "open"
	CanvasCompare = "compare"
)

// CanvasControl pauses, resumes or stops the agents designing canvas ID,
// or shows it in the apps.
type CanvasControl struct {
	ID     string   `json:"id"`
	Action string   `json:"action"`
	Frame  string   `json:"frame,omitempty"`
	Frames []string `json:"frames,omitempty"`
}

// CanvasAsset is a file a canvas shows, as an image: Name, under the
// canvas, and its Data.
type CanvasAsset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Data []byte `json:"data,omitempty"`
	// Path is where the host keeps it, as canvas.asset answers.
	Path string `json:"path,omitempty"`
}

// Render formats: a picture, a PDF, a frame's shapes as SVG, or a page
// to open anywhere, read-only (the whole canvas without a frame named).
const (
	RenderPNG  = "png"
	RenderPDF  = "pdf"
	RenderSVG  = "svg"
	RenderHTML = "html"
)

// CanvasRenderParams renders canvas ID's frame Frame (its ID or name; a
// device, "desktop", "tablet" or "mobile", the first frame at its width,
// "" the first, or for HTML the whole canvas) as Format.
type CanvasRenderParams struct {
	ID     string `json:"id"`
	Frame  string `json:"frame,omitempty"`
	Format string `json:"format,omitempty"`
	// Width, when set, reflows a mockup's design at that width (a phone's,
	// to compare designs at it), its height scaled. Older daemons ignore
	// it.
	Width int `json:"width,omitempty"`
	// At, when set, has a design's motion stand at that moment for a still:
	// "25" (percent of its length) or "1500ms". Older daemons ignore it.
	At string `json:"at,omitempty"`
	// Keep names a run of renders of the same design at several moments: the app keeps
	// its page between them and moves it, rather than loading it for each. Older
	// daemons and apps ignore it, and load it each time.
	Keep string `json:"keep,omitempty"`
}

// CanvasMotion is a design's motion as a render found it, in milliseconds: how
// long it is, and the moment the picture shows.
type CanvasMotion struct {
	Duration float64 `json:"duration"`
	At       float64 `json:"at"`
}

// CanvasRendered is a frame rendered: by the daemon ("daemon") or the app
// ("app").
type CanvasRendered struct {
	Data   []byte `json:"data"`
	Format string `json:"format"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	By     string `json:"by"`
	// Wider is how far a mockup's page reaches past the frame's width,
	// in pixels: a horizontal scroll it mustn't have. The app measures it
	// as it renders the page.
	Wider int `json:"wider,omitempty"`
	// Motion says the picture is one moment of a design that moves; nil for a
	// still design, and from older apps.
	Motion *CanvasMotion `json:"motion,omitempty"`
}

// CanvasRenderRequest asks the app to render HTML, a page of Width×Height,
// as Format; it answers request ID with MethodCanvasRenderReply.
type CanvasRenderRequest struct {
	ID     string `json:"id"`
	HTML   string `json:"html"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Format string `json:"format"`
	// Script, when set, runs in the page once it loaded, its value the
	// reply's Text, nothing captured: the page measured as rendered.
	// Older apps ignore it.
	Script string `json:"script,omitempty"`
	// Keep and At are CanvasRenderParams': HTML already stands at At, for an app that
	// has to load it.
	Keep string `json:"keep,omitempty"`
	At   string `json:"at,omitempty"`
}

// CanvasRenderReply is the app's render for request ID.
type CanvasRenderReply struct {
	ID    string `json:"id"`
	Data  []byte `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Wider int    `json:"wider,omitempty"` // CanvasRendered.Wider
	Text  string `json:"text,omitempty"`  // the value of the request's Script
	// Motion is CanvasRendered.Motion.
	Motion *CanvasMotion `json:"motion,omitempty"`
}

// CanvasGrid is a design's layout grid, as CSS grid lays it out: Cols
// columns and Rows rows of equal fractions within Margin of the frame's
// edges, ColGap and RowGap apart, and named Areas spanning cells.
type CanvasGrid struct {
	Cols   int        `json:"cols"`
	Rows   int        `json:"rows"`
	ColGap float64    `json:"colGap,omitempty"`
	RowGap float64    `json:"rowGap,omitempty"`
	Margin float64    `json:"margin,omitempty"`
	Areas  []GridArea `json:"areas,omitempty"`
	// Snap: shapes moved or resized on the design go to its lines.
	Snap bool `json:"snap,omitempty"`
}

// GridArea is a named area of a grid: from column Col and row Row (from
// 1), ColSpan columns wide and RowSpan rows tall.
type GridArea struct {
	Name    string `json:"name"`
	Col     int    `json:"col"`
	Row     int    `json:"row"`
	ColSpan int    `json:"colSpan"`
	RowSpan int    `json:"rowSpan"`
}

package ui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/canvas"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/canvasview"
)

// pageCanvas is the rail's Canvas page: the connected host's canvases.
const pageCanvas = "canvas"

// hostCanvases is the connected host's canvases, as last listed.
type hostCanvases struct {
	host   string
	list   []proto.CanvasInfo
	loaded bool
	err    string
	// open is a canvas an agent asked to show, at design frame, or designs
	// compare side by side, opened when the page draws.
	open    string
	frame   string
	compare []string
}

// canvasesOf is the connected host's canvases, listed anew when the host
// changed; the canvas events keep them current.
func (a *App) canvasesOf() *hostCanvases {
	if a.canvasList == nil || a.canvasList.host != a.host.ID {
		a.canvasList = &hostCanvases{host: a.host.ID}
		if a.canvases.Web() != nil {
			a.canvases.Web().Close()
		}
		a.canvases = canvasview.View{}
		hc, cl := a.canvasList, a.client
		if cl != nil {
			go func() {
				list, err := cl.Canvases()
				a.onMain(func() {
					hc.list, hc.loaded = list, err == nil
					if err != nil {
						hc.err = client.Explain(err)
					}
				})
			}()
		}
	}
	return a.canvasList
}

// canvasEvent applies the canvas events: the list, a canvas changed, one
// to show, a frame to render.
func (a *App) canvasEvent(ev proto.Event) func() {
	switch ev.Name {
	case proto.EventCanvasesChanged:
		var list []proto.CanvasInfo
		if json.Unmarshal(ev.Data, &list) != nil {
			return nil
		}
		return func() {
			if hc := a.canvasList; hc != nil && hc.host == a.host.ID {
				hc.list, hc.loaded, hc.err = list, true, ""
			}
		}
	case proto.EventCanvasChanged:
		var ch proto.CanvasChange
		if json.Unmarshal(ev.Data, &ch) != nil {
			return nil
		}
		return func() {
			hc := a.canvasesOf()
			if i := slices.IndexFunc(hc.list, func(x proto.CanvasInfo) bool { return x.ID == ch.Canvas.ID }); i >= 0 {
				hc.list[i] = ch.Canvas.CanvasInfo
			}
			if ch.Op == proto.CanvasOpen || ch.Op == proto.CanvasCompare {
				a.canvasAsked(ch)
				return
			}
			a.canvases.Apply(a.canvasOptions(), ch)
		}
	case proto.EventCanvasRender:
		var req proto.CanvasRenderRequest
		if json.Unmarshal(ev.Data, &req) != nil {
			return nil
		}
		return func() { a.renderCanvas(req) }
	}
	return nil
}

// canvasAsked is an agent asking to show the user a canvas (canvas open, goto,
// compare). It moves nothing: a card says so, and a click on it goes there.
func (a *App) canvasAsked(ch proto.CanvasChange) {
	hc := a.canvasesOf()
	id, frame := a.canvases.Showing()
	if a.page == pageCanvas && id == ch.Canvas.ID && ch.Op == proto.CanvasOpen && (ch.Frame == "" || ch.Frame == frame) {
		return // what it asks for already shows
	}
	text := "An agent opened it"
	switch {
	case ch.Op == proto.CanvasCompare:
		text = fmt.Sprintf("An agent lined up %d designs to compare", len(ch.Frames))
	case ch.Frame != "":
		text = "An agent points you to a design"
		if i := slices.IndexFunc(ch.Canvas.Frames, func(f proto.CanvasFrame) bool { return f.ID == ch.Frame }); i >= 0 {
			text = "An agent points you to " + ch.Canvas.Frames[i].Name
		}
	}
	a.toastAction("canvas:"+ch.Canvas.ID, ch.Canvas.Title, text, func() {
		a.page, hc.open, hc.frame, hc.compare = pageCanvas, ch.Canvas.ID, ch.Frame, nil
		if ch.Op == proto.CanvasCompare {
			hc.compare = ch.Frames
		}
	})
}

func (a *App) canvasPage(c *ui.Context) {
	o := a.canvasOptions()
	if hc := a.canvasList; hc != nil && hc.open != "" {
		// Shown in its space, an agent's opening one of a team's.
		if i := slices.IndexFunc(hc.list, func(x proto.CanvasInfo) bool { return x.ID == hc.open }); i >= 0 && hc.list[i].Team != a.space() {
			a.showSpace(hc.list[i].Team)
			o = a.canvasOptions()
		}
		switch {
		case len(hc.compare) > 0:
			a.canvases.CompareDesigns(o, hc.open, hc.compare)
		case hc.frame != "":
			a.canvases.Goto(o, hc.open, hc.frame)
		default:
			a.canvases.Open(o, hc.open)
		}
		hc.open, hc.frame, hc.compare = "", "", nil
	}
	a.canvases.Draw(c, o)
}

func (a *App) canvasOptions() canvasview.Options {
	hc, cl := a.canvasesOf(), a.client
	call := func(fn func() error, done func(error)) {
		go func() {
			err := fn()
			a.onMain(func() { done(err) })
		}()
	}
	canvasCall := func(fn func() (proto.Canvas, error), done func(proto.Canvas, error)) {
		var cv proto.Canvas
		call(func() (err error) { cv, err = fn(); return err }, func(err error) { done(cv, err) })
	}
	return canvasview.Options{
		Confirm:       a.confirm,
		ListOutside:   a.navShown(),
		Host:          a.host.Name,
		Canvases:      a.canvasesInSpace(hc.list),
		Spaces:        a.spaceChoices(),
		PickSpace:     a.showSpace,
		Unslop:        a.unslopCanvas,
		UnslopChoose:  a.chooseUnslop,
		UnslopReplay:  a.openReplay,
		Unslopping:    a.unslopping,
		UnslopErr:     a.unslopErr,
		UnslopDismiss: func() { a.unslopErr = "" },
		UnslopRun:     a.unslopRunOf(hc),
		UnslopCompare: a.compareCanvasUnslop,
		Build:         a.startBuild,
		Building:      a.buildStarting,
		BuildErr:      a.buildErr,
		BuildDismiss:  func() { a.buildErr = "" },
		BuildRun:      a.buildRunOf(),
		PickDirection: a.pickDirection,
		RenderWidth: func(id, frame string, width int, done func([]byte, error)) {
			var data []byte
			call(func() error {
				out, err := cl.RenderCanvas(proto.CanvasRenderParams{ID: id, Frame: frame, Format: proto.RenderPNG, Width: width})
				data = out.Data
				return err
			}, func(err error) { done(data, err) })
		},
		OpenReviewer: a.openSessionByID,
		AgentState:   a.agentState,
		OpenSession:  a.viewRegionAgent,
		Viewing:      a.regionViewing(),
		Space:        a.spaceName(a.space()),
		Team:         a.space(),
		Loaded:       hc.loaded,
		Err:          hc.err,
		Load: func(id string, done func(proto.Canvas, error)) {
			canvasCall(func() (proto.Canvas, error) { return cl.Canvas(id) }, done)
		},
		Create: func(p proto.CanvasCreateParams, done func(proto.Canvas, error)) {
			p.Team = a.space() // made in the space shown
			canvasCall(func() (proto.Canvas, error) { return cl.CreateCanvas(p) }, done)
		},
		Delete: func(id string, done func(error)) { call(func() error { return cl.DeleteCanvas(id) }, done) },
		Edit: func(e proto.CanvasEdit, done func(proto.Canvas, error)) {
			canvasCall(func() (proto.Canvas, error) { return cl.EditCanvas(e) }, done)
		},
		History: func(id string, done func(proto.CanvasHistory, error)) {
			var h proto.CanvasHistory
			call(func() (err error) { h, err = cl.CanvasHistory(id); return err }, func(err error) { done(h, err) })
		},
		Version: func(id string, n int, done func(proto.Canvas, error)) {
			canvasCall(func() (proto.Canvas, error) { return cl.CanvasVersion(id, n) }, done)
		},
		Undo: func(id string, done func(proto.Canvas, error)) {
			canvasCall(func() (proto.Canvas, error) { return cl.UndoCanvas(id) }, done)
		},
		Redo: func(id string, done func(proto.Canvas, error)) {
			canvasCall(func() (proto.Canvas, error) { return cl.RedoCanvas(id) }, done)
		},
		Restore: func(id string, n int, done func(proto.Canvas, error)) {
			canvasCall(func() (proto.Canvas, error) { return cl.RestoreCanvas(id, n) }, done)
		},
		Comments: func(id string, done func([]proto.CanvasComment, error)) {
			var list []proto.CanvasComment
			call(func() (err error) { list, err = cl.CanvasComments(id); return err }, func(err error) { done(list, err) })
		},
		Comment: func(p proto.CanvasCommentParams, done func([]proto.CanvasComment, error)) {
			var list []proto.CanvasComment
			call(func() (err error) { list, err = cl.CommentCanvas(p); return err }, func(err error) { done(list, err) })
		},
		GridHidden: func(id string) bool { return a.state.CanvasGridHidden[id] },
		SetGridHidden: func(id string, hidden bool) {
			if a.state.CanvasGridHidden == nil {
				a.state.CanvasGridHidden = map[string]bool{}
			}
			if hidden {
				a.state.CanvasGridHidden[id] = true
			} else {
				delete(a.state.CanvasGridHidden, id)
			}
			a.saveState()
		},
		Control: func(id, action string, done func(error)) {
			call(func() error { _, err := cl.ControlCanvas(id, action); return err }, done)
		},
		Render: func(id, frame, format string, done func([]byte, error)) {
			var data []byte
			call(func() error {
				out, err := cl.RenderCanvas(proto.CanvasRenderParams{ID: id, Frame: frame, Format: format})
				data = out.Data
				return err
			}, func(err error) { done(data, err) })
		},
		OffMain: func(work, then func()) {
			go func() {
				work()
				a.onMain(then)
			}()
		},
		PictureCached: func(key string, done func([]byte)) {
			go func() {
				data := readCanvasThumb(canvasThumbDir(), key)
				a.onMain(func() { done(data) })
			}()
		},
		PictureKeep: func(key string, data []byte) {
			go keepCanvasThumb(canvasThumbDir(), key, data)
		},
		SaveFile: func(name string, data []byte) {
			path, err := saveDownload(strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(name), data)
			if err != nil {
				a.status = err.Error()
				return
			}
			a.notify("Saved to " + path)
		},
		Projects: a.canvasProjects(),
		MakeCode: a.makeCode,
		MoveSpace: func() func(proto.CanvasInfo) {
			if len(a.spacesOf().list) == 0 {
				return nil
			}
			return a.openCanvasSpaceMove
		}(),
		Share: func(id string, stop bool, done func(proto.CanvasShare, error)) {
			var x proto.CanvasShare
			call(func() (err error) { x, err = cl.ShareCanvas(id, stop); return err }, func(err error) { done(x, err) })
		},
		AgentName: a.sessionAgentName,
		Web:       a.canvases.WebOf(a.canvasWeb),
		CanAsk:    true,
		AskAgent: func(draft string) {
			if cv := a.canvases.Canvas(); cv != nil {
				a.canvasDraft(*cv, draft)
			}
		},
		SendAgent: func(text string) {
			if cv := a.canvases.Canvas(); cv != nil {
				a.canvasTell(*cv, text)
			}
		},
	}
}

// sessionAgentName is the name of session id's agent, "An agent" for one
// gone.
func (a *App) sessionAgentName(id string) string {
	if s, ok := a.sessionByID(id); ok {
		return a.agentName(s.Agent)
	}
	return "An agent"
}

// makeCode starts an agent in project on turning canvas c into code, and
// shows its session.
func (a *App) makeCode(c proto.Canvas, project string) {
	cl := a.client
	agentID := a.notesAgentOf("")
	if cl == nil || agentID == "" {
		a.status = "No agent is installed on " + a.host.Name
		return
	}
	prompt := "Build the canvas “" + c.Title + "” (enowX canvas " + c.ID + ") in this project as real code. " +
		"Read it first: \"$ENOWX_BIN\" canvas show " + c.ID + " and its frames' HTML (--html --frame F), and look at each frame with \"$ENOWX_BIN\" canvas screenshot. " +
		"Use the project's stack, components and design tokens; one component per region of the design; keep its copy and layout, " +
		"responsive as its frames show. Keep the design's tokens (colors, type scale, spacing, radii) and its rules (what each color and size is for, what it avoids) " +
		"as the project's design file, .enowx/design.md, for the next change to follow; its brief, if the canvas has one (\"$ENOWX_BIN\" canvas brief " + c.ID + "), goes in .enowx/brief.md. " +
		"Then run the app and compare it with the canvas in the Browser, and say what differs."
	if project = a.projectFolder(project); project == "" {
		a.status = "Clone the team's project on " + a.host.Name + " first (the team's Projects tab)"
		return
	}
	params := proto.SessionCreateParams{Agent: agentID, Project: project, Prompt: prompt, SkipUnheard: a.skipPermissions, Cols: 120, Rows: 40}
	go func() {
		info, err := cl.CreateSession(params)
		a.onMain(func() {
			if err != nil {
				a.status = strings.TrimPrefix(err.Error(), proto.MethodSessionCreate+": ")
				return
			}
			a.upsert(info)
			a.selectSession(info)
		})
	}()
}

// openCanvasLink shows the canvas, or its design, a canvas:// link names
// (canvas.Link), on the connected host; false for another link.
func (a *App) openCanvasLink(link string) bool {
	id, design, ok := canvas.ParseLink(link)
	if !ok {
		return false
	}
	hc := a.canvasesOf()
	a.page, hc.open, hc.frame, hc.compare = pageCanvas, id, design, nil
	return true
}

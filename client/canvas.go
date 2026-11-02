package client

import "github.com/enowx/enowx/proto"

// Canvases lists the host's canvases, the latest changed first.
func (c *Client) Canvases() ([]proto.CanvasInfo, error) {
	var list []proto.CanvasInfo
	err := c.Call(proto.MethodCanvasList, nil, &list)
	return list, err
}

// Canvas reads a canvas with its frames.
func (c *Client) Canvas(id string) (proto.Canvas, error) {
	var cv proto.Canvas
	err := c.Call(proto.MethodCanvasGet, proto.CanvasRef{ID: id}, &cv)
	return cv, err
}

// CreateCanvas makes a canvas.
func (c *Client) CreateCanvas(p proto.CanvasCreateParams) (proto.Canvas, error) {
	var cv proto.Canvas
	err := c.Call(proto.MethodCanvasCreate, p, &cv)
	return cv, err
}

// DeleteCanvas deletes a canvas with its versions, comments and assets.
func (c *Client) DeleteCanvas(id string) error {
	return c.Call(proto.MethodCanvasDelete, proto.CanvasRef{ID: id}, nil)
}

// EditCanvas makes a change to a canvas, which every app sees at once.
func (c *Client) EditCanvas(e proto.CanvasEdit) (proto.Canvas, error) {
	var cv proto.Canvas
	err := c.Call(proto.MethodCanvasEdit, e, &cv)
	return cv, err
}

// CanvasHistory lists a canvas's versions.
func (c *Client) CanvasHistory(id string) (proto.CanvasHistory, error) {
	var h proto.CanvasHistory
	err := c.Call(proto.MethodCanvasHistory, proto.CanvasRef{ID: id}, &h)
	return h, err
}

// CanvasVersion reads a canvas as it was at version n.
func (c *Client) CanvasVersion(id string, n int) (proto.Canvas, error) {
	var cv proto.Canvas
	err := c.Call(proto.MethodCanvasVersion, proto.CanvasVersionRef{ID: id, N: n}, &cv)
	return cv, err
}

// UndoCanvas shows a canvas's version before the one shown.
func (c *Client) UndoCanvas(id string) (proto.Canvas, error) {
	var cv proto.Canvas
	err := c.Call(proto.MethodCanvasUndo, proto.CanvasRef{ID: id}, &cv)
	return cv, err
}

// RedoCanvas shows the version undone last.
func (c *Client) RedoCanvas(id string) (proto.Canvas, error) {
	var cv proto.Canvas
	err := c.Call(proto.MethodCanvasRedo, proto.CanvasRef{ID: id}, &cv)
	return cv, err
}

// RestoreCanvas makes a canvas as it was at version n its newest version.
func (c *Client) RestoreCanvas(id string, n int) (proto.Canvas, error) {
	var cv proto.Canvas
	err := c.Call(proto.MethodCanvasRestore, proto.CanvasVersionRef{ID: id, N: n}, &cv)
	return cv, err
}

// CanvasComments lists a canvas's comments.
func (c *Client) CanvasComments(id string) ([]proto.CanvasComment, error) {
	var list []proto.CanvasComment
	err := c.Call(proto.MethodCanvasComments, proto.CanvasRef{ID: id}, &list)
	return list, err
}

// CommentCanvas adds, resolves, reopens or deletes a comment.
func (c *Client) CommentCanvas(p proto.CanvasCommentParams) ([]proto.CanvasComment, error) {
	var list []proto.CanvasComment
	err := c.Call(proto.MethodCanvasComment, p, &list)
	return list, err
}

// ControlCanvas pauses, resumes or stops the agents designing a canvas.
func (c *Client) ControlCanvas(id, action string) (proto.CanvasInfo, error) {
	return c.ShowCanvas(proto.CanvasControl{ID: id, Action: action})
}

// ShowCanvas sends a control: shows a canvas at a design, or designs side
// by side, in the apps.
func (c *Client) ShowCanvas(p proto.CanvasControl) (proto.CanvasInfo, error) {
	var info proto.CanvasInfo
	err := c.Call(proto.MethodCanvasControl, p, &info)
	return info, err
}

// AddCanvasAssetAt keeps a file a canvas shows and says where the host
// keeps it.
func (c *Client) AddCanvasAssetAt(id, name string, data []byte) (proto.CanvasAsset, error) {
	var out proto.CanvasAsset
	err := c.Call(proto.MethodCanvasAsset, proto.CanvasAsset{ID: id, Name: name, Data: data}, &out)
	return out, err
}

// AddCanvasAsset keeps a file a canvas shows; it returns its name there.
func (c *Client) AddCanvasAsset(id, name string, data []byte) (string, error) {
	var out proto.CanvasAsset
	err := c.Call(proto.MethodCanvasAsset, proto.CanvasAsset{ID: id, Name: name, Data: data}, &out)
	return out.Name, err
}

// RenderCanvas renders a frame as a PNG or a PDF.
func (c *Client) RenderCanvas(p proto.CanvasRenderParams) (proto.CanvasRendered, error) {
	var out proto.CanvasRendered
	err := c.Call(proto.MethodCanvasRender, p, &out)
	return out, err
}

// CanvasRenderReply answers the daemon's request to render.
func (c *Client) CanvasRenderReply(r proto.CanvasRenderReply) error {
	return c.Call(proto.MethodCanvasRenderReply, r, nil)
}

// CanvasAsset reads a file a canvas shows.
func (c *Client) CanvasAsset(id, name string) ([]byte, error) {
	var out proto.CanvasAsset
	err := c.Call(proto.MethodCanvasAssetGet, proto.CanvasAsset{ID: id, Name: name}, &out)
	return out.Data, err
}

// CanvasChat records a message sent to a canvas's agent, or sets its agent
// or mode.
func (c *Client) CanvasChat(p proto.CanvasChatParams) (proto.CanvasChatLog, error) {
	var log proto.CanvasChatLog
	err := c.Call(proto.MethodCanvasChat, p, &log)
	return log, err
}

// CanvasChatLog reads a canvas's conversation with its agent.
func (c *Client) CanvasChatLog(id string) (proto.CanvasChatLog, error) {
	var log proto.CanvasChatLog
	err := c.Call(proto.MethodCanvasChatLog, proto.CanvasRef{ID: id}, &log)
	return log, err
}

// MarkCanvasChat says whether a message's changes were kept or undone.
func (c *Client) MarkCanvasChat(id string, n int, state string) (proto.CanvasChatLog, error) {
	var log proto.CanvasChatLog
	err := c.Call(proto.MethodCanvasChatMark, proto.CanvasChatMark{ID: id, N: n, State: state}, &log)
	return log, err
}

// ShareCanvas makes canvas id's read-only link (its own when it has one),
// or stops it.
func (c *Client) ShareCanvas(id string, stop bool) (proto.CanvasShare, error) {
	var out proto.CanvasShare
	err := c.Call(proto.MethodCanvasShare, proto.CanvasShareParams{ID: id, Stop: stop}, &out)
	return out, err
}

// CanvasShares lists the host's canvas links.
func (c *Client) CanvasShares() ([]proto.CanvasShare, error) {
	var out []proto.CanvasShare
	err := c.Call(proto.MethodCanvasShares, nil, &out)
	return out, err
}

// CanvasUnslop reads a mockup's design for what makes it look AI-made;
// started, on a copy of it, with what to ask the canvas's agent.
func (c *Client) CanvasUnslop(p proto.CanvasUnslopParams) (proto.CanvasUnslop, error) {
	var u proto.CanvasUnslop
	err := c.Call(proto.MethodCanvasUnslop, p, &u)
	return u, err
}

// UnslopStory is a canvas redesign's steps, to play back.
func (c *Client) UnslopStory(canvasID, frame string) (proto.UnslopStory, error) {
	var out proto.UnslopStory
	err := c.Call(proto.MethodCanvasUnslopSteps, proto.UnslopStepsParams{ID: canvasID, Frame: frame}, &out)
	return out, err
}

// UnslopLeaveOut makes a copy of a redesigned design without one step.
func (c *Client) UnslopLeaveOut(canvasID, frame string, step int) (proto.UnslopLeftOut, error) {
	var out proto.UnslopLeftOut
	err := c.Call(proto.MethodCanvasUnslopLeaveOut, proto.UnslopLeaveOutParams{ID: canvasID, Frame: frame, Step: step}, &out)
	return out, err
}

// BrowserUnslop reads the page in a session's Browser for what makes it
// look AI-made; started, with what to ask the session's agent.
// BrowserUnslopShots are the screenshots of a Browser unslop, before and after.
func (c *Client) BrowserUnslopShots(session string) (proto.UnslopShots, error) {
	var out proto.UnslopShots
	err := c.Call(proto.MethodBrowserUnslopShots, proto.BrowserUnslopParams{Session: session}, &out)
	return out, err
}

func (c *Client) BrowserUnslop(p proto.BrowserUnslopParams) (proto.CanvasUnslop, error) {
	var u proto.CanvasUnslop
	err := c.Call(proto.MethodBrowserUnslop, p, &u)
	return u, err
}

// DesignReview sends a reviewer session's verdict.
func (c *Client) DesignReview(p proto.DesignReviewParams) (proto.DesignReview, error) {
	var out proto.DesignReview
	err := c.Call(proto.MethodDesignReview, p, &out)
	return out, err
}

// CanvasBuild starts or advances a UI build on a canvas.
func (c *Client) CanvasBuild(p proto.CanvasBuildParams) (proto.CanvasBuild, error) {
	var out proto.CanvasBuild
	err := c.Call(proto.MethodCanvasBuild, p, &out)
	return out, err
}

// CanvasMatch compares a design with the picture it recreates.
func (c *Client) CanvasMatch(p proto.CanvasMatchParams) (proto.CanvasMatch, error) {
	var out proto.CanvasMatch
	err := c.Call(proto.MethodCanvasMatch, p, &out)
	return out, err
}

// CanvasVideo has the app render a moving design to an MP4, and waits for it.
func (c *Client) CanvasVideo(p proto.CanvasVideoParams) (proto.CanvasVideoResult, error) {
	var out proto.CanvasVideoResult
	err := c.Call(proto.MethodCanvasVideo, p, &out)
	return out, err
}

// CanvasVideoProgress says how far a video the daemon asked for has got.
func (c *Client) CanvasVideoProgress(p proto.CanvasVideoProgress) error {
	return c.Call(proto.MethodCanvasVideoProgress, p, nil)
}

// CanvasVideoReply answers the daemon's request for a video.
func (c *Client) CanvasVideoReply(r proto.CanvasVideoReply) error {
	return c.Call(proto.MethodCanvasVideoReply, r, nil)
}

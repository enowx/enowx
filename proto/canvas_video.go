package proto

// A moving design rendered to an MP4 by the app that serves Browsers, as it holds the
// web view and the encoder: the daemon asks it (EventCanvasVideo), it says how far it
// got and answers.
const (
	// MethodCanvasVideo renders a design's motion: CanvasVideoParams → CanvasVideoResult.
	MethodCanvasVideo = "canvas.video"
	// MethodCanvasVideoProgress and MethodCanvasVideoReply are the app's answers.
	MethodCanvasVideoProgress = "canvas.videoProgress"
	MethodCanvasVideoReply    = "canvas.videoReply"
	// EventCanvasVideo asks the serving app to render one: a CanvasVideoRequest.
	EventCanvasVideo = "canvas.video"
)

// CanvasVideoParams says what to render. Zero values are the defaults: 30 frames a
// second, the frame's own width, all of it (one cycle of a loop).
type CanvasVideoParams struct {
	ID    string  `json:"id"`
	Frame string  `json:"frame,omitempty"`
	FPS   int     `json:"fps,omitempty"`
	Width int     `json:"width,omitempty"`
	From  float64 `json:"from,omitempty"` // ms
	To    float64 `json:"to,omitempty"`   // ms, 0 for its end
	// Preview is for a design's short video in the community library: small, and
	// left in no folder of the user's. Its Data is all there is of it.
	Preview bool `json:"preview,omitempty"`
}

// CanvasVideoRequest is CanvasVideoParams made ready for the app.
type CanvasVideoRequest struct {
	ID       string  `json:"id"`
	HTML     string  `json:"html"`
	Name     string  `json:"name"` // the file's, without its extension
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	OutWidth int     `json:"outWidth"`
	FPS      int     `json:"fps"`
	From     float64 `json:"from,omitempty"`
	To       float64 `json:"to,omitempty"`
	Warning  string  `json:"warning,omitempty"`
	Preview  bool    `json:"preview,omitempty"`
}

// CanvasVideoProgress says how many of a video's frames are done.
type CanvasVideoProgress struct {
	ID    string `json:"id"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// CanvasVideoResult is a finished video. Path is where it is on the app's device; Data
// is the file too, when small enough to carry to another host.
type CanvasVideoResult struct {
	Path    string  `json:"path"`
	Data    []byte  `json:"data,omitempty"`
	Width   int     `json:"width"`
	Height  int     `json:"height"`
	Frames  int     `json:"frames"`
	Seconds float64 `json:"seconds"`
	Warning string  `json:"warning,omitempty"`
}

// CanvasVideoReply is the app's answer to request ID.
type CanvasVideoReply struct {
	ID     string            `json:"id"`
	Error  string            `json:"error,omitempty"`
	Result CanvasVideoResult `json:"result"`
}

package daemon

import (
	"path/filepath"

	"github.com/enowx/enowx/proto"
)

var canvasHandlers = map[string]handler{
	proto.MethodCanvasShare: with(func(c *conn, p proto.CanvasShareParams) (any, error) {
		return c.srv.shareCanvas(p)
	}),
	proto.MethodCanvasShares:      bare(func(c *conn) (any, error) { return c.srv.canvasShareList(), nil }),
	proto.MethodCanvasUnslopSteps: with(func(c *conn, p proto.UnslopStepsParams) (any, error) { return c.srv.unslopStory(p) }),
	proto.MethodCanvasUnslopLeaveOut: with(func(c *conn, p proto.UnslopLeaveOutParams) (any, error) {
		return c.srv.leaveOut(p)
	}),
	proto.MethodCanvasUnslop: with(func(c *conn, p proto.CanvasUnslopParams) (any, error) {
		return c.srv.canvasUnslop(p)
	}),
	proto.MethodCanvasList: bare(func(c *conn) (any, error) {
		list, err := c.srv.allCanvases()
		for i := range list {
			list[i].Paused = c.srv.canvasState.isPaused(list[i].ID)
		}
		return list, err
	}),
	proto.MethodCanvasGet: with(func(c *conn, p proto.CanvasRef) (any, error) {
		cv, err := c.srv.canvasFor(p.ID).Get(p.ID)
		if err != nil {
			return nil, err
		}
		return c.srv.withState(cv), nil
	}),
	proto.MethodCanvasCreate: with(func(c *conn, p proto.CanvasCreateParams) (any, error) {
		st, err := c.srv.canvasesIn(p.Team)
		if err != nil {
			return nil, err
		}
		cv, err := st.Create(p)
		if err != nil {
			return nil, err
		}
		cv.Team = p.Team
		c.srv.canvasesChanged()
		return cv, nil
	}),
	proto.MethodCanvasDelete: with(func(c *conn, p proto.CanvasRef) (any, error) {
		if err := c.srv.canvasFor(p.ID).Delete(p.ID); err != nil {
			return nil, err
		}
		c.srv.canvasState.setPaused(p.ID, false)
		_, _ = c.srv.shareCanvas(proto.CanvasShareParams{ID: p.ID, Stop: true}) // its link goes with it
		c.srv.canvasesChanged()
		return ok, nil
	}),
	proto.MethodCanvasMoveSpace: with(func(c *conn, p proto.SpaceMoveParams) (any, error) {
		return c.srv.moveCanvasSpace(p)
	}),
	proto.MethodCanvasEdit: with(func(c *conn, p proto.CanvasEdit) (any, error) {
		return c.srv.canvasEdit(p)
	}),
	proto.MethodCanvasHistory: with(func(c *conn, p proto.CanvasRef) (any, error) {
		return c.srv.canvasFor(p.ID).History(p.ID)
	}),
	proto.MethodCanvasVersion: with(func(c *conn, p proto.CanvasVersionRef) (any, error) {
		return c.srv.canvasFor(p.ID).Version(p.ID, p.N)
	}),
	proto.MethodCanvasUndo: with(func(c *conn, p proto.CanvasRef) (any, error) {
		cv, err := c.srv.canvasFor(p.ID).Undo(p.ID)
		return c.srv.canvasShown(cv, "undo", err)
	}),
	proto.MethodCanvasRedo: with(func(c *conn, p proto.CanvasRef) (any, error) {
		cv, err := c.srv.canvasFor(p.ID).Redo(p.ID)
		return c.srv.canvasShown(cv, "redo", err)
	}),
	proto.MethodCanvasRestore: with(func(c *conn, p proto.CanvasVersionRef) (any, error) {
		cv, err := c.srv.canvasFor(p.ID).Restore(p.ID, p.N)
		return c.srv.canvasShown(cv, "restore", err)
	}),
	proto.MethodCanvasComments: with(func(c *conn, p proto.CanvasRef) (any, error) {
		return c.srv.canvasFor(p.ID).Comments(p.ID)
	}),
	proto.MethodCanvasComment: with(func(c *conn, p proto.CanvasCommentParams) (any, error) {
		list, err := c.srv.canvasFor(p.ID).Comment(p)
		if err != nil {
			return nil, err
		}
		if cv, err := c.srv.canvasFor(p.ID).Get(p.ID); err == nil {
			c.srv.broadcast(proto.EventCanvasChanged, proto.CanvasChange{Canvas: c.srv.withState(cv), Op: "comment", Element: p.Comment.Element})
		}
		return list, nil
	}),
	proto.MethodCanvasControl: with(func(c *conn, p proto.CanvasControl) (any, error) {
		return c.srv.canvasControl(p)
	}),
	proto.MethodCanvasAsset: with(func(c *conn, p proto.CanvasAsset) (any, error) {
		name, err := c.srv.canvasFor(p.ID).AddAsset(p.ID, p.Name, p.Data)
		if err != nil {
			return nil, err
		}
		dir, _ := c.srv.canvasFor(p.ID).Dir(p.ID)
		return proto.CanvasAsset{ID: p.ID, Name: name, Path: filepath.Join(dir, "assets", name)}, nil
	}),
	proto.MethodCanvasAssetGet: with(func(c *conn, p proto.CanvasAsset) (any, error) {
		data, err := c.srv.canvasFor(p.ID).Asset(p.ID, p.Name)
		if err != nil {
			return nil, err
		}
		return proto.CanvasAsset{ID: p.ID, Name: p.Name, Data: data}, nil
	}),
	proto.MethodCanvasRender: with(func(c *conn, p proto.CanvasRenderParams) (any, error) {
		return c.srv.canvasRender(p)
	}),
	proto.MethodCanvasChat: with(func(c *conn, p proto.CanvasChatParams) (any, error) {
		return c.srv.canvasChatChanged(c.srv.canvasFor(p.ID).Chat(p))
	}),
	proto.MethodCanvasChatLog: with(func(c *conn, p proto.CanvasRef) (any, error) {
		return c.srv.canvasFor(p.ID).ChatLog(p.ID)
	}),
	proto.MethodCanvasChatMark: with(func(c *conn, p proto.CanvasChatMark) (any, error) {
		return c.srv.canvasChatChanged(c.srv.canvasFor(p.ID).ChatMark(p))
	}),
	proto.MethodCanvasVideo: with(func(c *conn, p proto.CanvasVideoParams) (any, error) { return c.srv.canvasVideo(p) }),
	proto.MethodCanvasVideoProgress: with(func(c *conn, p proto.CanvasVideoProgress) (any, error) {
		c.srv.videos.progress(p)
		return ok, nil
	}),
	proto.MethodCanvasVideoReply: with(func(c *conn, p proto.CanvasVideoReply) (any, error) {
		c.srv.videos.reply(p)
		return ok, nil
	}),
	proto.MethodCanvasRenderReply: with(func(c *conn, p proto.CanvasRenderReply) (any, error) {
		c.srv.canvasState.reply(p)
		return ok, nil
	}),
}

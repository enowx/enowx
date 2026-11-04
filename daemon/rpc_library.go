package daemon

import "github.com/enowx/enowx/proto"

// Sharing canvases' designs to the community library (library.go).

var libraryHandlers = map[string]handler{
	proto.MethodLibraryPrepare:   with(func(c *conn, p proto.LibraryPrepareParams) (any, error) { return c.srv.libraryPrepare(p) }),
	proto.MethodLibrarySaveDraft: with(func(c *conn, p proto.LibraryDraftParams) (any, error) { return ok, c.srv.librarySaveDraft(p) }),
	proto.MethodLibraryDeleteDraft: with(func(c *conn, p proto.LibraryDraftParams) (any, error) {
		return ok, c.srv.libraryDrafts().Delete(p.DraftID)
	}),
	proto.MethodLibraryPublish: with(func(c *conn, p proto.LibraryPublishParams) (any, error) {
		if err := c.fromTheApp(); err != nil {
			return nil, err
		}
		return c.srv.libraryPublish(p)
	}),
	proto.MethodLibraryStatus:   bare(func(c *conn) (any, error) { return c.srv.libraryStatus() }),
	proto.MethodLibraryWithdraw: with(func(c *conn, p proto.LibraryItemRef) (any, error) { return c.srv.libraryWithdraw(p) }),
	proto.MethodLibrarySearch:   with(func(c *conn, p proto.LibrarySearchParams) (any, error) { return c.srv.librarySearch(p) }),
	proto.MethodLibraryItem:     with(func(c *conn, p proto.LibraryItemRef) (any, error) { return c.srv.libraryItem(p) }),
	proto.MethodLibraryPrompt:   with(func(c *conn, p proto.LibraryItemRef) (any, error) { return c.srv.libraryPrompt(p) }),
	proto.MethodLibraryPicture:  with(func(c *conn, p proto.LibraryPictureParams) (any, error) { return c.srv.libraryPicture(p) }),
	proto.MethodLibraryAskPublish: with(func(c *conn, p proto.LibraryAskPublishParams) (any, error) {
		return c.srv.libraryAskPublish(p)
	}),
}

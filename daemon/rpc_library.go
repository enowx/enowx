package daemon

import "github.com/enowx/enowx/proto"

// Sharing canvases' designs to the community library (library.go).

var libraryHandlers = map[string]handler{
	proto.MethodLibraryPrepare:   with(func(c *conn, p proto.LibraryPrepareParams) (any, error) { return c.srv.libraryPrepare(p) }),
	proto.MethodLibrarySaveDraft: with(func(c *conn, p proto.LibraryDraftParams) (any, error) { return ok, c.srv.librarySaveDraft(p) }),
	proto.MethodLibraryDeleteDraft: with(func(c *conn, p proto.LibraryDraftParams) (any, error) {
		return ok, c.srv.libraryDrafts().Delete(p.DraftID)
	}),
	proto.MethodLibraryPublish:  with(func(c *conn, p proto.LibraryPublishParams) (any, error) { return c.srv.libraryPublish(p) }),
	proto.MethodLibraryStatus:   bare(func(c *conn) (any, error) { return c.srv.libraryStatus() }),
	proto.MethodLibraryWithdraw: with(func(c *conn, p proto.LibraryItemRef) (any, error) { return c.srv.libraryWithdraw(p) }),
}

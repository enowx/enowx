package daemon

import "github.com/enowx/enowx/proto"

// Taking something from the community library (library_reuse.go).

var libraryReuseHandlers = map[string]handler{
	proto.MethodLibraryUseInfo:   with(func(c *conn, p proto.LibraryUseRef) (any, error) { return c.srv.libraryUseInfo(p) }),
	proto.MethodLibraryOpen:      with(func(c *conn, p proto.LibraryOpenParams) (any, error) { return c.srv.libraryOpen(p) }),
	proto.MethodLibraryReference: with(func(c *conn, p proto.LibraryReferenceParams) (any, error) { return c.srv.libraryReference(p) }),
	proto.MethodLibrarySkill:     with(func(c *conn, p proto.LibrarySkillParams) (any, error) { return c.srv.librarySkill(p) }),
	proto.MethodLibraryNewer:     with(func(c *conn, p proto.CanvasRef) (any, error) { return c.srv.libraryNewer(p) }),
}

package daemon

import (
	"context"
	"time"

	"github.com/enowx/enowx/core/rag"
	"github.com/enowx/enowx/proto"
)

// ragHandlers are RAG's methods (daemon/rag.go).
var ragHandlers = map[string]handler{
	proto.MethodRAGSettings: bare(func(c *conn) (any, error) { return c.srv.rag.WithKeys(c.srv.ragSettings.Get()), nil }),
	proto.MethodRAGSetSettings: with(func(c *conn, p proto.RAGSettings) (any, error) {
		p.Projects = c.srv.ragSettings.Get().Projects // set one by one
		p.HasKey, p.HasPostgres = false, false        // the host's own, never kept in settings
		if err := c.srv.ragSettings.Set(p); err != nil {
			return nil, err
		}
		return c.srv.rag.WithKeys(c.srv.ragSettings.Get()), nil
	}),
	proto.MethodRAGSetKey: with(func(c *conn, p proto.RAGKey) (any, error) {
		if err := c.srv.rag.SetKey(p.Which, p.Value); err != nil {
			return nil, err
		}
		return c.srv.rag.WithKeys(c.srv.ragSettings.Get()), nil
	}),
	proto.MethodRAGTest: with(func(c *conn, p proto.RAGSettings) (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		dim, err := c.srv.rag.Test(ctx, p)
		out := proto.RAGTestResult{Dim: dim}
		if err != nil {
			out.Error = err.Error()
		}
		return out, nil
	}),
	proto.MethodRAGStatus: with(func(c *conn, p proto.RAGRef) (any, error) {
		dir, err := c.srv.ragRead(p)
		if err != nil {
			return nil, err
		}
		return c.srv.ragStatus(dir), nil
	}),
	proto.MethodRAGSetProject: with(func(c *conn, p proto.RAGProjectParams) (any, error) { return c.srv.setRAGProject(p) }),
	proto.MethodRAGIndex: with(func(c *conn, p proto.RAGRef) (any, error) {
		dir, err := c.srv.ragSetupDir(p)
		if err != nil {
			return nil, err
		}
		return c.srv.ragIndex(dir)
	}),
	proto.MethodRAGPlan: with(func(c *conn, p proto.RAGRef) (any, error) {
		dir, err := c.srv.ragSetupDir(p)
		if err != nil {
			return nil, err
		}
		return c.srv.rag.Plan(dir, c.srv.ragSettings.Project(rag.ProjectID(dir))), nil
	}),
	proto.MethodRAGSearch: with(func(c *conn, p proto.RAGSearchParams) (any, error) {
		dir, err := c.srv.ragRead(p.RAGRef)
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		hits, err := c.srv.rag.Search(ctx, dir, p.Query, p.K, p.Scope)
		if hits == nil {
			hits = []proto.RAGHit{}
		}
		return hits, err
	}),
	proto.MethodRAGRemember: with(func(c *conn, p proto.RAGRememberParams) (any, error) {
		dir, err := c.srv.ragDir(p.RAGRef)
		if err != nil {
			return nil, err
		}
		by := "user"
		if sess, err := c.srv.sessions.Get(p.Session); err == nil {
			by = sess.Info().Agent
		}
		m, err := c.srv.ragSettings.Remember(rag.ProjectID(dir), p.Text, by)
		if err == nil {
			c.srv.ragSoon(dir)
		}
		return m, err
	}),
	proto.MethodRAGMemories: with(func(c *conn, p proto.RAGRef) (any, error) {
		dir, err := c.srv.ragRead(p)
		if err != nil {
			return nil, err
		}
		list := c.srv.ragSettings.Memories(rag.ProjectID(dir))
		if list == nil {
			list = []proto.RAGMemory{}
		}
		return list, nil
	}),
	proto.MethodRAGForget: with(func(c *conn, p proto.RAGForgetParams) (any, error) {
		dir, err := c.srv.ragDir(p.RAGRef)
		if err != nil {
			return nil, err
		}
		if err := c.srv.ragSettings.Change(rag.ProjectID(dir), p.ID, p.Text); err != nil {
			return nil, err
		}
		c.srv.ragSoon(dir)
		return ok, nil
	}),
}

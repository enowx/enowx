package client

import "github.com/enowx/enowx/proto"

// RAG calls a RAG method of the host's daemon (proto.MethodRAG*).
func (c *Client) RAG(method string, p, out any) error { return c.Call(method, p, out) }

// RAGSettings are the host's RAG settings.
func (c *Client) RAGSettings() (proto.RAGSettings, error) {
	var out proto.RAGSettings
	err := c.Call(proto.MethodRAGSettings, nil, &out)
	return out, err
}

// SetRAGSettings keeps the host's RAG settings (its projects' kept as they are).
func (c *Client) SetRAGSettings(s proto.RAGSettings) (proto.RAGSettings, error) {
	var out proto.RAGSettings
	err := c.Call(proto.MethodRAGSetSettings, s, &out)
	return out, err
}

// RAGTest embeds a line with settings s.
func (c *Client) RAGTest(s proto.RAGSettings) (proto.RAGTestResult, error) {
	var out proto.RAGTestResult
	err := c.Call(proto.MethodRAGTest, s, &out)
	return out, err
}

// RAGStatus is a project's index.
func (c *Client) RAGStatus(ref proto.RAGRef) (proto.RAGStatus, error) {
	var out proto.RAGStatus
	err := c.Call(proto.MethodRAGStatus, ref, &out)
	return out, err
}

// SetRAGProject keeps a project's RAG settings.
func (c *Client) SetRAGProject(p proto.RAGProjectParams) (proto.RAGStatus, error) {
	var out proto.RAGStatus
	err := c.Call(proto.MethodRAGSetProject, p, &out)
	return out, err
}

// RAGIndex starts indexing a project.
func (c *Client) RAGIndex(ref proto.RAGRef) (proto.RAGStatus, error) {
	var out proto.RAGStatus
	err := c.Call(proto.MethodRAGIndex, ref, &out)
	return out, err
}

// RAGPlan is what indexing a project would send, and where.
func (c *Client) RAGPlan(ref proto.RAGRef) (proto.RAGPlan, error) {
	var out proto.RAGPlan
	err := c.Call(proto.MethodRAGPlan, ref, &out)
	return out, err
}

// RAGSearch searches a project by meaning.
func (c *Client) RAGSearch(p proto.RAGSearchParams) ([]proto.RAGHit, error) {
	var out []proto.RAGHit
	err := c.Call(proto.MethodRAGSearch, p, &out)
	return out, err
}

// RAGRemember keeps a memory of a project.
func (c *Client) RAGRemember(p proto.RAGRememberParams) (proto.RAGMemory, error) {
	var out proto.RAGMemory
	err := c.Call(proto.MethodRAGRemember, p, &out)
	return out, err
}

// RAGMemories are a project's memories.
func (c *Client) RAGMemories(ref proto.RAGRef) ([]proto.RAGMemory, error) {
	var out []proto.RAGMemory
	err := c.Call(proto.MethodRAGMemories, ref, &out)
	return out, err
}

// RAGForget removes a memory of a project, or changes its text.
func (c *Client) RAGForget(p proto.RAGForgetParams) error {
	return c.Call(proto.MethodRAGForget, p, nil)
}

// SetRAGKey keeps the embedder's key or the PostgreSQL URL on the host
// ("" removes it), and says the settings as they are then.
func (c *Client) SetRAGKey(which, value string) (proto.RAGSettings, error) {
	var out proto.RAGSettings
	err := c.Call(proto.MethodRAGSetKey, proto.RAGKey{Which: which, Value: value}, &out)
	return out, err
}

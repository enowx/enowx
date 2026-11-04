package proto

import "encoding/json"

// Hello is exchanged once in each direction when a connection opens.
type Hello struct {
	Protocol int    `json:"protocol"`
	Agent    string `json:"agent"` // "enowx/0.0.1" or "enowxd/0.0.1"
	// Parts: this side reads a frame over MaxPayload sent as parts (parts.go).
	Parts bool `json:"parts,omitempty"`
}

// Request is a client call. ID is unique per connection.
type Request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response answers the request with the same ID. Error is empty on success.
type Response struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Event is pushed by the daemon to every connection.
type Event struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data,omitempty"`
}

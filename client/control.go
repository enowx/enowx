package client

import (
	"encoding/json"
	"net"
	"strings"
	"sync"

	"github.com/enowx/enowx/proto"
)

// HostCall calls method with params on host ("" this one) through the
// daemon's host.call, as session from asks ("" the user's own terminal),
// decoding the result into out (which may be nil).
func (c *Client) HostCall(host, from, method string, params, out any) error {
	p := proto.HostCallParams{Host: host, Method: method, From: from}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		p.Params = raw
	}
	return c.Call(proto.MethodHostCall, p, out)
}

// OnHost is a client whose every call goes through c's daemon to host
// ("" c's own) with host.call, asked by session from: the same methods,
// for any host the app reaches. It has no events nor streams. Closing it
// leaves c open.
func (c *Client) OnHost(host, from string) (*Client, error) {
	near, far := net.Pipe()
	go relayCalls(c, host, from, far)
	return New(near, nil)
}

// relayCalls answers the requests on conn, a client's far end, each with
// a host.call on c.
func relayCalls(c *Client, host, from string, conn net.Conn) {
	defer conn.Close()
	if _, err := proto.ReadFrame(conn); err != nil { // the client's hello
		return
	}
	hello, _ := json.Marshal(proto.Hello{Protocol: c.Protocol, Agent: "enowx-relay/" + Version})
	var wmu sync.Mutex
	send := func(f proto.Frame) {
		wmu.Lock()
		defer wmu.Unlock()
		_ = proto.WriteFrame(conn, f)
	}
	send(proto.Frame{Type: proto.TypeHello, Payload: hello})
	for {
		f, err := proto.ReadFrame(conn)
		if err != nil {
			return
		}
		if f.Type != proto.TypeRequest {
			continue
		}
		var req proto.Request
		if json.Unmarshal(f.Payload, &req) != nil {
			continue
		}
		go func() {
			var raw json.RawMessage
			err := c.Call(proto.MethodHostCall, proto.HostCallParams{Host: host, Method: req.Method, Params: req.Params, From: from}, &raw)
			resp := proto.Response{ID: req.ID, Result: raw}
			if err != nil {
				resp.Result = nil
				resp.Error = strings.TrimPrefix(err.Error(), proto.MethodHostCall+": ")
			}
			payload, _ := json.Marshal(resp)
			send(proto.Frame{Type: proto.TypeResponse, Payload: payload})
		}()
	}
}

// ControlLog lists the requests made through host.call on the host lately.
func (c *Client) ControlLog() ([]proto.ControlLogEntry, error) {
	var out []proto.ControlLogEntry
	err := c.Call(proto.MethodHostControlLog, nil, &out)
	return out, err
}

// InterruptSession stops what session id's agent does, with its own key.
func (c *Client) InterruptSession(id string) error {
	return c.Call(proto.MethodSessionInterrupt, proto.SessionRef{ID: id}, nil)
}

// SessionAsk says what session id's agent waits on the user for.
func (c *Client) SessionAsk(id string) (proto.SessionAsk, error) {
	var out proto.SessionAsk
	err := c.Call(proto.MethodSessionAsk, proto.SessionRef{ID: id}, &out)
	return out, err
}

// AnswerSession answers what session id's agent asks: allow, always,
// deny, or a question's option number.
func (c *Client) AnswerSession(id, answer string) (proto.SessionAsk, error) {
	var out proto.SessionAsk
	err := c.Call(proto.MethodSessionAnswer, proto.SessionAnswerParams{ID: id, Answer: answer}, &out)
	return out, err
}

// AppSettings changes the app's settings set names, then reads them all.
func (c *Client) AppSettings(set map[string]string) (proto.AppSettings, error) {
	var out proto.AppSettings
	err := c.Call(proto.MethodAppSettings, proto.AppSettingsParams{Set: set}, &out)
	return out, err
}

// ProjectConfig reads a project's .enowx.toml.
func (c *Client) ProjectConfig(path string) (proto.ProjectConfig, error) {
	var out proto.ProjectConfig
	err := c.Call(proto.MethodProjectConfig, proto.ProjectRef{Path: path}, &out)
	return out, err
}

// SaveProjectConfig writes a project's .enowx.toml.
func (c *Client) SaveProjectConfig(cfg proto.ProjectConfig) (proto.ProjectConfig, error) {
	var out proto.ProjectConfig
	err := c.Call(proto.MethodProjectConfigSave, cfg, &out)
	return out, err
}

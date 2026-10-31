package daemon

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/enowx/enowx/proto"
)

// handler answers one RPC method on a connection.
type handler func(c *conn, params json.RawMessage) (any, error)

// handlers maps every method to its handler. Each rpc_*.go file lists the
// methods of its domain; add a method there, not here.
var handlers = merge(
	daemonHandlers,
	agentHandlers,
	accountHandlers,
	browserHandlers,
	projectHandlers,
	sessionHandlers,
	folderHandlers,
	envHandlers,
	worktreeHandlers,
	creditHandlers,
	reviewHandlers,
	fileHandlers,
	checkHandlers,
	commandHandlers,
	transferHandlers,
	projectSyncHandlers,
	trafficHandlers,
	androidHandlers,
	setupHandlers,
	hostSettingHandlers,
	pipelineHandlers,
	automationHandlers,
	hookHandlers,
	ragHandlers,
	decideHandlers,
	tokenHandlers,
	agentAPIHandlers,
	noteHandlers,
	noteProjectHandlers,
	syncHandlers,
	featureHandlers,
	notifyHandlers,
	noteIOHandlers,
	canvasHandlers,
	projectLinkHandlers,
	vaultHandlers,
	hibernateHandlers,
	cloudHandlers,
	sessionPeekHandlers,
	visitorHandlers,
	hostShareHandlers,
	teamChatHandlers,
	spacesGoneHandlers,
)

// slowMethods answer from a goroutine of their own, so that the
// connection's other requests and typing don't wait for them; their
// answers may overtake those of requests sent before.
var slowMethods = slow(methodsOf(reviewHandlers, tokenHandlers, cloudHandlers, sessionPeekHandlers, visitorHandlers, teamChatHandlers, fileHandlers, hibernateHandlers, noteIOHandlers, projectSyncHandlers, trafficHandlers, androidHandlers, agentAPIHandlers), proto.MethodSessionPorts, proto.MethodSessionDisplay, proto.MethodSessionDisplayStop, proto.MethodTunnelOpen,
	proto.MethodTransferPack, proto.MethodTransferRead, proto.MethodTransferWrite, proto.MethodTransferUnpack,
	proto.MethodSetupScan, proto.MethodEnvMCPTest, proto.MethodEnvSkillFetch, proto.MethodEnvSkillInstall, proto.MethodEnvPluginRun, proto.MethodChecksList, proto.MethodChecksRun, proto.MethodChecksSet,
	proto.MethodSessionRewind, proto.MethodAgentHistory, proto.MethodAutomationRun,
	proto.MethodSessionFanOut, proto.MethodBrowserAct, proto.MethodFeaturesRequest, proto.MethodNoteChat, proto.MethodNoteChatLog, proto.MethodCanvasRender,
	proto.MethodProjectCopyRequest, proto.MethodProjectSyncRequest, proto.MethodAutopushNow, proto.MethodAutopushSet, proto.MethodProjectKeepCopy, proto.MethodProjectFileRead,
	proto.MethodSessionCreate, proto.MethodSessionContinue,
	proto.MethodVaultAsk, proto.MethodVaultAttach, proto.MethodVaultScan,
	// An unslop asks the app it came from to read its page, and makes sessions.
	proto.MethodBrowserUnslop, proto.MethodBrowserUnslopShots, proto.MethodCanvasUnslop, proto.MethodCanvasBuild, proto.MethodCanvasMatch)

func methodsOf(groups ...map[string]handler) []string {
	var out []string
	for _, g := range groups {
		for method := range g {
			out = append(out, method)
		}
	}
	return out
}

func slow(methods []string, more ...string) map[string]bool {
	out := map[string]bool{}
	for _, m := range append(methods, more...) {
		out[m] = true
	}
	return out
}

func merge(groups ...map[string]handler) map[string]handler {
	all := map[string]handler{}
	for _, g := range groups {
		for method, h := range g {
			if _, dup := all[method]; dup {
				panic("daemon: method registered twice: " + method)
			}
			all[method] = h
		}
	}
	return all
}

// with adapts a handler taking decoded params of type P.
func with[P any](fn func(c *conn, p P) (any, error)) handler {
	return func(c *conn, raw json.RawMessage) (any, error) {
		var p P
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return fn(c, p)
	}
}

// bare adapts a handler that takes no params.
func bare(fn func(c *conn) (any, error)) handler {
	return func(c *conn, _ json.RawMessage) (any, error) { return fn(c) }
}

// ok is the result of methods that return nothing.
var ok = struct{}{}

func (c *conn) handle(req proto.Request) (any, error) {
	h, found := handlers[req.Method]
	if !found {
		return nil, fmt.Errorf("unknown method %q", req.Method)
	}
	if err := c.mayCall(req.Method); err != nil {
		return nil, err
	}
	if o, ok := visitorOverrides[req.Method]; ok && c.visitor.Load() != nil {
		return o(c, req.Params) // within what is shared with the visitor's team
	}
	return h(c, req.Params)
}

func (c *conn) reply(id uint64, value any, err error) {
	resp := proto.Response{ID: id}
	if err != nil {
		resp.Error = err.Error()
	} else if raw, merr := json.Marshal(value); merr != nil {
		resp.Error = merr.Error()
	} else {
		resp.Result = raw
	}
	payload, _ := json.Marshal(resp)
	// A file can grow after a sync manifest was read. Return an RPC error
	// instead of letting send close the connection on an oversized reply.
	if len(payload) > proto.MaxPayload {
		payload, _ = json.Marshal(proto.Response{ID: id,
			Error: fmt.Sprintf("response payload of %d bytes exceeds the %d-byte frame limit", len(payload), proto.MaxPayload)})
	}
	c.send(proto.Frame{Type: proto.TypeResponse, Payload: payload})
}

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return errors.New("missing params")
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("bad params: %w", err)
	}
	return nil
}

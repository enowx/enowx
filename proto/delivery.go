package proto

// MethodSessionDeliver queues one Chat message in a particular program run.
// Unlike session.input, replaying the same key never queues its text or Enter
// again. Older daemons refuse the method; callers must not fall back to input.
const MethodSessionDeliver = "session.deliver"

// DeliveryParams identify one logical send, including across RPC retries.
type DeliveryParams struct {
	ID   string `json:"id"`
	Run  int64  `json:"run"`
	Key  string `json:"key"`
	Text string `json:"text"`
}

// DeliveryResult describes queued terminal input, never agent acknowledgement.
// Interrupted means other input or a run change prevented the delayed Enter.
// Neither state authorizes pasting or pressing Enter again: inspect the terminal.
type DeliveryResult struct {
	Interrupted bool `json:"interrupted,omitempty"`
}

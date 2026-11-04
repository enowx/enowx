package client

import "github.com/enowx/enowx/proto"

// Deliver queues one message once. Keep Key and Run unchanged on retries.
// Success means terminal input was queued, not that the agent recorded a message.
func (c *Client) Deliver(p proto.DeliveryParams) (proto.DeliveryResult, error) {
	var result proto.DeliveryResult
	err := c.Call(proto.MethodSessionDeliver, p, &result)
	return result, err
}

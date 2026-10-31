package client

import "github.com/enowx/enowx/proto"

// DecideSettings are the host's decisions' settings.
func (c *Client) DecideSettings() (proto.DecideSettings, error) {
	var out proto.DecideSettings
	err := c.Call(proto.MethodDecideSettings, nil, &out)
	return out, err
}

// SetDecideSettings keeps them.
func (c *Client) SetDecideSettings(s proto.DecideSettings) (proto.DecideSettings, error) {
	var out proto.DecideSettings
	err := c.Call(proto.MethodDecideSetSettings, s, &out)
	return out, err
}

// DecideJudges are the judges on the host, in the order tried.
func (c *Client) DecideJudges() ([]proto.Judge, error) {
	var out []proto.Judge
	err := c.Call(proto.MethodDecideJudges, nil, &out)
	return out, err
}

// TestJudges tests judge id (all when "").
func (c *Client) TestJudges(id string) ([]proto.Judge, error) {
	var out []proto.Judge
	err := c.Call(proto.MethodDecideTest, proto.JudgeRef{ID: id}, &out)
	return out, err
}

// DecideLog is the latest decisions, as p filters them.
func (c *Client) DecideLog(p proto.DecideLogParams) ([]proto.Decision, error) {
	var out []proto.Decision
	err := c.Call(proto.MethodDecideLog, p, &out)
	return out, err
}

// MarkDecision says a decision was wrong (or not).
func (c *Client) MarkDecision(m proto.DecideMark) error {
	return c.Call(proto.MethodDecideMark, m, nil)
}

// AskDecision asks the host's judge a's question; an error when its use is
// off there.
func (c *Client) AskDecision(a proto.DecideAsk) (proto.Decision, error) {
	var out proto.Decision
	err := c.Call(proto.MethodDecideAsk, a, &out)
	return out, err
}

// DecideRoute is a model and effort for p's prompt, as the judge weighs
// how hard it is.
func (c *Client) DecideRoute(p proto.DecideRouteParams) (proto.RouteChoice, error) {
	var out proto.RouteChoice
	err := c.Call(proto.MethodDecideRoute, p, &out)
	return out, err
}

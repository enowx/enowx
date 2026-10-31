package client

import "github.com/enowx/enowx/proto"

// SpacesGone lists the teams whose spaces left the host as they were
// deleted, not seen yet; none from a daemon of before.
func (c *Client) SpacesGone() ([]proto.SpaceGone, error) {
	var list []proto.SpaceGone
	err := c.Call(proto.MethodSpacesGone, nil, &list)
	if IsUnknownMethod(err) {
		return nil, nil
	}
	return list, err
}

// SpaceGoneSeen forgets team's notice: the user saw it.
func (c *Client) SpaceGoneSeen(team string) error {
	err := c.Call(proto.MethodSpaceGoneSeen, proto.SpaceRef{Team: team}, nil)
	if IsUnknownMethod(err) {
		return nil
	}
	return err
}

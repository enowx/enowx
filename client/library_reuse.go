package client

import "github.com/enowx/enowx/proto"

// LibraryUseInfo says what the host's cloud says of an item.
func (c *Client) LibraryUseInfo(p proto.LibraryUseRef) (proto.LibraryUseInfo, error) {
	var out proto.LibraryUseInfo
	err := c.Call(proto.MethodLibraryUseInfo, p, &out)
	return out, err
}

// LibraryOpen makes a copy of a design on a new canvas.
func (c *Client) LibraryOpen(p proto.LibraryOpenParams) (proto.LibraryOpened, error) {
	var out proto.LibraryOpened
	err := c.Call(proto.MethodLibraryOpen, p, &out)
	return out, err
}

// LibraryReference keeps a design as a canvas's reference.
func (c *Client) LibraryReference(p proto.LibraryReferenceParams) (proto.LibraryReferenced, error) {
	var out proto.LibraryReferenced
	err := c.Call(proto.MethodLibraryReference, p, &out)
	return out, err
}

// LibrarySkill reads a design's skill, and installs it when asked.
func (c *Client) LibrarySkill(p proto.LibrarySkillParams) (proto.LibrarySkillAnswer, error) {
	var out proto.LibrarySkillAnswer
	err := c.Call(proto.MethodLibrarySkill, p, &out)
	return out, err
}

// LibraryNewer says how a copy stands against what it was copied from.
func (c *Client) LibraryNewer(canvas string) (proto.LibraryNewer, error) {
	var out proto.LibraryNewer
	err := c.Call(proto.MethodLibraryNewer, proto.CanvasRef{ID: canvas}, &out)
	return out, err
}

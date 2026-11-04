package client

import "github.com/enowx/enowx/proto"

// LibrarySearch lists what's live in the library of the host's cloud.
func (c *Client) LibrarySearch(p proto.LibrarySearchParams) (proto.LibraryResults, error) {
	var out proto.LibraryResults
	err := c.Call(proto.MethodLibrarySearch, p, &out)
	return out, err
}

// LibraryItem is one live item of the library.
func (c *Client) LibraryItem(id string) (proto.LibraryListing, error) {
	var out proto.LibraryListing
	err := c.Call(proto.MethodLibraryItem, proto.LibraryItemRef{ItemID: id}, &out)
	return out, err
}

// LibraryPrompt is an item's prompt with its credit; a view-only item has none.
func (c *Client) LibraryPrompt(id string) (proto.LibraryPromptAnswer, error) {
	var out proto.LibraryPromptAnswer
	err := c.Call(proto.MethodLibraryPrompt, proto.LibraryItemRef{ItemID: id}, &out)
	return out, err
}

// LibraryPicture is a live version's picture, as a WebP.
func (c *Client) LibraryPicture(p proto.LibraryPictureParams) ([]byte, error) {
	var out proto.LibraryPicture
	err := c.Call(proto.MethodLibraryPicture, p, &out)
	return out.Data, err
}

// LibraryAskPublish asks the app to show the user a design's publish form.
func (c *Client) LibraryAskPublish(p proto.LibraryAskPublishParams) (proto.LibraryPublishAsked, error) {
	var out proto.LibraryPublishAsked
	err := c.Call(proto.MethodLibraryAskPublish, p, &out)
	return out, err
}

// LibraryAskSkill asks the app to show the user a card for keeping a design as a skill.
func (c *Client) LibraryAskSkill(p proto.LibraryAskSkillParams) (proto.LibrarySkillAsked, error) {
	var out proto.LibrarySkillAsked
	err := c.Call(proto.MethodLibraryAskSkill, p, &out)
	return out, err
}

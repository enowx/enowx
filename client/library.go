package client

import "github.com/enowx/enowx/proto"

// LibraryPrepare checks a design and packs it as a draft, for the form.
func (c *Client) LibraryPrepare(p proto.LibraryPrepareParams) (proto.LibraryPrepared, error) {
	var out proto.LibraryPrepared
	err := c.Call(proto.MethodLibraryPrepare, p, &out)
	return out, err
}

// LibrarySaveDraft keeps what the form holds, on the host.
func (c *Client) LibrarySaveDraft(p proto.LibraryDraftParams) error {
	return c.Call(proto.MethodLibrarySaveDraft, p, nil)
}

// LibraryDeleteDraft forgets a draft.
func (c *Client) LibraryDeleteDraft(id string) error {
	return c.Call(proto.MethodLibraryDeleteDraft, proto.LibraryDraftParams{DraftID: id}, nil)
}

// LibraryPublish sends a draft to the cloud.
func (c *Client) LibraryPublish(p proto.LibraryPublishParams) (proto.LibraryDesign, error) {
	var out proto.LibraryDesign
	err := c.Call(proto.MethodLibraryPublish, p, &out)
	return out, err
}

// LibraryStatus says whether the host's cloud has a library, and how the
// account's designs stand.
func (c *Client) LibraryStatus() (proto.LibraryState, error) {
	var out proto.LibraryState
	err := c.Call(proto.MethodLibraryStatus, nil, &out)
	return out, err
}

// LibraryWithdraw takes a published design back.
func (c *Client) LibraryWithdraw(id string) (proto.LibraryDesign, error) {
	var out proto.LibraryDesign
	err := c.Call(proto.MethodLibraryWithdraw, proto.LibraryItemRef{ItemID: id}, &out)
	return out, err
}

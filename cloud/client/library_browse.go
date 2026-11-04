package client

import (
	"context"
	"net/url"
	"strings"

	"github.com/enowx/enowx/cloud/api"
)

// The library's public reads, as anyone may make them.

// LibrarySearch is a page of what's live, narrowed by q.
func (c *Client) LibrarySearch(ctx context.Context, q url.Values) (api.LibraryItems, error) {
	path := api.PathLibraryItems
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out api.LibraryItems
	status, body, err := c.Raw(ctx, "GET", path, nil)
	return out, answerLibrary(status, body, err, &out)
}

func libraryItemPath(pattern, id string) string {
	return strings.ReplaceAll(pattern, "{id}", url.PathEscape(id))
}

// LibraryItem is one item.
func (c *Client) LibraryItem(ctx context.Context, id string) (api.LibraryItem, error) {
	var out api.LibraryItem
	status, body, err := c.Raw(ctx, "GET", libraryItemPath(api.PathLibraryItem, id), nil)
	return out, answerLibrary(status, body, err, &out)
}

// LibraryPrompt is an item's prompt with its credit; 403 when it's view only.
func (c *Client) LibraryPrompt(ctx context.Context, id string) (string, error) {
	var out api.LibraryPrompt
	status, body, err := c.Raw(ctx, "GET", libraryItemPath(api.PathLibraryPrompt, id), nil)
	return out.Prompt, answerLibrary(status, body, err, &out)
}

// LibraryPicture is a live version's picture.
func (c *Client) LibraryPicture(ctx context.Context, id string, version int, part string) ([]byte, error) {
	status, body, err := c.Raw(ctx, "GET", libraryVersionPath(api.PathLibraryPicture, url.PathEscape(id), version, part), nil)
	if err := answerLibrary(status, body, err, nil); err != nil {
		return nil, err
	}
	return body, nil
}

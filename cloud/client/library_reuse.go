package client

import (
	"context"
	"strconv"
	"strings"

	"github.com/enowx/enowx/cloud/api"
)

// libraryReusePath is pattern for item id, at version when it's asked for.
func libraryReusePath(pattern, id string, version int) string {
	p := strings.ReplaceAll(pattern, "{id}", id)
	if version > 0 {
		p += "?version=" + strconv.Itoa(version)
	}
	return p
}

// LibraryAbout is an item as anyone may see it.
func (c *Client) LibraryAbout(ctx context.Context, id string) (api.LibraryItem, error) {
	var out api.LibraryItem
	status, body, err := c.Raw(ctx, "GET", libraryReusePath(api.PathLibraryItem, id, 0), nil)
	return out, answerLibrary(status, body, err, &out)
}

// LibraryVersions are the versions of an item that show or showed.
func (c *Client) LibraryVersions(ctx context.Context, id string) ([]api.LibraryVersion, error) {
	var out []api.LibraryVersion
	status, body, err := c.Raw(ctx, "GET", libraryReusePath(api.PathLibraryVersions, id, 0), nil)
	return out, answerLibrary(status, body, err, &out)
}

// LibrarySource is a version's bundle for a copy, waiting as long as a file takes.
func (c *Client) LibrarySource(ctx context.Context, id string, version int) (api.LibrarySource, error) {
	var out api.LibrarySource
	status, body, err := c.slow().Raw(ctx, "GET", libraryReusePath(api.PathLibrarySource, id, version), nil)
	return out, answerLibrary(status, body, err, &out)
}

// LibraryReference is a full page's picture, as a WebP.
func (c *Client) LibraryReference(ctx context.Context, id string, version int) ([]byte, error) {
	status, body, err := c.slow().Raw(ctx, "GET", libraryReusePath(api.PathLibraryReference, id, version), nil)
	if err := answerLibrary(status, body, err, nil); err != nil {
		return nil, err
	}
	return body, nil
}

// LibrarySkill is a full page's SKILL.md.
func (c *Client) LibrarySkill(ctx context.Context, id string, version int) (string, error) {
	status, body, err := c.slow().Raw(ctx, "GET", libraryReusePath(api.PathLibrarySkill, id, version), nil)
	if err := answerLibrary(status, body, err, nil); err != nil {
		return "", err
	}
	return string(body), nil
}

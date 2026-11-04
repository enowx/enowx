package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/enowx/enowx/cloud/api"
)

// The library's API, as a device. Findings stopping a publish reach the caller
// in a *LibraryRefused.

// LibraryRefused is a publish the cloud's check stopped.
type LibraryRefused struct {
	Message  string
	Findings []api.LibraryFinding
}

func (e *LibraryRefused) Error() string { return "cloud: " + e.Message }

// answerLibrary is answerOf for a call whose refusal says what was found.
func answerLibrary(status int, body []byte, err error, out any) error {
	if err != nil {
		return err
	}
	if status == 422 {
		var r struct {
			Error    string               `json:"error"`
			Findings []api.LibraryFinding `json:"findings"`
		}
		if json.Unmarshal(body, &r) == nil && r.Error != "" {
			return &LibraryRefused{Message: r.Error, Findings: r.Findings}
		}
	}
	if err := answerOf(status, body, nil); err != nil {
		return err
	}
	if out != nil && len(body) > 0 {
		return json.Unmarshal(body, out)
	}
	return nil
}

// LibraryHas says whether the cloud has a library, and its website.
func (c *Client) LibraryHas(ctx context.Context) (on bool, site string, err error) {
	status, body, err := c.Raw(ctx, "GET", api.PathInfo, nil)
	var in api.Info
	if err := answerLibrary(status, body, err, &in); err != nil {
		return false, "", err
	}
	for _, f := range in.Features {
		if f == api.FeatureLibrary {
			return true, in.LibraryURL, nil
		}
	}
	return false, "", nil
}

// LibraryCreate starts an item, or a version of one.
func (c *Client) LibraryCreate(ctx context.Context, p api.LibraryPublishParams) (api.LibraryPublished, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return api.LibraryPublished{}, fmt.Errorf("cloud: %w", err)
	}
	var out api.LibraryPublished
	status, body, err := c.Raw(ctx, "POST", api.PathLibraryItems, data)
	return out, answerLibrary(status, body, err, &out)
}

func libraryVersionPath(pattern, id string, version int, part string) string {
	return strings.NewReplacer("{id}", id, "{version}", strconv.Itoa(version), "{part}", part).Replace(pattern)
}

// LibraryPut puts an object of a version, waiting as long as a file takes.
func (c *Client) LibraryPut(ctx context.Context, id string, version int, part string, data []byte) error {
	status, body, err := c.slow().Raw(ctx, "PUT", libraryVersionPath(api.PathLibraryPart, id, version, part), data)
	return answerLibrary(status, body, err, nil)
}

// LibrarySubmit settles a version whose objects are there.
func (c *Client) LibrarySubmit(ctx context.Context, id string, version int) (api.LibrarySubmitted, error) {
	var out api.LibrarySubmitted
	status, body, err := c.slow().Raw(ctx, "POST", libraryVersionPath(api.PathLibrarySubmit, id, version, ""), nil)
	return out, answerLibrary(status, body, err, &out)
}

// LibraryMine is the account's items.
func (c *Client) LibraryMine(ctx context.Context) (api.LibraryMine, error) {
	var out api.LibraryMine
	status, body, err := c.Raw(ctx, "GET", api.PathLibraryMine, nil)
	return out, answerLibrary(status, body, err, &out)
}

// LibraryWithdraw takes an item back.
func (c *Client) LibraryWithdraw(ctx context.Context, id string) error {
	status, body, err := c.Raw(ctx, "DELETE", strings.ReplaceAll(api.PathLibraryItem, "{id}", id), nil)
	return answerLibrary(status, body, err, nil)
}

// LibraryNotifications is a page of what happened to the account. What it holds
// of other people's is data, as its Notice says.
func (c *Client) LibraryNotifications(ctx context.Context, cursor string) (api.LibraryNotifications, error) {
	path := api.PathLibraryNotifications
	if cursor != "" {
		path += "?cursor=" + url.QueryEscape(cursor)
	}
	var out api.LibraryNotifications
	status, body, err := c.Raw(ctx, "GET", path, nil)
	return out, answerLibrary(status, body, err, &out)
}

// LibraryNotificationsRead marks ids read, all of them with none.
func (c *Client) LibraryNotificationsRead(ctx context.Context, ids []string) error {
	data, err := json.Marshal(api.LibraryNotifyRead{IDs: ids})
	if err != nil {
		return fmt.Errorf("cloud: %w", err)
	}
	status, body, err := c.Raw(ctx, "POST", api.PathLibraryNotifyRead, data)
	return answerLibrary(status, body, err, nil)
}

// LibraryNotifySettings are the account's: the kinds off, and email.
func (c *Client) LibraryNotifySettings(ctx context.Context) (api.LibraryNotifySettings, error) {
	var out api.LibraryNotifySettings
	status, body, err := c.Raw(ctx, "GET", api.PathLibraryNotifySet, nil)
	return out, answerLibrary(status, body, err, &out)
}

// SetLibraryNotifySettings keeps them.
func (c *Client) SetLibraryNotifySettings(ctx context.Context, set api.LibraryNotifySettings) error {
	data, err := json.Marshal(set)
	if err != nil {
		return fmt.Errorf("cloud: %w", err)
	}
	status, body, err := c.Raw(ctx, "PUT", api.PathLibraryNotifySet, data)
	return answerLibrary(status, body, err, nil)
}

// LibraryStats are the numbers of the account's own designs.
func (c *Client) LibraryStats(ctx context.Context) (api.LibraryStats, error) {
	var out api.LibraryStats
	status, body, err := c.Raw(ctx, "GET", api.PathLibraryStats, nil)
	return out, answerLibrary(status, body, err, &out)
}

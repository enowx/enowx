package api

import (
	"regexp"
	"slices"
	"strings"
)

var handleShape = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// handlesKept are what a handle can't be: they'd pass for the site or its staff, or
// read as one of its addresses.
var handlesKept = []string{"admin", "administrator", "api", "auth", "browse", "collections", "creator", "creators", "design", "designs", "enowx", "enowx-ui", "feed", "help", "library", "me", "moderation", "moderator", "moderators", "new", "official", "open-for-work", "owner", "root", "rules", "saved", "security", "settings", "staff", "support", "system", "team", "terms", "ui"}

// LibraryHandleProblem says what's wrong with a new handle, "" for nothing.
func LibraryHandleProblem(h string) string {
	switch {
	case len(h) < 3 || len(h) > 30:
		return "a handle is 3 to 30 characters"
	case !handleShape.MatchString(h):
		return "a handle is small letters, digits and single hyphens, and starts and ends with a letter or digit"
	case slices.Contains(handlesKept, h) || strings.HasPrefix(h, "deleted-"):
		return "that handle is taken"
	}
	return ""
}

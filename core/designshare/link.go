package designshare

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// The enowx:// links the library's website opens the app with. Any page on the
// web can open one, with anything in it: it says only what's asked for, the app
// shows what the cloud says of that item, and nothing happens until the user
// says so.

// LinkScheme is the scheme the app handles.
const LinkScheme = "enowx"

// What a link asks.
const (
	LinkOpen      = "open"
	LinkReference = "reference"
	LinkSkill     = "skill"
)

// AppLink is an enowx:// link read.
type AppLink struct {
	Action string
	Item   string
	// Version is the version asked for, 0 for what shows.
	Version int
}

var itemID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// ValidItem says id can be an item's ID.
func ValidItem(id string) bool { return itemID.MatchString(id) }

// ParseAppLink reads enowx://library/<action>?item=<id>&version=<n>.
func ParseAppLink(raw string) (AppLink, error) {
	bad := errors.New("library: this isn't a link enowX knows")
	if len(raw) > 512 {
		return AppLink{}, bad
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, LinkScheme) || u.Host != "library" {
		return AppLink{}, bad
	}
	l := AppLink{Action: strings.Trim(u.Path, "/"), Item: u.Query().Get("item")}
	if l.Action != LinkOpen && l.Action != LinkReference && l.Action != LinkSkill {
		return AppLink{}, bad
	}
	if !ValidItem(l.Item) {
		return AppLink{}, bad
	}
	if v := u.Query().Get("version"); v != "" {
		if l.Version, err = strconv.Atoi(v); err != nil || l.Version < 1 {
			return AppLink{}, bad
		}
	}
	return l, nil
}

// LinkURL is the link that asks action of item at version (0 for what shows).
func LinkURL(action, item string, version int) string {
	q := url.Values{"item": {item}}
	if version > 0 {
		q.Set("version", strconv.Itoa(version))
	}
	return LinkScheme + "://library/" + action + "?" + q.Encode()
}

// ItemFromRef is the item an argument names: its ID, or its address on the
// website (/d/<id>/… or /design/<id>/…).
func ItemFromRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ValidItem(ref) {
		return ref, nil
	}
	if u, err := url.Parse(ref); err == nil {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i := 0; i+1 < len(parts); i++ {
			if (parts[i] == "d" || parts[i] == "design") && ValidItem(parts[i+1]) {
				return parts[i+1], nil
			}
		}
		if strings.EqualFold(u.Scheme, LinkScheme) {
			if l, err := ParseAppLink(ref); err == nil {
				return l.Item, nil
			}
		}
	}
	return "", errors.New("library: name a design by its ID or its address")
}

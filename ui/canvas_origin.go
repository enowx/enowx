package ui

import (
	"crypto/rand"
	"strings"
)

// designOrigin gives each mockup its own origin, isolated from the app and
// other designs. WebView2 normalizes hostnames before requesting them, so the
// URL must match its normalized form when MyGo serves the document.
func designOrigin() string {
	return "https://" + strings.ToLower(rand.Text()) + ".design.invalid/"
}

package ui

import (
	"crypto/rand"
	"strings"
)

// designOrigin is the address a mockup's page loads at, a new one each time. A
// page with no address counts as the app's own and may call into it (close the
// window, say); at this one it can't, and what it stores goes with it, so no
// design reaches another's. It is in lower case as WebView2 normalizes hostnames
// before requesting them, and the URL must match that form for MyGo to serve the
// document.
func designOrigin() string {
	return "https://" + strings.ToLower(rand.Text()) + ".design.invalid/"
}

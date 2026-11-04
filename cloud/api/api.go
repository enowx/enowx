// Package api is the cloud's HTTP API: its paths and the JSON they take
// and give, shared by the server (cloud/server) and its clients (the app,
// daemons). Like the daemon's protocol it only grows: a path, field or
// value once released stays (docs/cloud.md).
package api

// DefaultURL is enowX's own cloud, which the app signs in to unless the
// host was set to another (enowx-app cloud url).
const DefaultURL = "https://cloud.enowx.ai"

// Version is the API's version, raised when it gains something a client
// may look for; a server says its own in Info.
const Version = 1

// Paths.
const (
	PathHealth = "/healthz"
	PathInfo   = "/v1/info"
)

// Sign-in methods a server offers (Info.SignIn).
const (
	SignInGitHub = "github"
)

// Info is what a server is and can do, asked before anything else.
type Info struct {
	// Name says it is an enowX cloud.
	Name string `json:"name"`
	// Version is the server's release; API the API version it speaks.
	Version string `json:"version"`
	API     int    `json:"api"`
	// URL is the address its members reach it at.
	URL string `json:"url,omitempty"`
	// SignIn lists the sign-in methods set up (SignInGitHub).
	SignIn []string `json:"signIn"`
	// Features lists what this server has beyond teams (FeatureLibrary).
	Features []string `json:"features,omitempty"`
}

// Error is the body of a request that failed.
type Error struct {
	Error string `json:"error"`
}

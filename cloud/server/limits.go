package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// What every answer carries, and how often one address may knock on the
// doors open without a token: starting a device's sign-in, polling it,
// peeking at an invite, the device and invite pages. Limits are kept in
// this server's memory, per address, the proxy's word for it taken when
// it gives one (Cloudflare's, then X-Forwarded-For's first).

// limits are the requests an address may make to a door in a minute.
var limits = map[string]int{
	"POST /v1/auth/device":           10,
	"POST /v1/auth/device/token":     40, // a device polls every few seconds
	"POST /v1/invites/peek":          20,
	"GET /device":                    30,
	"POST /device":                   20,
	"POST /invite":                   20,
	"POST /invite/accept":            20,
	"GET /v1/auth/github":            20,
	"GET " + pathContributorPage:     30,
	"GET " + pathContributorStart:    20,
	"GET " + pathContributorCallback: 20,
	"GET /library/authorize":         30,
	"POST /v1/library/auth/exchange": 60,
}

// limiter counts requests per door and address, a minute at a time.
// Handlers on net/http's goroutines share it under mu.
type limiter struct {
	mu     sync.Mutex
	window time.Time
	counts map[string]int
}

// allow counts one request to door from addr, and says whether it is
// within the door's limit.
func (l *limiter) allow(door, addr string, max int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.Sub(l.window) > time.Minute || l.counts == nil {
		l.window, l.counts = now, map[string]int{}
	}
	k := door + " " + addr
	l.counts[k]++
	return l.counts[k] <= max
}

// clientAddr is where a request comes from.
func clientAddr(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
		return ip
	}
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// guard sets the headers every answer carries and refuses what may not
// go on: plain HTTP behind a proxy serving HTTPS, an address over a
// door's limit. It reports whether the request goes on.
func (s *Server) guard(w http.ResponseWriter, r *http.Request) bool {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store") // tokens, teams, pages signed in
	if s.secure() {
		h.Set("Strict-Transport-Security", "max-age=31536000")
		if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "http") {
			http.Redirect(w, r, s.base()+r.URL.RequestURI(), http.StatusPermanentRedirect)
			return false
		}
	}
	if max, ok := limits[r.Method+" "+r.URL.Path]; ok && !s.limits.allow(r.Method+" "+r.URL.Path, clientAddr(r), max) {
		w.Header().Set("Retry-After", "60")
		s.fail(w, http.StatusTooManyRequests, "too many tries from here: wait a minute")
		return false
	}
	return true
}

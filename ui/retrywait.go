package ui

import "time"

// connectSteady is how long a connection has to last to count as the host being
// back: one dropped sooner, as by a daemon failing each time, keeps backing off.
const connectSteady = time.Minute

// retryWait is how long to wait before trying a host again: double each time, up
// to most, from first again once a connection held. Its one user owns it.
type retryWait struct {
	first, most time.Duration
	next        time.Duration
}

// failed says how long to wait after a try that failed or didn't last.
func (r *retryWait) failed() time.Duration {
	wait := max(r.next, r.first)
	r.next = min(wait*2, r.most)
	return min(wait, r.most)
}

// held starts over, a connection having lasted.
func (r *retryWait) held() { r.next = r.first }

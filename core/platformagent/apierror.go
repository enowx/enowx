package platformagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// failure is what a failed request means for the loop.
type failure struct {
	status        int
	retryable     bool
	retryAfter    time.Duration
	describe      string // for a Notice: "rate limited"
	message       string // the API's own words
	boundThinking bool   // preserved thinking refused edited history
}

// classify reads an error from the stream or the request.
func classify(err error) failure {
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		// Not an answer of the API's: the connection, a stream cut short.
		return failure{retryable: true, describe: "connection lost"}
	}
	f := failure{status: apiErr.StatusCode, message: apiMessage(apiErr)}
	// An error event in a stream comes with the stream's 200.
	if f.status < 400 {
		f.status = statusOfType(string(apiErr.Type()))
	}
	switch {
	case f.status == http.StatusTooManyRequests:
		f.retryable, f.describe = true, "rate limited"
	case f.status == 529:
		f.retryable, f.describe = true, "the API is overloaded"
	case f.status == http.StatusRequestTimeout:
		f.retryable, f.describe = true, "the request timed out"
	case f.status >= 500:
		f.retryable, f.describe = true, fmt.Sprintf("the API failed (%d)", f.status)
	default:
		f.describe = fmt.Sprintf("the API refused the request (%d)", f.status)
	}
	if f.retryable && apiErr.Response != nil {
		f.retryAfter = parseRetryAfter(apiErr.Response.Header.Get("retry-after"), time.Now())
	}
	f.boundThinking = f.status == http.StatusBadRequest && strings.Contains(f.message, "bound to a different conversation")
	return f
}

func statusOfType(kind string) int {
	switch kind {
	case "rate_limit_error":
		return http.StatusTooManyRequests
	case "overloaded_error":
		return 529
	case "timeout_error":
		return http.StatusRequestTimeout
	case "invalid_request_error":
		return http.StatusBadRequest
	case "authentication_error":
		return http.StatusUnauthorized
	case "billing_error":
		return http.StatusPaymentRequired
	case "permission_error":
		return http.StatusForbidden
	case "not_found_error":
		return http.StatusNotFound
	case "request_too_large":
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusInternalServerError
}

// final is the error for a request that won't pass, in the user's terms.
func (f failure) final(scrub func(string) string) error {
	said := scrub(f.message)
	switch f.status {
	case http.StatusUnauthorized:
		return errors.New("the API key is wrong: the API refused it. Change it in enowX (Settings, vault, platform anthropic)")
	case http.StatusPaymentRequired:
		return fmt.Errorf("the API account can't pay for this: %s", said)
	case http.StatusForbidden:
		return fmt.Errorf("this API key isn't allowed to do this: %s", said)
	case http.StatusNotFound:
		return fmt.Errorf("the API doesn't know this model, or this key can't use it: %s", said)
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge:
		return fmt.Errorf("the API refused the request: %s", said)
	}
	if said != "" {
		return fmt.Errorf("%s: %s", f.describe, said)
	}
	return errors.New(f.describe)
}

// apiMessage is error.message of the API's error body.
func apiMessage(apiErr *anthropic.Error) string {
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(apiErr.RawJSON()), &body) != nil {
		return ""
	}
	return body.Error.Message
}

// parseRetryAfter reads the header: seconds or a date.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds >= 0 {
		// Capped so that a huge value doesn't overflow into a negative wait.
		return time.Duration(min(seconds, 1e9) * float64(time.Second))
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(at.Sub(now), 0)
	}
	return 0
}

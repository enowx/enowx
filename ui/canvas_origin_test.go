package ui

import (
	"net/url"
	"strings"
	"testing"
)

func TestDesignOriginCanonicalAndIsolated(t *testing.T) {
	seen := make(map[string]bool)
	for range 128 {
		origin := designOrigin()
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Path != "/" || u.Host != strings.ToLower(u.Host) || !strings.HasSuffix(u.Host, ".design.invalid") {
			t.Fatalf("invalid or non-canonical design origin %q: %v", origin, err)
		}
		label := strings.TrimSuffix(u.Host, ".design.invalid")
		if len(label) < 20 || strings.Trim(label, "abcdefghijklmnopqrstuvwxyz234567") != "" {
			t.Fatalf("unexpected random host label %q", label)
		}
		if seen[origin] {
			t.Fatalf("duplicate origin %q", origin)
		}
		seen[origin] = true
	}
}

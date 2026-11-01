package session

import (
	"strings"
	"testing"
	"time"
)

func TestExitWhy(t *testing.T) {
	if ExitWhy(0, time.Hour) != "" {
		t.Fatal("a clean exit needs no reason")
	}
	got := ExitWhy(129, 3*time.Second)
	if !strings.Contains(got, "SIGHUP") || !strings.Contains(got, "never got going") {
		t.Fatal(got)
	}
	if got := ExitWhy(1, time.Hour); strings.Contains(got, "never got going") || !strings.Contains(got, "code 1") {
		t.Fatal(got)
	}
}

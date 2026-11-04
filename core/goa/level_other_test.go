//go:build !windows

package goa

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

func TestLevels(t *testing.T) {
	for id, want := range map[string]string{"claude": proto.GOAEnforced, "codex": proto.GOAEnforced, "aider": proto.GOAWatched} {
		if got, _ := LevelOf(id); got != want {
			t.Errorf("%s: got %s want %s", id, got, want)
		}
	}
}

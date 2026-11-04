package goa

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

func TestLevels(t *testing.T) {
	// POSIX hook commands aren't installed on Windows. Tool restrictions
	// alone limit Claude; Codex and agents without either remain watched.
	for id, want := range map[string]string{"claude": proto.GOALimited, "codex": proto.GOAWatched, "aider": proto.GOAWatched} {
		if got, _ := LevelOf(id); got != want {
			t.Errorf("%s: got %s want %s", id, got, want)
		}
	}
}

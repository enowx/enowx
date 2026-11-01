package libraryview

import (
	"slices"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	got := splitCommand(`npx -y "@scope/server files" --root '~/my code'`)
	want := []string{"npx", "-y", "@scope/server files", "--root", "~/my code"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}

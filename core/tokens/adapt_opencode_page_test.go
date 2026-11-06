package tokens

import (
	"path/filepath"
	"testing"
)

// Read a page at a time, even of one part, OpenCode's database gives the same steps as
// read whole, and a second read gives none again (issue #47).
func TestOpenCodeReadInPagesCountsTheSame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.db")
	openCodeDB(t, path)
	whole, err := readOpenCode(path, &FileState{})
	if err != nil {
		t.Fatal(err)
	}
	saved := opencodePage
	t.Cleanup(func() { opencodePage = saved })
	opencodePage = 1
	st := &FileState{}
	paged, err := readOpenCode(path, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(whole) == 0 || len(paged) != len(whole) {
		t.Fatalf("whole %d, paged %d", len(whole), len(paged))
	}
	for i := range whole {
		if whole[i].ID != paged[i].ID || whole[i].Input != paged[i].Input || whole[i].Output != paged[i].Output {
			t.Errorf("%d: %+v, paged %+v", i, whole[i], paged[i])
		}
	}
	if again, _ := readOpenCode(path, st); len(again) != 0 {
		t.Fatalf("read again: %d", len(again))
	}
}

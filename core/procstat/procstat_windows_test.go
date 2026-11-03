package procstat

import (
	"testing"
	"unicode/utf16"
)

// A session's tag is found in an environment block, its name in any case.
func TestEnvValue(t *testing.T) {
	block := utf16.Encode([]rune("=C:=C:\\x\x00PATH=C:\\bin\x00enowx_session=abc\x00\x00"))
	if got := envValue(block, "ENOWX_SESSION"); got != "abc" {
		t.Errorf("got %q", got)
	}
	if got := envValue(block, "MISSING"); got != "" {
		t.Errorf("got %q", got)
	}
	if got := envValue(nil, "PATH"); got != "" {
		t.Errorf("got %q", got)
	}
}

// A parent started after its child is another process with the PID its parent had.
func TestDropStaleParents(t *testing.T) {
	list := []Proc{{PID: 2, PPID: 1}, {PID: 3, PPID: 9}, {PID: 4, PPID: 5}, {PID: 1}, {PID: 5}}
	dropStaleParents(list, map[int]uint64{1: 10, 2: 20, 3: 20, 4: 20, 5: 30})
	if list[0].PPID != 1 {
		t.Error("a real parent was dropped")
	}
	if list[1].PPID != 0 {
		t.Error("a parent gone stayed")
	}
	if list[2].PPID != 0 {
		t.Error("a process born after its child was taken for its parent")
	}
}

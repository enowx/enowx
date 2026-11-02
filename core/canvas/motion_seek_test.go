package canvas

import (
	"strings"
	"testing"
)

// A moment moves a page already loaded as it would have a page loading stand there.
func TestSeekScript(t *testing.T) {
	for at, want := range map[string]string{"25": "m.state().d*25/100", "25%": "m.state().d*25/100", "1500ms": "m.seek(1500)"} {
		got, err := SeekScript(at)
		if err != nil || !strings.Contains(got, want) {
			t.Errorf("%s: %v %s", at, err, got)
		}
	}
	if _, err := SeekScript("200"); err == nil {
		t.Error("past its length")
	}
}

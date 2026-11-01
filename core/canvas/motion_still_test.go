package canvas

import (
	"strings"
	"testing"
)

func TestMotionAtMoments(t *testing.T) {
	for at, want := range map[string]string{"25": "{pct:25}", "12.5%": "{pct:12.5}", "1500ms": "{ms:1500}", " 0 ": "{pct:0}"} {
		doc, err := MotionAt("<head></head>", at)
		if err != nil || !strings.Contains(doc, "window.__enowxMotionAt="+want+"</script>") {
			t.Errorf("%q: %s %v", at, doc, err)
		}
	}
	for _, at := range []string{"", "soon", "101", "-1", "1s", "</script>"} {
		if _, err := MotionAt("<head></head>", at); err == nil {
			t.Errorf("%q was taken", at)
		}
	}
}

func TestParseMotion(t *testing.T) {
	if m := ParseMotion(`{"d":8000,"t":4000,"p":false,"n":2,"f":3}`); m == nil || m.Duration != 8000 || m.At != 4000 {
		t.Errorf("%+v", m)
	}
	for _, text := range []string{"", "nonsense", `{"d":0,"t":0,"n":0}`, `{"d":100,"t":0,"n":0}`} {
		if m := ParseMotion(text); m != nil {
			t.Errorf("%q: %+v", text, m)
		}
	}
}

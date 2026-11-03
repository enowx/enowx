package canvas

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/enowx/enowx/proto"
)

// A still of a moving design shows one chosen moment, not whatever the load had
// reached: its end unless asked for another (canvas frames).

// SettleScript has a rendering page show the moment a still should, and says
// what it found as JSON (a proto.CanvasMotion), "" for a page without motion.
const SettleScript = `(function(){var m=window.enowx&&window.enowx.motion;return m?JSON.stringify(m.settle()):""})()`

// ParseMotion reads what SettleScript answered; nil for no motion at all.
func ParseMotion(text string) *proto.CanvasMotion {
	var m struct {
		D, T float64
		N    int
	}
	if text == "" || json.Unmarshal([]byte(text), &m) != nil || m.N == 0 || m.D <= 0 {
		return nil
	}
	return &proto.CanvasMotion{Duration: m.D, At: m.T}
}

// parseMoment reads a moment: "25" or "25%" of a design's length, or "1500ms".
func parseMoment(at string) (unit string, v float64, err error) {
	at = strings.TrimSpace(at)
	unit = "pct"
	text := strings.TrimSuffix(at, "%")
	if strings.HasSuffix(at, "ms") {
		unit, text = "ms", strings.TrimSuffix(at, "ms")
	}
	v, err = strconv.ParseFloat(text, 64)
	if err != nil || v < 0 || (unit == "pct" && v > 100) {
		return "", 0, fmt.Errorf("canvas: a moment is 0 to 100 (percent of its length) or a time as 1500ms, not %q", at)
	}
	return unit, v, nil
}

// MotionAt has doc's motion stand at at for a still: "25" or "25%" of its
// length, or "1500ms".
func MotionAt(doc, at string) (string, error) {
	unit, v, err := parseMoment(at)
	if err != nil {
		return "", err
	}
	return WithHead(doc, fmt.Sprintf("<script>window.__enowxMotionAt={%s:%g}</script>", unit, v)), nil
}

// SeekScript moves a page already loaded to moment at, and answers as SettleScript does.
func SeekScript(at string) (string, error) {
	unit, v, err := parseMoment(at)
	if err != nil {
		return "", err
	}
	ms := fmt.Sprintf("%g", v)
	if unit == "pct" {
		ms = fmt.Sprintf("m.state().d*%g/100", v)
	}
	return `(function(){var m=window.enowx&&window.enowx.motion;return m?JSON.stringify(m.seek(` + ms + `)):""})()`, nil
}

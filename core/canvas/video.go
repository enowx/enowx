package canvas

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/enowx/enowx/proto"
)

// A moving design as a video: not a recording of its screen, but the page moved to each
// frame's time and captured, so that it comes out the same each time, however fast the
// machine is.

// MostVideoMs is the longest video rendered.
const MostVideoMs = 10 * 60 * 1000

// VideoFramesAt is each frame's time, in ms, of a design duration long: from to to at
// fps. A loop's last frame is its first again, so it stops short of it; what ends
// includes its end.
func VideoFramesAt(duration float64, loop bool, from, to float64, fps int) ([]float64, error) {
	if duration <= 0 {
		return nil, errors.New("canvas: this design has no motion to render")
	}
	if fps <= 0 {
		return nil, errors.New("canvas: a video needs frames per second")
	}
	if to <= 0 || to > duration {
		to = duration
	}
	if from < 0 || from >= to {
		return nil, fmt.Errorf("canvas: the range starts at %.0f ms and ends at %.0f ms", from, to)
	}
	if to-from > MostVideoMs {
		return nil, errors.New("canvas: a video is 10 minutes at most: choose a shorter range")
	}
	step := 1000 / float64(fps)
	n := int(math.Ceil((to - from) / step))
	out := make([]float64, 0, n+1)
	for i := range n {
		out = append(out, from+float64(i)*step)
	}
	if !(loop && to == duration) {
		out = append(out, to)
	}
	return out, nil
}

// VideoSize is a video's size for a frame w×h at width out (0 its own): both even, as
// H.264 in yuv420p needs.
func VideoSize(w, h, out int) (int, int) {
	if out <= 0 {
		out = w
	}
	height := int(math.Round(float64(h) * float64(out) / math.Max(float64(w), 1)))
	return max(out&^1, 2), max(height&^1, 2)
}

// VideoStartScript holds a loaded page's motion still, for the frames to move it, and
// says what there is: its length and whether it loops.
const VideoStartScript = `(function(){var m=window.enowx&&window.enowx.motion;if(!m)return "";m.hold(true);return JSON.stringify(m.state())})()`

// ParseVideoMotion reads what VideoStartScript answered.
func ParseVideoMotion(text string) (duration float64, loop, ok bool) {
	var m struct {
		D float64
		N int
		L bool
	}
	if text == "" || json.Unmarshal([]byte(text), &m) != nil || m.N == 0 || m.D <= 0 {
		return 0, false, false
	}
	return m.D, m.L, true
}

// VideoSeekScript moves a page held still to ms.
func VideoSeekScript(ms float64) string {
	return fmt.Sprintf(`window.enowx.motion.seek(%g);1`, ms)
}

// VideoReadyScript says fonts and images have come.
const VideoReadyScript = `(function(){var f=!document.fonts||document.fonts.status==="loaded";var i=Array.prototype.every.call(document.images,function(x){return x.complete});return f&&i})()`

// MotionOffClock says a design's own code draws by a clock the frames can't move, the
// reason a video of it may not match it exactly.
func MotionOffClock(html string) bool {
	return strings.Contains(html, "setAnimationLoop") || strings.Contains(html, "requestAnimationFrame(")
}

// VideoWarning is what to say of f, "" for nothing.
func VideoWarning(f proto.CanvasFrame) string {
	if MotionOffClock(f.HTML) {
		return "Part of this design draws by its own clock (requestAnimationFrame or setAnimationLoop), so that part won't follow the video's time exactly. Add it with enowx.motion.add to render it frame by frame."
	}
	return ""
}

package tokens

import (
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// Aider keeps no usage file, but writes what it shows into the project's
// .aider.chat.history.md (quoted with "> "): a "# aider chat started at
// 2006-01-02 15:04:05" line a run, "Model: <id> with … edit format" when
// it starts, and after each answer "Tokens: 4.4k sent, 1.2k cache write,
// 3.3k cache hit, 120 received. Cost: $0.01 message, $0.05 session."
// The counts are as rounded as Aider prints them.

func aiderFiles(env Env) []string {
	var out []string
	for _, d := range env.Dirs {
		out = append(out, filepath.Join(d, ".aider.chat.history.md"))
	}
	return out
}

var (
	aiderStarted = regexp.MustCompile(`^# aider chat started at (\d{4}-\d\d-\d\d \d\d:\d\d:\d\d)`)
	aiderModel   = regexp.MustCompile(`^>?\s*(?:Main model|Model):\s*(\S+)`)
	aiderCost    = regexp.MustCompile(`Cost:\s*\$([\d.]+)\s*message`)
)

func readAider(path string, st *FileState) ([]Call, error) {
	var out []Call
	at := time.Unix(st.Mark, 0)
	err := readLines(path, st, func(line []byte) {
		text := string(line)
		if m := aiderStarted.FindStringSubmatch(text); m != nil {
			if t, err := time.ParseInLocation(time.DateTime, m[1], time.Local); err == nil {
				at, st.Mark = t, t.Unix()
			}
			return
		}
		if m := aiderModel.FindStringSubmatch(text); m != nil {
			st.Model = m[1]
			return
		}
		m := screenLine.FindStringSubmatch(text)
		if m == nil {
			return
		}
		c := Call{ID: "aider:" + path + ":" + strconv.FormatInt(st.Offset, 10), Time: at, Model: st.Model, Dir: filepath.Dir(path),
			Input: rounded(m[1]), CacheWrite: rounded(m[2]), CacheRead: rounded(m[3]), Output: rounded(m[4])}
		if cm := aiderCost.FindStringSubmatch(text); cm != nil {
			c.Cost, _ = strconv.ParseFloat(cm[1], 64)
		}
		out = append(out, c)
	})
	return out, err
}

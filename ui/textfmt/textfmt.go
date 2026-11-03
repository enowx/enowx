// Package textfmt writes values the way the interface shows them: sizes,
// percentages, plurals, first lines and paths of any host.
package textfmt

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// Plural picks one or many for n.
func Plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Bytes writes a size in B, KB, MB or GB.
func Bytes(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n < 1<<10:
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%d KB", n>>10)
}

// CPU writes a CPU percentage.
func CPU(pct float64) string { return fmt.Sprintf("%.0f%%", pct) }

// Age writes how long ago t was, in its largest whole unit: 40s, 14m,
// 2h, 3d.
func Age(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", max(int(d.Seconds()), 0))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// Ago writes how long since t in words, for a line to read: just now,
// 2 minutes ago, 3 hours ago, Yesterday, 4 days ago.
func Ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return Plural(int(d.Minutes()), "1 minute ago", fmt.Sprintf("%d minutes ago", int(d.Minutes())))
	case d < 24*time.Hour:
		return Plural(int(d.Hours()), "1 hour ago", fmt.Sprintf("%d hours ago", int(d.Hours())))
	case d < 48*time.Hour:
		return "Yesterday"
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}

// Until writes how long until t, to the minute under a day: 18m, 2h 10m,
// 5d. Past times read "now".
func Until(t, now time.Time) string {
	d := t.Sub(now)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		if m := int(d.Minutes()) % 60; m > 0 {
			return fmt.Sprintf("%dh %dm", int(d.Hours()), m)
		}
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// FirstLine is s up to its first line break, cut to n runes with an
// ellipsis.
func FirstLine(s string, n int) string {
	count, cut := 0, 0
	for i, r := range s {
		if r == '\r' || r == '\n' {
			return s[:i]
		}
		if count == n-1 {
			cut = i
		}
		if count == n {
			return s[:cut] + "…"
		}
		count++
	}
	return s
}

// JoinPath joins a host's folder and a name with that host's separator.
func JoinPath(dir, name string) string {
	sep := "/"
	if strings.Contains(dir, `\`) && !strings.Contains(dir, "/") {
		sep = `\`
	}
	return strings.TrimSuffix(dir, sep) + sep + name
}

// LastPathPart is the last element of a path of any host.
func LastPathPart(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 && i < len(p)-1 {
		return p[i+1:]
	}
	if p == "" {
		return "/"
	}
	return p
}

// TildePath writes p under home with ~, as people type it.
func TildePath(p, home string) string {
	switch {
	case home == "":
		return p
	case p == home:
		return "~"
	case strings.HasPrefix(p, home+"/") || strings.HasPrefix(p, home+`\`):
		return "~" + p[len(home):]
	}
	return p
}

// ShortHome writes paths under this device's home folder with ~.
func ShortHome(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}
	return path
}

var userHome = regexp.MustCompile(`^(/Users/[^/]+|/home/[^/]+|/root)(/|$)`)

// ShortPath is p to read in a narrow place: its home as ~ on any host, then
// cut in the middle to about n characters, its first folder and its end kept.
func ShortPath(p string, n int) string {
	if m := userHome.FindStringSubmatch(p); m != nil {
		p = "~" + p[len(m[1]):]
	}
	r := []rune(p)
	if len(r) <= n || n < 8 {
		return p
	}
	tail := n * 2 / 3
	return string(r[:n-tail-1]) + "…" + string(r[len(r)-tail:])
}

// ShortModel is a model's name to read in a narrow place: its maker left out.
func ShortModel(model string) string {
	if i := strings.LastIndexByte(model, '/'); i >= 0 {
		model = model[i+1:]
	}
	return strings.TrimPrefix(model, "claude-")
}

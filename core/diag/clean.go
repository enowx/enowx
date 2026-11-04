package diag

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Limits of what stays. See docs/diagnostics.md.
const (
	// MaxRotatedAge and MaxRotatedTotal bound the rotated logs of both programs;
	// NAME.1 of each always stays.
	MaxRotatedAge   = 30 * 24 * time.Hour
	MaxRotatedTotal = 24 << 20
	// MaxReportAge and MaxReportTotal bound the watchdog's freeze reports; the newest stays.
	MaxReportAge   = 30 * 24 * time.Hour
	MaxReportTotal = 64 << 20
	// MaxTrimAge and MaxTrimTotal bound the outputs `decide trim` saved. An agent reads
	// one just after it is written, so those younger than MinTrimAge, and the newest, stay.
	MaxTrimAge   = 7 * 24 * time.Hour
	MaxTrimTotal = 100 << 20
	MinTrimAge   = time.Hour
	// holdFor is how long a hold counts, should what took it have died.
	holdFor  = 10 * time.Minute
	holdFile = "diagnostics.hold"
)

var errHeld = errors.New("diag: a bundle is reading the logs")

// Only these names are ever removed: exact names for rotated logs, and the one shape
// each writer gives its files, in the one folder each writes.
var (
	reportName = regexp.MustCompile(`^(app|daemon)-(lag|stall)-[0-9]{8}-[0-9]{6}\.txt$`)
	trimName   = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9]+\.log$`)
)

// TrimDir is the folder `decide trim` saves whole outputs in.
func TrimDir() string { return filepath.Join(os.TempDir(), "enowx-trim") }

// Hold keeps the logs of data folder dir from rotating or being removed until the
// function it returns is called: a bundle reading them, in any process.
func Hold(dir string) func() {
	path := filepath.Join(dir, holdFile)
	_ = os.WriteFile(path, nil, 0o600)
	return func() { _ = os.Remove(path) }
}

// Held says a bundle is reading the logs now.
func Held(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, holdFile))
	return err == nil && time.Since(info.ModTime()) < holdFor
}

// Item is a log file that may go.
type Item struct {
	// Rel names it: under the data folder, or under trimRoot for an output.
	Rel  string
	Size int64
	When time.Time
	path string
}

const trimRoot = "enowx-trim/"

// statRegular is a file at path that is plainly a file, not a link to one.
func statRegular(path string) (os.FileInfo, bool) {
	info, err := os.Lstat(path)
	return info, err == nil && info.Mode().IsRegular()
}

// rotated are the rotated logs, each program's oldest first with its most recent left out.
func rotated(dir string) []Item {
	var out []Item
	for _, name := range []string{AppLog, DaemonLog} {
		for i := KeepRotated; i >= 2; i-- {
			rel := fmt.Sprintf("%s.%d", name, i)
			path := filepath.Join(dir, rel)
			if info, ok := statRegular(path); ok {
				out = append(out, Item{Rel: rel, Size: info.Size(), When: info.ModTime(), path: path})
			}
		}
	}
	return oldestFirst(out)
}

// reports are the freeze reports, oldest first, the newest left out.
func reports(dir string) []Item {
	return named(filepath.Join(dir, "watchdog"), "watchdog/", reportName)
}

// outputs are the outputs saved, oldest first, the newest left out.
func outputs() []Item { return named(TrimDir(), trimRoot, trimName) }

func named(folder, prefix string, shape *regexp.Regexp) []Item {
	if info, err := os.Lstat(folder); err != nil || !info.IsDir() {
		return nil // nor a link put in its place
	}
	list, err := os.ReadDir(folder)
	if err != nil {
		return nil
	}
	var out []Item
	for _, d := range list {
		if !shape.MatchString(d.Name()) {
			continue
		}
		path := filepath.Join(folder, d.Name())
		if info, ok := statRegular(path); ok {
			out = append(out, Item{Rel: prefix + d.Name(), Size: info.Size(), When: info.ModTime(), path: path})
		}
	}
	out = oldestFirst(out)
	if len(out) > 0 {
		out = out[:len(out)-1]
	}
	return out
}

func oldestFirst(items []Item) []Item {
	slices.SortStableFunc(items, func(a, b Item) int { return a.When.Compare(b.When) })
	return items
}

// expired are those of items older than age or, from the oldest, over total with what
// else stays (whole bytes).
func expired(items []Item, now time.Time, age time.Duration, total, kept int64) []Item {
	sum := kept
	for _, it := range items {
		sum += it.Size
	}
	var out []Item
	for _, it := range items {
		if now.Sub(it.When) > age || sum > total {
			out = append(out, it)
			sum -= it.Size
		}
	}
	return out
}

// Swept says what Sweep did.
type Swept struct {
	Files int
	Bytes int64
}

// Sweep removes what is past its limits, as a program does when it starts and every
// few hours. The current logs and each one's most recent rotated copy always stay, and
// nothing goes while a bundle reads them.
func Sweep(dir string, now time.Time) Swept {
	var s Swept
	if Held(dir) {
		return s
	}
	var kept int64
	for _, name := range []string{AppLog, DaemonLog} {
		if info, ok := statRegular(filepath.Join(dir, name+".1")); ok {
			kept += info.Size()
		}
	}
	gone := expired(rotated(dir), now, MaxRotatedAge, MaxRotatedTotal, kept)
	gone = append(gone, expired(reports(dir), now, MaxReportAge, MaxReportTotal, 0)...)
	for _, it := range expired(outputs(), now, MaxTrimAge, MaxTrimTotal, 0) {
		if now.Sub(it.When) >= MinTrimAge {
			gone = append(gone, it)
		}
	}
	for _, it := range gone {
		if os.Remove(it.path) == nil {
			s.Files++
			s.Bytes += it.Size
		}
	}
	return s
}

// Removable are the logs Clean cache offers: rotated copies past the most recent, and
// outputs saved a while ago. Freeze reports go there by their age, which it chooses.
func Removable(dir string, now time.Time) []Item {
	out := rotated(dir)
	for _, it := range outputs() {
		if now.Sub(it.When) >= MinTrimAge {
			out = append(out, it)
		}
	}
	return out
}

// Remove removes the file rel of those Removable lists now, and nothing else.
func Remove(dir string, now time.Time, rel string) (int64, error) {
	if Held(dir) {
		return 0, errHeld
	}
	for _, it := range Removable(dir, now) {
		if it.Rel == rel && !strings.Contains(rel, "..") {
			if err := os.Remove(it.path); err != nil {
				return 0, err
			}
			return it.Size, nil
		}
	}
	return 0, fmt.Errorf("diag: %s isn't a log that may go", rel)
}

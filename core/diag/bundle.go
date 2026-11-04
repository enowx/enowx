package diag

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/proto"
)

const (
	// bundleEach is the most of one file a bundle holds, its end.
	bundleEach = 6 << 20
	// bundleReports is how many freeze reports it holds, the newest.
	bundleReports = 5
	// ProfileFile is where a bundle holds what the programs measured when it was made
	// (`enowx-app profile`).
	ProfileFile = "profile.txt"
)

// logNames are the logs of data folder dir a bundle holds, in order. What `decide
// trim` saved isn't among them: that is a command's output.
func logNames() []string {
	var out []string
	for _, name := range []string{AppLog, DaemonLog} {
		out = append(out, name)
		for i := 1; i <= KeepRotated; i++ {
			out = append(out, fmt.Sprintf("%s.%d", name, i))
		}
	}
	return append(out, "watchdog/app.log", "watchdog/daemon.log")
}

// Collect reads the logs of data folder dir, masked once more (a log an older version
// wrote wasn't), holding them against rotating and removal meanwhile.
func Collect(dir string) []proto.DiagFile {
	release := Hold(dir)
	defer release()
	var out []proto.DiagFile
	add := func(rel string) {
		data, size, err := readTail(filepath.Join(dir, filepath.FromSlash(rel)), bundleEach)
		if err == nil {
			out = append(out, proto.DiagFile{Name: rel, Size: size, Text: Scrub(string(data))})
		}
	}
	for _, rel := range logNames() {
		add(rel)
	}
	list, _ := os.ReadDir(filepath.Join(dir, "watchdog"))
	var found []string
	for _, d := range list {
		if reportName.MatchString(d.Name()) {
			found = append(found, d.Name())
		}
	}
	slices.SortFunc(found, func(a, b string) int { return strings.Compare(reportStamp(b), reportStamp(a)) })
	for _, name := range found[:min(len(found), bundleReports)] {
		add("watchdog/" + name)
	}
	return out
}

func reportStamp(name string) string {
	return strings.TrimSuffix(name, ".txt")[max(len(name)-len(".txt")-len("20060102-150405"), 0):]
}

func readTail(path string, most int64) ([]byte, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	start := max(info.Size()-most, 0)
	data := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(data, start); err != nil && err != io.EOF {
		return nil, 0, err
	}
	return data, info.Size(), nil
}

// connectionLine is a line about a connection, or a failure of the protocol.
var connectionLine = regexp.MustCompile(`^\S+ (conn|host|proto)\.|frame too large|reply too large`)

// Recent are the last lines about connections across files, the latest last.
func Recent(files []proto.DiagFile, lines int) []string {
	var out []string
	for _, f := range files {
		if f.Name != AppLog && f.Name != DaemonLog && !strings.HasPrefix(f.Name, AppLog+".") && !strings.HasPrefix(f.Name, DaemonLog+".") {
			continue
		}
		tag := "app"
		if strings.HasPrefix(f.Name, DaemonLog) {
			tag = "daemon"
		}
		for _, line := range strings.Split(f.Text, "\n") {
			if connectionLine.MatchString(line) {
				out = append(out, stamped(line, tag))
			}
		}
	}
	slices.Sort(out) // they start with their times
	return out[max(len(out)-lines, 0):]
}

func stamped(line, tag string) string {
	stamp, rest, _ := strings.Cut(line, " ")
	return stamp + " " + tag + " " + rest
}

// WriteZip writes sets (a label, such as a host's name, to its files) as a zip.
func WriteZip(w io.Writer, about string, sets map[string][]proto.DiagFile) error {
	z := zip.NewWriter(w)
	put := func(name, text string) error {
		f, err := z.Create(name)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, bytes.NewReader([]byte(text)))
		return err
	}
	if err := put("about.txt", about+"\nMade "+time.Now().Format(time.RFC3339)+"\n"); err != nil {
		return err
	}
	labels := make([]string, 0, len(sets))
	for label := range sets {
		labels = append(labels, label)
	}
	slices.Sort(labels)
	for _, label := range labels {
		for _, f := range sets[label] {
			if err := put(zipPart(label)+"/"+f.Name, f.Text); err != nil {
				return err
			}
		}
	}
	return z.Close()
}

// zipPart is label as a folder's name.
func zipPart(label string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|.`, r) {
			return '-'
		}
		return r
	}, label)
}

package usersync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

const auditPath = "vault/audit.jsonl"

func auditLine(at time.Time, host string, n int) string {
	return fmt.Sprintf(`{"time":%q,"action":"use","credential":"c","session":"%s-%d","allowed":true}`, at.UTC().Format(time.RFC3339Nano), host, n)
}

// inOrder says want's lines all come in got, in want's order.
func inOrder(got, want []string) bool {
	i := 0
	for _, l := range got {
		if i < len(want) && l == want[i] {
			i++
		}
	}
	return i == len(want)
}

func TestMergeLogKeepsEveryLineInOrder(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for round := range 200 {
		var shared, onlyA, onlyB []string
		at := start
		for n := range r.Intn(20) {
			at = at.Add(time.Duration(r.Intn(3)) * time.Second)
			shared = append(shared, auditLine(at, "shared", n))
		}
		// Each host's own clock: b's may be well behind a's.
		atA, atB := at, at.Add(-time.Duration(r.Intn(600))*time.Second)
		for n := range r.Intn(20) {
			atA = atA.Add(time.Duration(r.Intn(3)) * time.Second)
			onlyA = append(onlyA, auditLine(atA, "a", n))
		}
		for n := range r.Intn(20) {
			atB = atB.Add(time.Duration(r.Intn(3)) * time.Second)
			onlyB = append(onlyB, auditLine(atB, "b", n))
		}
		a := slices.Concat(shared, onlyA)
		b := slices.Concat(shared, onlyB)
		merged := logLines(mergeLog(joinLines(a), joinLines(b)))
		if want := len(shared) + len(onlyA) + len(onlyB); len(merged) != want {
			t.Fatalf("round %d: %d lines, want %d", round, len(merged), want)
		}
		if !inOrder(merged, onlyA) || !inOrder(merged, onlyB) {
			t.Fatalf("round %d: a host's lines reordered:\n%s", round, strings.Join(merged, "\n"))
		}
		// Merging what's in already changes nothing, and it doesn't matter which is which.
		again := mergeLog(joinLines(merged), joinLines(b))
		if !bytes.Equal(again, joinLines(merged)) {
			t.Fatalf("round %d: merging again changed it", round)
		}
		if !bytes.Equal(mergeLog(joinLines(b), joinLines(a)), joinLines(merged)) {
			t.Fatalf("round %d: depends on the side", round)
		}
	}
}

// The same line twice on a host is two uses: they stay two, not three or one.
func TestMergeLogKeepsRepeatedLines(t *testing.T) {
	x, y := `{"time":"2026-11-01T00:00:00Z","action":"x"}`, `{"time":"2026-11-01T00:00:01Z","action":"y"}`
	got := logLines(mergeLog(joinLines([]string{x, x, y}), joinLines([]string{x, y, y})))
	if want := []string{x, x, y, y}; !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
	got = logLines(mergeLog([]byte("not json\n"), []byte("not json\nnor this")))
	if want := []string{"not json", "nor this"}; !slices.Equal(got, want) {
		t.Fatalf("lines saying no time: got %q", got)
	}
}

func joinLines(lines []string) []byte {
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// appendAudit adds a use on a host, as the vault does.
func appendAudit(t *testing.T, dir string, line string) {
	t.Helper()
	file := filepath.Join(dir, filepath.FromSlash(auditPath))
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	h, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if _, err := h.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

func readAudit(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(auditPath)))
	if err != nil {
		t.Fatal(err)
	}
	return logLines(data)
}

// Three hosts adding uses all the time, a hub syncing with each in turn, as the app does:
// no conflict, no line lost, each host's lines in their order, and the same file on all.
func TestAuditLogAcrossThreeHostsLosesNothing(t *testing.T) {
	dirs := map[string]string{"hub": t.TempDir(), "b": t.TempDir(), "c": t.TempDir()}
	hub, b, c := OpenFolder(dirs["hub"]), OpenFolder(dirs["b"]), OpenFolder(dirs["c"])
	baseB, baseC := &Base{Files: map[string]BaseFile{}}, &Base{Files: map[string]BaseFile{}}
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	wrote := map[string][]string{}
	add := func(host string, skew time.Duration) {
		at = at.Add(time.Second)
		line := auditLine(at.Add(skew), host, len(wrote[host]))
		wrote[host] = append(wrote[host], line)
		appendAudit(t, dirs[host], line)
	}
	sync := func(peer *Folder, base *Base) {
		t.Helper()
		rep, err := Sync(hub, peer, base)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Conflicts != 0 {
			t.Fatalf("%d conflicts", rep.Conflicts)
		}
	}
	for round := range 50 {
		for range 1 + round%3 {
			add("hub", 0)
		}
		for range 1 + round%2 {
			add("b", -time.Hour) // a clock an hour behind
		}
		add("c", 5*time.Minute)
		sync(b, baseB)
		sync(c, baseC)
	}
	sync(b, baseB)
	sync(c, baseC)
	sync(b, baseB)
	got := readAudit(t, dirs["hub"])
	for name, dir := range dirs {
		if !slices.Equal(readAudit(t, dir), got) {
			t.Fatalf("%s differs", name)
		}
	}
	total := 0
	for host, lines := range wrote {
		total += len(lines)
		if !inOrder(got, lines) {
			t.Fatalf("%s's lines lost or reordered", host)
		}
	}
	if len(got) != total {
		t.Fatalf("%d lines, wrote %d", len(got), total)
	}
	if list, _ := hub.Conflicts(); len(list) != 0 {
		t.Fatalf("conflicts: %+v", list)
	}
}

// Conflicts made before logs merged go by themselves, on every host, with every line of
// every version, the only copy of some of them, as a conflict's second version was.
func TestExistingLogConflictsSettleWithNoLineLost(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	a, b := OpenFolder(dirA), OpenFolder(dirB)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var all []string
	line := func(host string) string {
		at = at.Add(time.Second)
		l := auditLine(at, host, len(all))
		all = append(all, l)
		return l
	}
	// The file in place holds the hub's lines alone, as each conflict left it.
	inPlace := []string{line("a"), line("a")}
	appendAudit(t, dirA, inPlace[0])
	appendAudit(t, dirA, inPlace[1])
	appendAudit(t, dirB, inPlace[0])
	appendAudit(t, dirB, inPlace[1])
	writeConflict := func(id string, versions ...[]string) {
		c := proto.SyncConflict{ID: id, Path: auditPath, Created: at}
		for i, v := range versions {
			c.Versions = append(c.Versions, proto.SyncVersion{Host: fmt.Sprint("host", i), Data: joinLines(v)})
		}
		data, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		for _, dir := range []string{dirA, dirB} {
			file := filepath.Join(dir, filepath.FromSlash(ConflictsDir), id+".json")
			if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	first := []string{line("b"), line("b")}
	writeConflict("aaa", inPlace, first)
	writeConflict("bbb", inPlace, slices.Concat(first, []string{line("b")}))
	writeConflict("ccc", slices.Concat(inPlace, []string{line("a")}), []string{line("c")})
	// A conflict on another file stays the user's.
	other := proto.SyncConflict{ID: "ddd", Path: "settings/x.txt", Created: at, Versions: []proto.SyncVersion{{Host: "h", Data: []byte("one")}}}
	data, _ := json.Marshal(other)
	if err := os.WriteFile(filepath.Join(dirA, filepath.FromSlash(ConflictsDir), "ddd.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	base := &Base{Files: map[string]BaseFile{}}
	if _, err := Sync(a, b, base); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(a, b, base); err != nil {
		t.Fatal(err)
	}
	for _, f := range []*Folder{a, b} {
		list, err := f.Conflicts()
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].ID != "ddd" {
			t.Fatalf("conflicts left: %+v", list)
		}
	}
	for _, dir := range []string{dirA, dirB} {
		got := readAudit(t, dir)
		for _, l := range all {
			if !slices.Contains(got, l) {
				t.Fatalf("lost %s", l)
			}
		}
		if len(got) != len(all) {
			t.Fatalf("%d lines, want %d: a line doubled", len(got), len(all))
		}
	}
	// Deleted conflicts wait in the trash, not listed: nothing for the user to do there.
	trash, _ := a.Trash()
	for _, it := range trash {
		if strings.HasPrefix(it.Path, ConflictsDir) {
			t.Fatalf("listed in the trash: %s", it.Path)
		}
	}
}

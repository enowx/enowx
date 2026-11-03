// Package cacheclean says what a host's enowX folder keeps that can go, and takes it
// away when asked. Removing is real, so only what a look just now listed goes, and only
// under the few folders named here: everything else in the folder (the vault, accounts,
// notes, canvases, sessions, token history) can't be reached from here at all.
package cacheclean

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/proto"
)

// DefaultDays is how old things must be to count, unless told otherwise.
const DefaultDays = 30

// keepVersions is how many of a canvas's latest versions thinning leaves.
const keepVersions = 50

// removable are the only places under the folder anything is removed from.
var removable = []string{"screenshots/", "reviews/", "unslop/", "trash/", "intercept/profiles/", "canvas-chat/", "notes-chat/", "app-backup/", "worktrees/", "watchdog/"}

// Allowed says path p, under the folder, may be removed at all.
func Allowed(p string) bool {
	if p == "" || path.IsAbs(p) || path.Clean(p) != p || strings.HasPrefix(p, "../") || p == ".." {
		return false
	}
	return slices.ContainsFunc(removable, func(root string) bool { return strings.HasPrefix(p, root) && len(p) > len(root) })
}

// Env is what a look needs of the daemon.
type Env struct {
	Dir string
	Now time.Time
	// OlderDays: what's newer stays. 0 is DefaultDays.
	OlderDays int
	// InUse names the running session whose folder holds path, "" for none.
	InUse func(path string) string
	// Running says a process runs from path, or with it among its arguments.
	Running func(path string) bool
	// Worktree says what state a worktree is in, for the user to weigh.
	Worktree func(path string) string
	// RemoveWorktree removes a worktree, as git keeps track of them.
	RemoveWorktree func(path string) error
	// Canvases lists canvases with versions: each one's versions' folder, under Dir.
	Canvases func() []string
	// ThinCanvas thins the versions of the canvas in that folder to its latest keep,
	// and says what that freed.
	ThinCanvas func(folder string, keep int) (int64, error)
}

func (e Env) older() time.Time {
	days := e.OlderDays
	if days <= 0 {
		days = DefaultDays
	}
	return e.Now.AddDate(0, 0, -days)
}

// Size is what p takes on disk, what links point at not counted.
func Size(p string) int64 {
	var total int64
	_ = filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// List says what the folder takes, and what can go of it.
func List(e Env) proto.CacheReport {
	r := proto.CacheReport{Total: Size(e.Dir)}
	for _, k := range []func(Env) proto.CacheKind{screenshots, reviews, unslopRuns, traffic, helperFolders, logs, trash, canvasVersions, backups, worktrees, syncCopies} {
		kind := k(e)
		for _, it := range kind.Items {
			if it.Blocked == "" && !kind.Info {
				kind.Bytes += it.Bytes
			}
		}
		r.Kinds = append(r.Kinds, kind)
	}
	return r
}

// entries are the things directly in folder rel of the folder, as items.
func entries(e Env, rel string) []proto.CacheItem {
	list, err := os.ReadDir(filepath.Join(e.Dir, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	var out []proto.CacheItem
	for _, d := range list {
		if strings.HasPrefix(d.Name(), ".") {
			continue
		}
		info, err := d.Info()
		if err != nil {
			continue
		}
		p := rel + "/" + d.Name()
		out = append(out, proto.CacheItem{Path: p, Name: d.Name(), Bytes: Size(filepath.Join(e.Dir, filepath.FromSlash(p))), When: info.ModTime()})
	}
	return out
}

// olderOnes keeps items older than the age asked for.
func olderOnes(e Env, items []proto.CacheItem) []proto.CacheItem {
	limit := e.older()
	return slices.DeleteFunc(items, func(it proto.CacheItem) bool { return !it.When.Before(limit) })
}

func screenshots(e Env) proto.CacheKind {
	named := noteTexts(e)
	items := slices.DeleteFunc(olderOnes(e, entries(e, "screenshots")), func(it proto.CacheItem) bool {
		return slices.ContainsFunc(named, func(text string) bool { return strings.Contains(text, it.Name) })
	})
	return proto.CacheKind{ID: "screenshots", Name: "Pasted screenshots", Default: true, Items: items,
		About: "Pictures pasted into chats, kept so agents could open them. Those older than the age chosen go, except any a note names. An old chat shows a missing picture in their place."}
}

// noteTexts are the notes of the user's and teams' spaces, to look for names in.
func noteTexts(e Env) []string {
	dirs := []string{filepath.Join(e.Dir, "notes")}
	if teams, err := filepath.Glob(filepath.Join(e.Dir, "teams", "*", "notes")); err == nil {
		dirs = append(dirs, teams...)
	}
	var out []string
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
				return nil
			}
			if data, err := os.ReadFile(p); err == nil {
				out = append(out, string(data))
			}
			return nil
		})
	}
	return out
}

func reviews(e Env) proto.CacheKind {
	items := olderOnes(e, entries(e, "reviews"))
	blockInUse(e, items)
	return proto.CacheKind{ID: "reviews", Name: "Finished design reviews", Default: true, Items: items,
		About: "The screenshots and scores of independent reviews older than the age chosen. A canvas keeps the design itself."}
}

func unslopRuns(e Env) proto.CacheKind {
	return proto.CacheKind{ID: "unslop", Name: "Old unslop runs", Default: true, Items: olderOnes(e, entries(e, "unslop")),
		About: "What an unslop run noted of its steps, older than the age chosen. The design stays."}
}

func traffic(e Env) proto.CacheKind {
	items := entries(e, "intercept/profiles")
	for i := range items {
		if e.Running != nil && e.Running(filepath.Join(e.Dir, filepath.FromSlash(items[i].Path))) {
			items[i].Blocked = "A captured browser has it open"
		}
	}
	return proto.CacheKind{ID: "traffic", Name: "Traffic's browser profiles", Default: true, Items: items,
		About: "The browser profiles the Traffic tool opens to capture a site. Sign-ins and history in those captured browsers go; your own browser isn't touched. The tool's certificate stays."}
}

func helperFolders(e Env) proto.CacheKind {
	items := append(entries(e, "canvas-chat"), entries(e, "notes-chat")...)
	blockInUse(e, items)
	return proto.CacheKind{ID: "helpers", Name: "Canvas and Notes chats' folders", Default: true, Items: items,
		About: "The working folders of Canvas and Notes pages' agents with no session now. A chat started again makes its folder anew, and its conversation stays with the agent."}
}

func blockInUse(e Env, items []proto.CacheItem) {
	for i := range items {
		if e.InUse == nil {
			return
		}
		if id := e.InUse(filepath.Join(e.Dir, filepath.FromSlash(items[i].Path))); id != "" {
			items[i].Blocked = "Session " + id + " uses it"
		}
	}
}

func logs(e Env) proto.CacheKind {
	items := olderOnes(e, entries(e, "watchdog"))
	items = slices.DeleteFunc(items, func(it proto.CacheItem) bool { return !strings.HasSuffix(it.Name, ".txt") })
	for _, it := range diag.Removable(e.Dir, e.Now) {
		items = append(items, proto.CacheItem{Path: it.Rel, Name: path.Base(it.Rel), Bytes: it.Size, When: it.When})
	}
	if diag.Held(e.Dir) {
		for i := range items {
			items[i].Blocked = "A diagnostics bundle is reading it"
		}
	}
	return proto.CacheKind{ID: "logs", Name: "Logs", Default: true, Items: items,
		About: "Older rotated logs of the app and the daemon, whole outputs agents' commands left in the temporary folder, and freeze reports older than the age chosen. The current logs and each one's most recent rotated copy stay. The decisions' log keeps itself short."}
}

func trash(e Env) proto.CacheKind {
	return proto.CacheKind{ID: "trash", Name: "Deleted items", Items: entries(e, "trash"),
		About:   "Notes, canvases and settings deleted lately, kept 30 days to restore, then gone by themselves.",
		Warning: "Emptying it now means they can't be restored."}
}

func canvasVersions(e Env) proto.CacheKind {
	var items []proto.CacheItem
	if e.Canvases != nil {
		for _, rel := range e.Canvases() {
			bytes := Size(filepath.Join(e.Dir, filepath.FromSlash(rel)))
			items = append(items, proto.CacheItem{Path: rel, Name: path.Base(path.Dir(rel)), Bytes: bytes})
		}
	}
	return proto.CacheKind{ID: "canvas-versions", Name: "Canvas versions", Items: items,
		About:   fmt.Sprintf("Thins each canvas to its latest %d versions and those a redesign named. Designs stay as they are. What it frees is at most the size shown.", keepVersions),
		Warning: "Undo and Restore can't reach the versions removed."}
}

func backups(e Env) proto.CacheKind {
	items := entries(e, "app-backup")
	for i := range items {
		full := filepath.Join(e.Dir, filepath.FromSlash(items[i].Path))
		info, err := os.Lstat(full)
		switch {
		case err != nil:
			items[i].Blocked = "Can't be read"
		case info.Mode()&fs.ModeSymlink != 0:
			items[i].Blocked = "A link sessions may run through"
		case e.Running != nil && e.Running(full):
			items[i].Blocked = "enowX is running from it"
		}
	}
	return proto.CacheKind{ID: "backups", Name: "Old app backups", PerItem: true, Items: items,
		About: "Copies of the app kept from before an update. Pick each one, by its date."}
}

func worktrees(e Env) proto.CacheKind {
	var items []proto.CacheItem
	for _, repo := range entries(e, "worktrees") {
		for _, it := range entries(e, repo.Path) {
			it.Name = repo.Name + " / " + it.Name
			full := filepath.Join(e.Dir, filepath.FromSlash(it.Path))
			if e.Worktree != nil {
				it.Note = e.Worktree(full)
			}
			items = append(items, it)
		}
	}
	blockInUse(e, items)
	return proto.CacheKind{ID: "worktrees", Name: "Worktrees with no session", PerItem: true, Items: items,
		About:   "Branches' checked-out folders. Never picked for you: pick each one, weighing its state. Their branches stay in the repository.",
		Warning: "Uncommitted changes in a worktree removed are lost."}
}

func syncCopies(e Env) proto.CacheKind {
	var items []proto.CacheItem
	if bases, err := filepath.Glob(filepath.Join(e.Dir, "teams", "*", "sync", "base")); err == nil {
		for _, dir := range bases {
			rel, _ := filepath.Rel(e.Dir, dir)
			team := filepath.Base(filepath.Dir(filepath.Dir(dir)))
			items = append(items, proto.CacheItem{Path: filepath.ToSlash(rel), Name: team, Bytes: Size(dir)})
		}
	}
	return proto.CacheKind{ID: "sync-copies", Name: "Team sync's copies", Info: true, Items: items,
		About: "What team spaces last agreed on, which sync merges both sides' changes against. Without it a change made on two devices would be a conflict rather than a merge, so it isn't offered. Sync drops what it no longer shares by itself."}
}

// Clean takes away what p asks, as a look just now lists it, and says what happened,
// item by item. Nothing else can go.
func Clean(e Env, p proto.CacheCleanParams) proto.CacheCleanResult {
	var res proto.CacheCleanResult
	e.OlderDays = p.OlderDays
	for _, kind := range List(e).Kinds {
		whole := slices.Contains(p.Kinds, kind.ID) && !kind.PerItem
		for _, it := range kind.Items {
			if kind.Info || !whole && !slices.Contains(p.Items, it.Path) {
				continue
			}
			if it.Blocked != "" {
				res.Failed = append(res.Failed, proto.CacheFailed{Path: it.Path, Why: it.Blocked})
				continue
			}
			freed, err := cleanItem(e, kind.ID, it)
			if err != nil {
				res.Failed = append(res.Failed, proto.CacheFailed{Path: it.Path, Why: err.Error()})
				continue
			}
			res.Freed += freed
			res.Done++
		}
	}
	return res
}

func cleanItem(e Env, kind string, it proto.CacheItem) (int64, error) {
	full := filepath.Join(e.Dir, filepath.FromSlash(it.Path))
	switch {
	case kind == "canvas-versions":
		if e.ThinCanvas == nil {
			return 0, errors.New("not on this host")
		}
		return e.ThinCanvas(it.Path, keepVersions)
	case kind == "logs" && !strings.HasPrefix(it.Path, "watchdog/"):
		return diag.Remove(e.Dir, e.Now, it.Path)
	case !Allowed(it.Path):
		return 0, errors.New("not something this removes")
	case kind == "worktrees":
		if e.RemoveWorktree == nil {
			return 0, errors.New("not on this host")
		}
		if err := e.RemoveWorktree(full); err != nil {
			return 0, err
		}
		return it.Bytes, nil
	}
	if err := os.RemoveAll(full); err != nil {
		return 0, err
	}
	return it.Bytes, nil
}

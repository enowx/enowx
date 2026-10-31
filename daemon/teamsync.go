package daemon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/cloud/api"
	cloudclient "github.com/enowx/enowx/cloud/client"
	"github.com/enowx/enowx/cloud/seal"
	"github.com/enowx/enowx/core/autopush"
	"github.com/enowx/enowx/core/notes"
	"github.com/enowx/enowx/core/usersync"
	"github.com/enowx/enowx/proto"
)

// A team's space syncs with the cloud (docs/cloud.md): each file of its
// notes and canvases is an item, sealed here with the team's key, the
// cloud seeing its kind, size and version only. A sync takes what
// changed there since the last (merging with what changed here, as the
// user's own sync does), then sends what changed here, a file holding
// what looks like a secret left out. Agents' conversations and proposals
// stay on the host.

// teamSyncer runs the spaces' syncs, one at a time (run), on
// teamSyncLoop's goroutine or a sync asked for; mu guards last.
type teamSyncer struct {
	once  sync.Once
	pokes chan struct{}
	run   sync.Mutex
	mu    sync.Mutex
	last  map[string]syncStatus
}

type syncStatus struct {
	at  time.Time // when it last ended well
	err string
}

const (
	teamSyncEvery  = 30 * time.Second
	teamSyncSettle = 2 * time.Second // after a change, for the next ones
	teamSyncTime   = 2 * time.Minute // a sync's longest
	// maxTeamFile is the most a file shared holds: a sealed item's size,
	// less its path and the seal's.
	maxTeamFile = api.MaxItemSize - 4096
)

func (t *teamSyncer) poked() chan struct{} {
	t.once.Do(func() { t.pokes = make(chan struct{}, 1) })
	return t.pokes
}

// kick asks for a sync soon, after a change here.
func (t *teamSyncer) kick() {
	select {
	case t.poked() <- struct{}{}:
	default:
	}
}

func (t *teamSyncer) status(team string) (time.Time, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.last[team]
	return st.at, st.err
}

func (t *teamSyncer) ended(team string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last == nil {
		t.last = map[string]syncStatus{}
	}
	st := t.last[team]
	st.err = ""
	if err != nil {
		st.err = err.Error()
	} else {
		st.at = time.Now()
	}
	t.last[team] = st
}

// teamSyncLoop syncs every space now and then, and soon after a change,
// until done closes.
func (s *Server) teamSyncLoop(done <-chan struct{}) {
	tick := time.NewTicker(teamSyncEvery)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
		case <-s.teamSync.poked():
			select {
			case <-done:
				return
			case <-time.After(teamSyncSettle):
			}
		}
		for _, sp := range s.spaces() {
			s.syncSpace(sp.Team)
		}
	}
}

// syncSpace syncs team's space now, and says how it went.
func (s *Server) syncSpace(team string) (proto.Space, error) { return s.syncSpaceWith(team, true) }

// pullSpace takes what changed in team's space in the cloud, sending
// nothing: a chat open asks often.
func (s *Server) pullSpace(team string) (proto.Space, error) { return s.syncSpaceWith(team, false) }

func (s *Server) syncSpaceWith(team string, push bool) (proto.Space, error) {
	s.teamSync.run.Lock()
	defer s.teamSync.run.Unlock()
	var sp proto.Space
	for _, x := range s.spaces() {
		if x.Team == team {
			sp = x
		}
	}
	if sp.Team == "" {
		return proto.Space{}, fmt.Errorf("spaces: team %s isn't on this host", team)
	}
	st, err := s.cloud.store.Get()
	if err != nil {
		return sp, err
	}
	c, err := cloudclient.New(st.URL, st.Token)
	if err != nil {
		return sp, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), teamSyncTime)
	defer cancel()
	ts := &teamSync{srv: s, c: c, team: team, role: sp.Role, root: filepath.Join(s.teamsDir(), team), device: st.DeviceID, push: push}
	err = ts.run(ctx)
	if cloudclient.IsStatus(err, http.StatusNotFound) {
		// Not a member any more, or the team deleted: the team's copy goes
		// (teamgone.go), the user's own stays.
		deleted := teamDeleted(err)
		s.dropSpace(sp, deleted)
		if deleted {
			return sp, errors.New("cloud: " + sp.Name + " was deleted by its owner: its space is gone from this host")
		}
		return sp, errors.New("cloud: no longer a member of " + sp.Name + ": its space is gone from this host")
	}
	s.teamSync.ended(team, err)
	if err == nil && ts.skipped != "" {
		s.teamSync.ended(team, errors.New(ts.skipped))
	}
	if ts.changed {
		s.groupsChanged(nil) // and the notes
		s.canvasesChanged()
		s.teamProjectsChanged()
		s.broadcast(proto.EventTeamHostsChanged, s.teamHosts())
	}
	if len(ts.chats) > 0 {
		s.chatCame(team, ts.chats)
	}
	s.chatGone(team, ts.chatsGone)
	s.broadcast(proto.EventSpacesChanged, s.spaces())
	for _, x := range s.spaces() {
		if x.Team == team {
			sp = x
		}
	}
	return sp, err
}

// teamSync is one sync of a team's space.
type teamSync struct {
	srv        *Server
	c          *cloudclient.Client
	team, role string
	root       string // teams/<id>
	device     string
	keys       teamKeyring
	state      teamSyncState
	changed    bool     // files here changed
	skipped    string   // what wasn't sent, and why
	push       bool     // send what changed here too
	chats      []string // chat messages that came (teamchat.go)
	chatsGone  []string // and those deleted
}

// teamSyncState is what the space and the cloud last agreed on, in
// sync/state.json: the last change seen, and each file's item.
type teamSyncState struct {
	Seq   int64                 `json:"seq"`
	Files map[string]syncedFile `json:"files"` // by path in the space
	// Retry is the first change not taken yet, asked for again since
	// RetrySince for a while (teamtrust.go); 0 for none.
	Retry      int64     `json:"retry,omitempty"`
	RetrySince time.Time `json:"retrySince,omitzero"`
}

type syncedFile struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	Hash    string `json:"hash,omitempty"` // "" once deleted
	Deleted bool   `json:"deleted,omitempty"`
}

func (ts *teamSync) stateFile() string { return filepath.Join(ts.root, "sync", "state.json") }

// baseFile keeps a file as last agreed, to merge both sides' changes.
func (ts *teamSync) baseFile(p string) string {
	return filepath.Join(ts.root, "sync", "base", filepath.FromSlash(p))
}

func (ts *teamSync) run(ctx context.Context) error {
	if data, err := os.ReadFile(ts.stateFile()); err == nil {
		_ = json.Unmarshal(data, &ts.state)
	}
	if ts.state.Files == nil {
		ts.state.Files = map[string]syncedFile{}
	}
	var err error
	if ts.keys, err = ts.srv.teamKeys(ctx, ts.c, ts.team, ts.role); err != nil {
		return err
	}
	if err := ts.pull(ctx); err != nil {
		ts.save()
		return err
	}
	if ts.role != api.RoleViewer && ts.push {
		if err := ts.pushChanges(ctx); err != nil {
			ts.save()
			return err
		}
	}
	return ts.save()
}

func (ts *teamSync) save() error {
	data, _ := json.Marshal(ts.state)
	return writeTeamFile(ts.stateFile(), data)
}

// teamSynced says a path of a space is shared with the team: its
// projects, its notes' and canvases' files, not agents' conversations, proposals, versions
// kept for undo, or files being written.
func teamSynced(p string) bool {
	if p == "projects.json" || (strings.HasPrefix(p, "kit/") || strings.HasPrefix(p, "rules/") || strings.HasPrefix(p, "hosts/") || strings.HasPrefix(p, "chat/")) && !strings.Contains(p, "..") {
		return true // the team's projects (teamprojects.go), kit (teamkit.go), projects' rules (rules.go), shared hosts (hostshare.go), chat (teamchat.go)
	}
	if !strings.HasPrefix(p, "notes/") && !strings.HasPrefix(p, "canvas/") || strings.Contains(p, "..") {
		return false
	}
	base := path.Base(p)
	switch {
	case strings.HasPrefix(base, ".") || strings.HasSuffix(base, ".tmp"):
		return false
	case p == "notes/proposals.json" || p == "notes/icons.json":
		return false
	case strings.HasPrefix(p, "notes/.history/") || strings.HasPrefix(p, "notes/chats/"):
		return false
	case strings.HasPrefix(p, "canvas/") && base == "chat.json":
		return false
	}
	return true
}

// itemID is the item of path p in team's space: the same on every device,
// telling the cloud nothing of the path.
func itemID(team, p string) string {
	h := sha256.Sum256([]byte("enowx team item\x00" + team + "\x00" + p))
	return hex.EncodeToString(h[:16])
}

func itemKind(p string) string {
	switch {
	case strings.HasPrefix(p, "notes/") && path.Dir(p) == "notes" && path.Ext(p) == ".md":
		return api.ItemNote
	case strings.HasPrefix(p, "canvas/"):
		return api.ItemCanvas
	case strings.HasPrefix(p, "chat/"):
		return api.ItemChat
	}
	return api.ItemFile
}

func hashOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// pull takes what changed in the cloud since the last sync, what the
// team's keys open and judge lets in (teamtrust.go).
func (ts *teamSync) pull(ctx context.Context) error {
	byID := map[string]string{}
	for p, f := range ts.state.Files {
		byID[f.ID] = p
	}
	after := ts.pullFrom()
	for {
		page, err := ts.c.Items(ctx, ts.team, after)
		if err != nil {
			return err
		}
		for _, it := range page.Items {
			p, known := byID[it.ID]
			if known && ts.state.Files[p].Version >= it.Version {
				continue // this device's own, or seen
			}
			var data []byte
			if !it.Deleted {
				key, ok := ts.keys.keys[it.Epoch]
				if !ok {
					ts.srv.logger().Warn("cloud: an item sealed with a key this device never had", "team", ts.team, "epoch", it.Epoch)
					continue
				}
				payload, err := seal.OpenItem(key, it.ID, it.Version, it.Sealed)
				if err != nil {
					ts.srv.logger().Warn("cloud: an item that doesn't open", "team", ts.team, "item", it.ID, "err", err)
					continue
				}
				name, rest, ok := bytes.Cut(payload, []byte{0})
				// The path is sealed with it: the cloud can't move a file.
				if !ok || !teamSynced(string(name)) || itemID(ts.team, string(name)) != it.ID {
					continue
				}
				p, data = string(name), rest
			} else if !known {
				continue // made and deleted before this device saw it
			} else if !ts.deletionSealed(p, it) {
				// Anyone writing the cloud's database could mark it
				// deleted: only a mark sealed with the team's key is.
				if _, err := os.Stat(filepath.Join(ts.root, filepath.FromSlash(p))); err == nil {
					ts.keepOurs(p, it, "a deletion without its sealed mark")
					continue
				}
			}
			switch v, why := ts.judge(p, it, data); v {
			case refuse:
				ts.keepOurs(p, it, why)
				continue
			case tryLater:
				ts.later(p, it, why)
				continue
			}
			byID[it.ID] = p
			if err := ts.apply(p, it, data); err != nil {
				return err
			}
		}
		after = page.Seq
		ts.state.Seq = max(ts.state.Seq, page.Seq)
		if !page.More {
			if ts.state.Retry == 0 {
				ts.state.RetrySince = time.Time{}
			}
			return nil
		}
	}
}

// apply takes item it, file p's version data (nil once deleted), into the
// space, merged with what changed here since the last sync.
func (ts *teamSync) apply(p string, it api.Item, data []byte) error {
	full := filepath.Join(ts.root, filepath.FromSlash(p))
	local, err := os.ReadFile(full)
	missing := errors.Is(err, os.ErrNotExist)
	if err != nil && !missing {
		return err
	}
	f := ts.state.Files[p]
	localHash, remoteHash := "", ""
	if !missing {
		localHash = hashOf(local)
	}
	if !it.Deleted {
		remoteHash = hashOf(data)
	}
	write := data
	switch {
	case localHash == f.Hash || localHash == remoteHash:
		// Unchanged here since the last sync: it changes as it did there.
	case it.Deleted:
		// Changed here, deleted there: kept, and sent back as it is here.
		ts.state.Files[p] = syncedFile{ID: it.ID, Version: it.Version, Deleted: true}
		os.Remove(ts.baseFile(p))
		return nil
	default:
		base, _ := os.ReadFile(ts.baseFile(p))
		if merged, ok := usersync.MergeFile(p, base, local, data); ok && !missing {
			write = merged // sent next, as this device's change
		} else if itemKind(p) == api.ItemNote && !missing {
			// The cloud's wins; this host's stays among the note's versions.
			_ = notes.KeepVersion(filepath.Join(ts.root, "notes"), strings.TrimSuffix(path.Base(p), ".md"), local, true)
		}
	}
	if it.Deleted {
		if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		os.Remove(ts.baseFile(p))
		removeEmptyDirs(filepath.Dir(full), ts.root)
	} else {
		if err := writeTeamFile(full, write); err != nil {
			return err
		}
		if mergeable(p) {
			_ = writeTeamFile(ts.baseFile(p), data)
		}
	}
	ts.state.Files[p] = syncedFile{ID: it.ID, Version: it.Version, Hash: remoteHash, Deleted: it.Deleted}
	ts.changed = true
	switch {
	case !strings.HasPrefix(p, "chat/"):
	case strings.HasPrefix(p, "chat/reactions/"):
		ts.chats = append(ts.chats, p) // its message's reactions changed
	case it.Deleted:
		ts.chatsGone = append(ts.chatsGone, p)
	default:
		ts.chats = append(ts.chats, p)
	}
	return nil
}

// mergeable files keep their last agreed version, to merge.
func mergeable(p string) bool { return path.Ext(p) == ".md" || path.Ext(p) == ".json" }

// pushChanges sends what changed here since the last sync.
func (ts *teamSync) pushChanges(ctx context.Context) error {
	key, ok := ts.keys.keys[ts.keys.current]
	if !ok {
		return errNoTeamKey
	}
	var secrets, big []string
	seen := map[string]bool{}
	err := filepath.WalkDir(ts.root, func(full string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(ts.root, full)
		p := filepath.ToSlash(rel)
		if !teamSynced(p) {
			return nil
		}
		seen[p] = true
		data, err := os.ReadFile(full)
		if err != nil {
			return nil
		}
		f, known := ts.state.Files[p]
		h := hashOf(data)
		if known && !f.Deleted && f.Hash == h {
			return nil
		}
		if len(data) > maxTeamFile {
			big = append(big, p)
			return nil
		}
		if autopush.SecretFile(p) {
			secrets = append(secrets, p+": a file of secrets, by its name")
			return nil
		}
		if found := autopush.ScanSecrets(p, data); len(found) > 0 {
			secrets = append(secrets, found[0].String())
			return nil
		}
		id := itemID(ts.team, p)
		version := f.Version + 1
		sealed, err := seal.SealItem(key, id, version, append([]byte(p+"\x00"), data...))
		if err != nil {
			return err
		}
		it, err := ts.c.PutItem(ctx, ts.team, id, api.PutItemParams{Kind: itemKind(p), Version: version, Epoch: ts.keys.current, Sealed: sealed})
		if cloudclient.IsStatus(err, http.StatusConflict) {
			return nil // changed there meanwhile: the next sync merges it
		}
		if err != nil {
			return err
		}
		ts.state.Files[p] = syncedFile{ID: id, Version: it.Version, Hash: h}
		if mergeable(p) {
			_ = writeTeamFile(ts.baseFile(p), data)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for p, f := range ts.state.Files {
		if seen[p] || f.Deleted {
			continue
		}
		mark := sealDeletion(key, f.ID, f.Version+1, p)
		it, err := ts.c.PutItem(ctx, ts.team, f.ID, api.PutItemParams{Kind: itemKind(p), Version: f.Version + 1, Epoch: ts.keys.current, Deleted: true, Sealed: mark})
		if cloudclient.IsStatus(err, http.StatusConflict) {
			continue
		}
		if err != nil {
			return err
		}
		ts.state.Files[p] = syncedFile{ID: f.ID, Version: it.Version, Deleted: true}
		os.Remove(ts.baseFile(p))
	}
	var why []string
	if len(secrets) > 0 {
		why = append(why, "not shared, they hold what looks like a secret: "+strings.Join(secrets, "; "))
	}
	if len(big) > 0 {
		why = append(why, fmt.Sprintf("not shared, over %d MB: %s", maxTeamFile>>20, strings.Join(big, ", ")))
	}
	ts.skipped = strings.Join(why, ". ")
	return nil
}

// writeTeamFile writes data to file whole or not at all, its folder made.
func writeTeamFile(file string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, file); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// removeEmptyDirs removes dir and its parents while empty, up to root.
func removeEmptyDirs(dir, root string) {
	for dir != root && strings.HasPrefix(dir, root) {
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

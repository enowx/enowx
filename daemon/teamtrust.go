package daemon

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/seal"
	"github.com/enowx/enowx/proto"
)

// What a sync takes from the cloud, beyond an item opening with the team's
// key (docs/cloud.md, Keys): a deletion only with its sealed mark, which
// the cloud can't make; a file only some may write (the team's projects,
// projects' rules, the kit, shared hosts, chat messages) only from a
// device of one who may, as the cloud says who each device's account is
// and its role; and no note or canvas whose ID another space holds, so a
// team's copy never shadows one elsewhere. What isn't taken leaves this
// host's copy as it is, and isn't sent back as this host's change.

// deletedMark follows a deleted item's path in its sealed mark.
const deletedMark = "enowx team item deleted"

// teamRetryFor is how long a sync asks again for items it couldn't take
// yet (the team's members unread, a note's ID still in another space as a
// move between spaces reaches this host).
const teamRetryFor = 24 * time.Hour

// teamRetryLate is how often it asks after that.
const teamRetryLate = time.Hour

// teamRecheck is the version of the one pull of every item a host makes, when a version
// of enowX may have left it past items it hadn't taken.
const teamRecheck = 1

// sealDeletion is the mark of file p's deletion, item id at version: its
// path and deletedMark, sealed as an item's content is. nil when it
// wouldn't fit what the cloud keeps of a deleted item.
func sealDeletion(key []byte, id string, version int64, p string) []byte {
	sealed, err := seal.SealItem(key, id, version, []byte(p+"\x00"+deletedMark))
	if err != nil || len(sealed) > api.MaxTombstone {
		return nil
	}
	return sealed
}

// deletionSealed says whether deleted item it carries a mark sealed with
// one of the team's keys naming file p.
func (ts *teamSync) deletionSealed(p string, it api.Item) bool {
	key, ok := ts.keys.keys[it.Epoch]
	if !ok || len(it.Sealed) == 0 {
		return false
	}
	payload, err := seal.OpenItem(key, it.ID, it.Version, it.Sealed)
	if err != nil {
		return false
	}
	name, mark, ok := bytes.Cut(payload, []byte{0})
	return ok && string(name) == p && string(mark) == deletedMark && itemID(ts.team, p) == it.ID
}

// verdict is what a sync does with an item that opened.
type verdict int

const (
	take     verdict = iota
	refuse           // kept out, this host's copy stays
	tryLater         // asked again at the next syncs
)

// judge says whether item it, file p (data its content, nil once
// deleted), may be taken, and why not.
func (ts *teamSync) judge(p string, it api.Item, data []byte) (verdict, string) {
	if !it.Deleted && !ts.here(p) && ts.srv.heldElsewhere(ts.team, p) {
		return tryLater, "its ID is another space's"
	}
	guarded := p == "projects.json" || strings.HasPrefix(p, "rules/") || strings.HasPrefix(p, "kit/") ||
		strings.HasPrefix(p, "hosts/") || strings.HasPrefix(p, "chat/")
	if !guarded || it.UpdatedBy == ts.device {
		return take, ""
	}
	if dev, ok := strings.CutPrefix(p, "hosts/"); ok {
		if it.UpdatedBy != strings.TrimSuffix(dev, ".json") {
			return refuse, "a shared host is announced by its own device only"
		}
		return take, ""
	}
	if ts.keys.roles == nil {
		return tryLater, "the team's members couldn't be read"
	}
	account := ""
	for _, d := range ts.keys.devices {
		if d.Device.ID == it.UpdatedBy {
			account = d.Account
		}
	}
	role := ts.keys.roles[account]
	admin := role == api.RoleOwner || role == api.RoleAdmin
	switch {
	case role == "":
		return refuse, "written by a device that isn't a member's"
	case role == api.RoleViewer:
		return refuse, "written by a viewer's device"
	case p == "projects.json":
		if !admin {
			return refuse, "the team's projects are changed by its admins"
		}
	case strings.HasPrefix(p, "rules/"):
		id := strings.TrimSuffix(path.Base(p), ".md")
		byMembers := slices.ContainsFunc(ts.srv.readTeamProjects(ts.team), func(x storedTeamProject) bool {
			return x.ID == id && x.RulesBy != proto.RulesByAdmins
		})
		if !admin && !byMembers {
			return refuse, "this project's rules are changed by the team's admins"
		}
	case strings.HasPrefix(p, "chat/reactions/"):
		return ts.judgeReaction(p, it, data, account, admin)
	case strings.HasPrefix(p, "chat/"):
		var was proto.ChatMessage
		had := false
		if old, err := os.ReadFile(filepath.Join(ts.root, filepath.FromSlash(p))); err == nil {
			had = json.Unmarshal(old, &was) == nil
		}
		if it.Deleted {
			if had && was.Account != account && !admin {
				return refuse, "a message is deleted by its author or an admin"
			}
			return take, ""
		}
		var m proto.ChatMessage
		if json.Unmarshal(data, &m) != nil || m.Account != account {
			return refuse, "a message written as another member"
		}
		if had && was.Account != account {
			return refuse, "a message changed by another than its author"
		}
	}
	return take, ""
}

// judgeReaction judges a member's reactions to a message, file p: written
// by that member only, and deleted by them, an admin, or the message's
// author as it goes (or once it went).
func (ts *teamSync) judgeReaction(p string, it api.Item, data []byte, account string, admin bool) (verdict, string) {
	message, owner, ok := reactionOf(p)
	if !ok {
		return refuse, "not a message's reactions"
	}
	if it.Deleted {
		var m proto.ChatMessage
		raw, err := os.ReadFile(filepath.Join(ts.root, "chat", message+".json"))
		if errors.Is(err, os.ErrNotExist) || err == nil && json.Unmarshal(raw, &m) == nil && m.Account == account {
			return take, "" // the message is gone, or its author deletes it
		}
		if owner != account && !admin {
			return refuse, "a member's reactions are taken back by them"
		}
		return take, ""
	}
	var r storedReactions
	if owner != account || json.Unmarshal(data, &r) != nil || r.Account != account {
		return refuse, "reactions written as another member"
	}
	return take, ""
}

// here says whether what file p is of (a note, a canvas) is in this space
// on this host already.
func (ts *teamSync) here(p string) bool {
	kind, name := spaceItemOf(p)
	if kind == "" {
		return true
	}
	_, err := os.Stat(filepath.Join(ts.root, kind, name))
	return err == nil
}

// spaceItemOf is what a space's file p is of: ("notes", "<id>.md") for a
// note, ("canvas", "<id>") for a canvas's file, "" for anything else.
func spaceItemOf(p string) (kind, name string) {
	switch parts := strings.Split(p, "/"); {
	case len(parts) == 2 && parts[0] == "notes" && strings.HasSuffix(parts[1], ".md"):
		return "notes", parts[1]
	case len(parts) >= 3 && parts[0] == "canvas":
		return "canvas", parts[1]
	}
	return "", ""
}

// heldElsewhere says whether the note or canvas file p of team's space is
// of has its ID in another space here: the user's own, or another team's.
func (s *Server) heldElsewhere(team, p string) bool {
	kind, name := spaceItemOf(p)
	if kind == "" {
		return false
	}
	return slices.ContainsFunc(s.spacesHolding(kind, name), func(t string) bool { return t != team })
}

// spacesHolding are the spaces holding name in their kind's folder ("" the
// user's own).
func (s *Server) spacesHolding(kind, name string) []string {
	var out []string
	if holdsSomething(filepath.Join(s.dataDir, kind, name)) {
		out = append(out, "")
	}
	for _, sp := range s.spaces() {
		if holdsSomething(filepath.Join(s.teamsDir(), sp.Team, kind, name)) {
			out = append(out, sp.Team)
		}
	}
	return out
}

// holdsSomething says path is a file, or a folder with a file in it: an empty folder
// holds no note or canvas, whatever left it, and mustn't keep a team's out.
func holdsSomething(path string) bool {
	found := false
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// keepOurs leaves file p as it is here, item it not taken: the item's
// version is recorded so this host's next change goes after it, and what
// is here isn't sent as a change it didn't make.
func (ts *teamSync) keepOurs(p string, it api.Item, why string) {
	ts.srv.logger().Warn("cloud: a team's change not taken", "team", ts.team, "path", p, "device", it.UpdatedBy, "why", why)
	prev, known := ts.state.Files[p]
	f := syncedFile{ID: it.ID, Version: it.Version, Hash: prev.Hash, Deleted: prev.Deleted}
	if !known {
		if local, err := os.ReadFile(filepath.Join(ts.root, filepath.FromSlash(p))); err == nil {
			f.Hash = hashOf(local)
		} else {
			f.Deleted = true
		}
	}
	ts.state.Files[p] = f
}

// later has the next syncs ask again for item it, from its place on.
func (ts *teamSync) later(p string, it api.Item, why string) {
	ts.srv.logger().Info("cloud: a team's change taken later", "team", ts.team, "path", p, "why", why)
	if ts.state.Retry == 0 || it.Seq < ts.state.Retry {
		ts.state.Retry = it.Seq
	}
	if ts.state.RetrySince.IsZero() {
		ts.state.RetrySince = time.Now()
	}
}

// pullFrom is the sequence number a pull asks for changes after: the last
// seen, or before items to ask again for while it is time to.
func (ts *teamSync) pullFrom() int64 {
	// Once, for a host a version of enowX left past items it never took: every item
	// again, those it has passing by, as their versions say.
	if ts.state.Recheck < teamRecheck {
		ts.state.Retry, ts.state.RetrySince = 0, time.Time{}
		return 0
	}
	from, retry, since := ts.state.Seq, ts.state.Retry, ts.state.RetrySince
	if retry <= 0 || retry-1 >= from {
		ts.state.Retry, ts.state.RetrySince = 0, time.Time{}
		return from
	}
	// Never given up on, as what was seen isn't seen again: after a while, less often.
	if time.Since(since) > teamRetryFor {
		if time.Since(ts.state.RetryAt) < teamRetryLate {
			return from
		}
		ts.state.RetryAt = time.Now()
	}
	ts.state.Retry = 0 // what still waits says so again, as it comes
	return retry - 1
}

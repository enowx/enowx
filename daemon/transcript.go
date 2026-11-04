package daemon

import (
	"context"
	"github.com/enowx/enowx/core/git"
	"github.com/enowx/enowx/core/project"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/transcript"
	"github.com/enowx/enowx/proto"
)

// transcripts follows the transcript of each session shown as a chat.
// The map and each follower's used are guarded by mu; the rest of a
// follower by its own lock, held while it reads its file.
type transcripts struct {
	mu    sync.Mutex
	byID  map[string]*follower
	files map[string]string // transcript file → the session following it
}

type follower struct {
	mu      sync.Mutex
	reader  *transcript.Reader
	checked time.Time // when we last looked for a newer file
	used    time.Time // when the app last asked
	opened  bool      // reader is new and no chat was told: the next to ask starts over
}

// dropAfter is how long a conversation no chat shows stays in memory; a
// chat shown polls every second or so, and one shown again reads it anew.
const dropAfter = 2 * time.Minute

// lookEvery is how often a session's folder is searched for a newer
// transcript: an agent starts a new file on /clear, for example.
const lookEvery = 2 * time.Second

func (t *transcripts) follower(id string) *follower {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.byID == nil {
		t.byID, t.files = map[string]*follower{}, map[string]string{}
	}
	now := time.Now()
	t.dropLocked(now)
	f := t.byID[id]
	if f == nil {
		f = &follower{}
		t.byID[id] = f
	}
	f.used = now
	return f
}

// held says how many conversations are in memory, and their entries.
func (t *transcripts) held() (conversations, entries int) {
	t.mu.Lock()
	followers := make([]*follower, 0, len(t.byID))
	for _, f := range t.byID {
		followers = append(followers, f)
	}
	t.mu.Unlock()
	for _, f := range followers {
		f.mu.Lock()
		if f.reader != nil && f.reader.Len() > 0 {
			conversations++
			entries += f.reader.Len()
		}
		f.mu.Unlock()
	}
	return conversations, entries
}

// quietFollower is session id's follower, not counted as a chat asking.
func (t *transcripts) quietFollower(id string) *follower {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.byID == nil {
		t.byID, t.files = map[string]*follower{}, map[string]string{}
	}
	f := t.byID[id]
	if f == nil {
		f = &follower{used: time.Now()}
		t.byID[id] = f
	}
	return f
}

// dropLocked lets go of the conversations no chat asked for lately; their files stay claimed.
func (t *transcripts) dropLocked(now time.Time) {
	for other, g := range t.byID {
		if now.Sub(g.used) > dropAfter {
			delete(t.byID, other)
		}
	}
}

// keepDropping lets go of them with no chat left to ask, until done closes: only a chat
// asking used to, so the last ones shown stayed in memory for good.
func (t *transcripts) keepDropping(done <-chan struct{}) {
	tick := time.NewTicker(dropAfter / 2)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case now := <-tick.C:
			t.mu.Lock()
			t.dropLocked(now)
			t.mu.Unlock()
		}
	}
}

// claim gives path to session id unless another session follows it.
func (t *transcripts) claim(id, path, old string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if owner, ok := t.files[path]; ok && owner != id {
		return false
	}
	if t.files == nil {
		t.files = map[string]string{}
	}
	delete(t.files, old)
	t.files[path] = id
	return true
}

// owned is the file session id follows, "" for none.
func (t *transcripts) owned(id string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	for path, owner := range t.files {
		if owner == id {
			return path
		}
	}
	return ""
}

func (t *transcripts) forget(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.byID, id)
	for path, owner := range t.files {
		if owner == id {
			delete(t.files, path)
		}
	}
}

// transcript returns what changed in session id's conversation.
func (s *Server) transcript(p proto.TranscriptParams) (proto.Transcript, error) {
	sess, err := s.sessions.Get(p.ID)
	if err != nil {
		return proto.Transcript{}, err
	}
	info := sess.Info()
	name := "This session"
	if preset, ok := agent.Find(info.Agent); ok {
		name = preset.Name
	}
	if !transcript.Supports(info.Agent) {
		return proto.Transcript{Reason: name + " has no chat view yet; it shows in the terminal."}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return proto.Transcript{}, err
	}
	f := s.transcripts.follower(p.ID)
	f.mu.Lock()
	defer f.mu.Unlock()
	reset := s.findTranscript(p.ID, info, home, f) || f.opened
	f.opened = false
	if f.reader == nil {
		return proto.Transcript{Supported: true, Reason: "Nothing yet: the conversation shows here once " + name + " records it."}, nil
	}
	if err := f.reader.Update(); err != nil {
		return proto.Transcript{}, err
	}
	if f.reader.Restarted() {
		reset = true
	}
	since := p.Since
	if reset {
		since = 0
	}
	var entries []proto.TranscriptEntry
	var rev int64
	switch {
	case p.Before > 0 && !reset:
		entries, rev = f.reader.Before(p.Before, max(p.Tail, 1))
	case since == 0 && p.Tail > 0:
		entries, rev = f.reader.Before(f.reader.Len(), p.Tail)
	default:
		entries, rev = f.reader.Since(since)
	}
	out := proto.Transcript{Supported: true, Entries: entries, Rev: rev, Reset: reset,
		Model: f.reader.Model, Mode: f.reader.Mode, Effort: f.reader.Effort,
		ContextTokens: f.reader.Context}
	if out.ContextTokens > 0 {
		out.ContextWindow = f.reader.Window
		if out.ContextWindow == 0 {
			out.ContextWindow = agent.SettingsOf(info.Agent).ContextWindow(f.reader.Model)
		}
	}
	// The screen says a mode change at once; the transcript only with
	// the next message.
	if m := agent.SettingsOf(info.Agent).ModeShown(sess.Terminal().Text()); m != "" {
		out.Mode = m
	}
	return out, nil
}

// findTranscript has f follow the newest file of the conversation of session id, looking
// every lookEvery, and says when that is another file than it followed. f.mu is held.
// Finding reads nothing of the file: what only wants its name stays cheap.
func (s *Server) findTranscript(id string, info proto.SessionInfo, home string, f *follower) (reset bool) {
	if f.reader != nil && time.Since(f.checked) <= lookEvery {
		return false
	}
	f.checked = time.Now()
	since := info.Created
	if sess, err := s.sessions.Get(id); err == nil && sess.Meta().ConvoSince.After(since) {
		since = sess.Meta().ConvoSince
	}
	for _, found := range transcript.FindIn(info.Agent, home, info.Dir, s.agentStore(info.Agent, id, false)) {
		// Conversations older than the session belong to earlier ones;
		// a restored session's changes once its agent continues.
		if found.Changed.Before(since.Add(-time.Second)) {
			break
		}
		if f.reader != nil && f.reader.Path() == found.Path {
			break
		}
		old := ""
		if f.reader != nil {
			old = f.reader.Path()
		}
		if s.transcripts.claim(id, found.Path, old) {
			f.reader, reset = transcript.Open(info.Agent, found.Path), true
			break
		}
	}
	return reset
}

// rewind takes session id's agent back to before one of the user's
// messages, watching its screen as it goes.
func (s *Server) rewind(p proto.SessionRewindParams) error {
	sess, err := s.sessions.Get(p.ID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return agent.Rewind(ctx, sess.Info().Agent, p.Back, p.Text, agent.Screen{
		Text: func() string { return sess.Terminal().Text() },
		Keys: func(keys string) { s.sessions.Input(sess, []byte(keys)) },
	})
}

// transcriptPath is the file of session id's agent's conversation, as the
// chat finds it; "" when there is none it knows.
func (s *Server) transcriptPath(id string) string {
	sess, err := s.sessions.Get(id)
	if err != nil {
		return ""
	}
	info := sess.Info()
	home, err := os.UserHomeDir()
	if err != nil || !transcript.Supports(info.Agent) {
		return ""
	}
	// Not a chat's asking: it must neither read the conversation in nor keep it from
	// being let go, or each agent at work holds its whole conversation in memory.
	f := s.transcripts.quietFollower(id)
	f.mu.Lock()
	defer f.mu.Unlock()
	if s.findTranscript(id, info, home, f) {
		f.opened = true
	}
	if f.reader == nil {
		return ""
	}
	path := f.reader.Path()
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// history lists the conversations agents had in project p's folder and
// its worktrees, with the branch each folder has.
func (s *Server) history(p proto.HistoryParams) ([]proto.Conversation, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	proj := project.Inspect(p.Project)
	dirs := []string{proj.Path}
	if proj.IsGit {
		if out, err := git.Run(proj.Path, "worktree", "list", "--porcelain"); err == nil {
			for _, line := range strings.Split(out, "\n") {
				if dir, ok := strings.CutPrefix(line, "worktree "); ok && dir != proj.Path {
					dirs = append(dirs, dir)
				}
			}
		}
	}
	limit := p.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	list := transcript.History(home, dirs, limit)
	branches := map[string]string{}
	for i, c := range list {
		if _, ok := branches[c.Dir]; !ok {
			branches[c.Dir] = git.CurrentBranch(c.Dir)
		}
		list[i].Branch = branches[c.Dir]
	}
	return list, nil
}

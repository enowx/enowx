package decide

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/envsync"
	"github.com/enowx/enowx/proto"
)

// ErrOff is a use the user hasn't turned on: the caller does what it did
// without.
var ErrOff = errors.New("decide: this use is off")

// Engine is a host's judge. Safe from any goroutine.
type Engine struct {
	dir      string
	settings *Settings
	// KeyOf is the value of the vault's credential named cred.
	KeyOf func(cred string) (string, error)
	// Found is what the host has that can judge, asked as judges are
	// listed.
	Found func() Found
	// Run runs an agent's one-shot command (tests stand in for agents).
	Run func(ctx context.Context, dir, bin string, args []string) ([]byte, error)
	// Mask hides the secrets the host knows in what is logged.
	Mask func(string) string

	mu      sync.Mutex
	found   []proto.Judge
	foundAt time.Time
	tests   map[string]proto.Judge // the last test of each, by ID
	failed  map[string]time.Time   // judges that failed lately, left out a while
	cache   map[string]proto.Decision
	log     []proto.Decision
	busy    chan struct{} // judges asked at once
}

// Limits.
const (
	foundFor  = time.Minute      // judges found are listed again after
	cacheFor  = 10 * time.Minute // the same question gets the same answer
	failedFor = 5 * time.Minute  // a judge that failed is tried again after
	logMax    = 1000
	logInput  = 2000 // characters of the input kept in the log
	atOnce    = 3
)

// timeouts are how long a use waits on one judge.
var timeouts = map[string]time.Duration{
	proto.DecideApprove: 25 * time.Second, proto.DecideState: 30 * time.Second,
	proto.DecideLoop: 40 * time.Second, proto.DecideTrim: 90 * time.Second,
	// A prompt waits on its context; the rest wait on nobody.
	proto.DecideRoute: 30 * time.Second, proto.DecideContext: 20 * time.Second,
	proto.DecideMemory: 60 * time.Second, proto.DecideChat: 30 * time.Second, proto.DecideFlaky: 60 * time.Second,
}

// NewEngine is the judge of the daemon in dir, by s.
func NewEngine(dir string, s *Settings) *Engine {
	e := &Engine{dir: dir, settings: s, tests: map[string]proto.Judge{}, failed: map[string]time.Time{},
		cache: map[string]proto.Decision{}, busy: make(chan struct{}, atOnce)}
	if data, err := os.ReadFile(e.testsFile()); err == nil {
		_ = json.Unmarshal(data, &e.tests)
	}
	if data, err := os.ReadFile(e.logFile()); err == nil {
		_ = json.Unmarshal(data, &e.log)
	}
	return e
}

func (e *Engine) testsFile() string { return filepath.Join(e.dir, "decide-judges.json") }
func (e *Engine) logFile() string   { return filepath.Join(e.dir, "decide-log.json") }

// Settings are the engine's settings.
func (e *Engine) Settings() *Settings { return e.settings }

// Judges are the judges found and the user's own, in the order tried, as
// last tested.
func (e *Engine) Judges(ctx context.Context) []proto.Judge {
	e.mu.Lock()
	stale := time.Since(e.foundAt) > foundFor
	e.mu.Unlock()
	if stale && e.Found != nil {
		found := Detect(ctx, e.Found())
		e.mu.Lock()
		e.found, e.foundAt = found, time.Now()
		e.mu.Unlock()
	}
	st := e.settings.Get()
	e.mu.Lock()
	all := append(slices.Clone(e.found), st.Own...)
	for i, j := range all {
		if t, ok := e.tests[j.ID]; ok {
			all[i].Tested, all[i].OK, all[i].Error, all[i].Ms, all[i].Tokens, all[i].Cost = t.Tested, t.OK, t.Error, t.Ms, t.Tokens, t.Cost
		}
	}
	e.mu.Unlock()
	return Ordered(st, all)
}

func (e *Engine) judge(ctx context.Context, id string) (proto.Judge, bool) {
	for _, j := range e.Judges(ctx) {
		if j.ID == id {
			return j, true
		}
	}
	return proto.Judge{}, false
}

// testAsk is what a test asks: a command any judge allows.
var testAsk = proto.DecideAsk{Use: proto.DecideApprove, Command: "git status", Project: "shop"}

// Test asks judge id (every judge when "") a fixed question and keeps how
// it did.
func (e *Engine) Test(ctx context.Context, id string) []proto.Judge {
	var list []proto.Judge
	for _, j := range e.Judges(ctx) {
		if id == "" || j.ID == id {
			list = append(list, j)
		}
	}
	var wg sync.WaitGroup
	for i := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j := &list[i]
			q, _ := questionOf(testAsk, "")
			v, a, err := e.ask(ctx, *j, q, timeouts[proto.DecideApprove])
			j.Tested, j.Ms, j.Tokens, j.Cost, j.OK, j.Error = time.Now(), a.took.Milliseconds(), a.tokens, a.cost, err == nil, ""
			if err != nil {
				j.Error = err.Error()
			} else if v.Verdict != proto.VerdictAllow {
				j.OK, j.Error = false, "it didn't allow `git status`: "+v.Why
			}
		}()
	}
	wg.Wait()
	e.mu.Lock()
	for _, j := range list {
		e.tests[j.ID] = j
		delete(e.failed, j.ID)
	}
	data, _ := json.MarshalIndent(e.tests, "", "  ")
	e.mu.Unlock()
	_ = envsync.WriteFileAtomic(e.testsFile(), data, 0o600)
	return list
}

// ask asks judge j question q within wait, one of a few at once.
func (e *Engine) ask(ctx context.Context, j proto.Judge, q question, wait time.Duration) (verdict, answer, error) {
	select {
	case e.busy <- struct{}{}:
		defer func() { <-e.busy }()
	case <-ctx.Done():
		return verdict{}, answer{}, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	start := time.Now()
	a, err := e.call(ctx, j, systemFor(q), q.prompt)
	a.took = time.Since(start)
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("decide: no answer within %s", wait)
		}
		return verdict{}, a, err
	}
	v, err := parse(q, a.text)
	return v, a, err
}

// Request is a decision asked: the question, and whose (the session's
// agent, the project's ID for its policy).
type Request struct {
	proto.DecideAsk
	Agent     string // the session's agent, for the policy and how it is named
	AgentName string
	ProjectID string
	// Provider is where the session's agent's models come from by its
	// config (proto.SessionConfig.Provider); "" for the agent's own setting.
	Provider string
}

// agentProvider is where r's agent's models come from, as the host has it
// set.
func (e *Engine) agentProvider(agent string) string {
	if e.Found != nil {
		return e.Found().EffectiveProvider(agent)
	}
	return AgentProvider(agent)
}

// Decide asks the judges in turn until one answers as asked, and logs it.
// ErrOff when the use is off; a decision with Error when none answered.
func (e *Engine) Decide(ctx context.Context, r Request) (proto.Decision, error) {
	st := e.settings.Get()
	u := Use(st, r.Use)
	if r.Judge == "" && !u.On {
		return proto.Decision{}, ErrOff
	}
	q, err := questionOf(r.DecideAsk, r.AgentName)
	if err != nil {
		return proto.Decision{}, err
	}
	d := proto.Decision{ID: newID(), Use: r.Use, Session: r.Session, Project: r.Project, Agent: r.Agent, Time: time.Now(), Input: e.mask(head(q.prompt, logInput))}
	key := cacheKey(r)
	e.mu.Lock()
	if c, ok := e.cache[key]; ok && time.Since(c.Time) < cacheFor && r.Judge == "" {
		e.mu.Unlock()
		c.ID, c.Session, c.Time, c.Acted, c.Wrong, c.Note = newID(), r.Session, time.Now(), "", false, ""
		c.Ms, c.Tokens, c.Cost = 0, 0, 0
		c.Why = strings.TrimSuffix(c.Why, " (as before)") + " (as before)"
		e.Record(c)
		return c, nil
	}
	if r.Judge == "" && e.callsToday(r.Use) >= u.DailyCalls {
		e.mu.Unlock()
		d.Error = fmt.Sprintf("today's %d calls are used up", u.DailyCalls)
		e.Record(d)
		return d, nil
	}
	e.mu.Unlock()
	for _, j := range e.chain(ctx, st, u, r) {
		v, a, err := e.ask(ctx, j, q, timeouts[r.Use])
		d.Tokens += a.tokens
		d.Cost += a.cost
		if err != nil {
			d.Tried = append(d.Tried, j.ID+": "+err.Error())
			e.mu.Lock()
			e.failed[j.ID] = time.Now()
			e.mu.Unlock()
			if ctx.Err() != nil {
				break
			}
			continue
		}
		d.Judge, d.Ms, d.Verdict, d.Confidence, d.Why, d.Text = j.ID, a.took.Milliseconds(), v.Verdict, v.Confidence, v.Why, v.Text
		d.Keep = v.Keep
		break
	}
	if d.Judge == "" {
		d.Error = "no judge answered"
		if len(d.Tried) == 0 {
			d.Error = "no judge may judge this under the policy: Settings → Decisions"
		}
	} else if r.Judge == "" {
		e.mu.Lock()
		e.cache[key] = d
		e.mu.Unlock()
	}
	e.Record(d)
	return d, nil
}

// chain are the judges r is asked of, in turn.
func (e *Engine) chain(ctx context.Context, st proto.DecideSettings, u proto.DecideUse, r Request) []proto.Judge {
	if r.Judge != "" {
		j, ok := e.judge(ctx, r.Judge)
		if !ok {
			return nil
		}
		return []proto.Judge{j}
	}
	policy, provider := st.Policy, st.Provider
	if p, ok := st.Projects[r.ProjectID]; ok && p.Policy != "" {
		policy, provider = p.Policy, p.Provider
	}
	agentP := r.Provider
	if agentP == "" {
		agentP = e.agentProvider(r.Agent)
	}
	var out []proto.Judge
	e.mu.Lock()
	failed := maps(e.failed)
	e.mu.Unlock()
	for _, j := range e.Judges(ctx) {
		if slices.Contains(st.Off, j.ID) || !allowed(j, policy, provider, agentP) || u.Judge != "" && j.ID != u.Judge {
			continue
		}
		if at, ok := failed[j.ID]; ok && time.Since(at) < failedFor {
			continue
		}
		out = append(out, j)
	}
	return out
}

func maps(m map[string]time.Time) map[string]time.Time {
	out := make(map[string]time.Time, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// callsToday are the judges asked today for use: each tried counts, an
// answer from the cache doesn't. e.mu is held.
func (e *Engine) callsToday(use string) int {
	y, m, dd := time.Now().Date()
	n := 0
	for _, d := range e.log {
		if dy, dm, ddd := d.Time.Date(); d.Use != use || dy != y || dm != m || ddd != dd || strings.HasSuffix(d.Why, "(as before)") {
			continue
		}
		n += len(d.Tried)
		if d.Judge != "" {
			n++
		}
	}
	return n
}

func cacheKey(r Request) string {
	h := sha256.New()
	for _, s := range []string{r.Use, r.Agent, r.Project, r.Command, r.Output, r.Screen, strings.Join(r.Steps, "\n"), fmt.Sprint(r.Exit),
		r.Prompt, strings.Join(r.Parts, "\x00"), r.Text, r.From, r.Place, fmt.Sprint(r.Mention)} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

// tokenLike finds what looks like a secret in text logged: keys, tokens, a
// password in an address.
var tokenLike = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----|\b(sk-[A-Za-z0-9_-]{16,}|gh[pousr]_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|xox[abpr]-[A-Za-z0-9-]{10,}|AIza[0-9A-Za-z_-]{30,})\b|://[^/\s:@]+:[^/\s@]+@`)

func (e *Engine) mask(s string) string {
	if e.Mask != nil {
		s = e.Mask(s)
	}
	return tokenLike.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, "://") {
			return "://•••@"
		}
		return "•••"
	})
}

// Record keeps decision d in the log, or changes it there (by ID).
func (e *Engine) Record(d proto.Decision) {
	e.mu.Lock()
	if i := slices.IndexFunc(e.log, func(x proto.Decision) bool { return x.ID == d.ID }); i >= 0 {
		e.log[i] = d
	} else {
		e.log = append(e.log, d)
		if len(e.log) > logMax {
			e.log = slices.Clone(e.log[len(e.log)-logMax:])
		}
	}
	data, _ := json.Marshal(e.log)
	e.mu.Unlock()
	_ = envsync.WriteFileAtomic(e.logFile(), data, 0o600)
}

// Acted records what came of decision id.
func (e *Engine) Acted(id, acted string, saved int) (proto.Decision, bool) {
	e.mu.Lock()
	i := slices.IndexFunc(e.log, func(x proto.Decision) bool { return x.ID == id })
	if i < 0 {
		e.mu.Unlock()
		return proto.Decision{}, false
	}
	d := e.log[i]
	e.mu.Unlock()
	d.Acted, d.Saved = acted, saved
	e.Record(d)
	return d, true
}

// Mark says decision m.ID was wrong (or not), with the user's note.
func (e *Engine) Mark(m proto.DecideMark) error {
	e.mu.Lock()
	i := slices.IndexFunc(e.log, func(x proto.Decision) bool { return x.ID == m.ID })
	if i < 0 {
		e.mu.Unlock()
		return fmt.Errorf("decide: no decision %s", m.ID)
	}
	d := e.log[i]
	e.mu.Unlock()
	d.Wrong, d.Note = m.Wrong, m.Note
	e.Record(d)
	return nil
}

// Log is the decisions kept, the latest first, as p filters them.
func (e *Engine) Log(p proto.DecideLogParams) []proto.Decision {
	limit := p.Limit
	if limit <= 0 {
		limit = 200
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	out := []proto.Decision{}
	for i := len(e.log) - 1; i >= 0 && len(out) < limit; i-- {
		d := e.log[i]
		if (p.Use == "" || d.Use == p.Use) && (p.Session == "" || d.Session == p.Session) {
			out = append(out, d)
		}
	}
	return out
}

func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

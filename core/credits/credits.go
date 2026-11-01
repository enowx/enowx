// Package credits reads what each agent account signed in on this machine
// has left of its plan, with the agent's own login: Claude Code's and
// Codex's tokens go to the vendor's usage endpoint, the only place they are
// sent, and Kiro CLI is asked for its /usage. Tokens are read when needed,
// never kept, logged or refreshed: refreshing one would sign the agent out.
package credits

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/enowx/enowx/proto"
)

// Readings older than staleAfter are read again when asked for; a refresh
// asked for sooner than minGap after the last one waits for it.
const (
	staleAfter  = 5 * time.Minute
	minGap      = 30 * time.Second
	readTimeout = 45 * time.Second
)

// provider reads the accounts of one agent. It returns none when the
// agent is not installed or not signed in.
type provider func(ctx context.Context) []proto.CreditAccount

var providers = []provider{readClaude, readCodex, readKiro}

// kiroProvider is readKiro's place in providers.
const kiroProvider = 2

// Checker keeps the latest reading. Its fields are guarded by mu; the
// reading itself runs on a goroutine of its own, one at a time.
type Checker struct {
	// Homes, when set, are the agents' other accounts to read too.
	Homes func() []Home

	mu       sync.Mutex
	report   proto.CreditsReport
	started  time.Time
	checking bool
}

// Report returns the latest reading, and starts a new one when it is
// stale, or when refresh asks and the last one is not too recent.
func (c *Checker) Report(refresh bool) proto.CreditsReport {
	c.mu.Lock()
	defer c.mu.Unlock()
	since := time.Since(c.started)
	if !c.checking && (since > staleAfter || refresh && since > minGap) {
		c.checking, c.started = true, time.Now()
		go c.read(refresh)
	}
	r := c.report
	r.Checking = c.checking
	return r
}

// read reads every provider; one asked for by the user (asked) reads Kiro
// CLI too, which a reading on its own keeps as it last was: Kiro CLI,
// signed in through a browser whose session expired, opens the browser to
// sign in again, which only the user's asking may do.
func (c *Checker) read(asked bool) {
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	var homes []Home
	if c.Homes != nil {
		homes = c.Homes()
	}
	ctx = context.WithValue(ctx, skipKey{}, homes) // read as accounts of their own
	results := make([][]proto.CreditAccount, len(providers))
	var wg sync.WaitGroup
	for i, p := range providers {
		if !asked && i == kiroProvider {
			c.mu.Lock()
			for _, a := range c.report.Accounts {
				if a.Agent == "kiro" {
					results[i] = append(results[i], a)
				}
			}
			c.mu.Unlock()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = p(ctx)
		}()
	}
	own := make([]proto.CreditAccount, len(homes))
	for i, h := range homes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			own[i] = readHome(ctx, h)
		}()
	}
	wg.Wait()
	accounts := []proto.CreditAccount{}
	for _, r := range results {
		accounts = append(accounts, r...)
	}
	for _, a := range own {
		if a.Agent != "" {
			accounts = append(accounts, a)
		}
	}
	c.mu.Lock()
	c.report = proto.CreditsReport{Accounts: accounts, Checked: time.Now()}
	c.checking = false
	c.mu.Unlock()
}

// Home is one of an agent's other accounts: its ID and name, and the home
// it signs in in.
type Home struct {
	ID, Agent, Name, Path string
}

// readHome reads the plan of the account signed in in h, if it is.
func readHome(ctx context.Context, h Home) proto.CreditAccount {
	var acc proto.CreditAccount
	switch h.Agent {
	case "claude":
		raws := claudeKeychainItem(ctx, claudeService(h.Path))
		if file, err := os.ReadFile(filepath.Join(h.Path, ".credentials.json")); err == nil {
			raws = append(raws, file)
		}
		accs := readClaudeLogins(ctx, raws, "")
		if len(accs) == 0 {
			return proto.CreditAccount{Agent: "claude", Error: "not signed in yet"}.WithID(h.ID, h.Name)
		}
		acc = accs[0]
	case "codex":
		accs := readCodexAt(ctx, h.Path)
		if len(accs) == 0 {
			return proto.CreditAccount{Agent: "codex", Error: "not signed in yet"}.WithID(h.ID, h.Name)
		}
		acc = accs[0]
	default:
		return proto.CreditAccount{}
	}
	return acc.WithID(h.ID, h.Name)
}

// claudeService is the keychain item Claude Code keeps the login of
// configuration folder dir in: its name ends with the start of the
// folder's SHA-256.
func claudeService(dir string) string {
	sum := sha256.Sum256([]byte(norm.NFC.String(dir)))
	return "Claude Code-credentials-" + hex.EncodeToString(sum[:])[:8]
}

type skipKey struct{}

package ui

import (
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// fakeKeychain stands in for the system's keychain.
func fakeKeychain(t *testing.T) map[string][]byte {
	kept := map[string][]byte{}
	var mu sync.Mutex
	old := keychain
	keychain.get = func(k string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		if v, ok := kept[k]; ok {
			return v, nil
		}
		return nil, errors.New("not found")
	}
	keychain.set = func(k string, v []byte) error { mu.Lock(); kept[k] = v; mu.Unlock(); return nil }
	keychain.del = func(k string) error { mu.Lock(); delete(kept, k); mu.Unlock(); return nil }
	t.Cleanup(func() { keychain = old })
	return kept
}

// vaultHarness is the app on a real daemon, the Credentials page shown,
// what comes from the host applied between frames.
type vaultHarness struct {
	t     *testing.T
	app   *App
	cl    *client.Client // the app's connection
	other *client.Client // an agent's, on the same host
	tt    *ui.Tester
	mu    sync.Mutex
	later []func()
}

func newVaultHarness(t *testing.T) *vaultHarness {
	fakeKeychain(t)
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	h := &vaultHarness{t: t, app: New(Options{})}
	h.app.vlt = &vaultApp{dir: t.TempDir(), testing: map[string]bool{}}
	h.app.onMain = func(fn func()) { h.mu.Lock(); h.later = append(h.later, fn); h.mu.Unlock() }
	dial := func(onEvent func(proto.Event)) *client.Client {
		a, b := net.Pipe()
		go srv.ServeConn(a)
		c, err := client.New(b, onEvent)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	h.cl = dial(func(ev proto.Event) { h.app.handleEvent(1, ev) })
	h.other = dial(nil)
	h.app.gen, h.app.client, h.app.page = 1, h.cl, pageVault
	h.app.host = hosts.Host{ID: hosts.LocalID, Name: "this Mac"}
	h.app.vaultConnected(h.cl)
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(1000).Height(720).Background(theme.Background).TextColor(theme.Text).Children(func() {
			h.app.vaultPage(c)
			h.app.asksView(c)
		})
	}, 1000, 720)
	return h
}

func (h *vaultHarness) settle(what string, done func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		h.tt.Frame()
		if done() {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("%s: never; shows %q", what, h.tt.Texts())
		}
		time.Sleep(10 * time.Millisecond)
		h.mu.Lock()
		later := h.later
		h.later = nil
		h.mu.Unlock()
		for _, fn := range later {
			fn()
		}
	}
}

func (h *vaultHarness) click(label string) {
	h.t.Helper()
	if err := h.tt.Click(label); err != nil {
		h.t.Fatalf("%v; shows %q", err, h.tt.Texts())
	}
	h.tt.Frame()
}

// The user makes the vault, adds a GitHub token, locks and unlocks it;
// the host connected gets the ciphertext and an index without the token.
func TestVaultIsMadeFilledLockedAndUnlocked(t *testing.T) {
	h := newVaultHarness(t)
	h.settle("the page", func() bool { return h.tt.HasText("One vault for your logins") })
	h.tt.Type("correct horse")
	h.click("The same")
	h.tt.Type("correct horse")
	h.click("Make the vault")
	h.settle("the empty vault", func() bool { return h.tt.HasText("No credentials yet") })

	h.app.vaultOf().view.OpenAdd("")
	h.settle("the platforms", func() bool { return h.tt.HasText("Add a credential") })
	shot(t, h.tt, "vault-platforms.png")
	h.click("GitHub")
	h.settle("the form", func() bool { return h.tt.HasText("Add GitHub") })
	h.click("Paste the token")
	h.tt.Type("ghp_fakeTOKEN123456")
	h.click("you@example.com")
	h.tt.Type("ada")
	h.click("Add")
	h.settle("the credential listed", func() bool { return h.tt.HasText("github") && h.tt.HasText("GitHub · ada · …3456") })
	shot(t, h.tt, "vault-list.png")

	h.settle("the host's copy", func() bool {
		idx, err := h.other.VaultIndex()
		return err == nil && idx.Present && len(idx.Credentials) == 1
	})
	cp, _ := h.other.VaultGet()
	if strings.Contains(string(cp.Data), "ghp_fake") {
		t.Fatal("the host's copy holds the token in the clear")
	}

	h.click("Lock the vault")
	h.settle("locked", func() bool { return h.tt.HasText("The vault is locked") })
	h.click("Your master password")
	h.tt.Type("wrong horse")
	h.click("Unlock")
	h.settle("refused", func() bool { return h.tt.HasText("wrong master password") })
	h.click("Your master password")
	h.tt.Type("correct horse")
	h.click("Unlock")
	h.settle("unlocked", func() bool { return h.tt.HasText("GitHub · ada · …3456") })
}

// An agent on the host asks for a credential: the user is asked once,
// says Always on this host, and the agent gets its variables; the next
// ask goes through without asking. Locked, the user unlocks first.
func TestVaultLendsAfterTheUserAllowsTheHost(t *testing.T) {
	h := newVaultHarness(t)
	if err := h.app.vaultCreate("correct horse", false); err != nil {
		t.Fatal(err)
	}
	v := h.app.vaultOf()
	v.data.Add(vault.Credential{Platform: "cloudflare", Name: "cf", Fields: map[string]string{"token": "cf_fakeTOKEN987654"}})
	h.app.vaultSave()

	type result struct {
		lend proto.VaultLend
		err  error
	}
	ask := func() chan result {
		ch := make(chan result, 1)
		go func() {
			lend, err := h.other.VaultAsk(proto.VaultAsk{Action: proto.VaultEnv, Name: "cf", Command: "wrangler deploy"})
			ch <- result{lend, err}
		}()
		return ch
	}
	got := ask()
	h.settle("the question", func() bool { return h.tt.HasText("Let this Mac use cf?") })
	shot(t, h.tt, "vault-ask.png")
	h.click("Always on this Mac")
	var r result
	h.settle("the answer", func() bool {
		select {
		case r = <-got:
			return true
		default:
			return false
		}
	})
	if r.err != nil || r.lend.Env["CLOUDFLARE_API_TOKEN"] != "cf_fakeTOKEN987654" {
		t.Fatalf("lent %+v, %v", r.lend, r.err)
	}
	if c, _ := v.data.Find("cf"); len(c.Hosts) != 1 || c.Hosts[0] != hosts.LocalID {
		t.Fatalf("hosts: %v", c.Hosts)
	}
	got = ask()
	h.settle("lent without asking", func() bool {
		select {
		case r = <-got:
			return true
		default:
			return false
		}
	})
	if r.err != nil || r.lend.Env["CLOUDFLARE_API_TOKEN"] == "" {
		t.Fatalf("second: %+v %v", r.lend, r.err)
	}

	h.app.vaultLock()
	got = ask()
	h.settle("unlock first", func() bool { return h.tt.HasText("Unlock the vault") && h.tt.HasText("Unlock the vault to answer") })
	shot(t, h.tt, "vault-ask-locked.png")
	if h.tt.Focused("Unlock the vault to answer") {
		t.Fatal("the password field took the keyboard unasked")
	}
	h.click("Unlock the vault to answer")
	h.tt.Type("correct horse")
	h.click("Unlock and answer")
	h.settle("answered after unlocking", func() bool {
		select {
		case r = <-got:
			return true
		default:
			return false
		}
	})
	if r.err != nil || r.lend.Env["CLOUDFLARE_API_TOKEN"] == "" {
		t.Fatalf("after unlocking: %+v %v", r.lend, r.err)
	}
	h.app.vaultLock()
	got = ask()
	h.settle("unlock first, again", func() bool { return h.tt.HasText("Unlock the vault to answer") })
	h.click("Refuse")
	h.settle("refused while locked", func() bool {
		select {
		case r = <-got:
			return true
		default:
			return false
		}
	})
	if r.err == nil || !strings.Contains(r.err.Error(), "the vault is locked") {
		t.Fatalf("locked and refused: %v", r.err)
	}
	audit, _ := h.other.VaultAudit(0)
	if len(audit) != 4 {
		t.Fatalf("audit: %+v", audit)
	}
}

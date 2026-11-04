package transport

import (
	"errors"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestAsUserQuotesForTheShell(t *testing.T) {
	cmd := `echo "$HOME" 'it'"'"'s'`
	if got, want := AsUser("enowx", cmd), "su - enowx -c "+shellQuote(cmd); got != want {
		t.Fatalf("AsUser = %s, want %s", got, want)
	}
	// The quoted command must reach su's shell unchanged.
	out, err := exec.Command("sh", "-c", "printf %s "+shellQuote(cmd)).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != cmd {
		t.Fatalf("round trip = %s, want %s", out, cmd)
	}
	if AsUser("", cmd) != cmd {
		t.Fatal("AsUser without a user changed the command")
	}
}

func TestKnownAlgorithmsFollowKnownHosts(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{knownhosts.Normalize("100.64.0.2:22")}, key)
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	known, err := knownhosts.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := knownAlgorithms(known, "100.64.0.2:22"); !slices.Equal(got, []string{ssh.KeyAlgoED25519}) {
		t.Fatalf("known host: %v, want only %s", got, ssh.KeyAlgoED25519)
	}
	if got := knownAlgorithms(known, "100.64.0.9:22"); got != nil {
		t.Fatalf("unknown host: %v, want nil so the server picks", got)
	}
}

func TestWithSSHConfigFillsWhatTheAddressLeavesOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	config := "Host box\n  HostName 100.64.0.2\n  User deploy\n  Port 2222\n  IdentityFile ~/.ssh/work_key\n"
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	tg, err := ParseSSHTarget("box")
	if err != nil {
		t.Fatal(err)
	}
	got := tg.WithSSHConfig()
	if got.Host != "100.64.0.2" || got.User != "deploy" || got.Port != "2222" || !slices.Equal(got.IdentityFiles, []string{"~/.ssh/work_key"}) {
		t.Fatalf("box = %+v", got)
	}
	// What the address says wins over the config.
	tg, _ = ParseSSHTarget("root@box:22")
	if got := tg.WithSSHConfig(); got.User != "root" || got.Port != "22" {
		t.Fatalf("root@box:22 = %+v", got)
	}
}

func TestSSHTargetRoundTrips(t *testing.T) {
	for _, addr := range []string{"root@100.64.0.2", "deploy@[fd7a::2]:2222", "box", "me@vps:2200"} {
		tg, err := ParseSSHTarget(addr)
		if err != nil {
			t.Fatal(err)
		}
		if got := tg.String(); got != addr {
			t.Errorf("%s round-trips to %s", addr, got)
		}
	}
}

func TestSSHConfigHostsListsAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	cfg := "Host *\n  ServerAliveInterval 30\n\nHost box web-?\n  HostName 10.0.0.5\n  User deploy\n  Port 2222\n  ProxyJump bastion\n"
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	got := SSHConfigHosts()
	if len(got) != 1 || got[0].Alias != "box" {
		t.Fatalf("hosts %+v", got)
	}
	if r := got[0].Real; r.Host != "10.0.0.5" || r.User != "deploy" || r.Port != "2222" || r.Jump != "bastion" {
		t.Fatalf("box goes to %+v", r)
	}
}

// A nologin account says why on stdout, its stderr empty: the error says so too, not
// only the exit status.
func TestCommandErrorSaysWhatTheCommandSaid(t *testing.T) {
	exit := errors.New("Process exited with status 1")
	for _, c := range []struct{ stderr, stdout, want string }{
		{"", "This account is currently not available.\n", "Process exited with status 1: This account is currently not available."},
		{"denied\n", "other\n", "Process exited with status 1: denied"},
		{" \n", "\n", "Process exited with status 1"},
	} {
		err := commandError(exit, c.stderr, c.stdout)
		if err.Error() != c.want || !errors.Is(err, exit) {
			t.Errorf("stderr %q stdout %q: %v, want %q", c.stderr, c.stdout, err, c.want)
		}
	}
}

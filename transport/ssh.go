package transport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	sshconfig "github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// RemoteDaemonPath is where enowxd lives on servers, relative to $HOME.
const RemoteDaemonPath = HomeName + "/bin/enowxd"

// SSHTarget is where to connect: "user@host", "host:port" or "user@host:port".
type SSHTarget struct {
	User string // "" until WithSSHConfig: ~/.ssh/config's, else this user's
	Host string
	Port string // "" until WithSSHConfig: ~/.ssh/config's, else 22
	// IdentityFiles are the keys ~/.ssh/config names for the host.
	IdentityFiles []string
	// Jump is a host to go through, "[user@]host[:port]", as ssh's
	// ProxyJump: a bastion in front of a private network. "" for none.
	Jump string
}

// String formats the target as [user@]host, with :port when it isn't 22.
func (t SSHTarget) String() string {
	s := t.Host
	if t.Port != "" && t.Port != "22" {
		s = net.JoinHostPort(t.Host, t.Port)
	}
	if t.User != "" {
		s = t.User + "@" + s
	}
	return s
}

// ParseSSHTarget parses "[user@]host[:port]".
func ParseSSHTarget(s string) (SSHTarget, error) {
	s = strings.TrimSpace(s)
	var t SSHTarget
	if at := strings.LastIndex(s, "@"); at >= 0 {
		t.User, s = s[:at], s[at+1:]
	}
	if host, port, err := net.SplitHostPort(s); err == nil {
		t.Host, t.Port = host, port
	} else {
		t.Host = s
	}
	if t.Host == "" || strings.ContainsAny(t.Host, " /") {
		return SSHTarget{}, fmt.Errorf("transport: %q is not a host", s)
	}
	return t, nil
}

// WithSSHConfig fills what the target leaves out from ~/.ssh/config, as
// ssh does: the real host name of an alias, the user, the port and the
// keys. What is still missing gets ssh's defaults.
func (t SSHTarget) WithSSHConfig() SSHTarget {
	cfg := readSSHConfig()
	alias := t.Host
	if h := cfg.get(alias, "HostName"); h != "" {
		t.Host = strings.ReplaceAll(h, "%h", alias)
	}
	if t.User == "" {
		t.User = cfg.get(alias, "User")
	}
	if t.User == "" {
		if u, err := user.Current(); err == nil {
			t.User = u.Username
		}
	}
	if t.Port == "" {
		t.Port = cfg.get(alias, "Port")
	}
	if t.Port == "" {
		t.Port = "22"
	}
	t.IdentityFiles = cfg.getAll(alias, "IdentityFile")
	if t.Jump == "" {
		if j := cfg.get(alias, "ProxyJump"); j != "" && !strings.EqualFold(j, "none") {
			t.Jump = strings.TrimSpace(strings.Split(j, ",")[0]) // the first hop
		}
	}
	return t
}

// ConfigUser is the user ~/.ssh/config names for host, "" for none.
func ConfigUser(host string) string { return readSSHConfig().get(host, "User") }

// sshConfig is ~/.ssh/config; a missing or broken file sets nothing.
type sshConfig struct{ cfg *sshconfig.Config }

func readSSHConfig() sshConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return sshConfig{}
	}
	data, err := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		return sshConfig{}
	}
	cfg, err := sshconfig.DecodeBytes(data)
	if err != nil {
		return sshConfig{}
	}
	return sshConfig{cfg}
}

func (c sshConfig) get(alias, key string) string {
	if c.cfg == nil {
		return ""
	}
	v, _ := c.cfg.Get(alias, key)
	return v
}

func (c sshConfig) getAll(alias, key string) []string {
	if c.cfg == nil {
		return nil
	}
	v, _ := c.cfg.GetAll(alias, key)
	return v
}

// SSHOptions configures DialSSH.
type SSHOptions struct {
	// IdentityFile is a private key to try before the default ones.
	IdentityFile string
	// Password, if set, gives the password to try after the keys, for
	// password and keyboard-interactive logins. It is asked only if needed.
	Password func() (string, error)
	// Passphrase is asked for the passphrase of a key file the server
	// accepts. Nil skips keys with a passphrase.
	Passphrase func(path string) (string, error)
	// TrustHost is asked about a host missing from known_hosts, with its key's
	// type and SHA256 fingerprint ("ecdsa-sha2-nistp256 SHA256:…"); when it
	// answers true the key is added. Nil refuses
	// unknown hosts. Hosts whose key changed are always refused.
	TrustHost func(addr, fingerprint string) bool
	// Banner sees what the server says while signing in, as Tailscale SSH
	// asks to check the user in a browser (a link) before it lets them in:
	// it waits meanwhile. Returning an error gives up. Nil ignores them;
	// what a server said still explains a refusal.
	Banner func(msg string) error
	// Certificate, when set, signs the user in with a certificate for their
	// key, which it is given and asks the server's owner to sign (an
	// InstaBox machine's gateway takes nothing else), and trusts the hosts
	// whose keys hostAuthority signed, in place of known_hosts. Both are in
	// authorized_keys form.
	Certificate func(key ssh.PublicKey) (cert, hostAuthority string, err error)
	// Dial, when set, opens the connection to the server (enowX's own
	// tailnet, say) in place of a TCP dial; jump hosts aren't used then.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

// AuthError means the server refused every way of signing in offered.
type AuthError struct {
	Addr          string
	User          string
	Allowed       []string // the methods the server allows, as it said
	TriedPassword bool
}

func (e *AuthError) Error() string {
	how := "your SSH keys"
	if e.TriedPassword {
		how += " or the password"
	}
	msg := fmt.Sprintf("transport: %s refused to sign in as %q with %s.", e.Addr, e.User, how)
	if !e.AcceptsPassword() {
		return msg + " It accepts SSH keys only: add your public key to that user's ~/.ssh/authorized_keys"
	}
	return msg + " Check the user name and password"
}

// AcceptsPassword reports whether the server lets users sign in with a
// password, as far as it said.
func (e *AuthError) AcceptsPassword() bool {
	return len(e.Allowed) == 0 || slices.Contains(e.Allowed, "password") || slices.Contains(e.Allowed, "keyboard-interactive")
}

// SSHClient is an SSH connection to a server.
type SSHClient struct {
	client *ssh.Client
	Target SSHTarget
	jump   *SSHClient // the jump host it goes through, closed with it
}

// DialSSH connects and authenticates with the SSH agent and the user's
// keys, checking the host key against ~/.ssh/known_hosts.
func DialSSH(target SSHTarget, opts SSHOptions) (*SSHClient, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	knownPath := filepath.Join(home, ".ssh", "known_hosts")
	if err := os.MkdirAll(filepath.Dir(knownPath), 0o700); err != nil {
		return nil, err
	}
	if f, err := os.OpenFile(knownPath, os.O_CREATE|os.O_RDONLY, 0o600); err == nil {
		f.Close()
	}
	known, err := knownhosts.New(knownPath)
	if err != nil {
		return nil, fmt.Errorf("transport: read known_hosts: %w", err)
	}
	addr := net.JoinHostPort(target.Host, target.Port)
	var offered ssh.PublicKey
	check := trustOnFirstUse(known, knownPath, opts.TrustHost)
	cfg := &ssh.ClientConfig{
		User: target.User,
		Auth: authMethods(home, target, opts),
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			offered = key
			return check(hostname, remote, key)
		},
		// A server has a key of each type; ask for one known_hosts has, or
		// a known server looks like one whose key changed.
		HostKeyAlgorithms: knownAlgorithms(known, addr),
		Timeout:           15 * time.Second,
	}
	if opts.Certificate != nil {
		if err := certified(cfg, home, target, opts); err != nil {
			return nil, err
		}
	}
	// What the server says while signing in explains a refusal: Tailscale
	// SSH's tailnet policy says it there, then hangs up.
	var said []string
	var gaveUp error // the Banner callback's
	var hs *handshakeTimer
	cfg.BannerCallback = func(msg string) error {
		said = append(said, strings.TrimSpace(msg))
		hs.extend()
		if opts.Banner != nil {
			gaveUp = opts.Banner(msg)
		}
		return gaveUp
	}
	// The methods the server allows say whether a password could work.
	var allowed []string
	cfg.AuthCallback = func(ctx *ssh.ClientAuthContext) (ssh.AuthMethod, error) {
		allowed = ctx.AllowedMethods
		return nil, nil
	}
	var jump *SSHClient
	if target.Jump != "" && opts.Dial == nil {
		if jump, err = dialJump(target.Jump, opts); err != nil {
			return nil, err
		}
	}
	var conn *ssh.Client
	if opts.Dial != nil {
		conn, err = dialWith(opts.Dial, addr, cfg, &hs)
	} else {
		conn, err = dialThrough(jump, addr, cfg, &hs)
	}
	if err != nil {
		if jump != nil {
			jump.Close()
		}
		var keyErr *knownhosts.KeyError
		switch {
		case errors.As(err, &keyErr) && len(keyErr.Want) > 0:
			got := ""
			if offered != nil {
				got = " (it now shows " + offered.Type() + " " + ssh.FingerprintSHA256(offered) + ")"
			}
			return nil, fmt.Errorf("transport: the host key of %s changed%s; if that is expected, remove its line from %s", addr, got, knownPath)
		case errors.As(err, &keyErr):
			return nil, fmt.Errorf("transport: %s is not a trusted host", addr)
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, &AuthError{Addr: addr, User: target.User, Allowed: allowed, TriedPassword: opts.Password != nil}
		}
		if gaveUp != nil {
			return nil, fmt.Errorf("transport: %s: %w", addr, gaveUp)
		}
		if len(said) > 0 {
			return nil, fmt.Errorf("transport: %s, signing in as %q, said: %s", addr, target.User, strings.Join(said, " "))
		}
		return nil, fmt.Errorf("transport: ssh %s: %w", addr, err)
	}
	c := &SSHClient{client: conn, Target: target, jump: jump}
	go c.keepAlive(keepAliveEvery, keepAliveMissed)
	return c, nil
}

// certified makes cfg sign in with a certificate for the user's first key,
// opts.Certificate's, and trust the hosts its authority signed.
func certified(cfg *ssh.ClientConfig, home string, target SSHTarget, opts SSHOptions) error {
	signers := keySigners(home, target, opts)
	if len(signers) == 0 {
		return errors.New("transport: no SSH key to sign in with: make one with ssh-keygen -t ed25519")
	}
	certText, authText, err := opts.Certificate(signers[0].PublicKey())
	if err != nil {
		return fmt.Errorf("transport: a certificate for %s: %w", target, err)
	}
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(certText))
	cert, ok := pk.(*ssh.Certificate)
	if err != nil || !ok {
		return fmt.Errorf("transport: the certificate for %s doesn't read", target)
	}
	signer, err := ssh.NewCertSigner(cert, signers[0])
	if err != nil {
		return fmt.Errorf("transport: the certificate for %s: %w", target, err)
	}
	authority, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authText))
	if err != nil {
		return fmt.Errorf("transport: the host authority of %s doesn't read", target)
	}
	checker := &ssh.CertChecker{IsHostAuthority: func(auth ssh.PublicKey, _ string) bool {
		return bytes.Equal(auth.Marshal(), authority.Marshal())
	}}
	cfg.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	cfg.HostKeyCallback = checker.CheckHostKey
	cfg.HostKeyAlgorithms = nil
	return nil
}

// dialJump connects to the jump host named "[user@]host[:port]", with the
// same ways of signing in, and no jump of its own.
func dialJump(name string, opts SSHOptions) (*SSHClient, error) {
	jt, err := ParseSSHTarget(name)
	if err != nil {
		return nil, fmt.Errorf("transport: jump host: %w", err)
	}
	jt = jt.WithSSHConfig()
	jt.Jump = ""
	c, err := DialSSH(jt, opts)
	if err != nil {
		return nil, fmt.Errorf("transport: jump host %s: %w", jt, err)
	}
	return c, nil
}

// dialWith opens an SSH connection to addr over a connection dial makes.
func dialWith(dial func(ctx context.Context, network, addr string) (net.Conn, error), addr string, cfg *ssh.ClientConfig, hs **handshakeTimer) (*ssh.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	nc, err := dial(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return clientOver(nc, addr, cfg, hs)
}

// dialThrough opens an SSH connection to addr, through jump when set.
func dialThrough(jump *SSHClient, addr string, cfg *ssh.ClientConfig, hs **handshakeTimer) (*ssh.Client, error) {
	var nc net.Conn
	var err error
	if jump == nil {
		nc, err = net.DialTimeout("tcp", addr, cfg.Timeout)
	} else {
		nc, err = jump.client.Dial("tcp", addr)
	}
	if err != nil {
		return nil, err
	}
	return clientOver(nc, addr, cfg, hs)
}

// knownAlgorithms returns the host key algorithms of the keys known_hosts
// has for addr, nil for an unknown host.
func knownAlgorithms(known ssh.HostKeyCallback, addr string) []string {
	var keyErr *knownhosts.KeyError
	if err := known(addr, &net.TCPAddr{}, placeholderKey{}); !errors.As(err, &keyErr) {
		return nil
	}
	var algos []string
	for _, k := range keyErr.Want {
		switch t := k.Key.Type(); t {
		case ssh.KeyAlgoRSA:
			algos = append(algos, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA)
		default:
			algos = append(algos, t)
		}
	}
	slices.Sort(algos)
	return slices.Compact(algos)
}

// placeholderKey matches no known key, so known_hosts lists the real ones.
type placeholderKey struct{}

func (placeholderKey) Type() string                        { return "enowx-placeholder" }
func (placeholderKey) Marshal() []byte                     { return []byte("enowx-placeholder") }
func (placeholderKey) Verify([]byte, *ssh.Signature) error { return errors.New("placeholder key") }

// trustOnFirstUse checks known_hosts and asks trust about unknown hosts.
func trustOnFirstUse(known ssh.HostKeyCallback, path string, trust func(addr, fingerprint string) bool) ssh.HostKeyCallback {
	var mu sync.Mutex
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := known(hostname, remote, key)
		var keyErr *knownhosts.KeyError
		if err == nil || !errors.As(err, &keyErr) || len(keyErr.Want) > 0 || trust == nil {
			return err
		}
		if !trust(hostname, key.Type()+" "+ssh.FingerprintSHA256(key)) {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		f, ferr := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
		if ferr != nil {
			return ferr
		}
		defer f.Close()
		_, ferr = fmt.Fprintln(f, knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key))
		return ferr
	}
}

// Run runs command on the server with stdin and returns its output.
func (c *SSHClient) Run(command string, stdin io.Reader) (string, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	var out, errOut bytes.Buffer
	sess.Stdin, sess.Stdout, sess.Stderr = stdin, &out, &errOut
	if err := sess.Run(command); err != nil {
		return out.String(), commandError(err, errOut.String(), out.String())
	}
	return out.String(), nil
}

// commandError is err with what the command said: its stderr, else its stdout, as a
// nologin shell says why on stdout.
func commandError(err error, stderr, stdout string) error {
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		msg = strings.TrimSpace(stdout)
	}
	if msg == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, msg)
}

// AsUser returns command run through `su` as user, so a root login can run
// it as a regular user. An empty user returns command unchanged.
func AsUser(user, command string) string {
	if user == "" {
		return command
	}
	return "su - " + user + " -c " + shellQuote(command)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Bridge runs `enowxd bridge` on the server, as user when set (see AsUser),
// and returns its stdin/stdout as one stream to the daemon. Closing the
// stream closes the SSH connection.
func (c *SSHClient) Bridge(user string) (io.ReadWriteCloser, error) {
	return c.bridge(user, c)
}

// BridgeSession is Bridge for a stream that closes only itself, keeping
// the SSH connection for what follows.
func (c *SSHClient) BridgeSession(user string) (io.ReadWriteCloser, error) {
	return c.bridge(user, nil)
}

func (c *SSHClient) bridge(user string, owner io.Closer) (io.ReadWriteCloser, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return nil, err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := sess.Start(AsUser(user, `"$HOME/`+RemoteDaemonPath+`" bridge`)); err != nil {
		return nil, fmt.Errorf("transport: start bridge on %s: %w", c.Target, err)
	}
	return &sshStream{Reader: stdout, Writer: stdin, sess: sess, conn: owner}, nil
}

// Close closes the connection, and the jump host's under it.
func (c *SSHClient) Close() error {
	err := c.client.Close()
	if c.jump != nil {
		_ = c.jump.Close()
	}
	return err
}

// DialTCP connects to addr as the server sees it, through the SSH
// connection: "127.0.0.1:3000" is a port on the server.
func (c *SSHClient) DialTCP(addr string) (net.Conn, error) {
	conn, err := c.client.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("transport: reach %s on %s: %w", addr, c.Target.Host, err)
	}
	return conn, nil
}

// authMethods offers the user's keys, then the password. Go's client tries
// the publickey method once, so every key goes into that one method: the
// keys named for the host, the agent's, then the default files.
func authMethods(home string, target SSHTarget, opts SSHOptions) []ssh.AuthMethod {
	methods := []ssh.AuthMethod{ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
		return keySigners(home, target, opts), nil
	})}
	if password := opts.Password; password != nil {
		methods = append(methods,
			ssh.PasswordCallback(password),
			// Servers that ask for the password as a challenge get it for
			// every question, which is what OpenSSH does with one prompt.
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				pw, err := password()
				if err != nil {
					return nil, err
				}
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = pw
				}
				return answers, nil
			}),
		)
	}
	return methods
}

// agentWait is how long the SSH agent gets to list its keys.
const agentWait = 3 * time.Second

func keySigners(home string, target SSHTarget, opts SSHOptions) []ssh.Signer {
	expand := func(f string) string {
		if strings.HasPrefix(f, "~/") {
			return filepath.Join(home, f[2:])
		}
		return f
	}
	var named []string
	if opts.IdentityFile != "" {
		named = append(named, expand(opts.IdentityFile))
	}
	for _, f := range target.IdentityFiles {
		named = append(named, expand(f))
	}
	var defaults []string
	for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		defaults = append(defaults, filepath.Join(home, ".ssh", name))
	}

	var signers []ssh.Signer
	seen := map[string]bool{}
	add := func(s ssh.Signer) {
		if k := string(s.PublicKey().Marshal()); !seen[k] {
			seen[k] = true
			signers = append(signers, s)
		}
	}
	addFiles := func(files []string) {
		for _, f := range files {
			if s := fileSigner(f, opts.Passphrase); s != nil {
				add(s)
			}
		}
	}
	addFiles(named)
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if c, err := net.DialTimeout("unix", sock, agentWait); err == nil {
			// An agent that hangs (macOS's does at times) would hold the
			// connection at "connecting" for good: past agentWait, its
			// keys are left out.
			c.SetDeadline(time.Now().Add(agentWait))
			if list, err := agent.NewClient(c).Signers(); err == nil {
				for _, s := range list {
					add(s)
				}
				c.SetDeadline(time.Time{}) // its keys sign as the server asks
			}
		}
	}
	addFiles(defaults)
	return signers
}

// fileSigner reads a private key. A key with a passphrase asks for it only
// when the server accepts the key, so keys the server never wanted don't
// prompt.
func fileSigner(path string, passphrase func(path string) (string, error)) ssh.Signer {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	s, err := ssh.ParsePrivateKey(pem)
	if err == nil {
		return s
	}
	var missing *ssh.PassphraseMissingError
	if !errors.As(err, &missing) || passphrase == nil {
		return nil
	}
	pub := missing.PublicKey
	if pub == nil {
		// Older key formats hide the public key; the .pub file has it.
		data, err := os.ReadFile(path + ".pub")
		if err != nil {
			return nil
		}
		if pub, _, _, _, err = ssh.ParseAuthorizedKey(data); err != nil {
			return nil
		}
	}
	return &lockedSigner{pub: pub, path: path, pem: pem, passphrase: passphrase}
}

// lockedSigner is a key with a passphrase, unlocked on first use.
type lockedSigner struct {
	pub        ssh.PublicKey
	path       string
	pem        []byte
	passphrase func(path string) (string, error)

	once     sync.Once
	unlocked ssh.AlgorithmSigner
	err      error
}

func (l *lockedSigner) PublicKey() ssh.PublicKey { return l.pub }

func (l *lockedSigner) Sign(r io.Reader, data []byte) (*ssh.Signature, error) {
	return l.SignWithAlgorithm(r, data, "")
}

func (l *lockedSigner) SignWithAlgorithm(r io.Reader, data []byte, algorithm string) (*ssh.Signature, error) {
	l.once.Do(func() {
		pass, err := l.passphrase(l.path)
		if err != nil {
			l.err = err
			return
		}
		s, err := ssh.ParsePrivateKeyWithPassphrase(l.pem, []byte(pass))
		if err != nil {
			l.err = fmt.Errorf("transport: unlock %s: %w", l.path, err)
			return
		}
		as, ok := s.(ssh.AlgorithmSigner)
		if !ok {
			l.err = fmt.Errorf("transport: %s: unsupported key", l.path)
			return
		}
		l.unlocked = as
	})
	if l.err != nil {
		return nil, l.err
	}
	return l.unlocked.SignWithAlgorithm(r, data, algorithm)
}

type sshStream struct {
	io.Reader
	io.Writer
	sess *ssh.Session
	conn io.Closer // closed with the stream, when set
}

func (s *sshStream) Close() error {
	err := s.sess.Close()
	if s.conn != nil {
		err = s.conn.Close()
	}
	return err
}

// SSHConfigHost is a host ~/.ssh/config names, as the user would type it.
type SSHConfigHost struct {
	Alias string    // what to connect to: ssh resolves it with the file
	Real  SSHTarget // where it goes, the file applied
}

// SSHConfigHosts lists the hosts ~/.ssh/config names, by alias; patterns
// (*, ?, !) aren't hosts.
func SSHConfigHosts() []SSHConfigHost {
	cfg := readSSHConfig()
	if cfg.cfg == nil {
		return nil
	}
	var out []SSHConfigHost
	seen := map[string]bool{}
	for _, h := range cfg.cfg.Hosts {
		for _, p := range h.Patterns {
			alias := p.String()
			if alias == "" || strings.ContainsAny(alias, "*?!") || seen[alias] {
				continue
			}
			seen[alias] = true
			out = append(out, SSHConfigHost{Alias: alias, Real: SSHTarget{Host: alias}.WithSSHConfig()})
		}
	}
	return out
}

// SSH keepalives: an idle connection would be dropped unseen by a router
// or NAT on the way, and a dead one (the laptop slept, the network
// changed) noticed only at the next keystroke. One goes every
// keepAliveEvery; when keepAliveMissed go unanswered in a row the
// connection closes, so the app notices and connects again.
var (
	keepAliveEvery  = 15 * time.Second
	keepAliveMissed = 3
)

func (c *SSHClient) keepAlive(every time.Duration, missed int) {
	t := time.NewTicker(every)
	defer t.Stop()
	failed := 0
	for range t.C {
		answered := make(chan error, 1)
		go func() {
			_, _, err := c.client.SendRequest("keepalive@openssh.com", true, nil)
			answered <- err
		}()
		select {
		case err := <-answered:
			if err != nil && errors.Is(err, io.EOF) {
				return // closed
			}
			if err == nil {
				failed = 0
				continue
			}
		case <-time.After(every):
		}
		if failed++; failed >= missed {
			_ = c.Close()
			return
		}
	}
}

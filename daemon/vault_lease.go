package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
)

// A session's credentials: the app lends their values when the session
// starts (those asked for, and those it puts in every session), the daemon
// keeps them for it in memory, its files in a folder only the user reads,
// and forgets them with it. Git, docker and ssh in it get the vault's
// logins through enowX's helpers, which ask the app at each use.

// vaultSessionEnv is the environment new session id (of project; a tab of
// parent) gets from the vault: the credentials of names, those the app
// puts in every session, and its helpers.
func (s *Server) vaultSessionEnv(id, project, parent string, names []string) ([]string, []string) {
	idx := s.vaultIndex()
	if !idx.Present {
		return nil, nil
	}
	l := &vaultLease{env: map[string]string{}}
	if p := s.vaultLease(parent); p != nil {
		// A tab has its session's.
		l.names, l.secrets = slices.Clone(p.names), slices.Clone(p.secrets)
		for k, v := range p.env {
			l.env[k] = v
		}
	} else if len(names) > 0 || slices.ContainsFunc(idx.Credentials, func(c proto.CredentialInfo) bool { return c.Auto }) {
		if s.vault.served() {
			a := proto.VaultAsk{Action: proto.VaultSession, Session: id, Names: names}
			req := proto.VaultRequest{VaultAsk: a, Project: project}
			req.Hostname, _ = os.Hostname()
			lend, err := s.vaultRequest(req)
			s.vaultAudit(a, lend, err)
			if err != nil {
				s.logger().Warn("vault: a session starts without its credentials", "id", id, "err", err)
			} else {
				s.vaultTake(id, l, lend)
			}
		}
	}
	s.vaultHelpers(id, l, idx)
	s.vault.mu.Lock()
	if s.vault.leases == nil {
		s.vault.leases = map[string]*vaultLease{}
	}
	if old := s.vault.leases[id]; old != nil {
		old.stop()
	}
	s.vault.leases[id] = l
	s.vault.mu.Unlock()
	return l.environ(), l.names
}

// vaultTake keeps what the app lent session id: its variables, its files
// written to the session's folder of them.
func (s *Server) vaultTake(id string, l *vaultLease, lend proto.VaultLend) {
	for k, v := range lend.Env {
		l.env[k] = v
		l.secrets = append(l.secrets, v)
	}
	if len(lend.Files) > 0 {
		dir := filepath.Join(s.leaseDir(id, l), "files")
		if err := os.MkdirAll(dir, 0o700); err == nil {
			for k, content := range lend.Files {
				path := filepath.Join(dir, k)
				if os.WriteFile(path, []byte(content), 0o600) == nil {
					l.env[k] = path
					l.secrets = append(l.secrets, content)
				}
			}
		}
	}
	for _, n := range lend.Credentials {
		if !slices.Contains(l.names, n) {
			l.names = append(l.names, n)
		}
	}
}

// leaseDir is session id's folder of files and helpers, made on first use.
func (s *Server) leaseDir(id string, l *vaultLease) string {
	if l.dir == "" {
		l.dir = filepath.Join(s.vaultRunDir(), id)
		_ = os.MkdirAll(l.dir, 0o700)
	}
	return l.dir
}

// vaultHelpers points git, docker and ssh of session id at the vault's
// helpers, for the kinds of logins the vault holds.
func (s *Server) vaultHelpers(id string, l *vaultLease, idx proto.VaultIndex) {
	if s.HookBin == "" {
		return
	}
	var registries, keys []string
	git := false
	for _, c := range idx.Credentials {
		git = git || len(c.GitHosts) > 0
		registries = append(registries, c.Registries...)
		if c.PublicKey != "" {
			keys = append(keys, c.Name)
		}
	}
	h := map[string]string{}
	if git {
		for _, kv := range s.vaultGitEnv() {
			k, v, _ := strings.Cut(kv, "=")
			h[k] = v
		}
	}
	if len(registries) > 0 {
		dir := filepath.Join(s.leaseDir(id, l), "docker")
		bin := filepath.Join(s.leaseDir(id, l), "bin")
		if dockerConfig(dir, registries) == nil && os.MkdirAll(bin, 0o700) == nil {
			link := filepath.Join(bin, "docker-credential-enowx"+exeSuffix)
			_ = os.Remove(link)
			if os.Symlink(s.HookBin, link) == nil {
				h["DOCKER_CONFIG"] = dir
				h["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
			}
		}
	}
	if len(keys) > 0 {
		if sock, stop, err := s.startSSHAgent(id); err == nil {
			h["SSH_AUTH_SOCK"], l.agent = sock, stop
		} else {
			s.logger().Warn("vault: no SSH agent for a session", "id", id, "err", err)
		}
	}
	for k, v := range h {
		if _, lent := l.env[k]; !lent {
			l.env[k] = v
		}
	}
}

// dockerConfig writes docker's settings for a session: the user's, with
// the vault's helper for its registries.
func dockerConfig(dir string, registries []string) error {
	conf := map[string]any{}
	if home, err := os.UserHomeDir(); err == nil {
		if data, err := os.ReadFile(filepath.Join(home, ".docker", "config.json")); err == nil {
			_ = json.Unmarshal(data, &conf)
		}
	}
	helpers, _ := conf["credHelpers"].(map[string]any)
	if helpers == nil {
		helpers = map[string]any{}
	}
	for _, r := range registries {
		helpers[r] = "enowx"
	}
	conf["credHelpers"] = helpers
	data, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600)
}

// environ is the lease's variables as NAME=value entries.
func (l *vaultLease) environ() []string {
	out := make([]string, 0, len(l.env))
	for k, v := range l.env {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out
}

// stop ends the lease's agent and removes its files.
func (l *vaultLease) stop() {
	if l.agent != nil {
		l.agent()
	}
	if l.dir != "" {
		_ = os.RemoveAll(l.dir)
	}
}

func (s *Server) vaultLease(id string) *vaultLease {
	if id == "" {
		return nil
	}
	s.vault.mu.Lock()
	defer s.vault.mu.Unlock()
	return s.vault.leases[id]
}

// vaultEnvFor is the environment session id's program starts with again:
// its lease's, or (after the daemon restarted) its credentials lent again
// while an app answers.
func (s *Server) vaultEnvFor(id, project string, names []string) []string {
	if l := s.vaultLease(id); l != nil {
		return l.environ()
	}
	env, _ := s.vaultSessionEnv(id, project, "", names)
	return env
}

// vaultAttach lends credentials names to session id for the programs it
// starts from now on, and records them with it.
func (s *Server) vaultAttach(p proto.VaultAttach) ([]string, error) {
	sess, err := s.sessions.Get(p.Session)
	if err != nil {
		return nil, err
	}
	l := s.vaultLease(p.Session)
	if l == nil {
		s.vaultSessionEnv(p.Session, sess.Meta().Project, "", nil)
		if l = s.vaultLease(p.Session); l == nil {
			return nil, errors.New("vault: this host has no vault yet: open Credentials in the app")
		}
	}
	for _, name := range p.Names {
		lend, err := s.vaultAsk(proto.VaultAsk{Action: proto.VaultEnv, Name: name, Session: p.Session, Command: "creds env"})
		if err != nil {
			return nil, err
		}
		s.vault.mu.Lock()
		s.vaultTake(p.Session, l, lend)
		s.vault.mu.Unlock()
	}
	s.vault.mu.Lock()
	names := slices.Clone(l.names)
	s.vault.mu.Unlock()
	s.sessions.SetCreds(sess, names)
	return names, nil
}

// vaultRelease forgets what session id was lent.
func (s *Server) vaultRelease(id string) {
	s.vault.mu.Lock()
	l := s.vault.leases[id]
	delete(s.vault.leases, id)
	s.vault.mu.Unlock()
	if l != nil {
		l.stop()
	}
}

// vaultReleaseAll forgets every lease, as the daemon stops.
func (s *Server) vaultReleaseAll() {
	s.vault.mu.Lock()
	leases := s.vault.leases
	s.vault.leases = nil
	s.vault.mu.Unlock()
	for _, l := range leases {
		l.stop()
	}
	_ = os.RemoveAll(s.vaultRunDir())
}

// lentSecrets is every value lent to a session now, to mask wherever it
// would show (Traffic).
func (s *Server) lentSecrets() []string {
	s.vault.mu.Lock()
	defer s.vault.mu.Unlock()
	var out []string
	for _, l := range s.vault.leases {
		out = append(out, l.secrets...)
	}
	return append(out, s.cfgKeys.all()...) // configs' keys (configs.go)
}

// maskLent masks in text every value lent to a session now: what another
// session's agent or the chat view reads of a session doesn't hand it a
// credential the session printed.
func (s *Server) maskLent(text string) string {
	secrets := s.lentSecrets()
	if len(secrets) == 0 {
		return text
	}
	return vault.Redact(text, secrets)
}

// maskLentIn is v with what maskLent masks masked in its JSON.
func maskLentIn[T any](s *Server, v T) T {
	secrets := s.lentSecrets()
	if len(secrets) == 0 {
		return v
	}
	data, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out T
	if json.Unmarshal([]byte(vault.Redact(string(data), secrets)), &out) != nil {
		return v
	}
	return out
}

// vaultGitEnv has git ask the vault for logins after its own helpers
// (gh's, the system's), through the app: a session's git, and the auto
// push's. Nil without enowX's command to ask with.
func (s *Server) vaultGitEnv() []string {
	if s.HookBin == "" {
		return nil
	}
	return []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=!'" + strings.ReplaceAll(s.HookBin, "'", `'\''`) + "' creds git-helper"}
}

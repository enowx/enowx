package daemon

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/enowx/enowx/proto"
)

// Keys typed for one host (a judge's, RAG's embedder's, a PostgreSQL URL)
// moved to the vault, which alone shares them: the settings then name the
// credential, as they already can, so every host, old versions too, reads it
// from the vault, each asking the app the first time. The app asks the user
// to keep each; a key goes only once the vault has it and lends it back the
// same. A value never leaves through this but to the app's vault, and is in
// no answer, error or log.

// keepInVault adds value as a credential named from base, says which, and
// reuses one a try that stopped short left. what is shown to the user.
func (s *Server) keepInVault(base, platform, field, value, what string) (string, error) {
	lendsIt := func(name string) error {
		got, err := s.vaultKey(what)(name)
		if err != nil {
			return err
		}
		if got != value {
			return errors.New("the vault's value isn't the one typed")
		}
		return nil
	}
	creds := s.vaultIndex().Credentials
	for n := 1; n < 100; n++ {
		name := base
		if n > 1 {
			name += "-" + strconv.Itoa(n)
		}
		if slices.ContainsFunc(creds, func(c proto.CredentialInfo) bool { return c.Name == name }) {
			if lendsIt(name) == nil {
				return name, nil
			}
			continue
		}
		if _, err := s.vaultAsk(proto.VaultAsk{Action: proto.VaultAdd, Name: name, Platform: platform,
			Fields: map[string]string{field: value}, Command: what}); err != nil {
			return "", err
		}
		return name, lendsIt(name)
	}
	return "", errors.New("no free credential name")
}

// credSlug is label as part of a credential's name.
func credSlug(label string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(label) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// keysToVault moves every key typed on this host to the vault. What fails
// stays as it was, and is said; the others go on.
func (s *Server) keysToVault() proto.HostKeysMoved {
	out := proto.HostKeysMoved{Moved: []proto.HostKeyMoved{}, Failed: []proto.HostKeyFailed{}}
	move := func(what, base, platform, field, value string, use func(cred string) error) {
		cred, err := s.keepInVault(base, platform, field, value, what+", kept in the vault for every host")
		if err == nil {
			err = use(cred)
		}
		if err != nil {
			s.logger().Warn("a key not moved to the vault", "what", what, "err", err)
			out.Failed = append(out.Failed, proto.HostKeyFailed{What: what, Err: err.Error()})
			return
		}
		s.logger().Info("a key moved to the vault", "what", what, "credential", cred)
		out.Moved = append(out.Moved, proto.HostKeyMoved{What: what, Credential: cred})
	}
	if key := s.rag.TypedKey(proto.RAGKeyEmbedder); key != "" {
		move("RAG's embedder key", "enowx-rag-embedder", "api-key", "value", key, func(cred string) error {
			st := s.ragSettings.Get()
			st.KeyCred = cred
			if err := s.ragSettings.Set(st); err != nil {
				return err
			}
			return s.rag.SetKey(proto.RAGKeyEmbedder, "")
		})
	}
	if url := s.rag.TypedKey(proto.RAGKeyPostgres); url != "" {
		move("RAG's PostgreSQL URL", "enowx-rag-postgres", "postgres", "url", url, func(cred string) error {
			st := s.ragSettings.Get()
			st.PostgresCred = cred
			if err := s.ragSettings.Set(st); err != nil {
				return err
			}
			return s.rag.SetKey(proto.RAGKeyPostgres, "")
		})
	}
	for _, k := range s.judge.TypedKeys() {
		move(fmt.Sprintf("the key of the judge %s", k.Label), "enowx-judge-"+credSlug(k.Label), "api-key", "value", k.Key, func(cred string) error {
			return s.judge.MoveKeyToCred(k.ID, cred)
		})
	}
	return out
}

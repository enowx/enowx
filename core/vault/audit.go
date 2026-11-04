package vault

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/enowx/enowx/proto"
)

// auditKeep is how many uses an audit log keeps.
const auditKeep = 2000

// Audit is a host's record of the credentials used there: which, when,
// for which session, allowed or not. It holds no values. Safe for
// concurrent use.
type Audit struct {
	mu   sync.Mutex
	path string
}

// OpenAudit keeps the record in the file at path (JSON, a line each).
func OpenAudit(path string) *Audit { return &Audit{path: path} }

// Locker is held while the record's file is written; what replaces the file holds it too,
// or a use recorded meanwhile would go with the old file.
func (a *Audit) Locker() sync.Locker { return &a.mu }

// Add records a use; an older part goes when the record grows long.
func (a *Audit) Add(e proto.VaultAuditEntry) error {
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(a.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if st, serr := os.Stat(a.path); serr == nil && st.Size() > auditKeep*400 {
		all := a.read(0)
		var b bytes.Buffer
		for _, e := range all[max(len(all)-auditKeep/2, 0):] {
			l, _ := json.Marshal(e)
			b.Write(append(l, '\n'))
		}
		_ = os.WriteFile(a.path, b.Bytes(), 0o600)
	}
	return err
}

// List returns the last limit uses (0: all), oldest first.
func (a *Audit) List(limit int) []proto.VaultAuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.read(limit)
}

func (a *Audit) read(limit int) []proto.VaultAuditEntry {
	out := []proto.VaultAuditEntry{}
	f, err := os.Open(a.path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e proto.VaultAuditEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

package agentapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/enowx/enowx/core/envsync"
)

// Unset takes the API enowX wrote into agent id's files back out, once a
// config holds it: the files as the user had them (the original backup)
// when nothing but enowX's API changed them since, byte for byte; else
// only enowX's entries removed, the user's later changes kept. Either
// way the files as they are now are backed up first. restored says
// which.
func Unset(home, id string) (restored bool, err error) {
	a, ok := adapters[id]
	if !ok {
		return false, fmt.Errorf("agentapi: %s takes no API of the user's", id)
	}
	backups, err := Backups(home, id)
	if err != nil {
		return false, err
	}
	var original *Backup
	for i := range backups {
		if backups[i].Original {
			original = &backups[i]
		}
	}
	if original != nil {
		same, err := clearedIsOriginal(home, id, a, *original)
		if err != nil {
			return false, err
		}
		if same {
			return true, Restore(home, id, original.ID)
		}
	}
	return false, Clear(home, id)
}

// clearedIsOriginal says the agent's files, enowX's API taken out of a
// copy of them, are what the original backup kept: no other change.
func clearedIsOriginal(home, id string, a adapter, b Backup) (bool, error) {
	tmp, err := os.MkdirTemp("", "enowx-agentapi-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)
	for _, rel := range a.files {
		data, err := readFile(home, rel)
		if err != nil {
			return false, err
		}
		if _, statErr := os.Stat(filepath.Join(home, rel)); statErr == nil {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(tmp, rel)), 0o700); err != nil {
				return false, err
			}
			if err := os.WriteFile(filepath.Join(tmp, rel), data, 0o600); err != nil {
				return false, err
			}
		}
	}
	if err := a.clear(tmp); err != nil {
		return false, err
	}
	m, err := readManifest(home, id, b.ID)
	if err != nil {
		return false, err
	}
	dir := filepath.Join(backupsDir(home, id), b.ID)
	for _, rel := range a.files {
		now, nowErr := os.ReadFile(filepath.Join(tmp, rel))
		var was []byte
		wasThere := false
		for i, e := range m.Entries {
			if e.Path == rel && !e.Absent {
				wasThere = true
				if was, err = os.ReadFile(filepath.Join(dir, fmt.Sprint(i))); err != nil {
					return false, err
				}
			}
		}
		if errors.Is(nowErr, fs.ErrNotExist) {
			now = nil
		} else if nowErr != nil {
			return false, nowErr
		}
		if !sameContent(now, was, wasThere) {
			return false, nil
		}
	}
	return true, nil
}

// sameContent says a file's content now is what was kept: the same bytes,
// the same JSON, or empty where there was no file.
func sameContent(now, was []byte, wasThere bool) bool {
	if !wasThere {
		var x any
		if len(bytes.TrimSpace(now)) == 0 {
			return true
		}
		if parseJSONC(now, &x) != nil {
			return false
		}
		m, ok := pruned(x).(map[string]any)
		return ok && len(m) == 0
	}
	if bytes.Equal(bytes.TrimSpace(now), bytes.TrimSpace(was)) {
		return true
	}
	var x, y any
	if parseJSONC(now, &x) != nil || parseJSONC(was, &y) != nil {
		return false
	}
	a, _ := json.Marshal(pruned(x))
	b, _ := json.Marshal(pruned(y))
	return bytes.Equal(a, b)
}

// parseJSONC reads JSON with comments, as agents' settings allow.
func parseJSONC(data []byte, v any) error {
	plain, _, err := envsync.EditJSON(data, []string{"\x00"}, nil) // removes nothing: drops the comments
	if err != nil {
		return err
	}
	return json.Unmarshal(plain, v)
}

// pruned is v without the objects left empty, as taking enowX's keys out
// leaves the objects that held them.
func pruned(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	for k, x := range m {
		x = pruned(x)
		if xm, ok := x.(map[string]any); ok && len(xm) == 0 {
			delete(m, k)
			continue
		}
		m[k] = x
	}
	return m
}

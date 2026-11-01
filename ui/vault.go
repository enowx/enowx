package ui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/vaultview"
	"github.com/enowx/enowx/ui/widget"
)

// The credential vault (docs/vault.md) is this device's: the app keeps it
// in <enowX dir>/vault, sealed, and its key in memory while unlocked (and
// in the keychain when the user lets it). Hosts get a copy of the
// ciphertext and its index, and ask the app for a credential when a
// session needs one (vault_requests.go).

// pageVault is the rail's Credentials page.
const pageVault = "vault"

// vaultLockAfter is how long the vault stays unlocked unused.
const vaultLockAfter = 15 * time.Minute

// vaultKeychainKey names the master key in the system's keychain.
const vaultKeychainKey = "vault-key"

// keychain is the system's credential store; tests replace it.
var keychain = struct {
	get func(string) ([]byte, error)
	set func(string, []byte) error
	del func(string) error
}{mygo.Secrets.Get, mygo.Secrets.Set, mygo.Secrets.Delete}

// vaultApp is the device's vault, owned by the main thread.
type vaultApp struct {
	dir     string // its folder; empty for <enowX dir>/vault
	loaded  bool
	file    *vault.File
	key     []byte
	data    *vault.Vault
	used    time.Time
	view    vaultview.View
	testing map[string]bool
	found   []proto.VaultFound
	scan    bool
	audit   []proto.VaultAuditEntry
	asks    []*vaultRequestAsk // questions waiting for the user, the first shown
	warned  map[string]string  // credential ID → the day its expiry was told
	keyRead bool               // the keychain was tried this run
}

// vaultOf is the vault, read from disk the first time.
func (a *App) vaultOf() *vaultApp {
	if a.vlt == nil {
		a.vlt = &vaultApp{testing: map[string]bool{}}
	}
	v := a.vlt
	if !v.loaded {
		v.loaded = true
		if data, err := os.ReadFile(v.path("vault.json")); err == nil {
			if f, err := vault.Parse(data); err == nil {
				v.file = f
			}
		}
	}
	return v
}

func (v *vaultApp) path(name string) string {
	dir := v.dir
	if dir == "" {
		home, err := transport.Dir()
		if err != nil {
			home = filepath.Join(".", transport.HomeName)
		}
		dir = filepath.Join(home, "vault")
	}
	return filepath.Join(dir, name)
}

func (v *vaultApp) state() string {
	switch {
	case v.file == nil:
		return vaultview.StateNone
	case v.key == nil:
		return vaultview.StateLocked
	}
	return vaultview.StateOpen
}

// copyOf is what hosts keep of the vault: the file and its index.
func (v *vaultApp) copyOf() (proto.VaultCopy, bool) {
	if v.file == nil {
		return proto.VaultCopy{}, false
	}
	out := proto.VaultCopy{Data: v.file.Bytes()}
	if v.data != nil {
		out.Index = v.data.Index(v.file.Updated)
	} else if raw, err := os.ReadFile(v.path("index.json")); err == nil {
		_ = json.Unmarshal(raw, &out.Index)
	}
	out.Index.Present = true
	return out, true
}

// vaultCreate makes the vault with a master password.
func (a *App) vaultCreate(password string, remember bool) error {
	v := a.vaultOf()
	f, key, err := vault.Create(password)
	if err != nil {
		return err
	}
	v.file, v.key, v.data = f, key, &vault.Vault{}
	a.vaultRemember(remember)
	return a.vaultSave()
}

// vaultUnlock opens the vault with its master password.
func (a *App) vaultUnlock(password string, remember bool) error {
	v := a.vaultOf()
	if v.file == nil {
		return errors.New("no vault yet")
	}
	key, err := v.file.Unlock(password)
	if err != nil {
		return err
	}
	if err := a.vaultOpen(key); err != nil {
		return err
	}
	a.vaultRemember(remember)
	return nil
}

// vaultOpen decrypts the vault with key.
func (a *App) vaultOpen(key []byte) error {
	v := a.vaultOf()
	data, err := v.file.Open(key)
	if err != nil {
		return err
	}
	v.key, v.data, v.used = key, data, time.Now()
	a.vaultReminders()
	a.vaultAnswerWaiting()
	return nil
}

// vaultRemember keeps the key in the keychain, or takes it out.
func (a *App) vaultRemember(on bool) {
	v := a.vaultOf()
	a.state.VaultRemember = on
	key := v.key
	go func() {
		if on && key != nil {
			_ = keychain.set(vaultKeychainKey, []byte(base64.StdEncoding.EncodeToString(key)))
		} else if !on {
			_ = keychain.del(vaultKeychainKey)
		}
	}()
}

// vaultFromKeychain unlocks the vault with the key the keychain keeps,
// once a run, off the main thread: the system may ask the user. then runs
// on the main thread after.
func (a *App) vaultFromKeychain(then func()) {
	v := a.vaultOf()
	if v.key != nil || v.file == nil || !a.state.VaultRemember || v.keyRead {
		then()
		return
	}
	v.keyRead = true
	go func() {
		raw, err := keychain.get(vaultKeychainKey)
		a.onMain(func() {
			if err == nil && v.key == nil {
				if key, derr := base64.StdEncoding.DecodeString(string(raw)); derr == nil {
					_ = a.vaultOpen(key)
				}
			}
			then()
		})
	}()
}

// vaultLock forgets the key; the keychain's stays, if any.
func (a *App) vaultLock() {
	v := a.vaultOf()
	clear(v.key)
	v.key, v.data, v.keyRead = nil, nil, false
}

// vaultTick locks the vault left unused, and tells of expiring
// credentials; the app calls it every minute.
func (a *App) vaultTick() {
	v := a.vlt
	if v == nil || v.key == nil {
		return
	}
	if time.Since(v.used) > vaultLockAfter && len(v.asks) == 0 && a.page != pageVault {
		a.vaultLock()
		return
	}
	a.vaultReminders()
}

// vaultReminders tells of credentials expiring within a week, once a day
// each.
func (a *App) vaultReminders() {
	v := a.vlt
	if v == nil || v.data == nil {
		return
	}
	if v.warned == nil {
		v.warned = map[string]string{}
	}
	now := time.Now()
	day := now.Format(time.DateOnly)
	for _, c := range v.data.Credentials {
		if c.ExpiresSoon(now, 7*24*time.Hour) && v.warned[c.ID] != day {
			v.warned[c.ID] = day
			a.notify(c.Name + " expires on " + c.Expires.Local().Format("2 Jan") + ": rotate it in Credentials")
		}
	}
}

// vaultSave seals the vault, writes it here and gives the hosts connected
// their copy.
func (a *App) vaultSave() error {
	v := a.vaultOf()
	if v.key == nil || v.data == nil {
		return errors.New("the vault is locked")
	}
	v.used = time.Now()
	if err := v.file.Seal(v.data, v.key); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(v.path("vault.json")), 0o700); err != nil {
		return err
	}
	index, _ := json.MarshalIndent(v.data.Index(v.file.Updated), "", "  ")
	if err := writeAtomic(v.path("vault.json"), v.file.Bytes()); err != nil {
		return err
	}
	_ = writeAtomic(v.path("index.json"), index)
	copy, _ := v.copyOf()
	for _, cl := range a.vaultClients() {
		go func() { _, _ = cl.VaultPut(copy) }()
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// vaultClients is the connections to hosts: the one shown and those
// watched.
func (a *App) vaultClients() []*client.Client {
	var out []*client.Client
	if a.client != nil {
		out = append(out, a.client)
	}
	for _, w := range a.watches {
		if w.cl != nil {
			out = append(out, w.cl)
		}
	}
	return out
}

// vaultConnected serves the vault's requests on a new connection and
// brings the host's copy and this device's to the newer, both ways.
func (a *App) vaultConnected(cl *client.Client) {
	go func() {
		if err := cl.ServeVault(); err != nil {
			return // an older daemon: no vault there
		}
		host, err := cl.VaultGet()
		if err != nil {
			return
		}
		a.onMain(func() {
			v := a.vaultOf()
			switch {
			case v.file == nil && len(host.Data) > 0:
				// Made on another device: this one takes the host's.
				if f, err := vault.Parse(host.Data); err == nil {
					v.file = f
					_ = os.MkdirAll(filepath.Dir(v.path("vault.json")), 0o700)
					_ = writeAtomic(v.path("vault.json"), host.Data)
					raw, _ := json.MarshalIndent(host.Index, "", "  ")
					_ = writeAtomic(v.path("index.json"), raw)
				}
			case v.file != nil:
				f, err := vault.Parse(host.Data)
				if err == nil && f.Updated.After(v.file.Updated) && v.key != nil {
					if data, err := f.Open(v.key); err == nil {
						v.file, v.data = f, data
						_ = writeAtomic(v.path("vault.json"), host.Data)
						return
					}
				}
				if err != nil || v.file.Updated.After(f.Updated) {
					if copy, ok := v.copyOf(); ok {
						go func() { _, _ = cl.VaultPut(copy) }()
					}
				}
			}
		})
	}()
}

// vaultHosts is the hosts a credential may be used on, for the page.
func (a *App) vaultHosts() []vaultview.Host {
	var out []vaultview.Host
	for _, h := range a.allHosts() {
		out = append(out, vaultview.Host{ID: h.ID, Name: h.Name})
	}
	return out
}

// vaultPage is the Credentials page.
func (a *App) vaultPage(c *ui.Context) {
	v := a.vaultOf()
	if v.key == nil && !v.keyRead && a.state.VaultRemember {
		a.vaultFromKeychain(func() {})
	}
	if v.key != nil {
		v.used = time.Now()
	}
	o := a.vaultOptions()
	a.viewHeader(c, func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { widget.Breadcrumb(c, "This device", "Credentials") })
		v.view.Toolbar(c, o)
		if widget.IconButton(c, icon.Close, "Back to sessions", false).Clicked() {
			a.page = ""
		}
	})
	v.view.Body(c, o)
}

// openVault shows the Credentials page, with the uses on the host shown.
func (a *App) openVault() {
	a.togglePage(pageVault)
	if a.page != pageVault {
		return
	}
	if cl := a.client; cl != nil {
		go func() {
			list, err := cl.VaultAudit(50)
			a.onMain(func() {
				if err == nil {
					a.vaultOf().audit = list
				}
			})
		}()
	}
}

// vaultTest checks credential id with its platform, off the main thread.
func (a *App) vaultTest(id string, done func(status, message string)) {
	v := a.vaultOf()
	if v.data == nil {
		done("", "the vault is locked")
		return
	}
	c, ok := v.data.Find(id)
	if !ok {
		done("", "no credential "+id)
		return
	}
	cred := *c
	v.testing[id] = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		status, msg := vault.Test(ctx, nil, cred)
		a.onMain(func() {
			delete(v.testing, id)
			if v.data != nil {
				if c, ok := v.data.Find(cred.ID); ok && status != proto.CredRevoked && status != proto.CredExpired {
					c.Status, c.Checked = status, time.Now().UTC()
					_ = a.vaultSave()
				}
			}
			done(status, msg)
		})
	}()
}

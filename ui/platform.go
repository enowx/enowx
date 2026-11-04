package ui

import (
	"cmp"
	"errors"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/platformagent/record"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/chatview"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/platformview"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/vaultview"
	"github.com/enowx/enowx/ui/widget"
)

// Claude Platform in the app: what its chat says that another agent's doesn't, and
// its sheet of API keys and system prompt, opened from Settings → Agents, its chat and
// the new session dialog. Its keys live in the vault like any other.

// platformVaultID is the vault's platform for the agent's keys.
const platformVaultID = "anthropic"

// openPlatform opens the agent's sheet on tab, and reads when its keys were last used.
func (a *App) openPlatform(tab int) {
	a.platformSheetView.Open(tab)
	a.vaultFromKeychain(func() {})
	cl := a.client
	if cl == nil {
		return
	}
	go func() {
		list, err := cl.VaultAudit(200)
		a.onMain(func() {
			if err == nil {
				a.vaultOf().audit = list
			}
		})
	}()
}

// platformSheet draws the sheet, and the vault's form when it opened that. Over the
// Credentials page, that page draws the form.
func (a *App) platformSheet(c *ui.Context) {
	if !a.platformSheetView.IsOpen() {
		return
	}
	v := a.vaultOf()
	vo := a.vaultOptions()
	a.platformSheetView.Sheet(c.Key("platform-sheet"), a.platformOptions(c, v, vo))
	if a.page != pageVault {
		v.view.Dialogs(c.Key("platform-vault"), vo)
	}
}

// platformOptions are what the sheet shows of the vault and the host. No value of a
// key goes in them.
func (a *App) platformOptions(c *ui.Context, v *vaultApp, vo vaultview.Options) platformview.Options {
	o := platformview.Options{
		Host:      a.host.Name,
		VaultOpen: vo.State == vaultview.StateOpen,
		Gate:      func() { v.view.Gate(c, vo) },
		Keys:      platformKeys(vo),
		AddKey:    func() { v.view.OpenAdd(platformVaultID) },
		TestKey: func(id string, done func(string)) {
			a.vaultTest(id, func(_, message string) { done(message) })
		},
		RemoveKey:  vo.Actions.Remove,
		SetDefault: vo.Actions.SetDefault,
		OpenURL:    vo.Actions.OpenURL,
		Confirm:    a.confirm,
	}
	o.LoadPrompt = func(done func(proto.PlatformPrompt, error)) {
		a.platformPromptCall(done, func(cl *client.Client) (proto.PlatformPrompt, error) { return cl.PlatformPrompt() })
	}
	o.SavePrompt = func(text string, done func(proto.PlatformPrompt, error)) {
		a.platformPromptCall(done, func(cl *client.Client) (proto.PlatformPrompt, error) { return cl.SavePlatformPrompt(text) })
	}
	o.ResetPrompt = func(done func(proto.PlatformPrompt, error)) {
		a.platformPromptCall(done, func(cl *client.Client) (proto.PlatformPrompt, error) { return cl.ResetPlatformPrompt() })
	}
	return o
}

// platformPromptCall runs a call of the prompt's off the main thread, done on it.
func (a *App) platformPromptCall(done func(proto.PlatformPrompt, error), call func(*client.Client) (proto.PlatformPrompt, error)) {
	cl := a.client
	if cl == nil {
		done(proto.PlatformPrompt{}, errors.New("not connected to the host"))
		return
	}
	go func() {
		p, err := call(cl)
		if err != nil {
			err = errors.New(client.Explain(err))
		}
		a.onMain(func() { done(p, err) })
	}()
}

// platformKeys are the vault's Anthropic credentials as the sheet shows them.
func platformKeys(vo vaultview.Options) []platformview.Key {
	now := time.Now()
	var out []platformview.Key
	for _, cr := range vo.Credentials {
		if cr.Platform != platformVaultID {
			continue
		}
		status, col := vaultview.Status(cr, now)
		k := platformview.Key{ID: cr.ID, Name: cr.Name, Status: status, Color: col, Default: cr.Default, Testing: vo.Testing[cr.ID]}
		for _, e := range vo.Audit {
			if e.Credential == cr.Name && e.Allowed && e.Time.After(k.LastUsed) {
				k.LastUsed = e.Time
			}
		}
		out = append(out, k)
	}
	return out
}

// platformKeyState is what the device knows of a key for the agent.
func (a *App) platformKeyState() platformview.KeyState {
	v := a.vaultOf()
	switch v.state() {
	case vaultview.StateNone:
		return platformview.KeyMissing
	case vaultview.StateLocked:
		a.vaultFromKeychain(func() {}) // once a run: it may be open by the next frame
		return platformview.KeyLocked
	}
	now := time.Now()
	for _, cr := range v.data.Credentials {
		if st := cr.State(now); cr.Platform == platformVaultID && st != proto.CredRevoked && st != proto.CredExpired {
			return platformview.KeyHave
		}
	}
	return platformview.KeyMissing
}

// platformChat gives the agent's chat its own words and what they lead to.
func (a *App) platformChat(o *chatview.Options, s proto.SessionInfo, ch *chatview.Chat) {
	name := o.AgentName
	o.Placeholder = "Message " + name + ", / for commands"
	o.OpenConsole = func() { go func() { _ = openExternal(platformview.ConsoleURL) }() }
	o.NoticeAction = func(notice string) (string, func()) {
		if platformview.KeyRefused(notice) {
			return "API keys", func() { a.openPlatform(platformview.TabKeys) }
		}
		return "", nil
	}
	o.Empty = func(c *ui.Context) {
		model, effort, budget := ch.Facts()
		set := agent.SettingsOf(s.Agent)
		platformview.Welcome{
			Name:     name,
			Model:    choiceLabel(set.Models, cmp.Or(model, record.DefaultModel)),
			Effort:   choiceLabel(set.Efforts, cmp.Or(effort, record.DefaultEffort)),
			Budget:   cmp.Or(budget, record.DefaultBudget),
			Key:      a.platformKeyState(),
			OpenKeys: func() { a.openPlatform(platformview.TabKeys) },
		}.Draw(c)
	}
}

// choiceLabel is how id reads among list, or id when it isn't one of them.
func choiceLabel(list []agent.Choice, id string) string {
	if ch, ok := agent.FindChoice(list, id); ok {
		return ch.Label
	}
	return id
}

// platformAgentRow is the agent's row in Settings → Agents, over its configs.
func (a *App) platformAgentRow(c *ui.Context) {
	ui.Column(c.Key("platform-row")).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
		widget.Heading(c, "API keys and system prompt")
		widget.CardNote(c, "Claude Platform runs on your Anthropic API key, kept in the vault, with a system prompt you can replace on each host.", 0)
		ui.Row(c).FillWidth().Gap(theme.S2).Wrap().Children(func() {
			if widget.Button(c.Key("keys"), widget.Secondary, icon.Key, "API keys", "").Label("Claude Platform's API keys").Clicked() {
				a.openPlatform(platformview.TabKeys)
			}
			if widget.Button(c.Key("prompt"), widget.Secondary, icon.Pencil, "System prompt", "").Label("Claude Platform's system prompt").Clicked() {
				a.openPlatform(platformview.TabPrompt)
			}
		})
	})
}

// platformNewSessionNote is what the new session dialog says of a missing key.
func (a *App) platformNewSessionNote(c *ui.Context) {
	platformview.KeyNotice(c.Key("platform-key"), a.platformKeyState(), func() { a.openPlatform(platformview.TabKeys) })
}

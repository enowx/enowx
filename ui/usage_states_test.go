package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/creditview"
	"github.com/enowx/enowx/ui/tokenview"
	"github.com/enowx/enowx/ui/vaultview"
)

// auditVault gives the app a vault of its own folder: none yet, for the states
// before there is one. Every value in it is made up.
func auditVault(t *testing.T, a *App) *vaultApp {
	a.vlt = &vaultApp{dir: t.TempDir(), testing: map[string]bool{}}
	a.page = pageVault
	return a.vaultOf()
}

// auditCredentials makes the vault and fills it with one of each state.
func auditCredentials(t *testing.T, a *App, extra ...vault.Credential) *vaultApp {
	v := auditVault(t, a)
	if err := a.vaultCreate("audit password", false); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, c := range append([]vault.Credential{
		{Platform: "github", Name: "github", Type: vault.TypeToken, Fields: map[string]string{"token": "made-up-a1b2"}, Status: proto.CredValid, Auto: true},
		{Platform: "anthropic", Name: "anthropic", Type: vault.TypeToken, Fields: map[string]string{"api_key": "made-up-9f3c"}, Status: proto.CredInvalid},
		{Platform: "cloudflare", Name: "cloudflare", Type: vault.TypeToken, Fields: map[string]string{"token": "made-up-0c4b"}, Status: proto.CredValid, Expires: now.Add(72 * time.Hour)},
		{Platform: "hetzner", Name: "hetzner", Type: vault.TypeToken, Fields: map[string]string{"token": "made-up-77de"}, Expires: now.Add(-time.Hour)},
		{Platform: "openai", Name: "work-openai", Type: vault.TypeToken, Fields: map[string]string{"api_key": "made-up-5e10"}, Revoked: true},
		{Platform: "made-up-platform", Name: "made-up", Type: vault.TypeToken, Fields: map[string]string{"value": "made-up-9999"}, Notes: "a platform without a mark"},
	}, extra...) {
		if _, err := v.data.Add(c); err != nil {
			t.Fatal(err)
		}
	}
	// Not left as the first of its platform's default.
	if c, ok := v.data.Find("anthropic"); ok {
		c.Default = false
	}
	v.audit = []proto.VaultAuditEntry{
		{Time: now.Add(-3 * time.Hour), Credential: "anthropic", Action: proto.VaultEnv, Detail: "its login failed", Allowed: false},
		{Time: now.Add(-time.Hour), Credential: "github", Action: proto.VaultGit, Detail: "GOA session tabs · git push", Allowed: true},
	}
	return v
}

// auditAsk puts a question of the vault's up.
func auditAsk(t *testing.T, a *App, ask vaultview.Ask) {
	v := auditCredentials(t, a)
	v.asks = []*vaultRequestAsk{{ask: ask, until: time.Now().Add(time.Hour)}}
}

// usageStatePages are the states of the pages that show by what they're given.
func usageStatePages(t *testing.T) []auditPage {
	const gb = 1 << 30
	usage := func(st proto.DaemonStats) func(a *App) {
		return func(a *App) {
			a.page, a.tok.pageTab = pageUsage, ""
			a.usage.Show(st, procstat.Usage{RSS: 310 << 20, CPU: 5})
		}
	}
	tokens := func(tab string, change func(d *tokenview.Data, v *tokenview.View)) func(a *App) {
		return func(a *App) {
			a.page = pageUsage
			a.devSampleData("tokens")
			a.tok.view = tokenview.View{Tab: tab}
			if change != nil {
				change(&a.tok.data, &a.tok.view)
			}
		}
	}
	account := func(name string, acc proto.CreditAccount) auditPage {
		return auditPage{name: name, open: func(a *App) {
			a.page = pageCredits
			a.credits.Show(proto.CreditsReport{Accounts: []proto.CreditAccount{acc}})
		}}
	}
	pages := []auditPage{
		{name: "usage-no-report", open: usage(proto.DaemonStats{})},
		{name: "usage-tight-none", open: usage(proto.DaemonStats{Supported: true,
			Host: proto.HostUsage{CPUs: 8, MemTotal: 24 * gb, MemUsed: 23 * gb, CPU: 90, DiskTotal: 994 * gb, DiskUsed: 950 * gb}})},
		{name: "usage-others", open: func(a *App) {
			usage(proto.DaemonStats{Supported: true, Host: proto.HostUsage{CPUs: 8, MemTotal: 24 * gb, MemUsed: 14 * gb, CPU: 38}})(a)
			a.watches = map[string]*hostWatch{
				"labs": {host: hosts.Host{ID: "labs", Name: "enowxlabs"}, status: hostStatus{state: hostOnline, usage: proto.HostUsage{CPUs: 4, MemTotal: 16 * gb, MemUsed: 5 * gb, CPU: 30}}},
				"old":  {host: hosts.Host{ID: "old", Name: "old-vps"}},
			}
		}},
		{name: "tokens-off", open: tokens(tokenview.TabOverview, func(d *tokenview.Data, _ *tokenview.View) {
			d.Settings.On, d.Summary = false, proto.TokensSummary{}
		})},
		{name: "tokens-loading", open: tokens(tokenview.TabOverview, func(d *tokenview.Data, v *tokenview.View) {
			d.Loading, v.Project = true, "/work/a-project-with-a-long-name"
		})},
		{name: "tokens-error", open: tokens(tokenview.TabOverview, func(d *tokenview.Data, _ *tokenview.View) {
			d.Err = "enowxlabs: connection lost"
		})},
		{name: "tokens-week", open: tokens(tokenview.TabOverview, func(_ *tokenview.Data, v *tokenview.View) { v.Range = 1 })},
		{name: "tokens-month", open: tokens(tokenview.TabOverview, func(_ *tokenview.Data, v *tokenview.View) { v.Range = 2 })},
		{name: "tokens-year", open: tokens(tokenview.TabOverview, func(_ *tokenview.Data, v *tokenview.View) { v.Range = 4 })},
		{name: "tokens-no-budgets", open: tokens(tokenview.TabBudgets, func(d *tokenview.Data, _ *tokenview.View) { d.Budgets = nil })},
		{name: "tokens-budget-levels", open: tokens(tokenview.TabBudgets, func(d *tokenview.Data, _ *tokenview.View) {
			d.Budgets[0].Fraction, d.Budgets[1].Fraction = 0.85, 1.2
		})},
		{name: "credits-none", open: func(a *App) { a.page = pageCredits; a.credits.Show(proto.CreditsReport{}) }},
		{name: "credits-loading", open: func(a *App) { a.page = pageCredits; a.credits = creditview.Tracker{} }},
		{name: "credits-error", open: func(a *App) { a.page = pageCredits; a.credits.ShowError(errors.New("the daemon didn't answer")) }},
		{name: "credits-older", open: func(a *App) { a.page = pageCredits; a.credits.ShowError(errors.New("unknown method")) }},
		account("credits-account-error", proto.CreditAccount{Agent: "claude", Plan: "Claude Max", Error: "Its sign-in has expired. Sign in again to read its limits.", Note: "A note the agent's vendor adds."}),
		{name: "vault-none", open: func(a *App) { auditVault(t, a) }},
		{name: "vault-locked", open: func(a *App) { auditCredentials(t, a); a.vaultLock() }},
		{name: "vault-empty", open: func(a *App) {
			v := auditVault(t, a)
			if err := a.vaultCreate("audit password", false); err != nil {
				t.Fatal(err)
			}
			_ = v
		}},
		{name: "vault-states", open: func(a *App) {
			v := auditCredentials(t, a)
			c, _ := v.data.Find("github")
			v.testing[c.ID] = true
		}},
		{name: "vault-groups", open: func(a *App) {
			var more []vault.Credential
			for i := range 6 {
				more = append(more, vault.Credential{Platform: "generic", Name: fmt.Sprintf("generic-%d", i), Type: vault.TypeToken, Fields: map[string]string{"value": "made-up"}})
			}
			auditCredentials(t, a, more...)
		}},
		{name: "vault-ask-lend", open: func(a *App) {
			auditAsk(t, a, vaultview.Ask{Always: true, Host: "enowxlabs", Title: "Let enowxlabs use github?",
				About: "Claude Code in “Dialog footers” on enowxlabs asks for it, for git push. The values go to that session only."})
		}},
		{name: "vault-ask-locked", open: func(a *App) {
			auditAsk(t, a, vaultview.Ask{Locked: true, Title: "Unlock the vault", About: "Claude Code on enowxlabs asks for github."})
			a.vaultLock()
		}},
		{name: "vault-ask-add", open: func(a *App) {
			auditAsk(t, a, vaultview.Ask{Title: "Keep made-up in the vault?", About: "Claude Code on enowxlabs asks to keep a new GitHub credential, made-up."})
		}},
		{name: "vault-ask-remove", open: func(a *App) {
			auditAsk(t, a, vaultview.Ask{Danger: true, Title: "Remove github from the vault?", About: "Claude Code on enowxlabs asks to remove it. Hosts can't use it afterwards."})
		}},
		{name: "vault-api-keys", open: func(a *App) {
			auditCredentials(t, a)
			a.page, a.settingsSection = pageSettings, sectionAgents
			a.agentAPIsOf().edit = &agentAPIForm{open: true, agent: proto.AgentAPIInfo{Agent: "opencode", Name: "OpenCode"}, effort: -1}
		}},
	}
	return pages
}

// auditFlow is a state reached by using the page, as the user would.
type auditFlow struct {
	name  string
	open  func(a *App)
	steps func(t *testing.T, tt *ui.Tester)
}

// auditFlows draws each flow's end at the audit's sizes, its layout checked.
func auditFlows(t *testing.T, h *notesHarness, flows []auditFlow) {
	t.Helper()
	only := os.Getenv("AUDIT")
	var problems []string
	for _, f := range flows {
		if only != "" && !strings.Contains(f.name, only) {
			continue
		}
		for _, size := range auditSizes {
			closeAudit(h.app)
			h.app.watches, h.app.confirming = nil, nil
			f.open(h.app)
			tt := ui.NewTester(func(c *ui.Context) { h.app.view(c) }, size[0], size[1])
			h.tt = tt
			for range 6 {
				h.settle("", func() bool { return true })
			}
			f.steps(t, tt)
			for range 4 {
				h.settle("", func() bool { return true })
			}
			name := fmt.Sprintf("%s-%dx%d", f.name, size[0], size[1])
			shot(t, tt, filepath.Join("audit", name+".png"))
			for _, pr := range layoutProblems(tt, float32(size[0]), float32(size[1]), false) {
				if strings.Contains(pr, `"Project" out of in "Agent"`) {
					continue // New session's own row: dialog-new-session reports it
				}
				problems = append(problems, name+": "+pr)
			}
		}
	}
	if len(problems) > 0 {
		t.Errorf("%d layout problems:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

// clicks clicks each label in turn, a frame after each.
func clicks(t *testing.T, tt *ui.Tester, labels ...string) {
	t.Helper()
	for _, l := range labels {
		// What opened a step ago may still be coming in.
		err := tt.Click(l)
		for try := 0; err != nil && try < 40; try++ {
			time.Sleep(10 * time.Millisecond)
			tt.Frame()
			err = tt.Click(l)
		}
		if err != nil {
			t.Fatalf("%v; shows %q", err, tt.Texts())
		}
		tt.Frame()
		tt.Frame()
	}
}

// usageFlows are the dialogs and their errors, and what the words say there.
func usageFlows(t *testing.T) []auditFlow {
	see := func(t *testing.T, tt *ui.Tester, text string) {
		t.Helper()
		if !tt.HasText(text) {
			t.Fatalf("no %q; shows %q", text, tt.Texts())
		}
	}
	tokens := func(tab string) func(a *App) {
		return func(a *App) {
			a.page = pageUsage
			a.devSampleData("tokens")
			a.tok.view = tokenview.View{Tab: tab}
		}
	}
	vaultOpen := func(a *App) { auditCredentials(t, a) }
	return []auditFlow{
		{"vault-new-session-open", func(a *App) { auditCredentials(t, a); a.page = ""; a.openNewSession(a.projects[0]) }, func(t *testing.T, tt *ui.Tester) {
			see(t, tt, "Credentials (optional)")
		}},
		{"vault-new-session-locked", func(a *App) {
			auditCredentials(t, a)
			a.vaultLock()
			a.page = ""
			a.openNewSession(a.projects[0])
		}, func(t *testing.T, tt *ui.Tester) {
			see(t, tt, "Unlock the vault (Credentials) to start the session with logins from it.")
		}},
		{"vault-make-short", func(a *App) { auditVault(t, a) }, func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "Make the vault")
			see(t, tt, "The master password needs at least 8 characters.")
		}},
		{"vault-make-differ", func(a *App) { auditVault(t, a) }, func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "At least 8 characters")
			tt.Type("made-up password")
			clicks(t, tt, "Make the vault")
			see(t, tt, "The two passwords differ.")
		}},
		{"vault-unlock-wrong", func(a *App) { auditCredentials(t, a); a.vaultLock() }, func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "Your master password")
			tt.Type("made-up wrong")
			clicks(t, tt, "Unlock")
			see(t, tt, "wrong master password")
		}},
		{"vault-menu-default", vaultOpen, func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "More for github")
			see(t, tt, "Revoke: no host may use it")
			if tt.HasText("Make it GitHub's default") {
				t.Error("the default offers to become the default")
			}
		}},
		{"vault-menu-revoked", vaultOpen, func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "More for anthropic")
			see(t, tt, "Make it Anthropic's default")
		}},
		{"vault-edit", vaultOpen, func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "More for github", "Edit…")
			see(t, tt, "Token (empty keeps it)")
			if tt.HasText("Open the token page") {
				t.Error("editing offers the quick ways")
			}
		}},
		{"vault-edit-error", vaultOpen, func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "More for github", "Edit…", "2027-01-31")
			tt.Type("soon")
			clicks(t, tt, "Save")
			see(t, tt, "The expiry date reads as YYYY-MM-DD.")
		}},
		{"vault-change-differ", func(a *App) { auditCredentials(t, a); a.vlt.view.OpenChange() }, func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "At least 8 characters")
			tt.Type("made-up one")
			clicks(t, tt, "Change it")
			see(t, tt, "The two new passwords differ.")
		}},
		{"usage-stop-all", func(a *App) { a.page, a.tok.pageTab = pageUsage, ""; a.devSampleData("usage") }, func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "Stop all agents…")
			see(t, tt, "Their sessions stay, to continue.")
		}},
		{"tokens-budget-sheet", tokens(tokenview.TabBudgets), func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "Add a budget", "Save")
			see(t, tt, "set tokens, a cost, or both")
		}},
		{"tokens-budget-target", tokens(tokenview.TabBudgets), func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "Add a budget", "Everything", "An agent", "Save")
			see(t, tt, "say which agent")
		}},
		{"tokens-budget-remove", tokens(tokenview.TabBudgets), func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "Remove")
			see(t, tt, "Remove this budget?")
		}},
		{"tokens-price-sheet", tokens(tokenview.TabPrices), func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "Add price", "Save")
			see(t, tt, "name the model, or a prefix ending in *")
		}},
		{"tokens-price-remove", tokens(tokenview.TabPrices), func(t *testing.T, tt *ui.Tester) {
			clicks(t, tt, "Remove")
			see(t, tt, "Remove your price for qwen3:8b?")
		}},
	}
}

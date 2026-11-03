package ui

import (
	"testing"
	"time"

	"github.com/enowx/enowx/core/vault"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/tokenview"
	"github.com/enowx/enowx/ui/vaultview"
)

// usageAuditPages are Resource usage, Credits and the vault in their states.
// Last of the audit: they make the vault, which the page above shows locked.
func usageAuditPages(t *testing.T) []auditPage {
	tokens := func(tab string) func(a *App) {
		return func(a *App) {
			a.page = pageUsage
			a.devSampleData("tokens")
			a.tok.view = tokenview.View{Tab: tab}
		}
	}
	openVault := func(a *App) *vaultApp {
		a.page = pageVault
		v := a.vaultOf()
		if v.data == nil {
			_ = a.vaultCreate("audit password", false)
		}
		v.view = vaultview.View{}
		if len(v.data.Credentials) == 0 {
			for _, c := range []vault.Credential{
				{Platform: "github", Name: "github", Account: "someone-with-a-very-long-account-name@example.com", Type: vault.TypeToken, Fields: map[string]string{"token": "made-up-value-a1b2"}, Default: true, Auto: true},
				{Platform: "anthropic", Name: "anthropic", Type: vault.TypeToken, Fields: map[string]string{"api_key": "made-up-value-9f3c"}},
				{Platform: "cloudflare", Name: "cloudflare", Type: vault.TypeToken, Fields: map[string]string{"token": "made-up-value-0c4b"}, Expires: time.Now().Add(72 * time.Hour)},
			} {
				_, _ = v.data.Add(c)
			}
		}
		a.devSampleData("vault")
		return v
	}
	return []auditPage{
		{name: "usage-sample", open: func(a *App) { a.page = pageUsage; a.tok.pageTab = ""; a.devSampleData("usage") }},
		{name: "usage-overview", open: tokens(tokenview.TabOverview)},
		{name: "usage-budgets", open: tokens(tokenview.TabBudgets)},
		{name: "usage-prices", open: tokens(tokenview.TabPrices)},
		{name: "usage-coverage", open: tokens(tokenview.TabCoverage)},
		{name: "credits-sample", open: func(a *App) { a.page = pageCredits; a.devSampleData("credits") }},
		{name: "vault-open", open: func(a *App) { openVault(a) }},
		{name: "vault-import-found", open: func(a *App) {
			v := openVault(a)
			v.scan = false
			v.found = []proto.VaultFound{
				{Platform: "github", Name: "github", Account: "someone", Source: "gh auth token", Fields: map[string]string{"token": "made-up-value-a1b2"}},
				{Platform: "aws", Name: "aws", Source: "aws configure", Note: "a default profile", Fields: map[string]string{"access_key_id": "made-up-id"}},
			}
			v.view.OpenImport()
		}},
		{name: "vault-import-scanning", open: func(a *App) { v := openVault(a); v.scan, v.found = true, nil; v.view.OpenImport() }},
		{name: "vault-import-none", open: func(a *App) { v := openVault(a); v.scan, v.found = false, nil; v.view.OpenImport() }},
		{name: "vault-form-found", open: func(a *App) {
			v := openVault(a)
			v.scan = false
			v.found = []proto.VaultFound{{Platform: "cloudflare", Name: "cloudflare", Account: "someone", Source: "wrangler", Fields: map[string]string{"token": "made-up-value-77de"}}}
			v.view.OpenAdd("cloudflare")
		}},
		{name: "vault-picker", open: func(a *App) { openVault(a).view.OpenAdd("") }},
		{name: "vault-form", open: func(a *App) { openVault(a).view.OpenAdd("github") }},
		{name: "vault-import", open: func(a *App) { openVault(a).view.OpenImport() }},
		{name: "vault-change", open: func(a *App) { openVault(a).view.OpenChange() }},
		{name: "vault-details", open: func(a *App) {
			v := openVault(a)
			v.view.OpenDetails(v.data.Credentials[0].ID)
		}},
	}
}

// TestLayoutAuditUsage draws them as TestLayoutAudit draws the others.
func TestLayoutAuditUsage(t *testing.T) {
	fakeKeychain(t) // never the owner's: these pages make and open a vault
	h := newNotesHarness(t)
	seedAudit(t, h)
	auditPagesOn(t, h, append(usageAuditPages(t), usageStatePages(t)...), func() { h.app.watches = nil })
	auditFlows(t, h, usageFlows(t))
}

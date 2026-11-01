package ui

import (
	"time"

	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/tokenview"
)

// devSampleData fills a view with made-up numbers, for a development build's
// pictures and the layout audit: its own home has no accounts, no history and
// nothing measured worth showing.
func (a *App) devSampleData(what string) {
	const mb = 1 << 20
	const gb = 1 << 30
	now := time.Now()
	switch what {
	case "credits":
		a.credits.Show(proto.CreditsReport{Checked: now.Add(-2 * time.Minute), Accounts: []proto.CreditAccount{
			{Agent: "claude", Plan: "Claude Max", Windows: []proto.CreditWindow{
				{Label: "5 hours", Used: 100, ResetsAt: now.Add(2 * time.Hour)},
				{Label: "Week", Used: 64, ResetsAt: now.Add(48 * time.Hour)}}},
			{Agent: "claude", Plan: "Claude Team", AccountName: "Work account", Windows: []proto.CreditWindow{
				{Label: "5 hours", Used: 34, ResetsAt: now.Add(4 * time.Hour)},
				{Label: "Week", Used: 21, ResetsAt: now.Add(120 * time.Hour)}}},
			{Agent: "codex", Plan: "ChatGPT Pro", Windows: []proto.CreditWindow{
				{Label: "5 hours", Used: 12, ResetsAt: now.Add(3 * time.Hour)},
				{Label: "Week", Used: 28, ResetsAt: now.Add(24 * time.Hour)}}},
			{Agent: "kiro", Plan: "Kiro", AccountName: "Team account", Windows: []proto.CreditWindow{
				{Label: "This month", Used: 91, Detail: "910 of 1000 credits"}}},
		}})
	case "usage":
		a.usage.Show(proto.DaemonStats{Supported: true,
			Host:   proto.HostUsage{CPUs: 8, MemTotal: 24 * gb, MemUsed: 14 * gb, CPU: 38, DiskTotal: 994 * gb, DiskUsed: 412 * gb},
			Daemon: proto.Usage{RSS: 96 * mb, CPU: 1, Procs: 1},
			Sessions: []proto.SessionUsage{
				{ID: "GOA session tabs", Usage: proto.Usage{RSS: 812 * mb, CPU: 22, Procs: 11}},
				{ID: "Host keys to the vault", Usage: proto.Usage{RSS: 540 * mb, CPU: 12, Procs: 9}},
				{ID: "Notification toasts", Usage: proto.Usage{RSS: 488 * mb, CPU: 9, Procs: 8}},
				{ID: "Session pointer drag", Usage: proto.Usage{RSS: 128 * mb, CPU: 3, Procs: 4}},
				{ID: "Notes and Canvas lists", Usage: proto.Usage{RSS: 96 * mb, CPU: 2, Procs: 3}},
				{ID: "Outline at the newest", Usage: proto.Usage{RSS: 24 * mb, CPU: 1, Procs: 2}},
				{ID: "Write rules with AI", Usage: proto.Usage{RSS: 24 * mb, CPU: 1, Procs: 2}},
				{ID: "a shell", Usage: proto.Usage{RSS: 8 * mb, CPU: 1, Procs: 1}},
			}}, procstat.Usage{RSS: 310 * mb, CPU: 5})
	case "tokens":
		a.tok.pageTab = usageTabTokens
		a.tok.gen, a.tok.fetched = a.gen, now.Add(time.Hour) // nothing fetched over it
		a.tok.data = sampleTokens(now)
	case "vault":
		v := a.vaultOf()
		v.audit = []proto.VaultAuditEntry{
			{Time: now.Add(-5 * time.Hour), Credential: "postgres", Action: proto.VaultEnv, Detail: "RAG on enowxlabs: its login failed", Allowed: false},
			{Time: now.Add(-3 * time.Hour), Credential: "anthropic", Action: proto.VaultEnv, Detail: "RAG on this Mac", Allowed: true},
			{Time: now.Add(-2 * time.Hour), Credential: "github", Action: proto.VaultGit, Detail: "Dialog footers · gh pr view", Allowed: true},
			{Time: now.Add(-time.Hour), Credential: "github", Action: proto.VaultGit, Detail: "GOA session tabs · git push", Allowed: true},
		}
	}
}

// sampleTokens are made-up numbers for the Tokens tab.
func sampleTokens(now time.Time) tokenview.Data {
	counts := func(in, cached, out int64, calls int, cost float64) proto.TokenCounts {
		return proto.TokenCounts{Input: in, CacheRead: cached, Output: out, Calls: calls, Cost: cost}
	}
	var days []proto.TokenDay
	for i, use := range []int64{12, 19, 15, 25, 20, 5, 9, 11} {
		day := now.AddDate(0, 0, i-7).Format(time.DateOnly)
		days = append(days, proto.TokenDay{Day: day, TokenCounts: counts(use*100000, use*60000, use*8000, int(use)*30, float64(use)*0.8)})
	}
	total := counts(8800000, 5280000, 720000, 5140, 46.2)
	day := func(d int) time.Time { return now.AddDate(0, 0, -d) }
	return tokenview.Data{
		Summary: proto.TokensSummary{Total: total, Days: days, Groups: []proto.TokenGroup{
			{Key: "/work/enowx", Label: "enowx", TokenCounts: counts(5400000, 3200000, 500000, 3100, 28.4)},
			{Key: "/work/enowx-cloud", Label: "enowx-cloud", TokenCounts: counts(2200000, 1300000, 100000, 1200, 11.1)},
			{Key: "/work/enowx-web", Label: "enowx-web", TokenCounts: counts(1200000, 780000, 120000, 840, 6.7)},
		}, Own: counts(1200, 0, 300, 15, 0.0026), OwnGroups: []proto.TokenGroup{
			{Key: proto.TokensOwnJudge, TokenCounts: counts(1000, 0, 250, 12, 0.0022)},
			{Key: proto.TokensOwnRAG, TokenCounts: counts(200, 0, 50, 3, 0.0004)},
		}},
		Hosts: []tokenview.HostTotal{
			{Name: "this Mac", Live: true, On: true, Total: counts(6500000, 3900000, 500000, 3900, 34.1)},
			{Name: "enowxlabs", Live: true, On: true, Total: counts(2300000, 1380000, 220000, 1240, 12.1)},
			{Name: "old-vps", Updated: day(4), On: true},
		},
		Settings: proto.TokensSettings{On: true, Screen: true, BackfillDays: 30},
		Budgets: []proto.TokenBudgetState{
			{TokenBudget: proto.TokenBudget{ID: "a", Period: proto.BudgetWeek, Scope: proto.BudgetAll, Cost: 75, AtLimit: proto.BudgetAsk}, Fraction: 0.62, Used: total, Since: day(3)},
			{TokenBudget: proto.TokenBudget{ID: "b", Period: proto.BudgetWeek, Scope: proto.BudgetProject, Target: "enowx-cloud", Cost: 15, AtLimit: proto.BudgetNotify}, Fraction: 0.74, Used: counts(2200000, 1300000, 100000, 1200, 11.1), Since: day(3)},
			{TokenBudget: proto.TokenBudget{ID: "c", Period: proto.BudgetDay, Scope: proto.BudgetAgent, Target: "claude", Tokens: 4000000, AtLimit: proto.BudgetStop}, Fraction: 0.47, Used: counts(1200000, 600000, 100000, 300, 5.4), Since: now},
		},
		Prices: []proto.TokenPrice{
			{Model: "qwen3:8b", Config: "Local model"},
			{Model: "claude-opus-5-5", Provider: "anthropic", Input: 5, CacheRead: 0.5, CacheWrite: 6.25, Output: 25, Shipped: true},
			{Model: "claude-sonnet-5-5", Provider: "anthropic", Input: 3, CacheRead: 0.3, CacheWrite: 3.75, Output: 15, Shipped: true},
			{Model: "gpt-5.5-codex", Provider: "openai", Input: 2.5, CacheRead: 0.25, Output: 10, Shipped: true},
		},
		Coverage: []proto.TokenCoverage{
			{Agent: "claude", Name: "Claude Code", Layer: proto.TokenLayerRecords, Records: "30 days read back", Where: "~/.claude/projects", Checked: true},
			{Agent: "codex", Name: "Codex", Layer: proto.TokenLayerRecords, Records: "30 days read back", Where: "~/.codex/sessions"},
			{Agent: "opencode", Name: "OpenCode", Layer: proto.TokenLayerTraffic, Records: "Through the interceptor, new sessions only"},
			{Agent: "kiro", Name: "Kiro CLI", Layer: proto.TokenLayerScreen, Records: "The counts it prints"},
			{Agent: "gemini", Name: "Gemini CLI", Layer: proto.TokenLayerNone, Records: "Turn on metering to count it"},
		},
	}
}

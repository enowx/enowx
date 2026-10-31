package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/core/tokens"
	"github.com/enowx/enowx/proto"
)

const usageUsage = `usage: enowx-app usage [--by agent|model|project|session|config|host] [--since 7d] [--host H] [--json]
       enowx-app usage session [SESSION] | budgets | coverage`

// usageCmd runs `enowx usage …`: the tokens agents used (docs/tokens.md).
func (r runner) usageCmd(args []string) error {
	fs := flag.NewFlagSet("usage", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	by := fs.String("by", "", "")
	since := fs.String("since", "7d", "")
	host := fs.String("host", "", "")
	asJSON := fs.Bool("json", false, "")
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	var words []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return errors.New(usageUsage)
		}
		if args = fs.Args(); len(args) > 0 {
			words, args = append(words, args[0]), args[1:]
		}
	}
	out := func(v any) error {
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Fprintln(r.out, string(b))
		return nil
	}
	switch sub {
	case "", "summary":
		from, err := parseSince(*since, time.Now())
		if err != nil {
			return err
		}
		q := proto.TokensQuery{Since: from, By: *by}
		s, err := r.tokenSummary(q, *host)
		if err != nil {
			return err
		}
		if *asJSON {
			return out(s)
		}
		r.printTokenSummary(s, q)
		return nil
	case "session":
		ref := "self"
		if len(words) > 0 {
			ref = words[0]
		}
		info, err := r.session(ref)
		if err != nil {
			return err
		}
		var s proto.TokensSummary
		if err := r.c.Call(proto.MethodTokensSession, proto.SessionRef{ID: info.ID}, &s); err != nil {
			return err
		}
		if *asJSON {
			return out(s)
		}
		fmt.Fprintf(r.out, "Session %s (%s): %s\n", info.ID, info.Agent, countsLine(s.Total))
		for _, g := range s.Groups {
			fmt.Fprintf(r.out, "  %-28s %s\n", groupName(g), countsLine(g.TokenCounts))
		}
		return nil
	case "budgets":
		var list []proto.TokenBudgetState
		if err := r.c.Call(proto.MethodTokensBudgets, nil, &list); err != nil {
			return err
		}
		if *asJSON {
			return out(list)
		}
		if len(list) == 0 {
			fmt.Fprintln(r.out, "No budgets (Resource usage → Tokens → Budgets).")
		}
		for _, b := range list {
			what := "everything"
			if b.Scope != proto.BudgetAll {
				what = b.Scope + " " + b.Target
			}
			fmt.Fprintf(r.out, "%-6s %-24s %3d%%  %s  at 100%%: %s\n", b.Period, what, int(b.Fraction*100), countsLine(b.Used), orDefault(b.AtLimit, proto.BudgetNotify))
		}
		return nil
	case "coverage":
		var list []proto.TokenCoverage
		if err := r.c.Call(proto.MethodTokensCoverage, nil, &list); err != nil {
			return err
		}
		if *asJSON {
			return out(list)
		}
		for _, c := range list {
			checked := ""
			if c.Checked {
				checked = " (checked)"
			}
			fmt.Fprintf(r.out, "%-20s %-8s %s%s\n", c.Name, c.Layer, c.Records, checked)
		}
		return nil
	}
	return errors.New(usageUsage)
}

func orDefault(s, or string) string {
	if s == "" {
		return or
	}
	return s
}

// tokenSummary is this host's summary, or host's (by its synced daily
// totals), or every host's split by host.
func (r runner) tokenSummary(q proto.TokensQuery, host string) (proto.TokensSummary, error) {
	var own proto.TokensSummary
	local := q
	if q.By == proto.TokensByHost {
		local.By = ""
	}
	if err := r.c.Call(proto.MethodTokensSummary, local, &own); err != nil {
		return own, err
	}
	if host == "" && q.By != proto.TokensByHost {
		return own, nil
	}
	var hosts []proto.TokensHostDays
	if err := r.c.Call(proto.MethodTokensHosts, nil, &hosts); err != nil {
		return own, err
	}
	if host != "" {
		for _, h := range hosts {
			if h.Host == host || strings.EqualFold(h.Name, host) {
				s := tokens.SummarizeRows(h.Rows, q, time.Now())
				s.Host, s.HostID, s.On = h.Name, h.Host, true
				return s, nil
			}
		}
		return own, fmt.Errorf("no totals of host %q here: hosts sync theirs once tracking is on there", host)
	}
	// Split by host: this one's now, the others' as last synced.
	all := own
	all.Groups = []proto.TokenGroup{{Key: own.HostID, Label: own.Host + " (this host)", TokenCounts: own.Total}}
	for _, h := range hosts {
		if h.Host == own.HostID {
			continue
		}
		s := tokens.SummarizeRows(h.Rows, q, time.Now())
		all.Total.Add(s.Total)
		all.Groups = append(all.Groups, proto.TokenGroup{Key: h.Host, Label: h.Name + " (synced " + h.Updated.Local().Format("Jan 2 15:04") + ")", TokenCounts: s.Total})
	}
	return all, nil
}

func (r runner) printTokenSummary(s proto.TokensSummary, q proto.TokensQuery) {
	if !s.On {
		fmt.Fprintln(r.out, "Token tracking is off on this host (Resource usage → Tokens, or the app's settings).")
	}
	fmt.Fprintf(r.out, "%s, %s to %s: %s\n", orDefault(s.Host, "This host"), s.Since.Local().Format("Jan 2"), s.Until.Local().Format("Jan 2 15:04"), countsLine(s.Total))
	for _, g := range s.Groups {
		fmt.Fprintf(r.out, "  %-32s %s\n", groupName(g), countsLine(g.TokenCounts))
	}
	if q.By == "" && len(s.Days) > 0 {
		for _, d := range s.Days {
			fmt.Fprintf(r.out, "  %s  %s\n", d.Day, countsLine(d.TokenCounts))
		}
	}
	if s.Own.Calls > 0 {
		fmt.Fprintf(r.out, "enowX's own (judge, RAG), apart: %s\n", countsLine(s.Own))
	}
	for _, b := range s.Budgets {
		fmt.Fprintf(r.out, "Budget %s %s %s: %d%% used\n", b.Period, b.Scope, b.Target, int(b.Fraction*100))
	}
}

func groupName(g proto.TokenGroup) string {
	if g.Label != "" && g.Label != g.Key {
		if g.Key == "" {
			return g.Label
		}
		return g.Label + " (" + shorten(g.Key, 12) + ")"
	}
	return orDefault(g.Key, "unknown")
}

// countsLine says counts in a line: input, cached, output, cost.
func countsLine(c proto.TokenCounts) string {
	line := fmt.Sprintf("%s in, %s cached, %s written to cache, %s out", tokenCount(c.Input), tokenCount(c.CacheRead), tokenCount(c.CacheWrite), tokenCount(c.Output))
	if c.Reasoning > 0 {
		line += " (" + tokenCount(c.Reasoning) + " reasoning)"
	}
	line += fmt.Sprintf(", %d calls", c.Calls)
	if c.Cost > 0 {
		line += fmt.Sprintf(", $%.2f", c.Cost)
		if c.Equivalent > 0 {
			line += fmt.Sprintf(" ($%.2f of it API-equivalent, on plans)", c.Equivalent)
		}
	}
	if c.Unpriced > 0 {
		line += fmt.Sprintf(", %d unpriced", c.Unpriced)
	}
	return line
}

func tokenCount(n int64) string {
	switch {
	case n >= 1e9:
		return strconv.FormatFloat(float64(n)/1e9, 'f', 1, 64) + "B"
	case n >= 1e6:
		return strconv.FormatFloat(float64(n)/1e6, 'f', 1, 64) + "M"
	case n >= 1e4:
		return strconv.FormatFloat(float64(n)/1e3, 'f', 0, 64) + "k"
	}
	return strconv.FormatInt(n, 10)
}

// parseSince reads "7d", "24h", "2w", "30m" back from now, or a date.
func parseSince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.ParseInLocation(time.DateOnly, s, time.Local); err == nil {
		return t, nil
	}
	if len(s) > 1 {
		n, err := strconv.Atoi(s[:len(s)-1])
		if err == nil && n >= 0 {
			switch s[len(s)-1] {
			case 'm':
				return now.Add(-time.Duration(n) * time.Minute), nil
			case 'h':
				return now.Add(-time.Duration(n) * time.Hour), nil
			case 'd':
				return now.AddDate(0, 0, -n), nil
			case 'w':
				return now.AddDate(0, 0, -7*n), nil
			}
		}
	}
	if slices.Contains([]string{"today", "day"}, s) {
		return tokens.PeriodStart(proto.BudgetDay, now), nil
	}
	return time.Time{}, fmt.Errorf("usage: --since takes 30m, 24h, 7d, 2w, today or a date (2006-01-02), not %q", s)
}

// usageTools are token usage as MCP tools: what this session and the
// host's agents spend, for GOA and agents to weigh.
var usageTools = []mcpTool{
	{name: "usage_summary", description: "Tokens the host's agents used: total, per day, split by agent, model, project, session, config or host, with costs (subscriptions' API-equivalent) and budgets near their limit. Near a budget, prefer cheaper models.",
		props: map[string]mcpProp{"by": {"string", "Split by: agent, model, project, session, config, host"}, "since": {"string", "How far back: 30m, 24h, 7d (default), 2w, or a date"},
			"host": {"string", "Another host's synced totals, by name"}},
		args: func(a map[string]any) []string {
			return withFlag(withFlag(withFlag([]string{"summary"}, a, "by", "by"), a, "since", "since"), a, "host", "host")
		}},
	{name: "usage_session", description: "Tokens a session used so far (this one by default), by model, with its cost.",
		props: map[string]mcpProp{"session": {"string", "A session's ID or name (default: this session)"}},
		args: func(a map[string]any) []string {
			if s := str(a, "session"); s != "" {
				return []string{"session", s}
			}
			return []string{"session", "self"}
		}},
}

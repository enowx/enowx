package tokens

import (
	"cmp"
	"slices"

	"github.com/enowx/enowx/proto"
)

// Merge sums several hosts' summaries of one query q as one: every
// host's numbers together. Split by host, each host is a group (its
// Label the host's name).
func Merge(q proto.TokensQuery, parts []proto.TokensSummary) proto.TokensSummary {
	var out proto.TokensSummary
	days := map[string]*proto.TokenDay{}
	groups := map[string]*proto.TokenGroup{}
	own := map[string]*proto.TokenGroup{}
	for _, p := range parts {
		out.On = out.On || p.On
		if out.Since.IsZero() || p.Since.Before(out.Since) {
			out.Since = p.Since
		}
		if p.Until.After(out.Until) {
			out.Until = p.Until
		}
		out.Total.Add(p.Total)
		out.Own.Add(p.Own)
		out.Budgets = append(out.Budgets, p.Budgets...)
		for _, d := range p.Days {
			if days[d.Day] == nil {
				days[d.Day] = &proto.TokenDay{Day: d.Day}
			}
			days[d.Day].Add(d.TokenCounts)
		}
		if q.By == proto.TokensByHost {
			key := cmp.Or(p.HostID, p.Host)
			add(groups, key, p.Total)
			groups[key].Label = p.Host
		} else {
			for _, g := range p.Groups {
				add(groups, g.Key, g.TokenCounts)
				if groups[g.Key].Label == "" {
					groups[g.Key].Label = g.Label
				}
			}
		}
		for _, g := range p.OwnGroups {
			add(own, g.Key, g.TokenCounts)
		}
	}
	for _, d := range days {
		out.Days = append(out.Days, *d)
	}
	slices.SortFunc(out.Days, func(a, b proto.TokenDay) int { return cmp.Compare(a.Day, b.Day) })
	out.Groups, out.OwnGroups = sorted(groups), sorted(own)
	return out
}

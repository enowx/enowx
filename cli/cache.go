package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
)

const cacheUsage = `usage: enowx-app cache [clean]
  (none) | list [--older DAYS]  what this host's enowX folder takes, and what can go
  clean --kind a,b [--item PATH]... [--older DAYS] --yes
                                take that away; without --yes, says what would go.
                                Old backups and worktrees go by --item only.`

// cache shows what a host's enowX folder takes, and cleans what can go.
func (r runner) cache(args []string) error {
	cmd := "list"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("cache", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	older := fs.Int("older", 0, "")
	kinds := fs.String("kind", "", "")
	yes := fs.Bool("yes", false, "")
	var items multiFlag
	fs.Var(&items, "item", "")
	if err := fs.Parse(args); err != nil {
		return errors.New(cacheUsage)
	}
	report, err := r.c.CacheList(proto.CacheListParams{OlderDays: *older})
	if err != nil {
		return err
	}
	switch cmd {
	case "list":
		return r.cacheShow(report)
	case "clean":
		p := proto.CacheCleanParams{OlderDays: *older, Items: items}
		if *kinds != "" {
			p.Kinds = strings.Split(*kinds, ",")
		}
		if len(p.Kinds) == 0 && len(p.Items) == 0 {
			return errors.New(cacheUsage)
		}
		if !*yes {
			return r.cacheWouldGo(report, p)
		}
		res, err := r.c.CacheClean(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(r.out, "Freed %s from %d items\n", textfmt.Bytes(uint64(max(res.Freed, 0))), res.Done)
		for _, f := range res.Failed {
			fmt.Fprintf(r.out, "Not removed: %s: %s\n", f.Path, f.Why)
		}
		if len(res.Failed) > 0 {
			return fmt.Errorf("cache: %d items not removed", len(res.Failed))
		}
		return nil
	}
	return errors.New(cacheUsage)
}

func (r runner) cacheShow(report proto.CacheReport) error {
	fmt.Fprintf(r.out, "This host's enowX folder takes %s\n\n", textfmt.Bytes(uint64(report.Total)))
	w := tabwriter.NewWriter(r.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KIND\tCAN FREE\tNAME")
	for _, k := range report.Kinds {
		size := textfmt.Bytes(uint64(k.Bytes))
		if k.Info {
			size = "not offered"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", k.ID, size, k.Name)
		if k.PerItem {
			for _, it := range k.Items {
				line := it.Note
				if it.Blocked != "" {
					line = it.Blocked
				}
				fmt.Fprintf(w, "  %s\t%s\t%s\n", it.Path, textfmt.Bytes(uint64(it.Bytes)), line)
			}
		}
	}
	return w.Flush()
}

// cacheWouldGo says what a clean would take, without taking it.
func (r runner) cacheWouldGo(report proto.CacheReport, p proto.CacheCleanParams) error {
	var total int64
	for _, k := range report.Kinds {
		whole := !k.PerItem && !k.Info && slices.Contains(p.Kinds, k.ID)
		for _, it := range k.Items {
			if !k.Info && (whole || slices.Contains(p.Items, it.Path)) && it.Blocked == "" {
				fmt.Fprintf(r.out, "%s\t%s\n", it.Path, textfmt.Bytes(uint64(it.Bytes)))
				total += it.Bytes
			}
		}
		if whole && k.Warning != "" {
			fmt.Fprintln(r.out, k.Name+": "+k.Warning)
		}
	}
	fmt.Fprintf(r.out, "Would free %s. Add --yes to do it: removing is real.\n", textfmt.Bytes(uint64(total)))
	return nil
}

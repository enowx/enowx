package cli

import (
	"errors"
	"fmt"

	"github.com/enowx/enowx/core/goa"
	"github.com/enowx/enowx/proto"
)

// goaStall shows how long a task may not move before GOA is told, on this
// host, and changes the limits given.
func (r runner) goaStall(args []string) error {
	fs := flagSet("goa stall")
	blank := fs.String("blank", "", "")
	idle := fs.String("idle", "", "")
	stuck := fs.String("stuck", "", "")
	if words, err := parseAll(fs, args); err != nil || len(words) > 0 {
		return errors.New("usage: enowx-app goa stall [--blank D] [--idle D] [--stuck D] (90s, 10m, 1h; 0 its default)")
	}
	var l proto.GOAStall
	if err := r.c.Call(proto.MethodGOAStall, nil, &l); err != nil {
		return err
	}
	changed := false
	for _, f := range []struct {
		text  string
		limit *int
	}{{*blank, &l.Blank}, {*idle, &l.Idle}, {*stuck, &l.Stuck}} {
		if f.text == "" {
			continue
		}
		seconds, err := goa.ParseStall(f.text)
		if err != nil {
			return err
		}
		*f.limit, changed = seconds, true
	}
	if changed {
		if err := r.c.Call(proto.MethodGOAStallSave, l, &l); err != nil {
			return err
		}
	}
	fmt.Fprintf(r.out, "A task counts as stalled on this host after:\n  blank %-7s its screen still empty\n  idle  %-7s its session hibernated or gone, or its agent not at work\n  stuck %-7s its agent says it works, but nothing shows or uses the CPU\n",
		goa.StallText(l.Blank), goa.StallText(l.Idle), goa.StallText(l.Stuck))
	return nil
}

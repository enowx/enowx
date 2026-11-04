package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/enowx/enowx/proto"
)

const syncUsage = `usage: enowx-app sync COMMAND
  (none) | status        this host's synced data, and its syncs with the other hosts
  now [--no-wait]        ask the enowX app to sync this host now, and wait for it
  hosts                  the hosts this one synced with, and when
  conflicts              changes made on two hosts that don't merge
  show ID [--version N]  print a version of a conflict (the first unless N)
  resolve ID --keep HOST|N|both
                         settle a conflict: keep a host's version (or the Nth), or both
  trash                  synced files deleted lately, kept 30 days
  restore ID             bring a deleted file back
  forget --yes           remove the synced data from this host (to its trash)

The user's notes, groups, canvases, settings and vault are synced between
hosts by the enowX app, which reaches them; project files are not`

// sync shows and drives how the user's data is synced between hosts.
func (r runner) sync(args []string) error {
	cmd := "status"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	keep := fs.String("keep", "", "")
	noWait := fs.Bool("no-wait", false, "")
	yes := fs.Bool("yes", false, "")
	version := fs.Int("version", 1, "")
	arg, err := parseWithArg(fs, args)
	if err != nil {
		return errors.New(syncUsage)
	}
	switch cmd {
	case "status":
		st, err := r.c.SyncStatus()
		if err != nil {
			return err
		}
		fmt.Fprintf(r.out, "%s (%s): %d files, %d bytes synced", st.Name, st.ID, st.Files, st.Size)
		if st.Conflicts > 0 {
			fmt.Fprintf(r.out, ", %d conflict(s) to settle (sync conflicts)", st.Conflicts)
		}
		fmt.Fprintln(r.out)
		return r.syncPeers(st.Peers)
	case "hosts":
		st, err := r.c.SyncStatus()
		if err != nil {
			return err
		}
		return r.syncPeers(st.Peers)
	case "now":
		before, err := r.c.SyncStatus()
		if err != nil {
			return err
		}
		if err := r.c.SyncRequest(); err != nil {
			return err
		}
		if *noWait {
			fmt.Fprintln(r.out, "asked")
			return nil
		}
		deadline := time.Now().Add(time.Minute)
		for time.Now().Before(deadline) {
			time.Sleep(time.Second)
			st, err := r.c.SyncStatus()
			if err != nil {
				return err
			}
			if synced := newerPeers(before.Peers, st.Peers); len(synced) > 0 {
				return r.syncPeers(synced)
			}
		}
		return errors.New("sync: no enowX app synced this host within a minute: it syncs when the app on the user's computer is open and connected to it")
	case "conflicts":
		list, err := r.c.SyncConflicts()
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Fprintln(r.out, "no conflicts")
		}
		for _, c := range list {
			name := c.Path
			if c.Title != "" {
				name = c.Title + " (" + c.Path + ")"
			}
			var vs []string
			for i, v := range c.Versions {
				s := fmt.Sprintf("%d: %s's", i+1, v.Host)
				if v.Deleted {
					s += " (deleted)"
				}
				vs = append(vs, s)
			}
			fmt.Fprintf(r.out, "%s\t%s\t%s\n", c.ID, name, strings.Join(vs, ", "))
		}
		return nil
	case "show":
		if arg == "" {
			return errors.New(syncUsage)
		}
		v, err := r.c.SyncConflictVersion(arg, *version)
		if err != nil {
			return err
		}
		if v.Deleted {
			fmt.Fprintf(r.out, "%s deleted it\n", v.Host)
			return nil
		}
		_, err = r.out.Write(v.Data)
		return err
	case "resolve":
		if arg == "" || *keep == "" {
			return errors.New(syncUsage)
		}
		if err := r.c.SyncResolve(arg, *keep); err != nil {
			return err
		}
		_ = r.c.SyncRequest()
		return nil
	case "trash":
		list, err := r.c.SyncTrash()
		if err != nil {
			return err
		}
		for _, it := range list {
			name := it.Path
			if it.Title != "" {
				name = it.Title + " (" + it.Path + ")"
			}
			fmt.Fprintf(r.out, "%s\t%s\t%s\n", it.ID, name, it.Deleted.Local().Format("2006-01-02 15:04"))
		}
		return nil
	case "restore":
		if arg == "" {
			return errors.New(syncUsage)
		}
		if err := r.c.SyncRestore(arg); err != nil {
			return err
		}
		_ = r.c.SyncRequest()
		return nil
	case "forget":
		if !*yes {
			return errors.New("sync: forget moves every note, canvas and setting of this host to its trash; run it with --yes once the user agreed")
		}
		return r.c.SyncForget()
	case "help":
		fmt.Fprintln(r.out, syncUsage)
		return nil
	}
	return errors.New(syncUsage)
}

func (r runner) syncPeers(peers []proto.SyncPeerStatus) error {
	if len(peers) == 0 {
		fmt.Fprintln(r.out, "not synced with another host yet")
		return nil
	}
	w := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
	for _, p := range peers {
		state := fmt.Sprintf("%d sent, %d received, %d merged", p.Sent, p.Received, p.Merged)
		if p.Conflict > 0 {
			state += fmt.Sprintf(", %d conflict(s)", p.Conflict)
		}
		if p.Err != "" {
			state = "failed: " + p.Err
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, p.Last.Local().Format("2006-01-02 15:04:05"), state)
	}
	return w.Flush()
}

// newerPeers are the peers of after that synced since before.
func newerPeers(before, after []proto.SyncPeerStatus) []proto.SyncPeerStatus {
	var out []proto.SyncPeerStatus
	for _, p := range after {
		i := slices.IndexFunc(before, func(b proto.SyncPeerStatus) bool { return b.Peer == p.Peer })
		if i < 0 || p.Last.After(before[i].Last) {
			out = append(out, p)
		}
	}
	return out
}

// syncTools are the syncs of the user's data as MCP tools.
var syncTools = []mcpTool{
	{name: "sync_status", description: "How the user's data (notes, groups, canvases, settings, vault) is synced between this host and the others: files, conflicts, each host's last sync.",
		args: func(map[string]any) []string { return []string{"status"} }},
	{name: "sync_now", description: "Ask the enowX app to sync this host's data with the other hosts now, and wait until it did (it needs the app open and connected).",
		args: func(map[string]any) []string { return []string{"now"} }},
	{name: "sync_conflicts", description: "Changes made on two hosts that didn't merge: ID, note or file, and the hosts' versions.",
		args: func(map[string]any) []string { return []string{"conflicts"} }},
	{name: "sync_resolve", description: "Settle a sync conflict: keep one host's version (by host name or number from sync_conflicts), or both (the other becomes a copy). Ask the user which, unless they said.",
		props: map[string]mcpProp{"id": {"string", "The conflict's ID"}, "keep": {"string", "A host's name, a version's number, or both"}}, required: []string{"id", "keep"},
		args: func(a map[string]any) []string { return []string{"resolve", str(a, "id"), "--keep", str(a, "keep")} }},
	{name: "sync_trash", description: "Synced files (notes, …) deleted lately on any host, kept 30 days: ID, name, when.",
		args: func(map[string]any) []string { return []string{"trash"} }},
	{name: "sync_restore", description: "Bring a deleted note or file back, by its ID from sync_trash.",
		props: map[string]mcpProp{"id": {"string", "The deleted file's ID"}}, required: []string{"id"},
		args: func(a map[string]any) []string { return []string{"restore", str(a, "id")} }},
}

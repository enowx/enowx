package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/version"
)

const diagnosticsUsage = `usage: enowx-app diagnostics [--lines N] [--all] [--bundle FILE]
  (none)         the recent connection events of this host's app and daemon
  --all          every recent line of their logs, not only connections'
  --lines N      how many lines (default 50)
  --bundle FILE  write all the logs, masked, as a zip for a bug report`

// diagnostics shows what this host's logs say. It reads the files itself, for it to
// work with the daemon down.
func (r runner) diagnostics(args []string) error {
	fs := flag.NewFlagSet("diagnostics", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	lines := fs.Int("lines", 50, "")
	all := fs.Bool("all", false, "")
	bundle := fs.String("bundle", "", "")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return errors.New(diagnosticsUsage)
	}
	dir, err := transport.Dir()
	if err != nil {
		return err
	}
	files := diag.Collect(dir)
	if *bundle != "" {
		f, err := os.OpenFile(*bundle, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := diag.WriteZip(f, "enowX "+version.String, map[string][]proto.DiagFile{"this host": files}); err != nil {
			return err
		}
		fmt.Fprintf(r.out, "Wrote %d logs to %s\n", len(files), *bundle)
		return nil
	}
	found := diag.Recent(files, *lines)
	if *all {
		for _, f := range files {
			if f.Name == diag.AppLog || f.Name == diag.DaemonLog {
				text := strings.Split(strings.TrimRight(f.Text, "\n"), "\n")
				fmt.Fprintf(r.out, "== %s (%d bytes)\n%s\n", f.Name, f.Size, strings.Join(text[max(len(text)-*lines, 0):], "\n"))
			}
		}
		return nil
	}
	if len(found) == 0 {
		fmt.Fprintln(r.out, "No connection events logged yet.")
	}
	for _, line := range found {
		fmt.Fprintln(r.out, line)
	}
	return nil
}

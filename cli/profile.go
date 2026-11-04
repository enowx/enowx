package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/core/selfprofile"
	"github.com/enowx/enowx/proto"
)

const profileUsage = `usage: enowx-app profile [--cpu SECONDS] [--json] [--out DIR]
  (none)         what this host's running daemon and the app attached to it hold now
  --cpu SECONDS  also measure their CPU over that window (at most 60)
  --json         the numbers as JSON
  --out DIR      also write pprof files there, for go tool pprof, and profile.txt
It changes nothing and restarts nothing`

// profile prints what the running daemon and app hold and use (docs/performance.md).
func (r runner) profile(args []string) error {
	fs := flag.NewFlagSet("profile", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cpu := fs.Int("cpu", 0, "")
	asJSON := fs.Bool("json", false, "")
	out := fs.String("out", "", "")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 || *cpu < 0 || *cpu > 60 {
		return errors.New(profileUsage)
	}
	var got proto.Profile
	if err := r.c.Call(proto.MethodDaemonProfile, proto.ProfileParams{CPUSeconds: *cpu, Raw: *out != ""}, &got); err != nil {
		return fmt.Errorf("profile: %w", err)
	}
	if *out != "" {
		if err := writeProfileFiles(*out, got); err != nil {
			return err
		}
	}
	if got.Daemon.Raw != nil {
		got.Daemon.Raw = nil
	}
	if got.App != nil {
		got.App.Raw = nil
	}
	if *asJSON {
		b, _ := json.MarshalIndent(got, "", "  ")
		fmt.Fprintln(r.out, string(b))
		return nil
	}
	selfprofile.Write(r.out, got)
	return nil
}

func writeProfileFiles(dir string, p proto.Profile) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	put := func(name string, data []byte) error {
		if len(data) == 0 {
			return nil
		}
		return os.WriteFile(filepath.Join(dir, name), data, 0o600)
	}
	raws := map[string]*proto.ProfileRaw{"daemon": p.Daemon.Raw}
	if p.App != nil {
		raws["app"] = p.App.Raw
	}
	for who, raw := range raws {
		if raw == nil {
			continue
		}
		for name, data := range map[string][]byte{"heap": raw.Heap, "goroutine": raw.Goroutines, "cpu": raw.CPU} {
			if err := put(who+"-"+name+".pprof", data); err != nil {
				return err
			}
		}
	}
	return put(diag.ProfileFile, []byte(selfprofile.Text(p)))
}

// bundleProfile is the running programs' profile for a bundle, if the daemon answers:
// a bundle is also for when it doesn't.
func bundleProfile() (proto.DiagFile, bool) {
	c, err := Dial()
	if err != nil {
		return proto.DiagFile{}, false
	}
	defer c.Close()
	done := make(chan proto.DiagFile, 1)
	go func() {
		var got proto.Profile
		if c.Call(proto.MethodDaemonProfile, proto.ProfileParams{}, &got) == nil {
			done <- proto.DiagFile{Name: diag.ProfileFile, Text: diag.Scrub(selfprofile.Text(got))}
		}
	}()
	select {
	case f := <-done:
		return f, true
	case <-time.After(45 * time.Second):
		return proto.DiagFile{}, false
	}
}

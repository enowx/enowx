// Command terminalpatch runs Go with the pinned terminal source addition in a
// private module copy. It never patches cached module source.
package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/internal/quietcmd"
)

//go:embed guarded_input.go.txt
var source []byte

//go:embed guarded_input_test.go.txt
var tests []byte

func main() {
	out := flag.String("out", "", "absolute private go.mod output path (required)")
	flag.Parse()
	if err := run(*out, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "terminalpatch:", err)
		if e, ok := err.(interface{ ExitCode() int }); ok {
			os.Exit(e.ExitCode())
		}
		os.Exit(1)
	}
}

func run(out string, args []string) error {
	if out != "" {
		return prepare(out)
	}
	if len(args) == 0 {
		return fmt.Errorf("use -- <go arguments>, or -out /absolute/path/go.mod")
	}
	// Nested Go commands (MyGo, cross builds) inherit this same modfile.
	if os.Getenv("ENOWX_TERMINAL_PATCH") == "v0.6.2" {
		return runGo(args, "")
	}
	if strings.Contains(os.Getenv("GOFLAGS"), "-modfile") {
		return fmt.Errorf("cannot compose with an existing -modfile")
	}
	dir, err := os.MkdirTemp("", "enowx-terminalpatch-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	out = filepath.Join(dir, "go.mod")
	if err := prepare(out); err != nil {
		return err
	}
	return runGo(args, out)
}

func runGo(args []string, modfile string) error {
	cmd := quietcmd.Command("go", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = os.Environ()
	if modfile != "" {
		cmd.Env = append(cmd.Env, "GOFLAGS="+os.Getenv("GOFLAGS")+fmt.Sprintf(" %q", "-modfile="+modfile), "ENOWX_TERMINAL_PATCH=v0.6.2")
	}
	return cmd.Run()
}

func prepare(out string) error {
	if !filepath.IsAbs(out) {
		return fmt.Errorf("-out must be an absolute path")
	}
	cmd := quietcmd.Command("go", "list", "-m", "-json", "github.com/egoist/mygo")
	b, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("locate terminal module: %w", err)
	}
	var mod struct {
		Dir, Version string
		Replace      *json.RawMessage
	}
	if err := json.Unmarshal(b, &mod); err != nil {
		return err
	}
	if mod.Version != "v0.6.2" || mod.Replace != nil {
		return fmt.Errorf("requires unmodified github.com/egoist/mygo v0.6.2; review patch when upgrading")
	}
	if mod.Dir == "" {
		cmd = quietcmd.Command("go", "mod", "download", "-json", "github.com/egoist/mygo")
		b, err = cmd.Output()
		if err != nil {
			return fmt.Errorf("download pinned terminal module: %w", err)
		}
		if err := json.Unmarshal(b, &mod); err != nil {
			return err
		}
		if mod.Dir == "" {
			return fmt.Errorf("pinned terminal module has no source directory")
		}
	}
	b, err = os.ReadFile(filepath.Join(mod.Dir, "plugins", "terminal", "terminal.go"))
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != "7902afddc644b21ffcbc0d567a144b430ef579c8dd538f80499c99beb412738d" {
		return fmt.Errorf("terminal owner source differs from reviewed v0.6.2; review locking and queue layout")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0700); err != nil {
		return err
	}
	// Go forbids overlays beneath GOMODCACHE. Stage the pinned module in the
	// output fixture, rather than modifying the cache or committing a fork.
	private := filepath.Join(filepath.Dir(out), "mygo-v0.6.2")
	if err := os.CopyFS(private, os.DirFS(mod.Dir)); err != nil {
		return fmt.Errorf("stage private terminal dependency: %w", err)
	}
	for name, data := range map[string][]byte{
		"enowx_guarded_input.go":      source,
		"enowx_guarded_input_test.go": tests,
	} {
		dest := filepath.Join(private, "plugins", "terminal", name)
		if err := os.WriteFile(dest, data, 0600); err != nil {
			return err
		}
	}
	b, err = os.ReadFile("go.mod")
	if err != nil {
		return err
	}
	b = append(b, []byte(fmt.Sprintf("\nreplace github.com/egoist/mygo => %q\n", filepath.ToSlash(private)))...)
	if err := os.WriteFile(out, b, 0600); err != nil {
		return err
	}
	b, err = os.ReadFile("go.sum")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out[:len(out)-len(filepath.Ext(out))]+".sum", b, 0600); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "terminalpatch: private MyGo v0.6.2 plus guarded input API at %s; module cache untouched\n", private)
	return nil
}

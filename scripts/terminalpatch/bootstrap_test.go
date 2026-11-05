package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/enowx/enowx/internal/quietcmd"
)

// Only the expensive build is replaced: bootstrap, preparation and Go's flag
// parser run for real. Exit 73 stops the scripts before packaging or Docker.
func TestMain(m *testing.M) {
	if os.Getenv("ENOWX_BOOTSTRAP_FAKE_GO") == "1" {
		args := os.Args[1:]
		if len(args) > 0 && (args[0] == "tool" || args[0] == "build" && args[len(args)-1] == "./cmd/enowxd" || args[0] == "run" && len(args) > 1 && args[1] == "./cmd/enowxd") {
			cmd := quietcmd.Command(os.Getenv("ENOWX_BOOTSTRAP_REAL_GO"), "list", "-m", "-json", "github.com/egoist/mygo")
			b, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Fprint(os.Stderr, string(b))
				os.Exit(1)
			}
			var module struct{ Replace *struct{ Dir string } }
			if err := json.Unmarshal(b, &module); err != nil || module.Replace == nil {
				fmt.Fprintln(os.Stderr, "child did not select the private module:", string(b), err)
				os.Exit(1)
			}
			if _, err := os.Stat(filepath.Join(module.Replace.Dir, "plugins", "terminal", "enowx_guarded_input.go")); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			record, _ := json.Marshal(childBuild{Args: args, OS: os.Getenv("GOOS"), Arch: os.Getenv("GOARCH"), Flags: os.Getenv("GOFLAGS"), Module: module.Replace.Dir})
			if err := os.WriteFile(os.Getenv("ENOWX_BOOTSTRAP_RECORD"), record, 0600); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(73)
		}
		cmd := quietcmd.Command(os.Getenv("ENOWX_BOOTSTRAP_REAL_GO"), args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			if e, ok := err.(interface{ ExitCode() int }); ok {
				os.Exit(e.ExitCode())
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type childBuild struct {
	Args   []string
	OS     string
	Arch   string
	Flags  string
	Module string
}

func TestBuildScriptBootstrap(t *testing.T) {
	for _, target := range []bool{false, true} {
		for _, script := range []string{"daemons.sh", "release-local.sh", "test-distros.sh", "test-service.sh", "package.sh"} {
			t.Run(fmt.Sprintf("%s/cross=%v", script, target), func(t *testing.T) {
				root, shell := bootstrapFixture(t)
				osName, arch := runtime.GOOS, runtime.GOARCH
				if target {
					osName, arch = "linux", "arm64"
					if runtime.GOOS == osName && runtime.GOARCH == arch {
						osName, arch = "windows", "amd64"
					}
				}
				t.Setenv("GOOS", osName)
				t.Setenv("GOARCH", arch)
				cmd := quietcmd.Command(shell, "scripts/"+script, "test-version")
				cmd.Dir = root
				b, err := cmd.CombinedOutput()
				if e, ok := err.(interface{ ExitCode() int }); !ok || e.ExitCode() != 73 {
					t.Fatalf("wanted mocked build exit 73; got %v\n%s", err, b)
				}
				var child childBuild
				b, err = os.ReadFile(os.Getenv("ENOWX_BOOTSTRAP_RECORD"))
				if err != nil || json.Unmarshal(b, &child) != nil {
					t.Fatalf("child record: %s %v", b, err)
				}
				// Daemon and Docker routes deliberately select Linux themselves.
				wantOS, wantArch := osName, arch
				if script == "daemons.sh" || script == "package.sh" || strings.HasPrefix(script, "test-") {
					wantOS, wantArch = "linux", "amd64"
				}
				if child.OS != wantOS || child.Arch != wantArch || !strings.Contains(child.Flags, "-p=1") {
					t.Fatalf("child target/flags changed: %+v; want %s/%s", child, wantOS, wantArch)
				}
				t.Logf("real preparer and Go parser selected %s; mocked child retained %s/%s and -p=1", child.Module, child.OS, child.Arch)
			})
		}
	}
}

func TestBootstrapWrapperTarget(t *testing.T) {
	root, shell := bootstrapFixture(t)
	selected := filepath.Join(filepath.Dir(os.Getenv("HOST_GO")), "selected go.exe")
	b, err := os.ReadFile(os.Getenv("HOST_GO"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(selected, b, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOST_GO", filepath.ToSlash(selected))
	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "arm64")
	cmd := quietcmd.Command(shell, "scripts/terminalpatch.sh", "--", "tool", "mygo", "build", "-platform", "darwin/universal", "cmd/enowx")
	cmd.Dir = root
	b, err = cmd.CombinedOutput()
	if e, ok := err.(interface{ ExitCode() int }); !ok || e.ExitCode() != 73 {
		t.Fatalf("wrapper: %v\n%s", err, b)
	}
	b, err = os.ReadFile(os.Getenv("ENOWX_BOOTSTRAP_RECORD"))
	var child childBuild
	if err != nil || json.Unmarshal(b, &child) != nil || child.OS != "linux" || child.Arch != "arm64" || strings.Join(child.Args, " ") != "tool mygo build -platform darwin/universal cmd/enowx" {
		t.Fatalf("target or arguments changed: %s %v", b, err)
	}
	if _, err := os.Stat(child.Module); !os.IsNotExist(err) {
		t.Fatalf("wrapper did not remove its private copy: %v", err)
	}
}

func TestMakeBootstrapTarget(t *testing.T) {
	makeBin, err := exec.LookPath("make")
	if err != nil {
		t.Skip("GNU make is not installed")
	}
	root, _ := bootstrapFixture(t)
	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "arm64")
	cmd := quietcmd.Command(makeBin, "run-daemon")
	cmd.Dir = root
	b, err := cmd.CombinedOutput()
	// GNU make maps the child's sentinel to its own exit code 2.
	if e, ok := err.(interface{ ExitCode() int }); !ok || e.ExitCode() != 2 {
		t.Fatalf("make: %v\n%s", err, b)
	}
	b, err = os.ReadFile(os.Getenv("ENOWX_BOOTSTRAP_RECORD"))
	var child childBuild
	if err != nil || json.Unmarshal(b, &child) != nil || child.OS != "linux" || child.Arch != "arm64" {
		t.Fatalf("make target lost: %s %v", b, err)
	}
}

func bootstrapFixture(t *testing.T) (string, string) {
	t.Helper()
	shell := os.Getenv("ENOWX_TEST_SH")
	if shell == "" {
		if runtime.GOOS == "windows" {
			t.Skip("set ENOWX_TEST_SH to Git Bash for shell bootstrap checks")
		}
		shell = "/bin/sh"
	}
	// Every script is run in a disposable project, never in the checkout.
	root := filepath.Join(t.TempDir(), "project with spaces")
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"scripts/terminalpatch", "internal/quietcmd"} {
		if err := os.CopyFS(filepath.Join(root, dir), os.DirFS(filepath.Join("../..", dir))); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"go.mod", "go.sum", "Makefile", "scripts/daemons.sh", "scripts/release-local.sh", "scripts/test-distros.sh", "scripts/test-service.sh", "scripts/package.sh", "scripts/terminalpatch.sh"} {
		b, err := os.ReadFile(filepath.Join("../..", name))
		if os.IsNotExist(err) && name == "scripts/terminalpatch.sh" {
			continue // The regression also runs against the old source.
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), b, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd/enowx"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd/enowx/mygo.json"), []byte("{\"version\": \"test\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(filepath.Dir(root), "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"docker": "#!/bin/sh\nif [ \"$1\" = info ]; then echo x86_64; fi\n",
		"make":   "#!/bin/sh\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	// An executable fake Go can be invoked both by the shell and quietcmd
	// on Windows; it forwards every inexpensive operation to the real Go.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(bin, "go.exe")
	if runtime.GOOS != "windows" {
		fake = filepath.Join(bin, "go")
	}
	if err := os.WriteFile(fake, b, 0700); err != nil {
		t.Fatal(err)
	}
	realGo := os.Getenv("ENOWX_TEST_GO")
	if realGo == "" {
		cmd := quietcmd.Command("go", "env", "GOROOT")
		b, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		realGo = filepath.Join(strings.TrimSpace(string(b)), "bin", "go")
		if runtime.GOOS == "windows" {
			realGo += ".exe"
		}
	}
	// Forward slashes work for Go on Windows as well as the shell's PATH.
	t.Setenv("HOST_GO", filepath.ToSlash(fake))
	t.Setenv("ENOWX_BOOTSTRAP_FAKE_GO", "1")
	t.Setenv("ENOWX_BOOTSTRAP_REAL_GO", realGo)
	t.Setenv("ENOWX_BOOTSTRAP_RECORD", filepath.Join(root, "child.json"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOFLAGS", "-p=1")
	t.Setenv("ENOWX_TERMINAL_PATCH", "")
	for _, name := range []string{"HOME", "USERPROFILE", "ENOWX_HOME", "CODEX_HOME", "APPDATA", "LOCALAPPDATA", "TMP", "TEMP", "TMPDIR", "GOTMPDIR"} {
		dir := filepath.Join(root, name+" with spaces")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv(name, filepath.ToSlash(dir))
	}
	return root, shell
}

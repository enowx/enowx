package hooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// What changes the agent's shell stays in it; what would stop working wrapped
// isn't wrapped.
func TestWrapTrim(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"go test ./...", "enowx decide trim -- 'go test ./...'"},
		{"cd ui && go test ./...", "cd ui && enowx decide trim -- 'go test ./...'"},
		{"export A=1; B=2 make", "export A=1; enowx decide trim -- 'B=2 make'"},
		{"A=1; go test", "enowx decide trim -- 'A=1; go test'"},
	} {
		got, ok := WrapTrim("enowx", c.in)
		if !ok || got != c.want {
			t.Errorf("%q: %q %v, want %q", c.in, got, ok, c.want)
		}
	}
	for _, in := range []string{
		"cd ui", "cd ui &&", "go test && cd ui", "npm run dev", "pnpm dev", "docker compose up", "tail -f log", "sleep 9 &",
		"source env.sh", "export A=1", "vim x", "GOFLAGS=-x && cd a; ls -la", "enowx decide trim -- 'go test'",
	} {
		if got, ok := WrapTrim("enowx", in); ok {
			t.Errorf("%q wrapped: %q", in, got)
		}
	}
}

// A command wrapped does what it did: the whole of it reaches a shell, through its
// quoting, whatever it holds. Run by a shell, as a stand-in for enowX's wrapper.
func TestWrapTrimKeepsWhatACommandDoes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "trimmer")
	if err := os.WriteFile(bin, []byte("#!/bin/bash\nshift 3\nexec bash -c \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, cmd := range map[string]string{
		"heredoc with ;":   "python3 --version >/dev/null\ncat > f <<'EOF'\nR=os.environ[\"R\"]; P=1; Q='x'\nEOF\ncat f",
		"cd then heredoc":  "cd sub\ncat > f <<'EOF'\nR=1; P=2 && S=3\nEOF\ncat f",
		"lead and heredoc": "cd sub; cat > f <<EOF\na; b\nEOF\ncat f",
		"unquoted heredoc": "V=9\ncat <<EOF\nv=$V; w\nEOF",
		"for":              "for i in a b; do echo $i; done",
		"for after cd":     "cd sub\nfor i in a b; do echo $i; done",
		"for lines":        "for i in a b\ndo\n  echo \"$i;\"\ndone",
		"while":            "i=0; while [ $i -lt 2 ]; do echo $i; i=$((i+1)); done",
		"if":               "if true; then echo y; else echo n; fi",
		"quote holds ;":    "export A='x;y'; echo \"$A\"",
		"cd quote holds ;": "cd 'a;b' && pwd | sed 's/.*\\///'",
		"var holds &&":     "A=\"p && q\" && echo \"$A\"",
		"single quotes":    "echo 'it'\\''s; a' \"b;c\" d\\;e",
		"substitution":     "A=$(echo 1; echo 2); echo $A",
		"subshell":         "(echo a; echo b) | tr a-z A-Z",
		"comment":          "echo a # not; this",
		"lead comment":     "cd sub # here; not this\necho b",
		"pipe":             "printf 'a;b\\n' | cut -d';' -f2",
	} {
		t.Run(name, func(t *testing.T) {
			run := func(c string) string {
				work := t.TempDir()
				for _, d := range []string{"sub", "a;b"} {
					if err := os.Mkdir(filepath.Join(work, d), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				sh := exec.Command("bash", "-c", c)
				sh.Dir = work
				out, err := sh.CombinedOutput()
				text := fmt.Sprintf("%s|%v", out, err)
				for _, f := range []string{"f", "sub/f"} {
					if data, err := os.ReadFile(filepath.Join(work, f)); err == nil {
						text += "|" + f + ":" + string(data)
					}
				}
				return text
			}
			wrapped, ok := WrapTrim(bin, cmd)
			if !ok {
				return // left alone: it can't change
			}
			if got, want := run(wrapped), run(cmd); got != want {
				t.Errorf("wrapped as %q\n got %q\nwant %q", wrapped, got, want)
			}
		})
	}
}

// What changes the shell further on stays unwrapped, wherever it is.
func TestWrapTrimLeavesShellChangesAlone(t *testing.T) {
	for _, in := range []string{"ls\ncd x", "for d in a; do cd $d; done", "if true; then cd x; fi", "sleep 9 &\nwait"} {
		if got, ok := WrapTrim("enowx", in); ok {
			t.Errorf("%q wrapped: %q", in, got)
		}
	}
}

func TestExactRead(t *testing.T) {
	for _, in := range []string{"cat a.go", "sed -n 1,9p a.go", "git diff", "git show HEAD", "cd x && git diff"} {
		if !ExactRead(in) {
			t.Errorf("%q", in)
		}
	}
	for _, in := range []string{"go test ./...", "git status", "ls", "catalog"} {
		if ExactRead(in) {
			t.Errorf("%q", in)
		}
	}
}

// The narrow list sees test and build commands behind what comes before them.
func TestTrimCommandsSeeBehindALead(t *testing.T) {
	re := regexp.MustCompile(TrimCommands)
	for _, in := range []string{"go test ./...", "cd ui && go test ./ui", "export GOFLAGS=-x; go build ./...", "FOO=1 make", "make -j2", "cd a; cd b && pnpm install"} {
		if !re.MatchString(in) {
			t.Errorf("%q not matched", in)
		}
	}
	if re.MatchString("ls -la") || re.MatchString(strings.Repeat("x", 9)) {
		t.Error("matched too much")
	}
}

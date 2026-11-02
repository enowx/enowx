package hooks

import (
	"regexp"
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
		{"GOFLAGS=-x && cd a; ls -la", "GOFLAGS=-x && cd a; enowx decide trim -- 'ls -la'"},
	} {
		got, ok := WrapTrim("enowx", c.in)
		if !ok || got != c.want {
			t.Errorf("%q: %q %v, want %q", c.in, got, ok, c.want)
		}
	}
	for _, in := range []string{
		"cd ui", "cd ui &&", "go test && cd ui", "npm run dev", "pnpm dev", "docker compose up", "tail -f log", "sleep 9 &",
		"source env.sh", "export A=1", "vim x", "enowx decide trim -- 'go test'",
	} {
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

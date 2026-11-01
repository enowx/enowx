package accounts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/agent"
)

// A new agent can't be added without saying how it takes a config, or why not:
// the list in Settings → Agents reads this.
func TestEveryAgentSaysHowItTakesAConfig(t *testing.T) {
	for _, info := range agent.Presets {
		way, ok := agentWays[info.ID]
		if !ok {
			t.Errorf("%s isn't in agentWays: add how it takes a config, or why it can't", info.ID)
			continue
		}
		if way.Checked == "" {
			t.Errorf("%s doesn't say how it was checked", info.ID)
		}
		if w := WayOf(info.ID); w.None() && w.Why == "" {
			t.Errorf("%s has no config and no reason", info.ID)
		}
	}
}

// An account read from docs says so wherever it is offered, and only the two
// run for real have no such mark.
func TestAccountsFromDocsAreMarked(t *testing.T) {
	for id, k := range kinds {
		way := WayOf(id)
		if !way.Account || k.env == "" || k.home == "" || len(k.own) == 0 {
			t.Errorf("%s: %+v, %+v", id, way, k)
		}
		if !k.docsOnly {
			if id != "claude" && id != "codex" {
				t.Errorf("%s has no real sign-in behind it, so it must be docsOnly", id)
			}
			continue
		}
		if way.Checked != "docs" || !strings.Contains(way.Why, "read from its docs, not signed in") {
			t.Errorf("%s isn't marked as read from docs: %+v", id, way)
		}
	}
}

// A session on an account gets the agent's home variable, the user's other
// settings through links, and a sign-in that stays out of the user's own.
func TestAccountHomesFromDocs(t *testing.T) {
	cases := []struct {
		agent, env, home, signIn string
		signInDir                bool
	}{
		{"kimi", "KIMI_CODE_HOME", ".kimi-code", "credentials", true},
		{"mistral-vibe", "VIBE_HOME", ".vibe", ".env", false},
		{"pi", "PI_CODING_AGENT_DIR", filepath.Join(".pi", "agent"), "auth.json", false},
	}
	for _, tc := range cases {
		t.Run(tc.agent, func(t *testing.T) {
			user := t.TempDir()
			t.Setenv("HOME", user)
			t.Setenv("USERPROFILE", user)
			own := filepath.Join(user, tc.home)
			if err := os.MkdirAll(own, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(own, "settings"), []byte("mine"), 0o600); err != nil {
				t.Fatal(err)
			}
			signIn := filepath.Join(own, tc.signIn)
			if tc.signInDir {
				if err := os.Mkdir(signIn, 0o700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(signIn, []byte("the user's"), 0o600); err != nil {
				t.Fatal(err)
			}

			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			acc, err := s.Add(tc.agent, "Work")
			if err != nil {
				t.Fatal(err)
			}
			env, err := s.Env(tc.agent, acc.ID)
			if err != nil {
				t.Fatal(err)
			}
			home := s.Home(acc.ID)
			if len(env) != 1 || env[0] != tc.env+"="+home {
				t.Fatalf("env: %q", env)
			}
			if got, err := os.ReadFile(filepath.Join(home, "settings")); err != nil || string(got) != "mine" {
				t.Fatalf("the user's settings aren't shared: %q %v", got, err)
			}
			if _, err := os.Lstat(filepath.Join(home, tc.signIn)); !os.IsNotExist(err) {
				t.Fatalf("the user's sign-in reached the account: %v", err)
			}
		})
	}
}

// A team's member is kept from an account no real sign-in has tried.
func TestMembersGetNoAccountFromDocs(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for id, k := range kinds {
		_, err := s.AddMember(id, "Team", "m1")
		if k.docsOnly && err == nil {
			t.Errorf("%s gave a member an account", id)
		}
		if !k.docsOnly && err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
}

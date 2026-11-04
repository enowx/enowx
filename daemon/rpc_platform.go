package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/enowx/enowx/proto"
)

// The user's system prompt for Claude Platform, a file of this host's: the agent reads
// it when a session starts, in place of its built-in one.

// maxPlatformPrompt is the longest it may be: it goes with every request of a session.
const maxPlatformPrompt = 128 << 10

var platformHandlers = map[string]handler{
	proto.MethodPlatformPrompt: bare(func(c *conn) (any, error) { return c.srv.platformPrompt() }),
	proto.MethodPlatformPromptSave: with(func(c *conn, p proto.PlatformPrompt) (any, error) {
		return c.srv.savePlatformPrompt(p)
	}),
}

func (s *Server) platformPromptFile() string {
	return filepath.Join(s.dataDir, "claude-platform", "system-prompt.md")
}

// platformPrompt is the prompt the file holds; none is not an error.
func (s *Server) platformPrompt() (proto.PlatformPrompt, error) {
	file := s.platformPromptFile()
	out := proto.PlatformPrompt{Path: file}
	if info, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
		return out, nil
	} else if err != nil {
		return out, fmt.Errorf("system prompt: %w", err)
	} else if info.Size() > maxPlatformPrompt {
		// Not an error, or the app couldn't offer to reset it.
		out.Custom, out.TooLong = true, true
		return out, nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return out, fmt.Errorf("system prompt: %w", err)
	}
	out.Text, out.Custom = string(data), strings.TrimSpace(string(data)) != ""
	return out, nil
}

// savePlatformPrompt keeps p, refused when too long or holding a secret: the prompt is
// sent to the API and read by whoever opens the file.
func (s *Server) savePlatformPrompt(p proto.PlatformPrompt) (proto.PlatformPrompt, error) {
	file := s.platformPromptFile()
	if p.Reset {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return proto.PlatformPrompt{}, fmt.Errorf("system prompt: %w", err)
		}
		return s.platformPrompt()
	}
	if strings.TrimSpace(p.Text) == "" {
		return proto.PlatformPrompt{}, errors.New("system prompt: empty; reset it to go back to the built-in one")
	}
	if len(p.Text) > maxPlatformPrompt {
		return proto.PlatformPrompt{}, fmt.Errorf("system prompt: %d KB at most, it goes with every request", maxPlatformPrompt>>10)
	}
	if err := secretsIn(map[string][]byte{"the prompt": []byte(p.Text)}); err != nil {
		return proto.PlatformPrompt{}, errors.New("system prompt: it looks like it holds a secret: keep credentials in the vault")
	}
	if err := writeTeamFile(file, []byte(p.Text)); err != nil {
		return proto.PlatformPrompt{}, fmt.Errorf("system prompt: %w", err)
	}
	return s.platformPrompt()
}

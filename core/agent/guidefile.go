package agent

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Agents whose command line takes no instructions (GuideArgs) read their
// user's standing ones from files of their home. The guide goes there:
// as a file of enowX's own where the agent reads every file of a folder,
// else as a block of the user's file, between markers, kept current. Both
// say they apply only in an enowX session, since the agent reads them
// wherever it runs. Where each place comes from is in docs/agents.md.

// guidePlace is where an agent reads standing instructions, under the
// home.
type guidePlace struct {
	path string
	// own: a folder's file enowX writes whole, after head (front matter
	// the agent needs); else a block in the user's file.
	own  bool
	head string
	// or are files the agent reads instead when path isn't there: the
	// block goes in the first there, so that path made by enowX doesn't
	// hide the user's.
	or []string
}

func own(path, head string) guidePlace { return guidePlace{path: path, own: true, head: head} }
func block(path string, or ...string) guidePlace {
	return guidePlace{path: path, or: or}
}

// guidePlaces are the agents' places, by agent, for those whose command
// line takes no guide.
var guidePlaces = map[string][]guidePlace{
	"aider":        {own(".enowx-app/agent-guide.md", "")},
	"aug":          {own(".enowx-app/agent-guide.md", "")},
	"kiro":         {own(".kiro/steering/enowx.md", "---\ninclusion: always\n---\n\n")},
	"copilot":      {own(".copilot/instructions/enowx.instructions.md", "---\napplyTo: \"**\"\n---\n\n")},
	"antigravity":  {own(".gemini/config/rules/enowx.md", "---\ntrigger: always_on\n---\n\n")},
	"cline":        {own(".cline/rules/enowx.md", "")},
	"devin":        {own(".devin/rules/enowx.md", "")},
	"gemini":       {block(".gemini/GEMINI.md")},
	"opencode":     {own(".enowx-app/agent-guide.md", "")}, // named by ToolEnv's config
	"kilo":         {own(".enowx-app/agent-guide.md", "")},
	"ante":         {block(".ante/AGENTS.md", ".claude/CLAUDE.md")},
	"mimo-code":    {block(".config/mimocode/AGENTS.md")},
	"zcode":        {block(".zcode/AGENTS.md")},
	"pi":           {block(".pi/agent/APPEND_SYSTEM.md")},
	"goose":        {block(".config/goose/.goosehints")},
	"amp":          {block(".config/amp/AGENTS.md")},
	"crush":        {block(".config/crush/CRUSH.md")},
	"command-code": {block(".commandcode/AGENTS.md")},
	"mistral-vibe": {block(".vibe/AGENTS.md")},
	"rovo":         {block(".rovodev/AGENTS.md")},
	"jcode":        {block(".jcode/prompt-overlay.md")},
}

// guideOutside is what a place holds: guide, for enowX's sessions.
func guideOutside(guide string) string {
	return "When the environment variable ENOWX_SESSION is set, read on; otherwise ignore the rest of this file.\n\n" + guide + "\n"
}

// guideBlock is what a user's file holds: where the guide is, short, so
// the user's own instructions stay theirs.
const guideBlock = `When the environment variable ENOWX_SESSION is set, you are running inside an enowX session, which the user watches from the enowX app: run "$ENOWX_BIN" guide before starting, and use the tools it describes (the user's Browser, desktop apps on their screen, the HTTP traffic of what you run, Android devices, other agents in sessions of their own) rather than asking the user to do it by hand. Otherwise ignore this.`

const (
	blockStart = "<!-- enowX: kept by enowX, which rewrites what is between these lines -->"
	blockEnd   = "<!-- enowX: end -->"
)

// PlaceGuide puts the guide where agent id reads its user's standing
// instructions under home, or updates it there. Agents given it on their
// command line, and those enowX knows no place of, get nothing.
func PlaceGuide(id, home string) error { return PlaceGuideText(id, home, Guide) }

// PlaceGuideText is PlaceGuide with guide text: the guide of the features
// the user has on.
func PlaceGuideText(id, home, text string) error {
	var errs []error
	for _, p := range guidePlaces[Base(id)] {
		path := filepath.Join(home, p.path)
		var err error
		if p.own {
			err = writeIfChanged(path, []byte(p.head+guideOutside(text)))
		} else {
			if !exists(path) {
				for _, alt := range p.or {
					if exists(filepath.Join(home, alt)) {
						path = filepath.Join(home, alt)
						break
					}
				}
			}
			err = keepBlock(path, guideBlock)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// keepBlock puts text between the markers of the file at path, added at
// its end the first time, leaving the rest of the file as it is.
func keepBlock(path, text string) error {
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	block := []byte(blockStart + "\n" + text + "\n" + blockEnd)
	var out []byte
	if i := bytes.Index(old, []byte(blockStart)); i >= 0 {
		end := bytes.Index(old[i:], []byte(blockEnd))
		if end < 0 {
			return errors.New("agent: " + path + " has enowX's start marker without its end")
		}
		out = append(append(append([]byte{}, old[:i]...), block...), old[i+end+len(blockEnd):]...)
	} else {
		out = bytes.TrimRight(old, "\n")
		if len(out) > 0 {
			out = append(out, "\n\n"...)
		}
		out = append(append(out, block...), '\n')
	}
	return writeIfChanged(path, out)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeIfChanged(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm() // the user's file stays as private as it was
	}
	return os.WriteFile(path, data, mode)
}

// GuideNote is a line added to the first prompt of agent id when enowX has
// no other way to give it the guide, "" when it has one.
func GuideNote(id string) string {
	if len(GuideArgs(id)) > 0 || len(guidePlaces[Base(id)]) > 0 {
		return ""
	}
	return `(This runs in an enowX session: "$ENOWX_BIN" guide lists the tools it gives you.)`
}

package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// The app's own settings an agent reads and changes (app.settings, enowx
// settings app): those the Settings page keeps in the app's state, each a
// key with its value as text. The hosts' settings are their daemons'.

// appSetting is one setting: what it is, its value, and how it changes.
type appSetting struct {
	about string
	get   func(a *App) string
	set   func(a *App, v string) error
}

func parseOnOff(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "on", "true", "yes", "1":
		return true, nil
	case "off", "false", "no", "0":
		return false, nil
	}
	return false, fmt.Errorf("%q: on or off", v)
}

var appSettings = map[string]appSetting{
	"default-agent": {"the agent New session starts (enowx agents lists them)",
		func(a *App) string { return a.defaultAgent() },
		func(a *App, v string) error {
			if !slices.ContainsFunc(a.agents, func(ag proto.AgentInfo) bool { return ag.ID == v }) {
				return fmt.Errorf("no agent %q here", v)
			}
			a.state.DefaultAgent = v
			return nil
		}},
	"new-session-folder": {"in a git project, New session works in: worktree (a new branch and worktree) or project (the project folder)",
		func(a *App) string {
			if a.state.ProjectFolder {
				return "project"
			}
			return "worktree"
		},
		func(a *App, v string) error {
			if v != "project" && v != "worktree" {
				return fmt.Errorf("%q: worktree or project", v)
			}
			a.state.ProjectFolder = v == "project"
			return nil
		}},
	"skip-permissions": {"agents skip permission prompts (on/off)",
		func(a *App) string { return onOff(a.skipPermissions) },
		func(a *App, v string) (err error) { a.skipPermissions, err = parseOnOff(v); return }},
	"time-budget": {"minutes an agent may work, 0 none: " + strings.Trim(fmt.Sprint(budgetChoices), "[]"),
		func(a *App) string { return strconv.Itoa(a.state.DefaultBudget) },
		func(a *App, v string) error {
			n, err := strconv.Atoi(v)
			if err != nil || !slices.Contains(budgetChoices, n) {
				return fmt.Errorf("%q: one of %v", v, budgetChoices)
			}
			a.state.DefaultBudget = n
			return nil
		}},
	"terminal-font": {"the terminals' font family, \"\" the system's monospaced font",
		func(a *App) string { return a.state.TermFont },
		func(a *App, v string) error { a.setTermFont(v); return nil }},
	"terminal-font-size": {"the terminals' text size, 0 the default",
		func(a *App) string { return strconv.FormatFloat(float64(a.state.TermFontSize), 'f', -1, 32) },
		func(a *App, v string) error {
			f, err := strconv.ParseFloat(v, 32)
			if err != nil {
				return fmt.Errorf("%q: a size", v)
			}
			a.setTermSize(float32(f))
			return nil
		}},
	"theme": {"the palette: " + paletteIDs(),
		func(a *App) string { return a.state.Theme },
		func(a *App, v string) error {
			if v != "" && !slices.ContainsFunc(theme.Palettes, func(p theme.Palette) bool { return p.ID == v }) {
				return fmt.Errorf("no theme %q: %s", v, paletteIDs())
			}
			a.state.Theme = v
			a.applyTheme()
			return nil
		}},
	"theme-mode": {"dark, light or system",
		func(a *App) string { return nonEmpty(a.state.ThemeMode, "dark") },
		func(a *App, v string) error {
			if v != "dark" && v != "light" && v != "system" {
				return fmt.Errorf("%q: dark, light or system", v)
			}
			a.setAppearanceMode(v)
			return nil
		}},
	"accent": {"#RRGGBB replacing the theme's accent, \"\" its own",
		func(a *App) string { return a.state.Accent },
		func(a *App, v string) error {
			if v == "" {
				a.setAccent("")
				return nil
			}
			hex, ok := parseAccent(v)
			if !ok {
				return fmt.Errorf("%q: a color as #RRGGBB", v)
			}
			a.setAccent(hex)
			return nil
		}},
	"desktop-notifications": {"desktop banners (on/off)",
		func(a *App) string { return onOff(!a.state.NotifyOff) },
		func(a *App, v string) error {
			on, err := parseOnOff(v)
			a.state.NotifyOff = !on
			return err
		}},
	"port-cards": {"a card when a session opens a port (on/off)",
		func(a *App) string { return onOff(!a.state.QuietPorts) },
		func(a *App, v string) error {
			on, err := parseOnOff(v)
			a.state.QuietPorts = !on
			return err
		}},
	"stop-agents-on-quit": {"this device's agents stop when enowX quits (on/off)",
		func(a *App) string { return onOff(a.state.StopAgentsOnQuit) },
		func(a *App, v string) (err error) { a.state.StopAgentsOnQuit, err = parseOnOff(v); return }},
	"editor": {"the editor files open in: vscode, cursor or zed",
		func(a *App) string { return a.state.Editor },
		func(a *App, v string) error {
			if !slices.ContainsFunc(editors, func(e editor) bool { return e.id == v }) {
				return fmt.Errorf("%q: vscode, cursor or zed", v)
			}
			a.state.Editor = v
			return nil
		}},
}

func paletteIDs() string {
	var ids []string
	for _, p := range theme.Palettes {
		ids = append(ids, p.ID)
	}
	return strings.Join(ids, ", ")
}

// appSettingsAsked changes the settings an agent asked to, then answers
// them all. It runs on the main thread.
func (a *App) appSettingsAsked(req proto.AppRequest, reply func(any, error)) {
	var p proto.AppSettingsParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		reply(nil, err)
		return
	}
	keys := make([]string, 0, len(p.Set))
	for k := range p.Set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var errs []error
	for _, k := range keys {
		s, ok := appSettings[k]
		if !ok {
			errs = append(errs, fmt.Errorf("no app setting %q (enowx settings app lists them)", k))
			continue
		}
		if err := s.set(a, strings.TrimSpace(p.Set[k])); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", k, err))
		}
	}
	if len(p.Set) > 0 {
		a.saveState()
	}
	if err := errors.Join(errs...); err != nil {
		reply(nil, err)
		return
	}
	out := proto.AppSettings{Values: map[string]string{}, About: map[string]string{}}
	for k, s := range appSettings {
		out.Values[k], out.About[k] = s.get(a), s.about
	}
	reply(out, nil)
}

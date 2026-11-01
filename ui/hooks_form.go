package ui

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// hookForm adds a hook or changes one, in a dialog: as a form, its lists
// typed comma-separated, or as the hook's JSON.
type hookForm struct {
	open    bool
	hook    proto.Hook
	place   hookPlace
	adding  bool // a new hook, whose place can still be picked
	asJSON  bool
	json    string
	lists   map[string]*string // agents, projects, paths, branches, hosts
	numbers map[string]*string // max, every, timeout
	err     string
	testing bool
	tested  *proto.HookRun
}

func (f *hookForm) start(h proto.Hook, place hookPlace, adding bool) {
	*f = hookForm{open: true, hook: h, place: place, adding: adding}
	f.lists = map[string]*string{}
	for k, v := range map[string][]string{"agents": h.Agents, "projects": h.Projects, "paths": h.Paths, "branches": h.Branches, "hosts": h.Hosts} {
		s := strings.Join(v, ", ")
		f.lists[k] = &s
	}
	f.numbers = map[string]*string{}
	for k, v := range map[string]int{"max": h.Max, "every": h.Every, "timeout": h.Timeout} {
		s := ""
		if v > 0 {
			s = strconv.Itoa(v)
		}
		f.numbers[k] = &s
	}
}

// built is the form's hook, its text lists and numbers read, or the JSON
// typed.
func (f *hookForm) built() (proto.Hook, error) {
	if f.asJSON {
		var h proto.Hook
		if err := json.Unmarshal([]byte(f.json), &h); err != nil {
			return h, err
		}
		h.ID, h.Project, h.Session = f.hook.ID, "", ""
		return h, nil
	}
	h := f.hook
	h.Agents, h.Projects, h.Paths = splitList(*f.lists["agents"]), splitList(*f.lists["projects"]), splitList(*f.lists["paths"])
	h.Branches, h.Hosts = splitList(*f.lists["branches"]), splitList(*f.lists["hosts"])
	num := func(k string) int { n, _ := strconv.Atoi(strings.TrimSpace(*f.numbers[k])); return n }
	h.Max, h.Every, h.Timeout = num("max"), num("every"), num("timeout")
	h.Name = strings.TrimSpace(h.Name)
	return h, nil
}

// toggleJSON switches the form between its fields and the hook's JSON.
func (f *hookForm) toggleJSON() {
	if f.asJSON {
		h, err := f.built()
		if err != nil {
			f.err = "The JSON doesn't read: " + err.Error()
			return
		}
		f.start(h, f.place, f.adding)
		return
	}
	h, _ := f.built()
	h.ID, h.Project, h.Session = "", "", ""
	data, _ := json.MarshalIndent(h, "", "  ")
	f.asJSON, f.json, f.err = true, string(data), ""
}

func (a *App) hookFormSheet(c *ui.Context, st *hooksState) {
	f := &st.form
	if !f.open {
		return
	}
	title, about := "Add a hook", "Kept on "+a.host.Name+" or in a project."
	if !f.adding {
		title, about = "Edit hook", "Runs on "+a.host.Name+", for every agent that sees the event."
	}
	widget.FormSheet(c, &f.open, 640, title, about, func() {
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
		ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
			ui.Spacer(c)
			mode := 0
			if f.asJSON {
				mode = 1
			}
			if i := widget.Segmented(c, []string{"Fields", "JSON"}, mode, nil); i >= 0 && i != mode {
				f.toggleJSON()
			}
		})
		if f.asJSON {
			hookArea(c, &f.json, "The hook as JSON", "", true).MinHeight(320).MaxHeight(520)
			widget.CardNote(c, "Fields as in docs/hooks.md: event, action, command, context, reason, find, replace, url, message, agents, projects, tool, paths, text, branches, hosts, any, not, once, max, every, hours, schedule, background, timeout.", 0)
		} else {
			a.hookFields(c, st, f)
		}
		ui.Row(c).FillWidth().Shrink(0).Children(func() {
			if widget.Button(c, widget.Secondary, icon.Play, "Test", "").Disabled(f.testing).Clicked() {
				a.testHookForm(f)
			}
		})
		if r := f.tested; r != nil {
			a.hookTestResult(c, r)
		}
	}, func() {
		if widget.PrimaryButton(c, icon.Check, "Save", "").Disabled(st.busy).Clicked() {
			h, err := f.built()
			if err != nil {
				f.err = "The JSON doesn't read: " + err.Error()
			} else {
				f.err = ""
				a.saveHook(st, f.place, h, false)
			}
		}
	})
}

// hookArea is a text area of the form, a well like its inputs.
func hookArea(c *ui.Context, value *string, label, placeholder string, mono bool) ui.Element {
	t := ui.TextAreaBase(c, value).FillWidth().Padding(theme.S2, theme.S3).FontSize(theme.TextUI).Label(label).Radius(theme.RadiusSmall).Placeholder(placeholder)
	if mono {
		t.Font(theme.MonoFont)
	}
	return widget.Well(t, t.Focused())
}

// hookPlacePicker picks where a new hook is kept: this host, a project
// or a session.
func (a *App) hookPlacePicker(c *ui.Context, f *hookForm) {
	key := func(p hookPlace) string { return p.scope + "\x00" + p.dir + "\x00" + p.session }
	label := func(p hookPlace) string {
		switch p.scope {
		case proto.HookScopeProject:
			return "In project " + filepath.Base(p.dir)
		case proto.HookScopeSession:
			return "In session " + a.hookSessionName(p.session)
		}
		return "On " + a.host.Name
	}
	places := []hookPlace{hostPlace}
	for _, p := range a.projects {
		places = append(places, hookPlace{scope: proto.HookScopeProject, dir: p.Path})
	}
	for _, s := range a.sessions {
		if !s.Exited && s.Parent == "" && s.Agent != "" {
			places = append(places, hookPlace{scope: proto.HookScopeSession, session: s.ID})
		}
	}
	cur := key(f.place)
	sel := ui.SelectBase(c, &cur)
	widget.Trigger(sel.Trigger, sel.Open()).Label("Kept").Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, label(f.place), widget.RunStyle{}) })
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.TextSoft)
	})
	sel.Popup(func(panel ui.Element) {
		widget.Popup(panel.Margin(theme.S1, 0, 0, 0).Width(320))
		ui.Scroll(c).FillWidth().MaxHeight(320).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, p := range places {
					item := sel.Item(key(p))
					widget.PopupItem(item, item.Highlighted()).Children(func() {
						ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { widget.Running(c, label(p), widget.RunStyle{}) })
					})
				}
			})
		})
	})
	for _, p := range places {
		if key(p) == cur {
			f.place = p
		}
	}
}

// testHookForm runs the form's hook on a sample event.
func (a *App) testHookForm(f *hookForm) {
	cl := a.client
	if cl == nil {
		return
	}
	h, err := f.built()
	if err != nil {
		f.err = "The JSON doesn't read: " + err.Error()
		return
	}
	h.On = true
	ev := a.hookSampleEvent(h)
	f.testing, f.tested = true, nil
	go func() {
		r, err := cl.TestHook(proto.HookTest{Hook: h, Event: ev})
		a.onMain(func() {
			f.testing = false
			if err != nil {
				f.err = client.Explain(err)
				return
			}
			f.err, f.tested = "", &r
		})
	}()
}

// hookSampleEvent is what a test runs hook h on.
func (a *App) hookSampleEvent(h proto.Hook) proto.HookEvent {
	ev := proto.HookEvent{Event: h.Event, Session: "test", Agent: "claude", Prompt: "Fix the login redirect", Tool: "Bash", Command: "go test ./...",
		Paths: []string{"main.go"}, Op: "commit", Time: time.Now()}
	if p, ok := a.currentProject(); ok {
		ev.Dir, ev.Project = p.Path, p.Path
	}
	return ev
}

// hookTestWords says how a test went.
func hookTestWords(r proto.HookRun) (ui.Color, string) {
	switch {
	case r.Error != "":
		return theme.Error, r.Error
	case r.Blocked:
		return theme.Warning, "Refused"
	case r.Exit != 0:
		return theme.Error, "Exited with " + strconv.Itoa(r.Exit)
	}
	return theme.Done, "Ran in " + strconv.FormatInt(r.Ms, 10) + " ms"
}

func (a *App) hookTestResult(c *ui.Context, r *proto.HookRun) {
	col, what := hookTestWords(*r)
	ui.Column(c).FillWidth().Shrink(0).Gap(theme.S1).Padding(theme.S2, theme.S3).Radius(theme.RadiusSmall).Background(theme.Sunken).Children(func() {
		ui.Text(c, "Test: "+what).FontSize(theme.TextLabel).TextColor(col).FillWidth().MinWidth(0)
		if out := strings.TrimSpace(r.Output); out != "" {
			ui.Text(c, out).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft).FillWidth().MinWidth(0).MaxLines(8)
		}
	})
}
